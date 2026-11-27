package roundmodel

import (
	"errors"
	"testing"
	"time"
)

func tuple(b Binding, created time.Time, p IntentPolicy) Tuple {
	return Tuple{Version, testID(3000), created, b.Session, b.AccountEpoch, b.SecurityEpoch, b.FilterRevision, b.Scope, p.Digest()}
}

func TestIntentReplayWindowAndCleanupProof(t *testing.T) {
	b := testBinding()
	p := ExperimentalIntentPolicy()
	v := TupleValidator{p}
	// A maximum-future timestamp is acceptable now, but remains acceptable
	// until 12 minutes from now. Retaining only first_seen+10m is unsafe.
	tup := tuple(b, testTime().Add(2*time.Minute), p)
	id, e := v.ValidateTuple(testTime(), b, tup)
	if e != nil {
		t.Fatal(e)
	}
	if id.AcceptUntil != testTime().Add(12*time.Minute) || id.RetainUntil != testTime().Add(17*time.Minute) {
		t.Fatal("wrong bounds")
	}
	receipt := Receipt{id, testID(2000), b}
	for seconds := 0; seconds < 12*60; seconds++ {
		now := testTime().Add(time.Duration(seconds) * time.Second)
		again, e := v.ValidateTuple(now, b, tup)
		if e != nil || again != id {
			t.Fatal("identity moved", seconds, e)
		}
		if round, e := receipt.Resolve(again, b); e != nil || round != testID(2000) {
			t.Fatal("recreated committed intent")
		}
		if !now.Before(id.RetainUntil) {
			t.Fatal("accepted after legal receipt removal")
		}
	}
	badCleanup := testTime().Add(p.MaxAge)
	if !testTime().Add(11*time.Minute).After(badCleanup) || !testTime().Add(11*time.Minute).Before(id.AcceptUntil) {
		t.Fatal("counterexample missing")
	}
	for _, now := range []time.Time{id.AcceptUntil, id.RetainUntil, id.RetainUntil.Add(time.Hour)} {
		if _, e := v.ValidateTuple(now, b, tup); !errors.Is(e, ErrExpired) {
			t.Fatal("same complete request resurrected after cleanup", e)
		}
	}
	if _, e := v.ValidateTuple(testTime(), b, tuple(b, testTime().Add(2*time.Minute+time.Microsecond), p)); !errors.Is(e, ErrInvalid) {
		t.Fatal("future skew")
	}
	for _, year := range []int{-1, 1} {
		if _, e := v.ValidateTuple(testTime(), b, tuple(b, testTime().AddDate(year, 0, 0), p)); e == nil {
			t.Fatal("bad client clock")
		}
	}
}

func TestEveryIntentFieldDefinesIdentityAndPolicyCannotExtendOldKey(t *testing.T) {
	b := testBinding()
	p := ExperimentalIntentPolicy()
	v := TupleValidator{p}
	original := tuple(b, testTime(), p)
	id, e := v.ValidateTuple(testTime(), b, original)
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Tuple, *Binding){
		func(t *Tuple, b *Binding) { t.Nonce = testID(3001) },
		func(t *Tuple, b *Binding) { t.CreatedAt = t.CreatedAt.Add(time.Second) },
		func(t *Tuple, b *Binding) { b.Session = testDigest(7); t.Session = b.Session },
		func(t *Tuple, b *Binding) { b.AccountEpoch++; t.AccountEpoch = b.AccountEpoch },
		func(t *Tuple, b *Binding) { b.SecurityEpoch++; t.SecurityEpoch = b.SecurityEpoch },
		func(t *Tuple, b *Binding) { b.FilterRevision++; t.FilterRevision = b.FilterRevision },
		func(t *Tuple, b *Binding) { b.Scope = testDigest(8); t.Scope = b.Scope },
		func(t *Tuple, b *Binding) { b.Account = testID(1001) },
	} {
		next, context := original, b
		change(&next, &context)
		other, e := v.ValidateTuple(testTime(), context, next)
		if e != nil || other.Key == id.Key {
			t.Fatal("changed tuple reused identity", e)
		}
	}
	changed := p
	changed.MaxAge = 20 * time.Minute
	if _, e := (TupleValidator{changed}).ValidateTuple(testTime().Add(15*time.Minute), b, original); !errors.Is(e, ErrScope) {
		t.Fatal("policy expansion revived old key", e)
	}
	next := original
	next.Policy = changed.Digest()
	newID, e := (TupleValidator{changed}).ValidateTuple(testTime().Add(15*time.Minute), b, next)
	if e != nil || newID.Key == id.Key {
		t.Fatal("policy change must be new explicit identity", e)
	}
	// Merely changing server generation must not change the lookup key, even
	// though current authority/receipt binding rejects replay of the old round.
	current := b
	current.SessionGeneration++
	same, e := v.ValidateTuple(testTime(), current, original)
	if e != nil || same.Key != id.Key {
		t.Fatal("server generation changed identity key", e)
	}
	if _, e := (Receipt{id, testID(2000), b}).Resolve(same, current); !errors.Is(e, ErrScope) {
		t.Fatal("old generation round accepted", e)
	}
}

func TestIntentScopeMalformedAndReceiptRejections(t *testing.T) {
	b := testBinding()
	p := ExperimentalIntentPolicy()
	v := TupleValidator{p}
	base := tuple(b, testTime(), p)
	for _, change := range []func(*Tuple){func(t *Tuple) { t.Session = testDigest(9) }, func(t *Tuple) { t.AccountEpoch++ }, func(t *Tuple) { t.SecurityEpoch++ }, func(t *Tuple) { t.FilterRevision++ }, func(t *Tuple) { t.Scope = testDigest(9) }, func(t *Tuple) { t.Policy = testDigest(9) }} {
		x := base
		change(&x)
		if _, e := v.ValidateTuple(testTime(), b, x); !errors.Is(e, ErrScope) {
			t.Fatal("scope mismatch", e)
		}
	}
	for _, change := range []func(*Tuple){func(t *Tuple) { t.Version = 2 }, func(t *Tuple) { t.Nonce = "bad" }, func(t *Tuple) { t.CreatedAt = time.Time{} }, func(t *Tuple) { t.Session = Digest{} }, func(t *Tuple) { t.Scope = Digest{} }} {
		x := base
		change(&x)
		if _, e := v.ValidateTuple(testTime(), b, x); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := (TupleValidator{}).ValidateTuple(testTime(), b, base); !errors.Is(e, ErrInvalid) {
		t.Fatal("missing policy")
	}
	id, e := v.ValidateTuple(testTime(), b, base)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := (Receipt{}).Resolve(id, b); !errors.Is(e, ErrInvalid) {
		t.Fatal("invalid receipt")
	}
	other := id
	other.Key = testDigest(9)
	if _, e := (Receipt{id, testID(2000), b}).Resolve(other, b); !errors.Is(e, ErrConflict) {
		t.Fatal("receipt mismatch")
	}
}
