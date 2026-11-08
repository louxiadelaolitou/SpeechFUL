package authprivacyhttp

import (
	"context"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
	"net/http"
)

type emailResetBackend interface {
	ConfirmEmailReset(context.Context, authprivacy.EmailResetConfirmation) (authprivacy.EmailResetResult, error)
}

func (b *boundary) emailResetConfirmation(w http.ResponseWriter, r *http.Request, backend EligibilityBackend, key [32]byte, installation [16]byte) {
	service, ok := backend.(emailResetBackend)
	if !ok {
		b.fail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	object, err := readRequestObject(r, 2048, []string{"flowId", "otp", "binding"}, nil)
	if err != nil {
		b.bad(w, err)
		return
	}
	flow, e1 := stringField(object, "flowId")
	otp, e2 := stringField(object, "otp")
	binding, e3 := stringField(object, "binding")
	f, e4 := protocol.DecodeCanonicalBase64url(flow, 32)
	v, e5 := protocol.DecodeCanonicalBase64url(binding, 32)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || !otpPattern.MatchString(otp) {
		b.bad(w, errMalformed)
		return
	}
	req := authprivacy.EmailResetConfirmation{Key: key, InstallationID: installation, OTP: otp}
	copy(req.FlowID[:], f)
	copy(req.Binding[:], v)
	result, err := service.ConfirmEmailReset(r.Context(), req)
	if err != nil {
		b.eligibilityFailure(w, err)
		return
	}
	if result.State == "UNREGISTERED" {
		writeJSON(w, 200, map[string]string{"state": result.State})
		return
	}
	if result.State != "PROOF_AVAILABLE" || result.Proof == "" {
		b.fail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	writeJSON(w, 200, map[string]string{"state": result.State, "proof": result.Proof})
}
