package roundmodel

import (
	"crypto/sha256"
	"encoding/json"
	"time"
)

// Tuple is an INTERNAL comparison model for the proposed single-Start option.
// It is not a final HTTP schema. ALL fields define identity; a nonce by itself
// is never a lookup key. Changed fields mean a new operation, subject to quotas.
type Tuple struct {
	Version        int
	Nonce          ID
	CreatedAt      time.Time
	Session        Digest
	AccountEpoch   uint64
	SecurityEpoch  uint64
	FilterRevision uint64
	Scope          Digest
	Policy         Digest // pins all acceptance/retention parameters into identity
}

type IntentPolicy struct{ MaxAge, FutureSkew, ReceiptGrace time.Duration }

func (p IntentPolicy) Digest() Digest {
	b, _ := json.Marshal(p)
	return Digest(sha256.Sum256(append([]byte("HNUHOLE/RECO/INTENT-POLICY/V1\x00"), b...)))
}

func ExperimentalIntentPolicy() IntentPolicy {
	return IntentPolicy{10 * time.Minute, 2 * time.Minute, 5 * time.Minute}
}
func (p IntentPolicy) valid() bool {
	return p.MaxAge > 0 && p.MaxAge <= 30*time.Minute && p.FutureSkew >= 0 && p.FutureSkew <= 2*time.Minute && p.ReceiptGrace > 0 && p.ReceiptGrace <= 5*time.Minute &&
		p.MaxAge%time.Microsecond == 0 && p.FutureSkew%time.Microsecond == 0 && p.ReceiptGrace%time.Microsecond == 0
}

type IntentIdentity struct {
	Key                      Digest
	AcceptUntil, RetainUntil time.Time
}

// IntentValidator keeps Prepare and single-Start protocols replaceable. Any
// adapter must validate current authorization/control before calling one.
type IntentValidator interface {
	ValidateTuple(time.Time, Binding, Tuple) (IntentIdentity, error)
}
type TupleValidator struct{ Policy IntentPolicy }

func (v TupleValidator) ValidateTuple(now time.Time, b Binding, t Tuple) (IntentIdentity, error) {
	if !v.Policy.valid() || !validTime(now) || !b.valid() || t.Version != Version || !t.Nonce.Valid() || !validTime(t.CreatedAt) || !t.Session.valid() || !t.Scope.valid() {
		return IntentIdentity{}, ErrInvalid
	}
	if t.Session != b.Session || t.AccountEpoch != b.AccountEpoch || t.SecurityEpoch != b.SecurityEpoch || t.FilterRevision != b.FilterRevision || t.Scope != b.Scope || t.Policy != v.Policy.Digest() {
		return IntentIdentity{}, ErrScope
	}
	until := t.CreatedAt.Add(v.Policy.MaxAge)
	if t.CreatedAt.After(now.Add(v.Policy.FutureSkew)) {
		return IntentIdentity{}, ErrInvalid
	}
	if !now.Before(until) {
		return IntentIdentity{}, ErrExpired
	}
	// Bind the private account as well as the complete immutable tuple. Do not
	// include current server generation in the key: authority changes must not
	// silently reinterpret an old request as a new identity.
	payload, err := json.Marshal(struct {
		Domain  string
		Account ID
		Tuple   Tuple
	}{"HNUHOLE/RECO/START-IDENTITY/V1", b.Account, t})
	if err != nil {
		return IntentIdentity{}, ErrInvalid
	}
	return IntentIdentity{Digest(sha256.Sum256(payload)), until, until.Add(v.Policy.ReceiptGrace)}, nil
}

// Receipt stores a committed identity even after its round expires. The adapter
// must atomically lookup-or-create by Key, and retain until RetainUntil. It must
// not treat ErrExpired/ErrScope as permission to create a different round.
type Receipt struct {
	Identity IntentIdentity
	RoundID  ID
	Binding  Binding
}

func (r Receipt) Resolve(identity IntentIdentity, current Binding) (ID, error) {
	if !r.RoundID.Valid() || !r.Binding.valid() || !r.Identity.Key.valid() || !validTime(r.Identity.AcceptUntil) || !validTime(r.Identity.RetainUntil) || !r.Identity.RetainUntil.After(r.Identity.AcceptUntil) {
		return "", ErrInvalid
	}
	if r.Identity != identity {
		return "", ErrConflict
	}
	if r.Binding != current {
		return "", ErrScope
	}
	return r.RoundID, nil
}
