package authprivacy

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

func emailResetProofFor(t *testing.T, l *lab, slot protocol.SlotID) EmailResetIntentRequest {
	t.Helper()
	d, err := l.gate.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	key, _ := random32()
	r := EmailResetIntentRequest{InstallationID: [16]byte{41}, IdempotencyKey: key}
	p := protocol.EmailResetProof{Epoch: 1, Environment: sha256.Sum256([]byte("lab")), Slot: slot, Binding: protocol.EmailResetBinding(r.InstallationID, r.IdempotencyKey), Nonce: key, Expires: uint64(d.TrustedAt.Add(2 * time.Minute).Unix())}
	m := p.MessageBytes()
	copy(p.Signature[:], ed25519.Sign(l.vPrivate, m[:]))
	r.Proof = p.Encode()
	return r
}

func TestEmailResetOTPIsolationPostgres(t *testing.T) {
	l, e, capture := newOTPTestEligibility(t, MailSent)
	ctx := context.Background()
	ticket, priv := l.ticket(t)
	_, signup := l.intent(t, ticket, priv, "email_reset_otp_user")
	if _, err := l.c.CommitSignup(ctx, signup); err != nil {
		t.Fatal(err)
	}
	if _, err := l.v.ReserveAfterQualification(ctx, []byte("reset@hainanu.edu.cn"), ticket.BootstrapKey); err != nil {
		t.Fatal(err)
	}
	request := otpFixtureRequest(t, "reset@hainanu.edu.cn")
	request.Purpose = "RESET"
	accepted, err := e.RequestOTP(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	_, code := capture.captured()
	req := EmailResetIntentRequest{InstallationID: [16]byte{42}, IdempotencyKey: [32]byte{43}}
	confirm := EmailResetConfirmation{Key: [32]byte{44}, FlowID: accepted.FlowID, InstallationID: request.InstallationID, Binding: protocol.EmailResetBinding(req.InstallationID, req.IdempotencyKey), OTP: code}
	if _, err = e.ConfirmEmailLogin(ctx, EmailLoginConfirmation{Key: confirm.Key, FlowID: confirm.FlowID, InstallationID: confirm.InstallationID, Binding: confirm.Binding, OTP: code}); !errors.Is(err, ErrOTPFlowInvalid) {
		t.Fatal("RESET OTP admitted as LOGIN", err)
	}
	bad := confirm
	bad.OTP = wrongOTP(code)
	if _, err = e.ConfirmEmailReset(ctx, bad); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal(err)
	}
	if _, err = e.ConfirmEmailReset(ctx, bad); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal(err)
	}
	if count(t, l.vp, `SELECT failed_attempts FROM v_auth.otp_flows WHERE flow_id=$1`, accepted.FlowID[:]) != 1 {
		t.Fatal("retry spent another attempt")
	}
	confirm.Key = [32]byte{45}
	result, err := e.ConfirmEmailReset(ctx, confirm)
	if err != nil || result.State != "PROOF_AVAILABLE" {
		t.Fatal(result.State, err)
	}
	replay, err := e.ConfirmEmailReset(ctx, confirm)
	if err != nil || replay != result {
		t.Fatal("confirmation replay changed", err)
	}
	req.Proof = result.Proof
	if _, err = l.c.CreateEmailSession(ctx, EmailSessionRequest{Proof: req.Proof, InstallationID: req.InstallationID, IdempotencyKey: req.IdempotencyKey}); !errors.Is(err, ErrEmailProofInvalid) {
		t.Fatal("reset proof created session", err)
	}
	if _, err = l.c.WithRecoveryPaused().CreateEmailResetIntent(ctx, req); err != nil {
		t.Fatal(err)
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.accounts`) != 1 || count(t, l.vp, `SELECT count(*) FROM v_auth.email_quota`) != 1 {
		t.Fatal("reset allocated account or quota")
	}
	if calls, _ := capture.captured(); calls != 1 {
		t.Fatal("duplicate email")
	}
}

func TestEmailResetPasswordAndPausedCredentialsPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	ticket, priv := l.ticket(t)
	l.c = l.c.WithRecoveryPaused()
	intent, signup := l.intent(t, ticket, priv, "email_reset_user")
	if intent.RecoveryCode != "" || signup.RecoveryCode != "" {
		t.Fatal("paused signup exposed a code")
	}
	created, err := l.c.CommitSignup(ctx, signup)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, `INSERT INTO c_auth.passkeys(credential_id,account_id,public_key,created_at,user_handle,cose_algorithm,sign_count,backup_eligible,backed_up,created_credential_version) VALUES($1,$2,$3,clock_timestamp(),decode(repeat('01',32),'hex'),-7,0,false,false,1)`, []byte{1}, created.AccountID, []byte{2})
	proof := emailResetProofFor(t, l, ticket.Slot)
	wrong := proof
	wrong.InstallationID[0] ^= 1
	if _, err = l.c.CreateEmailResetIntent(ctx, wrong); !errors.Is(err, ErrEmailProofInvalid) {
		t.Fatal("wrong binding admitted", err)
	}
	reset, err := l.c.CreateEmailResetIntent(ctx, proof)
	if err != nil || reset.NewRecoveryCode != "" {
		t.Fatal("reset must not return a code", err)
	}
	replay, err := l.c.CreateEmailResetIntent(ctx, proof)
	if err != nil || (replay.ID != reset.ID || !replay.ExpiresAt.Equal(reset.ExpiresAt) || replay.Username != reset.Username) {
		t.Fatal("lost response created another intent", err)
	}
	req := recoveryTestRequest(t, reset)
	worker := sessionTestPasswordWorker(t, 2)
	if err = l.c.CommitPasswordReset(ctx, req, worker); err != nil {
		t.Fatal(err)
	}
	if _, err = l.c.GetCurrentSession(ctx, created.SessionToken); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("old session survived", err)
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.sessions WHERE revoked_at IS NULL`) != 0 {
		t.Fatal("reset logged in")
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.passkeys WHERE account_id=$1`, created.AccountID) != 1 {
		t.Fatal("paused credential deleted")
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.recovery_codes WHERE account_id=$1 AND activation_version=1`, created.AccountID) != 1 {
		t.Fatal("paused recovery data changed")
	}
	processor := &fixedRecoveryProcessor{}
	if err = l.c.CommitPasswordReset(ctx, req, processor); err != nil || processor.calls != 0 {
		t.Fatal("replay reran password KDF", err)
	}
	if state, err := l.c.GetPasswordResetResult(ctx, req.IdempotencyKey, req.IntentID); err != nil || state != "COMMITTED" {
		t.Fatal(state, err)
	}
	if _, err = l.c.CreateEmailResetIntent(ctx, proof); !errors.Is(err, ErrResetIntentInvalid) {
		t.Fatal("consumed proof reopened reset", err)
	}
	login := sessionTestRequest(t, "email_reset_user")
	if _, err = l.c.CreateSession(ctx, login, worker); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatal("old password survived", err)
	}
	login.Password = labResetPassword
	login.IdempotencyKey = [32]byte{81}
	if _, err = l.c.CreateSession(ctx, login, worker); err != nil {
		t.Fatal("new password cannot login", err)
	}
	if _, err = l.c.CreateRecoveryCodeResetIntent(ctx, "AAAAAAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, ErrRecoveryProofInvalid) {
		t.Fatal(err)
	}
	if _, err = l.c.CreatePasskeyResetOptions(ctx); err == nil {
		t.Fatal("paused passkey offered")
	}
}

