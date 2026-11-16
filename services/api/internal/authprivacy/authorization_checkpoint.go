package authprivacy

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func temporaryEvidenceReason(reason string) bool {
	return reason == "EVIDENCE_UNAVAILABLE" || reason == "EVIDENCE_EXPIRED"
}

// reconcileReadCheckpoint runs with both the SQL Gate row and signed anchor
// locked. It never reopens a frozen Gate, changes generation, or replays a
// business transaction. A legacy checkpoint without an ID cannot be replayed.
func (g *PostgresAuthorizationGate) reconcileReadCheckpoint(ctx context.Context, tx pgx.Tx, row *gateRow, a authorizationAnchor, exists bool) error {
	r := a.ReadAdvance
	if !exists || r == nil || row.state != "OPEN" || a.Frozen || a.Domain != g.domain ||
		row.generation <= 0 || a.Generation != uint64(row.generation) ||
		row.checkpoint == "" || row.checkpoint != r.PreviousID ||
		row.version <= 0 || uint64(row.version) != r.PreviousVersion || !row.highwater.Equal(r.PreviousHighwater) {
		return nil
	}
	id, err := uuid.Parse(a.CheckpointID)
	previous, previousErr := uuid.Parse(r.PreviousID)
	if err != nil || previousErr != nil || id == uuid.Nil || previous == uuid.Nil || id == previous ||
		id.String() != a.CheckpointID || a.Version > math.MaxInt64 ||
		a.Version < r.PreviousVersion || a.Highwater.Before(r.PreviousHighwater) ||
		r.IssuedAt.IsZero() || !r.ValidUntil.After(r.IssuedAt) || r.ValidUntil.Sub(r.IssuedAt) > authorizationEvidenceTTL ||
		a.Highwater.Before(r.IssuedAt) || a.Highwater.After(r.ValidUntil) {
		return ErrAuthorizationUnavailable
	}
	// This is historical bookkeeping, not permission to serve a request. The
	// caller must still inspect current evidence/time before authorizing work.
	_, err = tx.Exec(ctx, `UPDATE `+g.schema+`.authorization_gate SET trusted_high_watermark=$1,
		evidence_version=$2,evidence_issued_at=$3,evidence_valid_until=$4,checkpoint_id=$5 WHERE singleton_id=1`,
		a.Highwater, int64(a.Version), r.IssuedAt, r.ValidUntil, a.CheckpointID)
	if err == nil {
		row.highwater, row.version, row.checkpoint = a.Highwater, int64(a.Version), a.CheckpointID
	}
	return err
}

func nextReadCheckpoint(a authorizationAnchor, predecessor gateRow, e AuthorizationEvidence, at time.Time, offline bool) authorizationAnchor {
	a.Highwater, a.Version, a.CheckpointID = at, e.Version, uuid.NewString()
	a.ReadAdvance = nil
	if predecessor.checkpoint != "" {
		a.ReadAdvance = &authorizationReadAdvance{
			PreviousID: predecessor.checkpoint, PreviousVersion: uint64(predecessor.version), PreviousHighwater: predecessor.highwater,
			IssuedAt: e.IssuedAt, ValidUntil: e.ValidUntil,
		}
	}
	if !offline {
		a.OfflineEvidence = nil
	}
	return a
}
