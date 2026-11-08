package authprivacy

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Closure paused before the Gate must not block a peer sender holding it.
func TestClosureDMSendGateOrderPostgres(t *testing.T) {
	for _, image := range []bool{false, true} {
		name := "text"
		if image {
			name = "image"
		}
		t.Run(name, func(t *testing.T) {
			control := newGateControl(t)
			l := control.lab
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			enablePostLab(t, l)
			var err error
			l.c, err = l.c.WithModeration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			f := dmGovernanceFromLab(t, l)
			request, _ := closureTestRequest(t, f.b.SessionToken)
			accepted, err := l.c.RequestAccountClosure(ctx, request, sessionTestPasswordWorker(t, 1))
			if err != nil {
				t.Fatal(err)
			}
			advanceClosureClock(t, control, accepted.DueAt)
			barrier := &identityClosureCommitBarrier{AuthorizationGate: l.c.gate, captured: make(chan struct{}), release: make(chan struct{})}
			closer := *l.c
			closer.gate = barrier
			closureDone := make(chan error, 1)
			go func() {
				closed, e := closer.finalizeAccountClosure(ctx, f.b.AccountID)
				if e == nil && !closed {
					e = errors.New("closure did not finalize")
				}
				closureDone <- e
			}()
			select {
			case <-barrier.captured:
			case <-ctx.Done():
				t.Fatal("closure did not reach pre-Gate barrier")
			}
			sendDone := make(chan error, 1)
			selection, data := imageFixture(t, "png")
			go func() {
				var e error
				var response PostResponse
				if image {
					response, e = l.c.SendDMImage(ctx, f.a.SessionToken, uuid.NewString(), DMImageIntent{IdentityID: f.ia, PeerID: f.ib, ConversationID: f.conv, GroupID: uuid.NewString(), GroupSize: 1, Selection: selection, Data: base64.StdEncoding.EncodeToString(data), Attempt: 1})
				} else {
					response, e = l.c.SendDMText(ctx, f.a.SessionToken, uuid.NewString(), DMTextIntent{IdentityID: f.ia, PeerID: f.ib, ConversationID: f.conv, Text: "probe", Attempt: 1})
				}
				if e == nil {
					r := response.Value.(DMReceipt)
					if r.State != "FAILED" || r.ErrorCode != "CONTACT_UNAVAILABLE" {
						e = errors.New("pending closure peer accepted message")
					}
				}
				sendDone <- e
			}()
			select {
			case sendErr := <-sendDone:
				close(barrier.release)
				if sendErr != nil {
					t.Fatalf("peer send while closure paused: %v", sendErr)
				}
			case <-ctx.Done():
				close(barrier.release)
				t.Fatal("peer send blocked on pre-Gate closure identity lock")
			}
			if closeErr := <-closureDone; closeErr != nil {
				t.Fatal(closeErr)
			}
			if count(t, l.cp, `SELECT count(*) FROM c_auth.accounts WHERE account_id=$1 AND state='CLOSED'`, f.b.AccountID) != 1 ||
				count(t, l.cp, `SELECT count(*) FROM public.community_identities WHERE identity_id=$1 AND deleted_at IS NOT NULL AND nickname IS NULL`, f.ib) != 1 ||
				count(t, l.cp, `SELECT count(*) FROM c_auth.receipt_outbox WHERE purpose='RELEASED'`) != 1 {
				t.Fatal("closure did not atomically commit account, identity and release")
			}

		})
	}
}

