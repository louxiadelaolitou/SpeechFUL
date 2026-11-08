package authprivacy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

type EmailResetIntentRequest struct {
	Proof          string
	InstallationID [16]byte
	IdempotencyKey [32]byte
}

// This only issues a restricted reset capability. It never creates a session,
// cancels closure, discloses an email to C, or returns a recovery code.
func (c *Community) CreateEmailResetIntent(ctx context.Context, req EmailResetIntentRequest) (PasswordResetIntent, error) {
	var zero PasswordResetIntent
	d, err := c.gate.Snapshot(ctx)
	if err != nil {
		return zero, err
	}
	proof, err := c.verifier.VerifyEmailReset(req.Proof, d.TrustedAt)
	if err != nil || req.InstallationID == ([16]byte{}) || req.IdempotencyKey == ([32]byte{}) || proof.Binding != protocol.EmailResetBinding(req.InstallationID, req.IdempotencyKey) {
		return zero, ErrEmailProofInvalid
	}
	var account uuid.UUID
	err = c.pool.QueryRow(ctx, `SELECT a.account_id FROM c_auth.accounts a JOIN c_auth.slot_ledger l USING(account_id) WHERE l.slot_id=$1 AND l.state='ACTIVE' AND a.state IN ('ACTIVE','PENDING_CLOSE')`, proof.Slot[:]).Scan(&account)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, ErrEmailUnregistered
	}
	if err != nil {
		return zero, ErrAuthorizationUnavailable
	}
	id, err := random32()
	if err != nil {
		return zero, ErrAuthorizationUnavailable
	}
	var hiddenDigest [32]byte
	if _, err = rand.Read(hiddenDigest[:]); err != nil {
		return zero, ErrAuthorizationUnavailable
	}
	proofDigest := digest("HNUHOLE/C-EMAIL-RESET-PROOF/V1", []byte(req.Proof))
	operationDigest := digest("HNUHOLE/C-EMAIL-RESET-OPERATION/V1", req.IdempotencyKey[:])
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return zero, ErrAuthorizationUnavailable
	}
	defer c.gate.Abort(ctx, tx)
	var state, username string
	var version, generation int64
	err = tx.QueryRow(ctx, `SELECT state,COALESCE(username,''),credential_version,reset_generation FROM c_auth.accounts WHERE account_id=$1 FOR UPDATE`, account).Scan(&state, &username, &version, &generation)
	if err != nil {
		return zero, ErrAuthorizationUnavailable
	}
	if _, err = lockRestriction(ctx, tx, account); err != nil {
		return zero, ErrAuthorizationUnavailable
	}
	var closure pendingClosure
	if state == "PENDING_CLOSE" {
		closure, err = lockPendingClosure(ctx, tx, account)
		if err != nil {
			return zero, ErrAuthorizationUnavailable
		}
	}
	var priorID []byte
	var priorProof []byte
	var priorState string
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT intent_id,state,expires_at,proof_digest FROM c_auth.reset_intents WHERE operation_digest=$1 FOR UPDATE`, operationDigest[:]).Scan(&priorID, &priorState, &expires, &priorProof)
	prior := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return zero, ErrAuthorizationUnavailable
	}
	if prior && !hmac.Equal(priorProof, proofDigest[:]) {
		return zero, ErrConflict
	}
	if prior && (priorState != "ACTIVE" || len(priorID) != 32) {
		return zero, ErrResetIntentInvalid
	}
	if !prior && generation == math.MaxInt64 {
		return zero, ErrAuthorizationUnavailable
	}
	_, err = c.gate.CommitAuthorized(ctx, tx, d.Generation, func(final AuthorizationDecision) error {
		if !recoveryAccountAllowed(state, closure, final.TrustedAt) {
			return ErrEmailUnregistered
		}
		checked, e := c.verifier.VerifyEmailReset(req.Proof, final.TrustedAt)
		if e != nil || checked.Binding != proof.Binding {
			return ErrEmailProofInvalid
		}
		var owns bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM c_auth.slot_ledger WHERE slot_id=$1 AND account_id=$2 AND state='ACTIVE')`, proof.Slot[:], account).Scan(&owns); e != nil {
			return ErrAuthorizationUnavailable
		}
		if !owns {
			return ErrEmailUnregistered
		}
		if prior {
			if !final.TrustedAt.Before(expires) {
				return ErrResetIntentExpired
			}
			copy(id[:], priorID)
			return nil
		}
		expires = final.TrustedAt.UTC().Add(10 * time.Minute).Truncate(time.Microsecond)
		if _, e = tx.Exec(ctx, `UPDATE c_auth.reset_intents SET state='ABANDONED',new_recovery_digest=NULL,terminal_at=$2 WHERE account_id=$1 AND state='ACTIVE'`, account, final.TrustedAt); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO c_auth.reset_intents(intent_id,account_id,credential_version,reset_generation,authorization_generation,state,new_recovery_digest,created_at,expires_at,proof_kind,proof_digest,operation_digest) VALUES($1,$2,$3,$4,$5,'ACTIVE',$6,$7,$8,'EMAIL',$9,$10)`, id[:], account, version, generation+1, int64(final.Generation), hiddenDigest[:], final.TrustedAt, expires, proofDigest[:], operationDigest[:]); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE c_auth.accounts SET reset_generation=$2,active_reset_intent_id=$3 WHERE account_id=$1`, account, generation+1, id[:])
		return e
	})
	if err != nil {
		var collision *pgconn.PgError
		if errors.As(err, &collision) && collision.Code == "23505" && (collision.ConstraintName == "email_reset_operation_once" || collision.ConstraintName == "email_reset_proof_once") {
			return zero, ErrConflict
		}
		return zero, err
	}
	return PasswordResetIntent{ID: id, ExpiresAt: expires, Username: username}, nil
}
