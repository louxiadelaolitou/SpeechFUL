package authprivacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"os"
	"sync"
	"testing"
	"time"
)

func TestAuthorizationSignatureMemoExactInputsAndOwnedCopies(t *testing.T) {
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("same signature is not the same authorization decision")
	signature := ed25519.Sign(key, message)
	originalPublic, originalMessage, originalSignature := bytes.Clone(public), bytes.Clone(message), bytes.Clone(signature)
	memo := &authorizationSignatureMemo{}
	for n := 0; n < 2; n++ {
		if !memo.verify(public, message, signature) {
			t.Fatal("valid signature rejected")
		}
	}
	if memo.verifications != 1 {
		t.Fatal("unchanged exact signature was recomputed", memo.verifications)
	}
	public[0] ^= 1
	if memo.verify(public, message, signature) {
		t.Fatal("changed public key accepted")
	}
	copy(public, originalPublic)
	message[0] ^= 1
	if memo.verify(public, message, signature) {
		t.Fatal("changed message accepted")
	}
	copy(message, originalMessage)
	signature[0] ^= 1
	if memo.verify(public, message, signature) {
		t.Fatal("changed signature accepted")
	}
	copy(signature, originalSignature)
	before := memo.verifications
	if !memo.verify(originalPublic, originalMessage, originalSignature) || memo.verifications != before {
		t.Fatal("caller buffer mutation poisoned the owned memo")
	}
	if memo.verify(public, message, signature[:63]) || memo.verify(public[:31], message, signature) {
		t.Fatal("malformed cryptographic inputs accepted")
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if memo.verify(other, message, signature) {
		t.Fatal("different configured key accepted cached signature")
	}
	prefix := []byte("bound-prefix/")
	prefixed := append(bytes.Clone(prefix), message...)
	prefixedSignature := ed25519.Sign(key, prefixed)
	if !memo.verifyWithPrefix(public, string(prefix), message, prefixedSignature) {
		t.Fatal("prefix signature rejected")
	}
	if memo.verifyWithPrefix(public, "other-prefix/", message, prefixedSignature) {
		t.Fatal("signature domain prefix was not bound")
	}
	if !memo.verify(public, prefixed, prefixedSignature) {
		t.Fatal("identical complete bytes differed by caller framing")
	}
}

func TestAuthorizationSignatureMemoBoundedAndConcurrent(t *testing.T) {
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	memo := &authorizationSignatureMemo{}
	for n := byte(0); n < 12; n++ {
		message := []byte{n}
		if !memo.verify(public, message, ed25519.Sign(key, message)) {
			t.Fatal("valid signature rejected")
		}
	}
	if len(memo.entries) != 4 || memo.verifications != 12 {
		t.Fatal("unbounded memo or missed verification")
	}
	before := memo.verifications
	message := []byte{0}
	if !memo.verify(public, message, ed25519.Sign(key, message)) || memo.verifications != before+1 {
		t.Fatal("evicted signature was trusted without verification")
	}
	huge := bytes.Repeat([]byte{1}, authorizationCheckpointRecordSize*2)
	signature := ed25519.Sign(key, huge)
	if !memo.verify(public, huge, signature) {
		t.Fatal("large valid signature rejected")
	}
	for _, entry := range memo.entries {
		if len(entry.message) > authorizationCheckpointRecordSize+len(authorizationCheckpointWriteMagic) {
			t.Fatal("oversized input retained")
		}
	}
	good := []byte("parallel verified bytes")
	sig := ed25519.Sign(key, good)
	bad := bytes.Clone(sig)
	bad[0] ^= 1
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				if !memo.verify(public, good, sig) || memo.verify(public, good, bad) {
					t.Error("concurrent memo changed signature decision")
				}
			}
		}()
	}
	wg.Wait()
}

func TestAuthorizationSignatureMemoNeverCachesEvidenceAuthority(t *testing.T) {
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	evidence := AuthorizationEvidence{Domain: "memo-test", Version: 1, Generation: 1, IssuedAt: now, TrustedAt: now, ValidUntil: now.Add(time.Minute)}
	signed, err := SignAuthorizationEvidence(key, evidence)
	if err != nil {
		t.Fatal(err)
	}
	gate := &PostgresAuthorizationGate{domain: evidence.Domain, signatures: &authorizationSignatureMemo{}}
	if _, err = gate.verifyEvidenceAt(signed, now, public, false); err != nil {
		t.Fatal(err)
	}
	if _, err = gate.verifyEvidenceAt(signed, now.Add(2*time.Minute), public, false); err == nil {
		t.Fatal("cached signature authorized expired evidence")
	}
	if _, err = gate.verifyEvidenceAt(signed, now.Add(-time.Minute), public, false); err == nil {
		t.Fatal("cached signature bypassed fresh clock bounds")
	}
	gate.domain = "different-domain"
	if _, err = gate.verifyEvidenceAt(signed, now, public, false); err == nil {
		t.Fatal("cached signature bypassed configured domain")
	}
	if gate.signatures.verifications != 1 {
		t.Fatal("test did not exercise cached verification")
	}
}

