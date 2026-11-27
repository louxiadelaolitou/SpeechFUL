package authprivacy

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

func (c *Community) writeRecommendationLikeOrLegacy(ctx context.Context, tx pgx.Tx, a uuid.UUID, id string, kind posts.InteractionKind, enabled bool, at time.Time) error {
	if c.recommendation != nil && c.recommendation.config.LikeTimeEnabled && kind == posts.PostLike && enabled {
		var capture bool
		if err := tx.QueryRow(ctx, `SELECT like_time_enabled FROM c_recommendation.control WHERE singleton_id=1 FOR SHARE`).Scan(&capture); err != nil {
			return err
		}
		if capture {
			_, err := tx.Exec(ctx, `INSERT INTO c_posts.post_likes(account_id,post_id,liked_at) VALUES($1,$2,$3) ON CONFLICT(account_id,post_id) DO NOTHING`, a, id, at)
			return err
		}
	}
	return writePostInteraction(ctx, tx, a, id, kind, enabled, at)
}

// RevokeRecommendation deletes bounded detail in the same final business tx;
// the disabled epoch tombstone prevents old finite tuples reviving after reset.
func (c *Community) RevokeRecommendation(ctx context.Context, bearer [32]byte) (PostResponse, error) {
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, a uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
		if _, err := tx.Exec(ctx, `SELECT singleton_id FROM c_recommendation.control WHERE singleton_id=1 FOR UPDATE`); err != nil {
			return nil, err
		}
		var already bool
		if e := tx.QueryRow(ctx, `SELECT disabled FROM c_recommendation.accounts WHERE account_id=$1`, a).Scan(&already); e == nil && already {
			return nil, nil
		} else if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nil, e
		}
		if _, err := tx.Exec(ctx, `INSERT INTO c_recommendation.accounts(account_id,epoch,disabled,last_at) VALUES($1,1,true,$2) ON CONFLICT(account_id) DO UPDATE SET epoch=c_recommendation.accounts.epoch+1,disabled=true,revision=c_recommendation.accounts.revision+1,last_at=EXCLUDED.last_at`, a, final.TrustedAt); err != nil {
			return nil, err
		}
		for _, table := range []string{"rounds", "intents", "history"} {
			if _, err := tx.Exec(ctx, `DELETE FROM c_recommendation.`+table+` WHERE account_id=$1`, a); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
}

// CleanRecommendation processes a finite oldest-first batch in short grouped
// transactions. maxRows counts changed data rows INCLUDING cascaded pages and
// account revision updates; the return value excludes revision-only bookkeeping.
// At most maxRows objects and 2*maxRows DATA-row mutations (update=2) are driven;
// trigger/ledger/Gate SQL is additional bounded overhead for the service profile.
// The cooperative deadline does not change the shared Gate's flock semantics.
func (c *Community) CleanRecommendation(ctx context.Context, maxRows int) (int, error) {
	if maxRows < 16 || maxRows > 32768 {
		return 0, ErrIntentInvalid
	}
	deadline := time.Now().Add(2 * time.Second)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Fix the candidate cutoff and expected generation for this bounded pass.
	// Every group still obtains its own final trusted time and Gate validation
	// in CommitAuthorized. A generation change therefore stops the pass; newly
	// due objects can wait for the next pass instead of publishing another
	// durable preflight checkpoint for every row (including the empty tail).
	d, err := c.gate.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	total, changed := 0, 0
	for examined := 0; examined < maxRows && changed < maxRows; examined++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		if !time.Now().Before(deadline) {
			break
		}
		object, err := c.nextRecommendationCleanup(ctx, d.TrustedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return total, err
		}
		if !time.Now().Before(deadline) {
			break
		}
		// A pass may drive several short transactions. Bound each group as
		// well, so a large expiry burst can yield between final Gate commits.
		// Yield between bounded transactions. Cancelling the current group's
		// SQL at the pass cutoff turns normal progress into a worker failure;
		// the caller's task deadline and cancellation still bound that group.
		n, used, err := c.cleanRecommendationObject(ctx, d, object, min(maxRows-changed, 4096))
		if err != nil {
			return total, err
		}
		if used == 0 {
			break
		} // stale candidate or insufficient remainder: no spin
		total, changed = total+n, changed+used
	}
	return total, nil
}

type recommendationCleanupObject struct {
	account, id string
	kind        int
}

func recommendationIdleGrace() time.Duration {
	p := rm.ExperimentalIntentPolicy()
	return p.MaxAge + p.FutureSkew + p.ReceiptGrace
}

func (c *Community) nextRecommendationCleanup(ctx context.Context, at time.Time) (recommendationCleanupObject, error) {
	var object recommendationCleanupObject
	// Reject fresh idle candidates before probing their child indexes.
	err := c.pool.QueryRow(ctx, `SELECT account_id::text,kind,id FROM (
 (SELECT account_id,0 kind,post_id::text id,due_at due FROM c_recommendation.history ORDER BY due_at,account_id,post_id LIMIT 1)
 UNION ALL (SELECT account_id,1,round_id::text,feedback_until FROM c_recommendation.rounds ORDER BY feedback_until,round_id LIMIT 1)
 UNION ALL (SELECT account_id,2,encode(key_digest,'hex'),retain_until FROM c_recommendation.intents ORDER BY retain_until,account_id,key_digest LIMIT 1)
 UNION ALL (SELECT a.account_id,3,a.account_id::text,a.last_at+($2::bigint*interval '1 microsecond') FROM c_recommendation.accounts a
  WHERE NOT a.disabled AND a.epoch=0 AND a.last_at<=$1::timestamptz-($2::bigint*interval '1 microsecond')
   AND NOT EXISTS(SELECT 1 FROM c_recommendation.rounds r WHERE r.account_id=a.account_id)
   AND NOT EXISTS(SELECT 1 FROM c_recommendation.intents i WHERE i.account_id=a.account_id)
   AND NOT EXISTS(SELECT 1 FROM c_recommendation.history h WHERE h.account_id=a.account_id)
  ORDER BY a.last_at,a.account_id LIMIT 1)
 ) q WHERE due<=$1 ORDER BY due,kind,account_id,id LIMIT 1`, at, recommendationIdleGrace().Microseconds()).Scan(&object.account, &object.kind, &object.id)
	return object, err
}

func (c *Community) cleanRecommendationObject(ctx context.Context, d AuthorizationDecision, object recommendationCleanupObject, remaining int) (int, int, error) {
	return c.cleanRecommendationGroup(ctx, d, object.kind, remaining)
}