func TestDMImageInspectionOutsideGateRechecksBlockPostgres(t *testing.T) {
	control := newGateControl(t)
	l := control.lab
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	enablePostLab(t, l)
	var err error
	l.c, err = l.c.WithModeration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := dmGovernanceFromLab(t, l)
	validation := &imageValidationBarrier{Validator: l.c.postImages.validator, captured: make(chan struct{}), release: make(chan struct{})}
	stale := *l.c
	images := *l.c.postImages
	images.validator = validation
	stale.postImages = &images
	selection, data := imageFixture(t, "png")
	done := make(chan error, 1)
	go func() {
		r, e := stale.SendDMImage(ctx, f.a.SessionToken, uuid.NewString(), DMImageIntent{IdentityID: f.ia, PeerID: f.ib, ConversationID: f.conv, GroupID: uuid.NewString(), GroupSize: 1, Selection: selection, Data: base64.StdEncoding.EncodeToString(data), Attempt: 1})
		if e == nil {
			receipt := r.Value.(DMReceipt)
			if receipt.State != "FAILED" || receipt.ErrorCode != "CONTACT_UNAVAILABLE" {
				e = errors.New("late inspection did not preserve block rejection")
			}
		}
		done <- e
	}()
	select {
	case <-validation.captured:
	case <-ctx.Done():
		t.Fatal("inspection did not start")
	}
	// This requires the same sender account and final Gate: neither may remain
	// locked while the external validator is paused.
	if _, err = l.c.ChangeDMBlock(ctx, f.a.SessionToken, uuid.NewString(), DMBlockInput{IdentityID: f.ia, ConversationID: f.conv, Blocked: true}); err != nil {
		close(validation.release)
		t.Fatal("inspection retained SQL/Gate locks", err)
	}
	close(validation.release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if count(t, l.cp, `SELECT count(*) FROM c_dm.messages WHERE kind='IMAGE'`) != 0 {
		t.Fatal("late inspection bypassed committed block")
	}
}

func TestDMImageInspectionRechecksAuthorityAndReceiptPostgres(t *testing.T) {
	for _, mode := range []string{"freeze", "recovery", "revoked", "closed", "sent"} {
		t.Run(mode, func(t *testing.T) {
			control := newGateControl(t)
			l := control.lab
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			enablePostLab(t, l)
			var err error
			l.c, err = l.c.WithModeration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			f := dmGovernanceFromLab(t, l)
			barrier := &imageValidationBarrier{Validator: l.c.postImages.validator, captured: make(chan struct{}), release: make(chan struct{})}
			stale, images := *l.c, *l.c.postImages
			images.validator = barrier
			stale.postImages = &images
			selection, data := imageFixture(t, "png")
			key := uuid.NewString()
			input := DMImageIntent{IdentityID: f.ia, PeerID: f.ib, ConversationID: f.conv, GroupID: uuid.NewString(), GroupSize: 1, Selection: selection, Data: base64.StdEncoding.EncodeToString(data), Attempt: 1}
			type answer struct {
				response PostResponse
				err      error
			}
			done := make(chan answer, 1)
			go func() { r, e := stale.SendDMImage(ctx, f.a.SessionToken, key, input); done <- answer{r, e} }()
			select {
			case <-barrier.captured:
			case <-ctx.Done():
				t.Fatal("inspection did not start")
			}
			var expected DMReceipt
			switch mode {
			case "freeze", "recovery":
				if err = control.gate.Freeze(ctx, "inspection fence"); err != nil {
					t.Fatal(err)
				}
				if mode == "recovery" {
					control.recover(t, AuthorizationRecoveryNormal, 3, 3)
				}
			case "revoked":
				if err = l.c.RevokeSession(ctx, SessionRevocationSecret(f.a.SessionToken)); err != nil {
					t.Fatal(err)
				}
			case "closed":
				r, e := l.c.CloseDMCommand(ctx, f.a.SessionToken, key, 1)
				if e != nil {
					t.Fatal(e)
				}
				expected = r.Value.(DMReceipt)
			case "sent":
				r, e := l.c.SendDMImage(ctx, f.a.SessionToken, key, input)
				if e != nil {
					t.Fatal(e)
				}
				expected = r.Value.(DMReceipt)
			}
			close(barrier.release)
			got := <-done
			wantImages := 0
			if mode == "closed" || mode == "sent" {
				if got.err != nil {
					t.Fatal(got.err)
				}
				r := got.response.Value.(DMReceipt)
				if r.State != expected.State || r.ErrorCode != expected.ErrorCode || r.MessageID != expected.MessageID {
					t.Fatal("late validation changed durable receipt", r, expected)
				}
				if mode == "sent" {
					wantImages = 1
					if r.State != "SENT" {
						t.Fatal(r)
					}
					// A second Inspect would panic by closing the barrier twice.
					replay, e := stale.SendDMImage(ctx, f.a.SessionToken, key, input)
					if e != nil || replay.Value.(DMReceipt).MessageID != r.MessageID {
						t.Fatal("replay failed", e)
					}
				} else if r.ErrorCode != "COMMAND_CLOSED" {
					t.Fatal(r)
				}
			} else if !errors.Is(got.err, ErrAuthorizationUnavailable) && !errors.Is(got.err, ErrSessionInvalid) {
				t.Fatal("late inspection crossed authority fence", got.err)
			}
			if count(t, l.cp, `SELECT count(*) FROM c_dm.messages WHERE kind='IMAGE'`) != wantImages ||
				count(t, l.cp, `SELECT count(*) FROM c_dm.media`) != wantImages ||
				count(t, l.cp, `SELECT count(*) FROM c_dm.image_groups`) != wantImages ||
				count(t, l.cp, `SELECT count(*) FROM c_dm.commands WHERE command_id=$1 AND state='SENT'`, key) != wantImages {
				t.Fatal("late inspection left partial/duplicate image state")
			}
			wantReceipts := 0
			if mode == "closed" || mode == "sent" {
				wantReceipts = 1
				if count(t, l.cp, `SELECT count(*) FROM c_dm.commands WHERE owner_account=$1 AND command_id=$2 AND state=$3 AND COALESCE(error_code,'')=$4`, f.a.AccountID, key, expected.State, expected.ErrorCode) != 1 {
					t.Fatal("winning terminal receipt was not durably preserved")
				}
			}
			if count(t, l.cp, `SELECT count(*) FROM c_dm.commands WHERE owner_account=$1 AND command_id=$2`, f.a.AccountID, key) != wantReceipts {
				t.Fatal("late inspection created an unauthorized receipt")
			}
		})
	}
}
