package authprivacy

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// observedAuthorizationTransaction preserves pgx.Tx behavior and only exists
// when a request explicitly enabled timing. The hold starts after the locking
// SELECT returns, and ends after the transaction's commit/rollback response.
// This client-observed interval excludes the lock-query round trip. A failed
// terminal command is marked uncertain; it cannot establish the server's exact
// lock-release time or whether an unknown commit took effect.
type observedAuthorizationTransaction struct {
	pgx.Tx
	ctx    context.Context
	mu     sync.Mutex
	finish func()
	ended  bool
}

func observeAuthorizationTransaction(ctx context.Context, tx pgx.Tx) pgx.Tx {
	if !readtiming.Enabled(ctx) {
		return tx
	}
	return &observedAuthorizationTransaction{Tx: tx, ctx: ctx}
}

func observeGateRowAcquired(ctx context.Context, tx pgx.Tx) {
	observed, ok := tx.(*observedAuthorizationTransaction)
	if !ok {
		return
	}
	observed.mu.Lock()
	defer observed.mu.Unlock()
	if observed.finish == nil && !observed.ended {
		observed.finish = readtiming.Span(ctx, "gate_hold")
	}
}

func (tx *observedAuthorizationTransaction) finishObservation(err error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.ended {
		return
	}
	tx.ended = true
	if tx.finish != nil {
		tx.finish()
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			readtiming.Span(tx.ctx, "gate_hold_boundary_uncertain")()
		}
	}
}

func (tx *observedAuthorizationTransaction) Commit(ctx context.Context) error {
	err := tx.Tx.Commit(ctx)
	tx.finishObservation(err)
	return err
}

func (tx *observedAuthorizationTransaction) Rollback(ctx context.Context) error {
	err := tx.Tx.Rollback(ctx)
	tx.finishObservation(err)
	return err
}

func (g *PostgresAuthorizationGate) observedAnchorWrite(ctx context.Context, a authorizationAnchor) error {
	defer readtiming.Span(ctx, "anchor_write")()
	return g.anchor.writeLocked(a)
}

func (g *PostgresAuthorizationGate) observedBusinessAnchorWrite(ctx context.Context, a authorizationAnchor) error {
	defer readtiming.Span(ctx, "anchor_write")()
	return g.anchor.writeBusinessLocked(a)
}

func (g *PostgresAuthorizationGate) observedAnchorLock(ctx context.Context, fn func(*authorizationAnchor, bool) error) error {
	finish := readtiming.Span(ctx, "anchor_lock_read_verify")
	entered := false
	defer func() {
		if !entered {
			finish()
		}
	}()
	return g.anchor.withLocked(func(a *authorizationAnchor, exists bool) error {
		entered = true
		finish()
		return fn(a, exists)
	})
}

func observedAuthorizationCommit(ctx context.Context, tx pgx.Tx) error {
	defer readtiming.Span(ctx, "sql_commit")()
	return tx.Commit(ctx)
}

func observedAuthorizationAbort(ctx context.Context, gate AuthorizationGate, tx pgx.Tx) {
	defer readtiming.Span(ctx, "gate_abort")()
	_ = gate.Abort(ctx, tx)
}
