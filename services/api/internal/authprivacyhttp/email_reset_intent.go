package authprivacyhttp

import (
	"context"
	"errors"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
	"net/http"
)

type emailResetIntentBackend interface {
	CreateEmailResetIntent(context.Context, authprivacy.EmailResetIntentRequest) (authprivacy.PasswordResetIntent, error)
}

func (b *boundary) emailResetIntent(w http.ResponseWriter, r *http.Request, backend RecoveryBackend) {
	service, ok := backend.(emailResetIntentBackend)
	if !ok {
		b.fail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	if hasHeader(r.Header, "Authorization") {
		b.bad(w, errMalformed)
		return
	}
	key, err := headerBytes(r.Header, "Idempotency-Key", 32)
	if err != nil {
		b.bad(w, err)
		return
	}
	object, err := readRequestObject(r, 2048, []string{"proof", "installationId"}, nil)
	if err != nil {
		b.bad(w, err)
		return
	}
	proof, e1 := stringField(object, "proof")
	installation, e2 := stringField(object, "installationId")
	raw, e3 := protocol.DecodeCanonicalBase64url(installation, 16)
	if e1 != nil || e2 != nil || e3 != nil {
		b.bad(w, errMalformed)
		return
	}
	if _, err = protocol.DecodeCanonicalBase64url(proof, protocol.EmailResetProofLength); err != nil {
		b.fail(w, 422, "EMAIL_PROOF_INVALID", 0)
		return
	}
	req := authprivacy.EmailResetIntentRequest{Proof: proof}
	copy(req.IdempotencyKey[:], key)
	copy(req.InstallationID[:], raw)
	result, err := service.CreateEmailResetIntent(r.Context(), req)
	if err != nil {
		if errors.Is(err, authprivacy.ErrEmailProofInvalid) || errors.Is(err, authprivacy.ErrEmailUnregistered) {
			b.sessionFailure(w, err)
		} else {
			b.recoveryClosureFailure(w, err)
		}
		return
	}
	if result.ID == ([32]byte{}) || result.ExpiresAt.IsZero() || !privateUsername.MatchString(result.Username) {
		b.fail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	writeJSON(w, 201, map[string]string{"resetIntentId": protocol.EncodeCanonicalBase64url(result.ID[:]), "expiresAt": utc(result.ExpiresAt), "username": result.Username})
}
