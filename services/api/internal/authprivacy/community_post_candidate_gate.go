package authprivacy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// Private candidate preparation needs verified time but neither renews a
// session nor establishes an authorization checkpoint. The final business
// transaction still uses CommitAuthorized. Unknown Gate implementations keep
// their original Snapshot behavior and their own generation source.
func (c *Community) postCandidateDecision(ctx context.Context, pool *pgxpool.Pool) (AuthorizationDecision, error) {
	defer readtiming.Span(ctx, "candidate_authority")()
	gate, ok := c.gate.(*PostgresAuthorizationGate)
	if !ok || gate.schema != "c_auth" {
		return c.gate.Snapshot(ctx)
	}
	tx, err := begin(ctx, pool)
	if err != nil {
		return AuthorizationDecision{}, ErrAuthorizationUnavailable
	}
	defer tx.Rollback(ctx)
	row, err := gate.readGateRow(ctx, tx)
	if err != nil {
		return AuthorizationDecision{}, ErrAuthorizationUnavailable
	}
	decision, err := gate.recheckLockedGateRow(ctx, tx, row)
	if err != nil {
		// A detected independent freeze must survive rollback of this read.
		_ = c.gate.Abort(ctx, tx)
		return AuthorizationDecision{}, ErrAuthorizationUnavailable
	}
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return AuthorizationDecision{}, ErrAuthorizationUnavailable
	}
	return decision, nil
}
