package authprivacy

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

var errReadConflict = errors.New("prepared read changed")

// readSubject binds private preparation to the exact session seen in the
// independent read snapshot. Only withPostSessionAt can authorize its release.
type readSubject struct {
	token      [32]byte
	account    uuid.UUID
	generation int64
	decision   AuthorizationDecision
}

type preparedReadAction func(pgx.Tx, uuid.UUID, AuthorizationDecision, restriction) (any, error)
type prepareReadAction func(context.Context, pgx.Tx, readSubject) (preparedReadAction, error)

// A plan owns private preparation until the final transaction commits. A
// non-nil plan returned with an error must still have a usable Revalidate:
// changed resources take precedence over a stale preparation/CPU error.
type preparedReadPlan struct {
	AfterPrepare func(context.Context) error
	Revalidate   func(pgx.Tx, uuid.UUID, AuthorizationDecision, restriction) error
	Complete     preparedReadAction
	// Release runs once per attempt, including success and panic. It must not
	// clear an array that Complete has transferred to the successful response.
	Release func()
}

type prepareReadPlanAction func(context.Context, pgx.Tx, readSubject) (*preparedReadPlan, error)
type preparedReadObserverKey struct{}

func (c *Community) nativePreparedReads() bool {
	g, ok := c.gate.(*PostgresAuthorizationGate)
	return ok && g.schema == "c_auth"
}

// withPreparedRead never holds caller or Gate write locks while preparing.
// Preparation errors stay private until the final authority and session check.
// A normal version race rolls back without renewal and is rebuilt once only.
func (c *Community) withPreparedRead(ctx context.Context, bearer [32]byte, prepare prepareReadAction) (PostResponse, error) {
	return c.withPreparedReadPlan(ctx, bearer, postError("COMMENT_SNAPSHOT_UNAVAILABLE", 503),
		func(ctx context.Context, tx pgx.Tx, subject readSubject) (*preparedReadPlan, error) {
			action, err := prepare(ctx, tx, subject)
			if err != nil || action == nil {
				return nil, err
			}
			// M1 performs its resource proof inside action, and retains its
			// outer Redis reservation Publish/Abort ownership unchanged.
			return &preparedReadPlan{
				Revalidate: func(pgx.Tx, uuid.UUID, AuthorizationDecision, restriction) error { return nil },
				Complete:   action,
			}, nil
		})
}

func (c *Community) withPreparedReadPlan(ctx context.Context, bearer [32]byte, conflictUnavailable error, prepare prepareReadPlanAction) (PostResponse, error) {
	if !c.PostsEnabled() || !c.nativePreparedReads() {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	if c.rankings != nil {
		finish := c.rankings.ForegroundStarted()
		defer finish()
	}
	decision, err := c.postCandidateDecision(ctx, c.pool)
	if err != nil {
		return PostResponse{}, err
	}
	token := sha256.Sum256(bearer[:])
	account, generation, err := c.precheckPostSearchSession(ctx, token, decision)
	if err != nil {
		return PostResponse{}, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			readtiming.Span(ctx, "prepared_read_rebuild")()
			// A concurrent command from this same session may have advanced its
			// last_activity_at. Refresh candidate time without accepting a new
			// Gate generation or rebinding the original session generation.
			fresh, e := c.postCandidateDecision(ctx, c.pool)
			if e != nil {
				return PostResponse{}, e
			}
			if fresh.Generation != decision.Generation {
				return PostResponse{}, ErrAuthorizationUnavailable
			}
			decision = fresh
		}
		subject := readSubject{token: token, account: account, generation: generation, decision: decision}
		response, e := c.preparedReadAttempt(ctx, token, subject, prepare)
		if !errors.Is(e, errReadConflict) {
			return response, e
		}
		readtiming.Span(ctx, "prepared_read_conflict")()
	}
	if conflictUnavailable == nil {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	return PostResponse{}, conflictUnavailable
}

func (c *Community) preparedReadAttempt(ctx context.Context, token [32]byte, subject readSubject, prepare prepareReadPlanAction) (PostResponse, error) {
	var plan *preparedReadPlan
	defer func() {
		if plan != nil && plan.Release != nil {
			plan.Release()
		}
	}()
	var pending error
	func() {
		defer readtiming.Span(ctx, "prepare_read")()
		prepareCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		tx, e := c.pool.BeginTx(prepareCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if e != nil {
			pending = ErrAuthorizationUnavailable
			return
		}
		defer tx.Rollback(context.Background())
		if _, e = tx.Exec(prepareCtx, `SET LOCAL lock_timeout = '5s'; SET LOCAL statement_timeout = '10s'`); e != nil {
			pending = ErrAuthorizationUnavailable
			return
		}
		a := credentialAccount{}
		s := credentialSession{}
		e = tx.QueryRow(prepareCtx, `SELECT a.account_id,a.state,a.session_generation,r.ban_state,r.ban_ends_at,
 s.session_generation,s.authorization_generation,s.expires_at,s.revoked_at,s.revocation_reason
 FROM c_auth.sessions s JOIN c_auth.accounts a USING(account_id) JOIN c_auth.account_restrictions r USING(account_id)
 WHERE s.token_digest=$1`, token[:]).Scan(&a.account, &a.state, &a.generation, &a.ban.state, &a.ban.ends,
			&s.generation, &s.authGeneration, &s.expires, &s.revoked, &s.reason)
		if e != nil {
			pending = ErrAuthorizationUnavailable
			if errors.Is(e, pgx.ErrNoRows) {
				pending = ErrSessionInvalid
			}
			return
		}
		if pending = s.validate(a, subject.decision); pending != nil {
			return
		}
		if a.account != subject.account || s.generation != subject.generation {
			pending = ErrSessionInvalid
			return
		}
		plan, pending = prepare(prepareCtx, tx, subject)
		if e = tx.Rollback(ctx); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			pending = ErrAuthorizationUnavailable
		}
	}()
	if plan != nil && plan.Revalidate == nil {
		// A plan without its resource proof must never release protected
		// data, including when preparation already produced an error.
		pending = ErrAuthorizationUnavailable
	}
	// The RO rollback and deferred connection release have both completed
	// before CPU preparation; no caller or Gate write lock exists yet.
	if pending == nil && plan != nil && plan.AfterPrepare != nil {
		pending = plan.AfterPrepare(ctx)
	}
	finalAt := time.Now()
	response, e := c.withPostSessionAt(ctx, token, subject.account, subject.decision, &subject.generation,
		func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, m restriction) (any, error) {
			defer readtiming.Span(ctx, "finalize_read")()
			if plan != nil && plan.Revalidate != nil {
				if err := plan.Revalidate(tx, a, d, m); err != nil {
					return nil, err
				}
			}
			if pending != nil {
				return nil, pending
			}
			if plan == nil || plan.Complete == nil {
				return nil, ErrSessionInvalid
			}
			return plan.Complete(tx, a, d, m)
		})
	if observe, ok := ctx.Value(preparedReadObserverKey{}).(func(time.Duration)); ok {
		observe(time.Since(finalAt))
	}
	return response, e
}
