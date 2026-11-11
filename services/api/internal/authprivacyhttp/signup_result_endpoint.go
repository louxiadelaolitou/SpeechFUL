package authprivacyhttp

import (
	"context"
	"net/http"
)

type signupResultBackend interface {
	GetSignupResult(context.Context, [32]byte, [32]byte) (string, error)
}

func (b *boundary) signupResultPublic(w http.ResponseWriter, r *http.Request, backend CommunityBackend) {
	if noBodyOrQuery(r) != nil || hasHeader(r.Header, "Idempotency-Key") {
		b.bad(w, errMalformed)
		return
	}
	key, err := sessionCapability(r.Header, "SignupResult")
	if err != nil {
		b.sessionCapabilityFailure(w, err)
		return
	}
	raw, err := headerBytes(r.Header, "Signup-Intent-ID", 32)
	if err != nil {
		b.bad(w, err)
		return
	}
	results, ok := backend.(signupResultBackend)
	if !ok {
		b.fail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	var id [32]byte
	copy(id[:], raw)
	state, err := results.GetSignupResult(r.Context(), key, id)
	if err != nil {
		b.recoveryClosureFailure(w, err)
		return
	}
	switch state {
	case "COMMITTED", "NOT_COMMITTED":
		writeJSON(w, 200, map[string]string{"state": state})
	case "PENDING":
		w.Header().Set("Retry-After", "2")
		writeJSON(w, 202, map[string]string{"state": state})
	default:
		b.fail(w, 503, "SERVICE_UNAVAILABLE", 0)
	}
}
