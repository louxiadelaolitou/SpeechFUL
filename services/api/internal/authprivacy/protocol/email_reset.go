package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

const PurposeEmailReset Purpose = "EMAIL-RESET/V1"
const EmailResetMessageLength = 163
const EmailResetProofLength = EmailResetMessageLength + 64

// Reset proofs have a distinct purpose and operation binding from login.
// The binding is an opaque one-operation commitment. V never receives C's
// installation or idempotency capability. Neither the email nor its hash is in C.
func EmailResetBinding(installation [16]byte, key [32]byte) [32]byte {
	value := append([]byte("HNUHOLE/EMAIL-RESET-BINDING/V1\x00"), installation[:]...)
	return sha256.Sum256(append(value, key[:]...))
}

type EmailResetProof struct {
	Epoch       uint32
	Environment [32]byte
	Slot        SlotID
	Binding     [32]byte
	Nonce       [32]byte
	Expires     uint64
	Signature   Signature
}

func (p EmailResetProof) MessageBytes() [EmailResetMessageLength]byte {
	var out [EmailResetMessageLength]byte
	n := copy(out[:], scope(PurposeEmailReset))
	binary.BigEndian.PutUint32(out[n:], p.Epoch)
	n += 4
	n += copy(out[n:], p.Environment[:])
	n += copy(out[n:], p.Slot[:])
	n += copy(out[n:], p.Binding[:])
	n += copy(out[n:], p.Nonce[:])
	binary.BigEndian.PutUint64(out[n:], p.Expires)
	return out
}
func (p EmailResetProof) Encode() string {
	m := p.MessageBytes()
	return EncodeCanonicalBase64url(append(m[:], p.Signature[:]...))
}
func (v *Verifier) VerifyEmailReset(encoded string, at time.Time) (EmailResetProof, error) {
	var p EmailResetProof
	raw, err := DecodeCanonicalBase64url(encoded, EmailResetProofLength)
	if err != nil {
		return p, err
	}
	prefix := scope(PurposeEmailReset)
	if !bytes.HasPrefix(raw, prefix) {
		return p, ErrPurpose
	}
	n := len(prefix)
	p.Epoch = binary.BigEndian.Uint32(raw[n:])
	n += 4
	copy(p.Environment[:], raw[n:n+32])
	n += 32
	copy(p.Slot[:], raw[n:n+32])
	n += 32
	copy(p.Binding[:], raw[n:n+32])
	n += 32
	copy(p.Nonce[:], raw[n:n+32])
	n += 32
	p.Expires = binary.BigEndian.Uint64(raw[n:])
	copy(p.Signature[:], raw[EmailResetMessageLength:])
	key, err := v.lookup(PurposeEmailReset, p.Epoch)
	if err != nil {
		return p, err
	}
	if p.Environment != sha256.Sum256([]byte(v.environment)) {
		return p, ErrKeyEnvironment
	}
	if p.Slot == (SlotID{}) || p.Binding == ([32]byte{}) || p.Nonce == ([32]byte{}) {
		return p, ErrEncoding
	}
	if at.Unix() < 0 || p.Expires <= uint64(at.Unix()) || p.Expires > uint64(at.Add(2*time.Minute).Unix()) {
		return p, ErrTicketExpired
	}
	message := p.MessageBytes()
	if err = VerifyStrict(key[:], message[:], p.Signature[:]); err != nil {
		return p, err
	}
	return p, nil
}