func TestEmailResetCannotCrossPasswordWorkFencePostgres(t *testing.T) {
	for _, mode := range []string{"version", "freeze", "gate_recovery"} {
		t.Run(mode, func(t *testing.T) {
			control := newGateControl(t)
			l := control.lab
			ctx := context.Background()
			ticket, priv := l.ticket(t)
			_, signup := l.intent(t, ticket, priv, "email_reset_fence")
			created, err := l.c.CommitSignup(ctx, signup)
			if err != nil {
				t.Fatal(err)
			}
			l.c = l.c.WithRecoveryPaused()
			intent, err := l.c.CreateEmailResetIntent(ctx, emailResetProofFor(t, l, ticket.Slot))
			if err != nil {
				t.Fatal(err)
			}
			worker := &pausingRecoveryProcessor{ready: make(chan struct{}), resume: make(chan struct{})}
			answer := make(chan error, 1)
			go func() { answer <- l.c.CommitPasswordReset(ctx, recoveryTestRequest(t, intent), worker) }()
			<-worker.ready
			want := ErrResetIntentInvalid
			if mode == "version" {
				mustExec(t, l.cp, `UPDATE c_auth.accounts SET credential_version=credential_version+1 WHERE account_id=$1`, created.AccountID)
			} else {
				if err = control.gate.Freeze(ctx, "email reset fence"); err != nil {
					t.Fatal(err)
				}
				want = ErrAuthorizationUnavailable
				if mode == "gate_recovery" {
					control.clock.set(control.clock.now().Add(time.Second))
					control.recover(t, AuthorizationRecoveryNormal, 3, 3)
				}
			}
			close(worker.resume)
			if err = <-answer; !errors.Is(err, want) {
				t.Fatal("reset crossed fence", err)
			}
			if count(t, l.cp, `SELECT count(*) FROM c_auth.request_results WHERE operation='PASSWORD_RESET'`) != 0 {
				t.Fatal("failed reset committed")
			}
		})
	}
}

