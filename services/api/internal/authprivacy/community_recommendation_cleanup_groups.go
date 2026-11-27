package authprivacy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Group cross-account expiry in one final authorized business transaction.
// Authentication rows are locked in account order BEFORE acquiring the Gate;
// normal account operations use that same account-before-Gate ordering.
func (c *Community) cleanRecommendationGroup(ctx context.Context, d AuthorizationDecision, kind, remaining int) (int, int, error) {
	limit := remaining
	switch kind {
	case 0:
		limit /= 2 // reserve one revision for every candidate account
	case 1:
		limit /= 12 // round + at most ten pages + one account revision
	case 2:
		limit /= 2 // at least one intent and one revision per locked account
	case 3:
	default:
		return 0, 0, ErrIntentInvalid
	}
	if limit == 0 {
		return 0, 0, nil
	}
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return 0, 0, err
	}
	defer c.gate.Abort(ctx, tx)
	rows, err := tx.Query(ctx, `SELECT a.account_id::text FROM c_auth.accounts a
 JOIN c_recommendation.accounts r USING(account_id)
 WHERE ($1::int=0 AND EXISTS(SELECT 1 FROM c_recommendation.history x WHERE x.account_id=a.account_id AND x.due_at<=$2))
 OR ($1::int=1 AND EXISTS(SELECT 1 FROM c_recommendation.rounds x WHERE x.account_id=a.account_id AND x.feedback_until<=$2))
 OR ($1::int=2 AND EXISTS(SELECT 1 FROM c_recommendation.intents x WHERE x.account_id=a.account_id AND x.retain_until<=$2))
 OR ($1::int=3 AND NOT r.disabled AND r.epoch=0 AND r.last_at+($4::bigint*interval '1 microsecond')<=$2
  AND NOT EXISTS(SELECT 1 FROM c_recommendation.rounds x WHERE x.account_id=a.account_id)
  AND NOT EXISTS(SELECT 1 FROM c_recommendation.intents x WHERE x.account_id=a.account_id)
  AND NOT EXISTS(SELECT 1 FROM c_recommendation.history x WHERE x.account_id=a.account_id))
 ORDER BY a.account_id LIMIT $3 FOR UPDATE OF a`, kind, d.TrustedAt, limit, recommendationIdleGrace().Microseconds())
	if err != nil {
		return 0, 0, err
	}
	accounts, err := recommendationCleanupIDs(rows)
	if err != nil || len(accounts) == 0 {
		return 0, 0, err
	}
	removed, used := 0, 0
	_, err = c.gate.CommitAuthorized(ctx, tx, d.Generation, func(final AuthorizationDecision) error {
		if _, e := tx.Exec(ctx, `SELECT singleton_id FROM c_recommendation.control WHERE singleton_id=1 FOR UPDATE`); e != nil {
			return e
		}
		changed := map[string]bool{}
		switch kind {
		case 0:
			// A stale selectivity estimate can prefer sorting the entire due
			// backlog even for this finite batch. Discourage that plan only
			// while locking the ordered history prefix. Preserve the connection's
			// prior setting before any other cleanup statement; an error rolls
			// back the transaction-local setting along with the transaction.
			var priorSort string
			if e := tx.QueryRow(ctx, `SELECT current_setting('enable_sort')`).Scan(&priorSort); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `SELECT set_config('enable_sort','off',true)`); e != nil {
				return e
			}
			// Qualify UUID inputs instead of the text output aliases so the
			// ordering matches the existing (due_at,account_id,post_id) index.
			rows, e := tx.Query(ctx, `SELECT h.account_id::text,h.post_id::text,h.delivery_until,h.exposure_until
 FROM c_recommendation.history h WHERE h.account_id=ANY($1::uuid[]) AND h.due_at<=$2
 ORDER BY h.due_at,h.account_id,h.post_id LIMIT $3 FOR UPDATE`, accounts, d.TrustedAt, remaining-len(accounts))
			if e != nil {
				return e
			}
			var expiredAccounts, expiredPosts, partialAccounts, partialPosts []string
			for rows.Next() {
				var account, post string
				var du, ex *time.Time
				if e = rows.Scan(&account, &post, &du, &ex); e != nil {
					break
				}
				changed[account] = true
				if (du == nil || !du.After(final.TrustedAt)) && (ex == nil || !ex.After(final.TrustedAt)) {
					expiredAccounts = append(expiredAccounts, account)
					expiredPosts = append(expiredPosts, post)
				} else {
					partialAccounts = append(partialAccounts, account)
					partialPosts = append(partialPosts, post)
				}
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `SELECT set_config('enable_sort',$1,true)`, priorSort); e != nil {
				return e
			}
			if len(expiredPosts) > 0 {
				tag, e := tx.Exec(ctx, `DELETE FROM c_recommendation.history h
 USING unnest($1::uuid[],$2::uuid[]) x(account_id,post_id)
 WHERE h.account_id=x.account_id AND h.post_id=x.post_id
 AND (h.delivery_until IS NULL OR h.delivery_until<=$3)
 AND (h.exposure_until IS NULL OR h.exposure_until<=$3)`, expiredAccounts, expiredPosts, final.TrustedAt)
				if e != nil {
					return e
				}
				if tag.RowsAffected() != int64(len(expiredPosts)) {
					return ErrAuthorizationUnavailable
				}
				removed += int(tag.RowsAffected())
			}
			if len(partialPosts) > 0 {
				tag, e := tx.Exec(ctx, `UPDATE c_recommendation.history h
 SET delivery_at=CASE WHEN h.delivery_until<=$3 THEN NULL ELSE h.delivery_at END,
 delivery_until=CASE WHEN h.delivery_until<=$3 THEN NULL ELSE h.delivery_until END,
 exposure_until=CASE WHEN h.exposure_until<=$3 THEN NULL ELSE h.exposure_until END
 FROM unnest($1::uuid[],$2::uuid[]) x(account_id,post_id)
 WHERE h.account_id=x.account_id AND h.post_id=x.post_id AND h.due_at<=$4`, partialAccounts, partialPosts, final.TrustedAt, d.TrustedAt)
				if e != nil {
					return e
				}
				if tag.RowsAffected() != int64(len(partialPosts)) {
					return ErrAuthorizationUnavailable
				}
				removed += int(tag.RowsAffected())
			}
		case 1:
			rows, e := tx.Query(ctx, `SELECT r.round_id::text,r.account_id::text,
 (SELECT count(*) FROM (SELECT 1 FROM c_recommendation.pages p WHERE p.round_id=r.round_id LIMIT 11) p)
 FROM c_recommendation.rounds r WHERE r.account_id=ANY($1::uuid[]) AND r.feedback_until<=$2
 ORDER BY r.feedback_until,r.round_id LIMIT $3 FOR UPDATE OF r`, accounts, d.TrustedAt, limit)
			if e != nil {
				return e
			}
			var rounds []string
			for rows.Next() {
				var id, account string
				var pages int
				if e = rows.Scan(&id, &account, &pages); e != nil {
					break
				}
				if pages > 10 {
					e = ErrAuthorizationUnavailable
					break
				}
				rounds = append(rounds, id)
				changed[account] = true
				removed += 1 + pages
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil || len(rounds) == 0 {
				return e
			}
			tag, e := tx.Exec(ctx, `DELETE FROM c_recommendation.rounds WHERE round_id=ANY($1::uuid[]) AND account_id=ANY($2::uuid[]) AND feedback_until<=$3`, rounds, accounts, final.TrustedAt)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != int64(len(rounds)) {
				return ErrAuthorizationUnavailable
			}
		case 2:
			rows, e := tx.Query(ctx, `WITH chosen AS MATERIALIZED (
 SELECT account_id,key_digest FROM c_recommendation.intents
 WHERE account_id=ANY($1::uuid[]) AND retain_until<=$2
 ORDER BY retain_until,account_id,key_digest LIMIT $3)
 DELETE FROM c_recommendation.intents i USING chosen x
 WHERE i.account_id=x.account_id AND i.key_digest=x.key_digest AND i.retain_until<=$4
 RETURNING i.account_id::text`, accounts, d.TrustedAt, remaining-len(accounts), final.TrustedAt)
			if e != nil {
				return e
			}
			owners, e := recommendationCleanupIDs(rows)
			if e != nil {
				return e
			}
			removed = len(owners)
			for _, account := range owners {
				changed[account] = true
			}
		case 3:
			tag, e := tx.Exec(ctx, `DELETE FROM c_recommendation.accounts a
 WHERE a.account_id=ANY($1::uuid[]) AND NOT a.disabled AND a.epoch=0 AND a.last_at<=$2
 AND NOT EXISTS(SELECT 1 FROM c_recommendation.rounds r WHERE r.account_id=a.account_id)
 AND NOT EXISTS(SELECT 1 FROM c_recommendation.intents i WHERE i.account_id=a.account_id)
 AND NOT EXISTS(SELECT 1 FROM c_recommendation.history h WHERE h.account_id=a.account_id)`, accounts, final.TrustedAt.Add(-recommendationIdleGrace()))
			if e != nil {
				return e
			}
			removed = int(tag.RowsAffected())
			used = removed
			return nil
		}
		if removed == 0 {
			return nil
		}
		ids := make([]string, 0, len(changed))
		for account := range changed {
			ids = append(ids, account)
		}
		tag, e := tx.Exec(ctx, `UPDATE c_recommendation.accounts SET revision=revision+1,last_at=$2 WHERE account_id=ANY($1::uuid[])`, ids, final.TrustedAt)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != int64(len(ids)) || removed+len(ids) > remaining {
			return ErrAuthorizationUnavailable
		}
		used = removed + len(ids)
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return removed, used, nil
}

func recommendationCleanupIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
