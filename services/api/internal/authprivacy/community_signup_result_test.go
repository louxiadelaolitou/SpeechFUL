package authprivacy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSignupResultPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	ticket, private := l.ticket(t)
	_, req := l.intent(t, ticket, private, "signup_result_user")
	if state, err := l.c.GetSignupResult(ctx, req.IdempotencyKey, req.IntentID); err != nil || state != "PENDING" {
		t.Fatal("live intent must remain pending", state, err)
	}
	created, err := l.c.CommitSignup(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if state, err := l.c.GetSignupResult(ctx, req.IdempotencyKey, req.IntentID); err != nil || state != "COMMITTED" {
			t.Fatal("committed original outcome", state, err)
		}
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.sessions WHERE account_id=$1`, created.AccountID) != 1 {
		t.Fatal("result lookup replayed a session")
	}
	wrong := req.IdempotencyKey
	wrong[0] ^= 1
	if _, err := l.c.GetSignupResult(ctx, wrong, req.IntentID); !errors.Is(err, ErrSignupResultNotFound) {
		t.Fatal("wrong key disclosed commit", err)
	}
	if err := l.gate.Freeze(ctx, "signup result test"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.c.GetSignupResult(ctx, req.IdempotencyKey, req.IntentID); err == nil {
		t.Fatal("frozen gate exposed a result")
	}
}

type signupResultDeadlineGate struct {
	AuthorizationGate
	at           time.Time
	beforeCommit func()
}

func (g *signupResultDeadlineGate) Snapshot(ctx context.Context) (AuthorizationDecision, error) {
	d, err := g.AuthorizationGate.Snapshot(ctx)
	if !g.at.IsZero() {
		d.TrustedAt = g.at
	}
	return d, err
}

func (g *signupResultDeadlineGate) CommitAuthorized(ctx context.Context, tx pgx.Tx, generation uint64, checks ...func(AuthorizationDecision) error) (AuthorizationDecision, error) {
	if g.beforeCommit != nil {
		g.beforeCommit()
	}
	return g.AuthorizationGate.CommitAuthorized(ctx, tx, generation, func(d AuthorizationDecision) error {
		if !g.at.IsZero() {
			d.TrustedAt = g.at
		}
		for _, check := range checks {
			if err := check(d); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestSignupResultFinalGateFreezePostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	ticket, private := l.ticket(t)
	_, req := l.intent(t, ticket, private, "signup_freeze_user")
	l.c.gate = &signupResultDeadlineGate{AuthorizationGate: l.gate, beforeCommit: func() {
		if err := l.gate.Freeze(ctx, "signup result final fence"); err != nil {
			t.Fatal(err)
		}
	}}
	if state, err := l.c.GetSignupResult(ctx, req.IdempotencyKey, req.IntentID); err == nil || state != "" {
		t.Fatal("query exposed a result after the final gate froze", state, err)
	}
}

func TestSignupResultExpiredIntentFencesLateCommitPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	ticket, private := l.ticket(t)
	intent, req := l.intent(t, ticket, private, "signup_expired_user")
	original := l.c.gate
	l.c.gate = &signupResultDeadlineGate{AuthorizationGate: original, at: intent.ExpiresAt}
	if state, err := l.c.GetSignupResult(ctx, req.IdempotencyKey, req.IntentID); err != nil || state != "NOT_COMMITTED" {
		t.Fatal("expired unlocked intent was not terminal", state, err)
	}
	if _, err := l.c.CommitSignup(ctx, req); !errors.Is(err, ErrIntentInvalid) {
		t.Fatal("late signup escaped authoritative deadline", err)
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.accounts`) != 0 {
		t.Fatal("result query or late signup created an account")
	}
}
