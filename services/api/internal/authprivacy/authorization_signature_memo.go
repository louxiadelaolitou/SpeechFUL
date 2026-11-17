package authprivacy

import (
	"bytes"
	"crypto/ed25519"
	"sync"
)

// This memo stores only the mathematical result of Ed25519 verification. It
// never stores an authorization decision, parsed evidence, time or file state.
// Every hit compares the exact public key, complete signed bytes and signature.
// File reads, framing checks and all current authority checks remain mandatory.
type authorizationSignatureMemo struct {
	mu            sync.Mutex
	entries       [4]verifiedAuthorizationSignature
	verifications uint64
}
type verifiedAuthorizationSignature struct {
	public, message, signature []byte
}

func (m *authorizationSignatureMemo) verify(public ed25519.PublicKey, message, signature []byte) bool {
	return m.verifyWithPrefix(public, "", message, signature)
}

// A fixed framing prefix is compared as part of the complete message, without
// allocating another padded-slot-sized buffer on a hit.
func (m *authorizationSignatureMemo) verifyWithPrefix(public ed25519.PublicKey, prefix string, message, signature []byte) bool {
	if len(public) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return false
	}
	if m == nil || len(prefix)+len(message) > authorizationCheckpointRecordSize+len(authorizationCheckpointWriteMagic) {
		if prefix == "" {
			return ed25519.Verify(public, message, signature)
		}
		full := append([]byte(prefix), message...)
		return ed25519.Verify(public, full, signature)
	}
	m.mu.Lock()
	for i, item := range m.entries {
		if bytes.Equal(public, item.public) && len(item.message) == len(prefix)+len(message) && string(item.message[:len(prefix)]) == prefix && bytes.Equal(message, item.message[len(prefix):]) && bytes.Equal(signature, item.signature) {
			copy(m.entries[1:i+1], m.entries[:i])
			m.entries[0] = item
			m.mu.Unlock()
			return true
		}
	}
	m.verifications++
	m.mu.Unlock()
	// Verify the owned copies that will enter the memo, not caller-owned buffers
	// followed by a later copy. A failed verification is never retained.
	full := make([]byte, len(prefix)+len(message))
	copy(full, prefix)
	copy(full[len(prefix):], message)
	item := verifiedAuthorizationSignature{append([]byte(nil), public...), full, append([]byte(nil), signature...)}
	if !ed25519.Verify(ed25519.PublicKey(item.public), item.message, item.signature) {
		return false
	}
	m.mu.Lock()
	copy(m.entries[1:], m.entries[:len(m.entries)-1])
	m.entries[0] = item
	m.mu.Unlock()
	return true
}
