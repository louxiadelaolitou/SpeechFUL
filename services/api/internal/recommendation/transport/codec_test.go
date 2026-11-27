package transport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"math"
	"strings"
	"testing"
	"time"

	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

func fixture(t *testing.T) (*Codec, Scope, time.Time, Key) {
	t.Helper()
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	key := Key{ID: 1, Secret: [32]byte{11}, IssueFrom: now.Add(-time.Hour), IssueUntil: now.Add(time.Hour), VerifyUntil: now.Add(time.Hour + MaxLifetime)}
	c, err := NewCodec("lab", 1, []Key{key})
	if err != nil {
		t.Fatal(err)
	}
	s := Scope{Binding: rm.Binding{Account: "aaaaaaaa-0000-4000-8000-000000000001", Session: rm.Digest{2}, SessionGeneration: 1, AccountEpoch: 3, SecurityEpoch: 4, FilterRevision: 5, Scope: rm.Digest{6}}}
	return c, s, now, key
}

func TestContextStableTupleAndCreateBeforeSeparateFromRetry(t *testing.T) {
	c, s, now, _ := fixture(t)
	cap, err := c.Context(s, now)
	if err != nil || len(cap.Context) > MaxContext {
		t.Fatal("context encoding", err)
	}
	input := Start{Version: 1, Nonce: "bbbbbbbb-0000-4000-8000-000000000001", CreatedAt: now.Add(9 * time.Minute), Context: cap.Context}
	body, _ := json.Marshal(input)
	var persisted Start
	if json.Unmarshal(body, &persisted) != nil {
		t.Fatal("persist request")
	}
	first, frozen, err := c.OpenStart(input, now.Add(9*time.Minute))
	if err != nil || frozen != s {
		t.Fatal("first tuple", err)
	}
	retry, _, err := c.OpenStart(persisted, now.Add(11*time.Minute))
	if err != nil || retry != first {
		t.Fatal("create-before invalidated original retry", err)
	}
	v := rm.TupleValidator{Policy: rm.ExperimentalIntentPolicy()}
	a, err := v.ValidateTuple(now.Add(9*time.Minute), s.Binding, first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.ValidateTuple(now.Add(11*time.Minute), s.Binding, retry)
	if err != nil || a != b {
		t.Fatal("identity changed", err)
	}
	changed := s.Binding
	changed.AccountEpoch++
	if _, err = v.ValidateTuple(now.Add(11*time.Minute), changed, retry); !errors.Is(err, rm.ErrScope) {
		t.Fatal("current epoch was silently refilled", err)
	}
	if _, err = v.ValidateTuple(now.Add(19*time.Minute), s.Binding, retry); !errors.Is(err, rm.ErrExpired) {
		t.Fatal("tuple did not expire", err)
	}
	input.CreatedAt = cap.CreateBefore
	if _, _, err = c.OpenStart(input, now.Add(11*time.Minute)); !errors.Is(err, ErrExpired) {
		t.Fatal("new tuple after create-before", err)
	}
}

func TestCiphertextBoundsCanonicalKindsEnvironmentAndTamper(t *testing.T) {
	c, s, now, key := fixture(t)
	cap, err := c.Context(s, now)
	if err != nil {
		t.Fatal(err)
	}
	r := Reference{Scope: s, RoundID: "cccccccc-0000-4000-8000-000000000001", Offset: 10, ExpiresAt: now.Add(30 * time.Minute)}
	cursor, err := c.SealReference(CursorKind, r, now)
	if err != nil || len(cursor) > MaxReference {
		t.Fatal("cursor", err)
	}
	r.Offset = 0
	r.FirstAt = now
	r.ExpiresAt = now.Add(10 * time.Minute)
	receipt, err := c.SealReference(ReceiptKind, r, now)
	if err != nil || len(receipt) > MaxReference {
		t.Fatal("receipt", err)
	}
	for _, token := range []string{cap.Context, cursor, receipt} {
		raw, _ := base64.RawURLEncoding.DecodeString(token)
		if bytes.Contains(raw, []byte(s.Binding.Account)) || bytes.Contains(raw, []byte("SessionGeneration")) {
			t.Fatal("private binding plaintext")
		}
	}
	for _, tc := range []struct{ kind, token string }{{CursorKind, receipt}, {ReceiptKind, cursor}, {ReceiptKind, cap.Context}, {CursorKind, cursor + "="}, {CursorKind, "\n" + cursor}, {CursorKind, strings.Repeat("A", MaxReference+1)}} {
		if _, err := c.OpenReference(tc.kind, tc.token, now); err == nil {
			t.Fatal("cross-kind/noncanonical accepted")
		}
	}
	for _, index := range []int{0, 1, 2, len(cursor)*3/4 - 1} {
		raw, _ := base64.RawURLEncoding.DecodeString(cursor)
		if index >= len(raw) {
			index = len(raw) - 1
		}
		raw[index] ^= 1
		if _, err := c.OpenReference(CursorKind, base64.RawURLEncoding.EncodeToString(raw), now); err == nil {
			t.Fatal("tampering accepted")
		}
	}
	other, err := NewCodec("other", 1, []Key{key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.OpenReference(CursorKind, cursor, now); err == nil {
		t.Fatal("environment crossover")
	}
	if _, err = c.OpenReference(CursorKind, cursor, now.Add(30*time.Minute)); !errors.Is(err, ErrExpired) {
		t.Fatal("cursor expiry", err)
	}
}

func TestBoundedKeyRotationRetainsOldReferences(t *testing.T) {
	c, s, now, key := fixture(t)
	cap, err := c.Context(s, now)
	if err != nil {
		t.Fatal(err)
	}
	next := Key{ID: 2, Secret: [32]byte{12}, IssueFrom: now, IssueUntil: now.Add(2 * time.Hour), VerifyUntil: now.Add(2*time.Hour + MaxLifetime)}
	rotated, err := c.Rotate(next, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	input := Start{Version: 1, Nonce: "bbbbbbbb-0000-4000-8000-000000000002", CreatedAt: now.Add(9 * time.Minute), Context: cap.Context}
	if _, _, err = rotated.OpenStart(input, now.Add(11*time.Minute)); err != nil {
		t.Fatal("old key dropped before retry", err)
	}
	third := next
	third.ID = 3
	third.Secret = [32]byte{13}
	if _, err = rotated.Rotate(third, now.Add(2*time.Minute)); err == nil {
		t.Fatal("third live key retained without bound")
	}
	if _, err = rotated.Rotate(third, key.VerifyUntil); err != nil {
		t.Fatal("expired key not released", err)
	}
	bad := key
	bad.VerifyUntil = bad.IssueUntil.Add(39 * time.Minute)
	if _, err = NewCodec("lab", 1, []Key{bad}); err == nil {
		t.Fatal("key verifier window too short")
	}
	if _, err = NewCodec("lab", 1, []Key{key, key}); err == nil {
		t.Fatal("duplicate key accepted")
	}
}

func TestProtocolChannelMaximumBindingAndChronologicalIsolation(t *testing.T) {
	c, s, now, key := fixture(t)
	s.ChannelID = "dddddddd-0000-4000-8000-000000000001"
	s.Binding.SessionGeneration = math.MaxInt64
	s.Binding.AccountEpoch = math.MaxInt64
	s.Binding.SecurityEpoch = math.MaxInt64
	s.Binding.FilterRevision = math.MaxInt64
	cap, err := c.Context(s, now)
	if err != nil || len(cap.Context) > MaxContext {
		t.Fatal("maximum context", err)
	}
	input := Start{Version: 1, Nonce: "bbbbbbbb-0000-4000-8000-000000000003", CreatedAt: now, Context: cap.Context, ChannelID: s.ChannelID}
	_, scope, err := c.OpenStart(input, now)
	if err != nil || scope != s {
		t.Fatal("maximum binding did not round trip", err)
	}
	input.ChannelID = ""
	if _, _, err = c.OpenStart(input, now); !errors.Is(err, ErrScope) {
		t.Fatal("channel changed silently", err)
	}
	r := Reference{Scope: s, RoundID: "cccccccc-0000-4000-8000-000000000001", Offset: 90, ExpiresAt: now.Add(30 * time.Minute)}
	token, err := c.SealReference(CursorKind, r, now)
	if err != nil || len(token) > MaxReference {
		t.Fatal("maximum cursor", err)
	}
	// Deliberately equal key bytes in this negative test prove AAD separation too.
	chrono, err := posts.NewCursorCodec(key.Secret[:], "lab")
	if err != nil {
		t.Fatal(err)
	}
	position := posts.CursorPosition{Scope: posts.CursorFeed, AccountID: string(s.Binding.Account), Ceiling: 2, LastAt: now, LastOrdinal: 1, Limit: 10, ExpiresAt: now.Add(30 * time.Minute), FilterRevision: 1}
	old, err := chrono.Encode(position)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.OpenReference(CursorKind, old, now); err == nil {
		t.Fatal("chronological cursor accepted as recommendation")
	}
	if _, err = chrono.Decode(token, position, now); err == nil {
		t.Fatal("recommendation cursor accepted as chronological")
	}
}
