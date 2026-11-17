package authprivacy

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

func TestControlledLabRegistrationClockBoundaryPostgres(t *testing.T) {
	control := newGateControl(t)
	l, ctx := control.lab, context.Background()
	window := time.Duration(protocol.AdmissionWindowSeconds) * time.Second
	// Deliberately separate the controlled clock from PostgreSQL wall time.
	// No sleep, system-clock adjustment or dependence on the current half hour.
	boundary := control.clock.now().Truncate(window).Add(3 * window)
	for n, delta := range []time.Duration{-time.Microsecond, 0, time.Microsecond} {
		t.Run(fmt.Sprintf("boundary_%d", n), func(t *testing.T) {
			at := boundary.Add(delta)
			advanceClosureClock(t, control, at)
			ticket, private := l.ticket(t)
			t.Logf("fixture decision=%s ticket_window=%d expected_window=%d", at.Format(time.RFC3339Nano), ticket.Window, uint32(at.Unix()/protocol.AdmissionWindowSeconds))
			_, request := l.intent(t, ticket, private, fmt.Sprintf("controlled_window_%d", n))
			created, e := l.c.CommitSignup(ctx, request)
			if e != nil {
				t.Fatal(e)
			}
			if ticket.Window != uint32(at.Unix()/protocol.AdmissionWindowSeconds) {
				t.Fatal("controlled fixture ticket used another clock")
			}
			if account, e := l.c.AuthorizeExistingSession(ctx, created.SessionToken); e != nil || account != created.AccountID {
				t.Fatalf("controlled registration did not grant its valid session: %v", e)
			}
			if !control.clock.now().Equal(at) {
				t.Fatal("registration moved the controlled clock")
			}
		})
	}
}

func TestControlledLabRegistrationRejectsForeignWindowsPostgres(t *testing.T) {
	control := newGateControl(t)
	l, ctx := control.lab, context.Background()
	window := time.Duration(protocol.AdmissionWindowSeconds) * time.Second
	boundary := control.clock.now().Truncate(window).Add(2 * window)
	at := boundary.Add(-time.Microsecond)
	advanceClosureClock(t, control, at)
	decision, e := control.gate.Snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name   string
		window uint32
		want   error
	}{
		{"next", uint32(boundary.Unix() / protocol.AdmissionWindowSeconds), protocol.ErrTicketNotYetValid},
		{"expired", uint32(at.Unix()/protocol.AdmissionWindowSeconds) - 2, protocol.ErrTicketExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ticket, _ := l.ticket(t)
			// Emulate an independently signed issuer on another admission window.
			// This reproduces the CI next-window input without changing either clock.
			ticket.Window = tc.window
			message := ticket.MessageBytes()
			copy(ticket.Signature[:], ed25519.Sign(l.vPrivate, message[:]))
			_, e := l.c.CreateSignupIntent(ctx, ticket.Encode(), "rejected_window_"+tc.name, PasswordMaterial{Hash: [32]byte{1}, Salt: [16]byte{1}, ParametersVersion: 1}, [16]byte{1}, decision.Generation)
			if !errors.Is(e, tc.want) {
				t.Fatalf("wrong admission decision: got %v want %v", e, tc.want)
			}
		})
	}
	if count(t, l.cp, `SELECT count(*) FROM c_auth.signup_intents`) != 0 || count(t, l.cp, `SELECT count(*) FROM c_auth.accounts`) != 0 {
		t.Fatal("invalid window admitted registration state")
	}
	if after, e := control.gate.Snapshot(ctx); e != nil || after.Generation != decision.Generation {
		t.Fatalf("invalid ticket altered Gate generation: %v", e)
	}
}