func TestEmailResetUnregisteredAndWrongCodePostgres(t *testing.T) {
	l, e, capture := newOTPTestEligibility(t, MailSent)
	ctx := context.Background()
	request := otpFixtureRequest(t, "newemail@hainanu.edu.cn")
	request.Purpose = "RESET"
	accepted, err := e.RequestOTP(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	_, code := capture.captured()
	req := EmailResetConfirmation{Key: [32]byte{1}, FlowID: accepted.FlowID, InstallationID: request.InstallationID, Binding: [32]byte{2}, OTP: wrongOTP(code)}
	if _, err = e.ConfirmEmailReset(ctx, req); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal(err)
	}
	if _, err = e.ConfirmEmailReset(ctx, req); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal(err)
	}
	if count(t, l.vp, `SELECT failed_attempts FROM v_auth.otp_flows WHERE flow_id=$1`, accepted.FlowID[:]) != 1 {
		t.Fatal("same wrong operation counted twice")
	}
	req.Key = [32]byte{3}
	req.OTP = code
	result, err := e.ConfirmEmailReset(ctx, req)
	if err != nil || result.State != "UNREGISTERED" || result.Proof != "" {
		t.Fatal("new email did not require explicit registration", err)
	}
	if count(t, l.vp, `SELECT count(*) FROM v_auth.email_quota`) != 0 || count(t, l.cp, `SELECT count(*) FROM c_auth.accounts`) != 0 {
		t.Fatal("login silently registered")
	}
}

func TestEmailResetPreservesBanAndClosurePostgres(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "buffer", true: "deadline"}[late], func(t *testing.T) {
			l := newLab(t)
			ctx := context.Background()
			ticket, priv := l.ticket(t)
			_, signup := l.intent(t, ticket, priv, "email_reset_closure")
			created, err := l.c.CommitSignup(ctx, signup)
			if err != nil {
				t.Fatal(err)
			}
			l.c = l.c.WithRecoveryPaused()
			intent, err := l.c.CreateEmailResetIntent(ctx, emailResetProofFor(t, l, ticket.Slot))
			if err != nil {
				t.Fatal(err)
			}
			mustExec(t, l.cp, `UPDATE c_auth.account_restrictions SET ban_state='BANNED',ban_ends_at=NULL,version=version+1 WHERE account_id=$1`, created.AccountID)
			if err = l.c.RevokeSession(ctx, created.RevokeSecret); err != nil {
				t.Fatal(err)
			}
			mustExec(t, l.cp, `UPDATE c_auth.accounts SET state='PENDING_CLOSE',closure_generation=1 WHERE account_id=$1`, created.AccountID)
			due := time.Now().Add(7 * 24 * time.Hour)
			if late {
				due = time.Now().Add(-time.Second)
			}
			mustExec(t, l.cp, `INSERT INTO c_auth.closure_requests(closure_id,status_digest,account_id,request_generation,state,due_at) VALUES($1,$2,$3,1,'PENDING',$4)`, make([]byte, 32), make([]byte, 32), created.AccountID, due)
			err = l.c.CommitPasswordReset(ctx, recoveryTestRequest(t, intent), &fixedRecoveryProcessor{})
			if late {
				if !errors.Is(err, ErrRecoveryProofInvalid) {
					t.Fatal("reset crossed closure deadline", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if count(t, l.cp, `SELECT count(*) FROM c_auth.account_restrictions WHERE account_id=$1 AND ban_state='BANNED'`, created.AccountID) != 1 || count(t, l.cp, `SELECT count(*) FROM c_auth.closure_requests WHERE account_id=$1 AND state='PENDING'`, created.AccountID) != 1 {
				t.Fatal("reset lifted ban or cancelled closure")
			}
			if late {
				if _, err = l.c.CreateEmailResetIntent(ctx, emailResetProofFor(t, l, ticket.Slot)); !errors.Is(err, ErrEmailUnregistered) {
					t.Fatal("expired closure issued intent", err)
				}
			}
		})
	}
}
