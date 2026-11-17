package authprivacy

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

// PasswordLoginLocked is confined to username/password login. Email proofs and
// password recovery never consume or clear this budget.
type PasswordLoginLocked struct{ RetryAfterSeconds int }

func (*PasswordLoginLocked) Error() string { return "password login temporarily locked" }

func (c *Community) passwordLoginSubject(username string) [32]byte {
	return requestMAC(c.requestKey, []byte("PASSWORD_LOGIN_LIMIT/V1"), []byte(username))
}

func passwordLoginLock(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, subject [32]byte) error {
	var seconds float64
	err := q.QueryRow(ctx, `SELECT GREATEST(0,EXTRACT(EPOCH FROM (locked_until-clock_timestamp())))
		FROM c_auth.password_login_limits WHERE subject_digest=$1 AND locked_until>clock_timestamp()`, subject[:]).Scan(&seconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return ErrAuthorizationUnavailable
	}
	return &PasswordLoginLocked{RetryAfterSeconds: max(1, min(300, int(math.Ceil(seconds))))}
}

// A shared SQL row serializes final success/failure decisions across devices
// and service instances. Password hashing itself stays outside SQL locks.
func lockPasswordLogin(ctx context.Context, tx pgx.Tx, subject [32]byte) error {
	if _, err := tx.Exec(ctx, `INSERT INTO c_auth.password_login_limits(subject_digest) VALUES($1) ON CONFLICT DO NOTHING`, subject[:]); err != nil {
		return ErrAuthorizationUnavailable
	}
	var locked []byte
	if err := tx.QueryRow(ctx, `SELECT subject_digest FROM c_auth.password_login_limits WHERE subject_digest=$1 FOR UPDATE`, subject[:]).Scan(&locked); err != nil {
		return ErrAuthorizationUnavailable
	}
	return passwordLoginLock(ctx, tx, subject)
}

func (c *Community) recordPasswordLoginFailure(ctx context.Context, subject, key, mac [32]byte, generation uint64) error {
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return ErrAuthorizationUnavailable
	}
	defer c.gate.Abort(ctx, tx)
	if err = lockPasswordLogin(ctx, tx, subject); err != nil {
		return err
	}
	if found, e := sessionReplay(ctx, tx, key, mac); found || e != nil {
		return e
	}
	var failures int
	_, err = c.gate.CommitAuthorized(ctx, tx, generation, func(final AuthorizationDecision) error {
		claim, e := tx.Exec(ctx, `INSERT INTO c_auth.request_results(operation,key_digest,state,request_hmac,hmac_key_version,result_code,expires_at)
			VALUES('SESSION_CREATE',$1,'LIVE',$2,1,'AUTHENTICATION_FAILED',$3) ON CONFLICT(key_digest) DO NOTHING`, key[:], mac[:], final.TrustedAt.Add(8*24*time.Hour))
		if e != nil {
			return e
		}
		if claim.RowsAffected() == 0 {
			_, e = sessionReplay(ctx, tx, key, mac)
			return e
		}
		return tx.QueryRow(ctx, `UPDATE c_auth.password_login_limits SET
			failures=CASE WHEN locked_until IS NOT NULL THEN 1 ELSE failures+1 END,
			locked_until=CASE WHEN locked_until IS NULL AND failures>=4 THEN clock_timestamp()+interval '5 minutes' ELSE NULL END
			WHERE subject_digest=$1 RETURNING failures`, subject[:]).Scan(&failures)
	})
	if err != nil {
		return err
	}
	if failures == 5 {
		return &PasswordLoginLocked{RetryAfterSeconds: 300}
	}
	return ErrAuthenticationFailed
}