func TestAuthorizationSignatureMemoWarmCheckpointStillReadsBothSlots(t *testing.T) {
	s := checkpointTestStore(t)
	anchor := authorizationAnchor{Domain: "memo-slots", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
	if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeLocked(anchor) }); err != nil {
		t.Fatal(err)
	}
	read := func() {
		t.Helper()
		if _, exists, err := s.readLocked(); err != nil || !exists {
			t.Fatal("checkpoint read failed", err)
		}
	}
	read()
	before := s.signatures.verifications
	read()
	read()
	if s.signatures.verifications != before {
		t.Fatal("unchanged checkpoint signature recomputed")
	}
	anchor.Highwater = anchor.Highwater.Add(time.Second)
	if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeBusinessLocked(anchor) }); err != nil {
		t.Fatal(err)
	}
	read()
	if s.signatures.verifications != before+2 {
		t.Fatal("expected only new outer and inner signature calculations", s.signatures.verifications-before)
	}
	slots, raw := checkpointSlotsTestRead(t, s)
	for _, slot := range []int{slots.latest, 1 - slots.latest} {
		damaged := bytes.Clone(raw)
		damaged[authorizationCheckpointHeaderSize+slot*authorizationCheckpointRecordSize+13] ^= 1
		if err := os.WriteFile(s.Path, damaged, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.readLocked(); err == nil {
			t.Fatal("warm memo hid changed checkpoint slot", slot)
		}
		if err := os.WriteFile(s.Path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		read()
	}
	if err := os.Truncate(s.Path, int64(len(raw)-1)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.readLocked(); err == nil {
		t.Fatal("warm memo hid file truncation")
	}
}

func TestAuthorizationSignatureMemoWarmGateRejectsRollbackPostgres(t *testing.T) {
	control := newGateControl(t)
	ctx := context.Background()
	if _, err := control.gate.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := control.gate.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	// A valid, already memoized signature is not authority for a changed SQL row.
	// This owned fixture's schema owner simulates restoring an old database
	// image; the ordinary monotonic trigger correctly forbids such an UPDATE.
	mustExec(t, control.lab.cp, `ALTER TABLE c_auth.authorization_gate DISABLE TRIGGER authorization_gate_monotonic`)
	mustExec(t, control.lab.cp, `UPDATE c_auth.authorization_gate SET trusted_high_watermark=trusted_high_watermark-interval '1 second' WHERE singleton_id=1`)
	mustExec(t, control.lab.cp, `ALTER TABLE c_auth.authorization_gate ENABLE TRIGGER authorization_gate_monotonic`)
	if _, err := control.gate.Snapshot(ctx); err == nil {
		t.Fatal("cached signed anchor accepted rolled-back SQL")
	}
	var state string
	if err := control.lab.cp.QueryRow(ctx, `SELECT gate_state FROM c_auth.authorization_gate WHERE singleton_id=1`).Scan(&state); err != nil || state != "FROZEN" {
		t.Fatal("rollback did not persist fail-closed state", err, state)
	}
}

// Isolated crypto CPU/count diagnostic, not the endpoint p95 acceptance test.
func BenchmarkAuthorizationSignatureMemoRequest(b *testing.B) {
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	type input struct{ message, signature []byte }
	var slots, anchors [128]input
	for n := range slots {
		message := make([]byte, authorizationCheckpointRecordSize-64)
		binary.BigEndian.PutUint64(message, uint64(n+1))
		slots[n] = input{message, ed25519.Sign(key, message)}
		small := message[:64]
		anchors[n] = input{small, ed25519.Sign(key, small)}
	}
	evidence := input{[]byte("constant valid independent evidence"), nil}
	evidence.signature = ed25519.Sign(key, evidence.message)
	for _, cached := range []bool{false, true} {
		name := "fresh"
		if cached {
			name = "memo"
		}
		b.Run(name, func(b *testing.B) {
			var anchorMemo, evidenceMemo *authorizationSignatureMemo
			if cached {
				anchorMemo = &authorizationSignatureMemo{}
				evidenceMemo = &authorizationSignatureMemo{}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				i, j := n%len(slots), (n+len(slots)-1)%len(slots)
				for repeat := 0; repeat < 2; repeat++ {
					for _, item := range []input{slots[i], slots[j], anchors[i]} {
						if !anchorMemo.verify(public, item.message, item.signature) {
							b.Fatal("signature rejected")
						}
					}
					if !evidenceMemo.verify(public, evidence.message, evidence.signature) {
						b.Fatal("evidence rejected")
					}
				}
				for _, item := range []input{slots[i], slots[j]} {
					if !anchorMemo.verify(public, item.message, item.signature) {
						b.Fatal("writer rejected")
					}
				}
			}
			b.StopTimer()
			computations := float64(b.N) * 10
			if cached {
				computations = float64(anchorMemo.verifications + evidenceMemo.verifications)
			}
			b.ReportMetric(computations/float64(b.N), "verifications/op")
		})
	}
}
