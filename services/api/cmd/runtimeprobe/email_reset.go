package main

import (
	"context"
	"errors"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

// Runs only against this probe's synthetic account in the isolated C/V lab.
// Respect V's real shared cooldown; no SQL edit or policy bypass accelerates it.
func (p *probe) emailPasswordReset(ctx context.Context, s state, vInstallation, cInstallation, priorOTP string, notBefore time.Time) (state, error) {
	for _, path := range []string{"/api/v1/auth/recovery-code-reset-intents", "/api/v1/auth/passkey-reset-options"} {
		if _, _, err := p.request(ctx, "POST", p.c, path, nil, nil, 503); err != nil {
			return s, err
		}
	}
	if delay := time.Until(notBefore); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return s, ctx.Err()
		}
	}
	sendKey, err := randomCapability(32)
	if err != nil {
		return s, err
	}
	requested, _, err := p.request(ctx, "POST", p.v, "/api/v1/email-reset/otp-requests", map[string]string{"email": s.Email}, map[string]string{"Idempotency-Key": sendKey, "V-Installation-ID": vInstallation}, 202)
	if err != nil {
		return s, err
	}
	flow, err := field(requested, "flowId")
	if err != nil {
		return s, err
	}
	otp, err := p.otp(ctx, s.Email, priorOTP)
	if err != nil {
		return s, err
	}
	intentKey, err := randomCapability(32)
	if err != nil {
		return s, err
	}
	confirmKey, err := randomCapability(32)
	if err != nil {
		return s, err
	}
	installRaw, err := protocol.DecodeCanonicalBase64url(cInstallation, 16)
	if err != nil {
		return s, err
	}
	keyRaw, err := protocol.DecodeCanonicalBase64url(intentKey, 32)
	if err != nil {
		return s, err
	}
	var install [16]byte
	copy(install[:], installRaw)
	var key [32]byte
	copy(key[:], keyRaw)
	binding := protocol.EmailResetBinding(install, key)
	confirmed, _, err := p.request(ctx, "POST", p.v, "/api/v1/email-reset/otp-confirmations", map[string]string{"flowId": flow, "otp": otp, "binding": protocol.EncodeCanonicalBase64url(binding[:])}, map[string]string{"Idempotency-Key": confirmKey, "V-Installation-ID": vInstallation}, 200)
	if err != nil {
		return s, err
	}
	if confirmed["state"] != "PROOF_AVAILABLE" {
		return s, errors.New("registered reset probe did not receive a reset proof")
	}
	proof, err := field(confirmed, "proof")
	if err != nil {
		return s, err
	}
	intent, _, err := p.request(ctx, "POST", p.c, "/api/v1/auth/email-reset-intents", map[string]string{"proof": proof, "installationId": cInstallation}, map[string]string{"Idempotency-Key": intentKey}, 201)
	if err != nil {
		return s, err
	}
	if _, exists := intent["newRecoveryCode"]; exists {
		return s, errors.New("email reset exposed a paused recovery code")
	}
	id, err := field(intent, "resetIntentId")
	if err != nil {
		return s, err
	}
	commitKey, err := randomCapability(32)
	if err != nil {
		return s, err
	}
	oldToken := s.Token
	s.Password = "IsolatedRuntimeChanged2026!"
	if _, _, err = p.request(ctx, "POST", p.c, "/api/v1/auth/password-resets", map[string]string{"resetIntentId": id, "newPassword": s.Password}, map[string]string{"Idempotency-Key": commitKey}, 204); err != nil {
		return s, err
	}
	outcome, _, err := p.request(ctx, "GET", p.c, "/api/v1/auth/password-reset-result", nil, map[string]string{"Authorization": "ResetResult " + commitKey, "Reset-Intent-ID": id}, 200)
	if err != nil {
		return s, err
	}
	if outcome["state"] != "COMMITTED" {
		return s, errors.New("reset result was not committed")
	}
	if _, err = p.directory(ctx, oldToken, 401); err != nil {
		return s, err
	}
	return p.login(ctx, s)
}
