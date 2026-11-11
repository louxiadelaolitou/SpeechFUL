package authprivacy

import (
	"context"
	"crypto/hmac"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSignupResultNotFound = errors.New("signup result not found")

// GetSignupResult only returns a non-secret outcome for the original command.
// A missing result alone never proves non-commit: the intent lock and final
// trusted deadline ensure a delayed CommitSignup cannot later create an account.
func (c *Community) GetSignupResult(ctx context.Context, key, id [32]byte) (string, error) {
	d, err := c.gate.Snapshot(ctx)
	if err != nil {
		return "", err
	}
	if key == ([32]byte{}) || id == ([32]byte{}) {
		return "", ErrSignupResultNotFound
	}
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return "", ErrAuthorizationUnavailable
	}
	defer c.gate.Abort(ctx, tx)
	var intentState string
	var expires time.Time
	intentErr := tx.QueryRow(ctx, `SELECT state,expires_at FROM c_auth.signup_intents WHERE intent_id=$1 FOR UPDATE`, id[:]).Scan(&intentState, &expires)
	if intentErr != nil && !errors.Is(intentErr, pgx.ErrNoRows) {
		return "", ErrAuthorizationUnavailable
	}
	digestKey := digest("HNUHOLE/C-REQUEST-TOMBSTONE/V1", key[:])
	var operation, state string
	var storedID []byte
	var code *string
	err = tx.QueryRow(ctx, `SELECT operation,state,intent_id,result_code FROM c_auth.request_results WHERE key_digest=$1`, digestKey[:]).Scan(&operation, &state, &storedID, &code)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAuthorizationUnavailable
	}
	answer := "PENDING"
	var responseErr error
	if found {
		switch {
		case operation != "SIGNUP":
			responseErr = ErrSignupResultNotFound
		case state == "EXPIRED":
			responseErr = ErrExpired
		case !hmac.Equal(storedID, id[:]):
			responseErr = ErrSignupResultNotFound
		case code != nil && *code == "ACCOUNT_CREATED":
			answer = "COMMITTED"
		case code != nil && (*code == "USERNAME_UNAVAILABLE" || *code == "SLOT_UNAVAILABLE"):
			answer = "NOT_COMMITTED"
		default:
			responseErr = ErrAuthorizationUnavailable
		}
	} else if intentErr != nil || intentState == "CONSUMED" {
		responseErr = ErrSignupResultNotFound
	}
	_, err = c.gate.CommitAuthorized(ctx, tx, d.Generation, func(final AuthorizationDecision) error {
		if !found && intentErr == nil && intentState != "CONSUMED" &&
			(intentState == "ABANDONED" || intentState == "EXPIRED" || !final.TrustedAt.Before(expires)) {
			answer = "NOT_COMMITTED"
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return answer, responseErr
}
