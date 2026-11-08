package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"
)

func TestEmailResetProofPurposeBindingAndTime(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	var key PublicKey
	copy(key[:], pub)
	v, err := NewVerifier("lab", []TrustedKey{{Environment: "lab", Purpose: PurposeEmailReset, Epoch: 1, PublicKey: key}})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	p := EmailResetProof{Epoch: 1, Environment: sha256.Sum256([]byte("lab")), Slot: SlotID{1}, Binding: EmailResetBinding([16]byte{2}, [32]byte{3}), Nonce: [32]byte{4}, Expires: uint64(at.Add(time.Minute).Unix())}
	sign := func(p EmailResetProof) string {
		m := p.MessageBytes()
		copy(p.Signature[:], ed25519.Sign(priv, m[:]))
		return p.Encode()
	}
	wire := sign(p)
	if _, err = v.VerifyEmailLogin(wire, at); err == nil {
		t.Fatal("reset proof admitted as login")
	}
	if p.Binding == EmailLoginBinding([16]byte{2}, [32]byte{3}) {
		t.Fatal("reset and login operation bindings collide")
	}
	parsed, err := v.VerifyEmailReset(wire, at)
	if err != nil || parsed.Binding != p.Binding {
		t.Fatal("valid reset proof rejected", err)
	}
	if _, err = v.VerifyRegistrationTicket(wire, at); err == nil {
		t.Fatal("reset proof admitted as registration")
	}
	for _, change := range []func(*EmailResetProof){func(p *EmailResetProof) { p.Expires = uint64(at.Unix()) }, func(p *EmailResetProof) { p.Expires = uint64(at.Add(3 * time.Minute).Unix()) }, func(p *EmailResetProof) { p.Environment = sha256.Sum256([]byte("other")) }, func(p *EmailResetProof) { p.Binding = [32]byte{} }, func(p *EmailResetProof) { p.Slot = SlotID{} }} {
		q := p
		change(&q)
		if _, err = v.VerifyEmailReset(sign(q), at); err == nil {
			t.Fatal("invalid reset proof accepted")
		}
	}
	raw, _ := DecodeCanonicalBase64url(wire, EmailResetProofLength)
	raw[30] ^= 1
	if _, err = v.VerifyEmailReset(EncodeCanonicalBase64url(raw), at); err == nil {
		t.Fatal("forged proof accepted")
	}
	if p.Binding == EmailResetBinding([16]byte{3}, [32]byte{3}) || p.Binding == EmailResetBinding([16]byte{2}, [32]byte{4}) {
		t.Fatal("operation binding collision")
	}
}
