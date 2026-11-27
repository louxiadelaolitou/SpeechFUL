package transport

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"math"
	"regexp"
	"time"

	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

type Key struct {
	ID                                 byte
	Secret                             [32]byte
	IssueFrom, IssueUntil, VerifyUntil time.Time
}
type keyState struct {
	key  Key
	aead cipher.AEAD
}
type Codec struct {
	environment string
	current     byte
	keys        map[byte]keyState
}

func NewCodec(environment string, current byte, keys []Key) (*Codec, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`).MatchString(environment) || current == 0 || len(keys) < 1 || len(keys) > 2 {
		return nil, ErrInvalid
	}
	c := &Codec{environment: environment, current: current, keys: map[byte]keyState{}}
	seen := map[[32]byte]bool{}
	for _, key := range keys {
		if key.ID == 0 || key.Secret == ([32]byte{}) || seen[key.Secret] || !ValidTime(key.IssueFrom) || !ValidTime(key.IssueUntil) || !ValidTime(key.VerifyUntil) || !key.IssueUntil.After(key.IssueFrom) || key.IssueUntil.Sub(key.IssueFrom) > 24*time.Hour || key.VerifyUntil.Before(key.IssueUntil.Add(MaxLifetime)) || key.VerifyUntil.After(key.IssueUntil.Add(time.Hour)) {
			return nil, ErrInvalid
		}
		if _, exists := c.keys[key.ID]; exists {
			return nil, ErrInvalid
		}
		block, err := aes.NewCipher(key.Secret[:])
		if err != nil {
			return nil, ErrInvalid
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, ErrInvalid
		}
		c.keys[key.ID] = keyState{key: key, aead: aead}
		seen[key.Secret] = true
	}
	if _, ok := c.keys[current]; !ok {
		return nil, ErrInvalid
	}
	return c, nil
}

// Rotate retains every still-verifiable old key and refuses a third live key.
// Reconstructed processes must restore this same bounded retained key set.
func (c *Codec) Rotate(next Key, now time.Time) (*Codec, error) {
	if c == nil || !ValidTime(now) || now.Before(next.IssueFrom) || !now.Before(next.IssueUntil) {
		return nil, ErrInvalid
	}
	keys := []Key{}
	for _, old := range c.keys {
		if now.Before(old.key.VerifyUntil) {
			keys = append(keys, old.key)
		}
	}
	keys = append(keys, next)
	return NewCodec(c.environment, next.ID, keys)
}

type binding struct {
	Account    rm.ID     `json:"a"`
	Session    rm.Digest `json:"s"`
	Generation uint64    `json:"g"`
	Epoch      uint64    `json:"e"`
	Security   uint64    `json:"x"`
	Filter     uint64    `json:"f"`
	Scope      rm.Digest `json:"q"`
}

func pack(b rm.Binding) binding {
	return binding{b.Account, b.Session, b.SessionGeneration, b.AccountEpoch, b.SecurityEpoch, b.FilterRevision, b.Scope}
}
func (b binding) unpack() rm.Binding {
	return rm.Binding{Account: b.Account, Session: b.Session, SessionGeneration: b.Generation, AccountEpoch: b.Epoch, SecurityEpoch: b.Security, FilterRevision: b.Filter, Scope: b.Scope}
}
func (b binding) valid() bool {
	return b.Account.Valid() && b.Session != (rm.Digest{}) && b.Scope != (rm.Digest{}) && b.Generation > 0 && b.Generation <= math.MaxInt64 && b.Epoch <= math.MaxInt64 && b.Security <= math.MaxInt64 && b.Filter <= math.MaxInt64
}

type payload struct {
	Version      int        `json:"v"`
	Binding      binding    `json:"b"`
	Channel      string     `json:"h,omitempty"`
	Issued       time.Time  `json:"i"`
	Expires      time.Time  `json:"e"`
	CreateBefore *time.Time `json:"c,omitempty"`
	Policy       *rm.Digest `json:"p,omitempty"`
	Round        rm.ID      `json:"r,omitempty"`
	Offset       int        `json:"o,omitempty"`
	FirstAt      *time.Time `json:"f,omitempty"`
}

func (p payload) valid(kind string) bool {
	if p.Version != Version || !p.Binding.valid() || (p.Channel != "" && !ValidID(p.Channel)) || !ValidTime(p.Issued) || !ValidTime(p.Expires) || !p.Expires.After(p.Issued) || p.Expires.Sub(p.Issued) > MaxLifetime {
		return false
	}
	if kind == ContextKind {
		return p.CreateBefore != nil && ValidTime(*p.CreateBefore) && p.CreateBefore.Equal(p.Issued.Add(ContextWindow)) && p.Expires.Equal(p.CreateBefore.Add(rm.ExperimentalIntentPolicy().MaxAge)) && p.Policy != nil && *p.Policy == rm.ExperimentalIntentPolicy().Digest() && p.Round == "" && p.Offset == 0 && p.FirstAt == nil
	}
	if p.CreateBefore != nil || p.Policy != nil || !p.Round.Valid() || p.Offset < 0 || p.Offset > 90 || p.Offset%10 != 0 {
		return false
	}
	if kind == CursorKind {
		return p.Offset > 0 && p.FirstAt == nil
	}
	return kind == ReceiptKind && p.FirstAt != nil && ValidTime(*p.FirstAt) && !p.FirstAt.After(p.Issued) && p.Expires.After(*p.FirstAt) && !p.Expires.After(p.FirstAt.Add(10*time.Minute))
}
func ValidToken(token string, max int) bool {
	if len(token) < 1 || len(token) > max {
		return false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(raw) >= 30 && base64.RawURLEncoding.EncodeToString(raw) == token
}
func (c *Codec) aad(kind string, header []byte) []byte {
	return append([]byte("HNUHOLE/RECOMMENDATION/V1\x00"+c.environment+"\x00"+kind+"\x00"), header...)
}
func tokenLimit(kind string) int {
	if kind == ContextKind {
		return MaxContext
	}
	return MaxReference
}
func (c *Codec) seal(kind string, p payload) (string, error) {
	if c == nil || !p.valid(kind) {
		return "", ErrUnavailable
	}
	k, ok := c.keys[c.current]
	if !ok || p.Issued.Before(k.key.IssueFrom) || !p.Issued.Before(k.key.IssueUntil) || p.Expires.After(k.key.VerifyUntil) {
		return "", ErrUnavailable
	}
	plain, err := json.Marshal(p)
	if err != nil {
		return "", ErrUnavailable
	}
	raw := make([]byte, 2+k.aead.NonceSize())
	raw[0], raw[1] = Version, c.current
	if _, err = rand.Read(raw[2:]); err != nil {
		return "", ErrUnavailable
	}
	raw = k.aead.Seal(raw, raw[2:], plain, c.aad(kind, raw[:2]))
	token := base64.RawURLEncoding.EncodeToString(raw)
	if len(token) > tokenLimit(kind) {
		return "", ErrUnavailable
	}
	return token, nil
}
func (c *Codec) open(kind, token string, now time.Time) (payload, error) {
	var p payload
	if c == nil || !ValidTime(now) || !ValidToken(token, tokenLimit(kind)) {
		return p, ErrInvalid
	}
	raw, _ := base64.RawURLEncoding.Strict().DecodeString(token)
	if raw[0] != Version {
		return p, ErrInvalid
	}
	k, ok := c.keys[raw[1]]
	if !ok {
		return p, ErrInvalid
	}
	if len(raw) < 2+k.aead.NonceSize()+k.aead.Overhead() {
		return p, ErrInvalid
	}
	plain, err := k.aead.Open(nil, raw[2:2+k.aead.NonceSize()], raw[2+k.aead.NonceSize():], c.aad(kind, raw[:2]))
	if err != nil {
		return p, ErrInvalid
	}
	if json.Unmarshal(plain, &p) != nil || !p.valid(kind) {
		return payload{}, ErrInvalid
	}
	canonical, err := json.Marshal(p)
	if err != nil || !bytes.Equal(canonical, plain) {
		return payload{}, ErrInvalid
	}
	if p.Issued.Before(k.key.IssueFrom) || !p.Issued.Before(k.key.IssueUntil) || p.Expires.After(k.key.VerifyUntil) || now.Before(p.Issued) {
		return payload{}, ErrInvalid
	}
	if !now.Before(p.Expires) || !now.Before(k.key.VerifyUntil) {
		return payload{}, ErrExpired
	}
	return p, nil
}
func (c *Codec) Context(scope Scope, now time.Time) (Capability, error) {
	before := now.Add(ContextWindow)
	policy := rm.ExperimentalIntentPolicy().Digest()
	p := payload{Version: Version, Binding: pack(scope.Binding), Channel: scope.ChannelID, Issued: now, Expires: before.Add(rm.ExperimentalIntentPolicy().MaxAge), CreateBefore: &before, Policy: &policy}
	token, err := c.seal(ContextKind, p)
	if err != nil {
		return Capability{}, err
	}
	return Capability{Available: true, Context: token, IssuedAt: now, CreateBefore: before}, nil
}

// OpenStart reconstructs every tuple binding field from the original encrypted
// context. Current authority is compared by the business transaction afterwards.
func (c *Codec) OpenStart(input Start, now time.Time) (rm.Tuple, Scope, error) {
	p, err := c.open(ContextKind, input.Context, now)
	if err != nil {
		return rm.Tuple{}, Scope{}, err
	}
	if input.Version != Version || !ValidID(input.Nonce) || !ValidTime(input.CreatedAt) || (input.ChannelID != "" && !ValidID(input.ChannelID)) {
		return rm.Tuple{}, Scope{}, ErrInvalid
	}
	if input.ChannelID != p.Channel {
		return rm.Tuple{}, Scope{}, ErrScope
	}
	if input.CreatedAt.Before(p.Issued) || !input.CreatedAt.Before(*p.CreateBefore) {
		return rm.Tuple{}, Scope{}, ErrExpired
	}
	b := p.Binding.unpack()
	t := rm.Tuple{Version: Version, Nonce: rm.ID(input.Nonce), CreatedAt: input.CreatedAt, Session: b.Session, AccountEpoch: b.AccountEpoch, SecurityEpoch: b.SecurityEpoch, FilterRevision: b.FilterRevision, Scope: b.Scope, Policy: *p.Policy}
	return t, Scope{Binding: b, ChannelID: p.Channel}, nil
}
func (c *Codec) SealReference(kind string, r Reference, now time.Time) (string, error) {
	p := payload{Version: Version, Binding: pack(r.Scope.Binding), Channel: r.Scope.ChannelID, Issued: now, Expires: r.ExpiresAt, Round: r.RoundID, Offset: r.Offset}
	if kind == ReceiptKind {
		p.FirstAt = &r.FirstAt
	}
	return c.seal(kind, p)
}
func (c *Codec) OpenReference(kind, token string, now time.Time) (Reference, error) {
	if kind != CursorKind && kind != ReceiptKind {
		return Reference{}, ErrInvalid
	}
	p, err := c.open(kind, token, now)
	if err != nil {
		return Reference{}, err
	}
	r := Reference{Scope: Scope{Binding: p.Binding.unpack(), ChannelID: p.Channel}, RoundID: p.Round, Offset: p.Offset, ExpiresAt: p.Expires}
	if p.FirstAt != nil {
		r.FirstAt = *p.FirstAt
	}
	return r, nil
}
