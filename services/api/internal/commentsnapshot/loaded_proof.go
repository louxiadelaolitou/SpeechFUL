package commentsnapshot

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"time"
)

// LoadedProof binds a fully MAC-authenticated immutable payload to a later
// atomic liveness check. SHA-1 is only a change detector, never authentication.
// Redis hashes at most MaxSnapshotBytes; no ID array crosses the final RPC.
type LoadedProof struct {
	ref         Ref
	fingerprint [sha1.Size]byte
	size        int
}

func (s *Store) ProveLoaded(snapshot Snapshot) (LoadedProof, error) {
	data, e := Encode(s.mac, snapshot, s.config.MaxSnapshotBytes)
	if e != nil {
		return LoadedProof{}, e
	}
	return LoadedProof{snapshot.Ref, sha1.Sum(data), len(data)}, nil
}

func (s *Store) ValidateLoaded(ctx context.Context, p LoadedProof, at time.Time) error {
	if p.ref.Backend != Redis || p.ref.Epoch != s.epoch || p.size < payloadHeader+32 || p.size > s.config.MaxSnapshotBytes || !p.ref.Expires.After(at) {
		return ErrInvalid
	}
	v, e := s.call(ctx, false, []string{s.keys()[0], s.key(p.ref.ID)}, s.request("validate"), nil)
	if e != nil {
		return e
	}
	if code(v) == "INVALID" {
		return ErrInvalid
	}
	if len(v) != 4 || code(v) != "PROOF" {
		return ErrIntegrity
	}
	fingerprint, e := hex.DecodeString(str(v[1]))
	if e != nil || !hmac.Equal(fingerprint, p.fingerprint[:]) {
		return ErrIntegrity
	}
	now, e := strconv.ParseInt(str(v[2]), 10, 64)
	if e != nil {
		return ErrIntegrity
	}
	size, e := strconv.Atoi(str(v[3]))
	if e != nil || size != p.size {
		return ErrIntegrity
	}
	if !p.ref.Expires.After(time.UnixMilli(now)) {
		return ErrInvalid
	}
	return nil
}
