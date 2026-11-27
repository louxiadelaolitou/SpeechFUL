// Package roundmodel contains pure, bounded recommendation state transitions.
// A Binding is a comparison value, NOT proof of authorization. A future adapter
// must obtain all facts in the existing final authorized business transaction
// and atomically persist the returned round AND account history using revisions.
// This package performs no IO, reads no clock, and enables no collection.
package roundmodel

import (
	"errors"
	"math"
	"sort"
	"time"
)

const (
	Version            = 1
	PageSize           = 10
	MaxIDs             = 100
	MaxDeliveryHistory = 1024
	MaxExposureHistory = 2048
	MaxHistory         = MaxExposureHistory
)

var (
	ErrInvalid  = errors.New("invalid recommendation model input")
	ErrScope    = errors.New("recommendation binding changed")
	ErrExpired  = errors.New("recommendation operation expired")
	ErrConflict = errors.New("recommendation revision conflict")
	ErrCapacity = errors.New("recommendation capacity unavailable")
	ErrCodec    = errors.New("invalid recommendation storage record")
)

type ID string
type Digest [32]byte

func (id ID) Valid() bool {
	if len(id) != 36 || id == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for i, c := range []byte(id) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (d Digest) valid() bool { return d != (Digest{}) }
func validTime(t time.Time) bool {
	return !t.IsZero() && t.Location() == time.UTC && t.Year() >= 2000 && t.Year() <= 2100 && t.Nanosecond()%1000 == 0
}

type Binding struct {
	Account           ID
	Session           Digest // domain-separated binding to the authenticated bearer, never a public field
	SessionGeneration uint64
	AccountEpoch      uint64
	SecurityEpoch     uint64
	FilterRevision    uint64
	Scope             Digest
}

func (b Binding) valid() bool {
	return b.Account.Valid() && b.Session.valid() && b.SessionGeneration > 0 && b.Scope.valid()
}

// Policy is an immutable per-round experiment snapshot, not an enable switch.
type Policy struct {
	RoundTTL       time.Duration
	FeedbackGrace  time.Duration
	DeliveryWindow time.Duration
	ExposureWindow time.Duration
	DeliveryLimit  int
	ExposureLimit  int
}

func ExperimentalPolicy() Policy {
	return Policy{30 * time.Minute, 10 * time.Minute, 5 * time.Minute, 72 * time.Hour, MaxDeliveryHistory, MaxExposureHistory}
}

func (p Policy) Validate() error {
	if p.RoundTTL <= 0 || p.RoundTTL > 30*time.Minute || p.FeedbackGrace <= 0 || p.FeedbackGrace > 10*time.Minute ||
		p.DeliveryWindow <= 0 || p.DeliveryWindow > 5*time.Minute || p.ExposureWindow < p.FeedbackGrace || p.ExposureWindow > 72*time.Hour ||
		p.DeliveryLimit < 1 || p.DeliveryLimit > MaxDeliveryHistory || p.ExposureLimit < 1 || p.ExposureLimit > MaxExposureHistory {
		return ErrInvalid
	}
	for _, d := range []time.Duration{p.RoundTTL, p.FeedbackGrace, p.DeliveryWindow, p.ExposureWindow} {
		if d%time.Microsecond != 0 {
			return ErrInvalid
		}
	}
	return nil
}

type Manifest struct {
	RoundID    ID
	Binding    Binding
	ConfigHash Digest
	Seed       uint64
	CreatedAt  time.Time
	Policy     Policy
	IDs        []ID
}

type Page struct {
	FirstAt   time.Time
	Delivered uint16
	Omitted   uint16
	Consumed  uint16
}

type State struct {
	Version  int
	Revision uint64
	LastAt   time.Time
	Manifest Manifest
	Pages    []Page
}

type Delivery struct {
	ID    ID
	At    time.Time
	Until time.Time
}
type Exposure struct {
	ID    ID
	Until time.Time
}
type History struct {
	Account    ID
	Revision   uint64
	LastAt     time.Time
	Deliveries []Delivery
	Exposures  []Exposure
}

// Record is a private storage-model snapshot, never a public feed DTO.
type Record struct {
	State   State
	History History
}
type Context struct {
	Now     time.Time
	Binding Binding
}
type Revisions struct {
	Round   uint64
	History uint64
}

func New(m Manifest) (State, error) {
	if err := validateManifest(m); err != nil {
		return State{}, err
	}
	m.IDs = append([]ID{}, m.IDs...)
	return State{Version: Version, Revision: 1, LastAt: m.CreatedAt, Manifest: m, Pages: make([]Page, pageCount(len(m.IDs)))}, nil
}

func validateManifest(m Manifest) error {
	if !m.RoundID.Valid() || !m.Binding.valid() || !m.ConfigHash.valid() || !validTime(m.CreatedAt) || len(m.IDs) > MaxIDs || m.IDs == nil {
		return ErrInvalid
	}
	if err := m.Policy.Validate(); err != nil {
		return err
	}
	seen := make(map[ID]bool, len(m.IDs))
	for _, id := range m.IDs {
		if !id.Valid() || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

func pageCount(n int) int {
	if n == 0 {
		return 1
	}
	return (n + PageSize - 1) / PageSize
}
func pageBits(n int) uint16 { return uint16(1<<n) - 1 }
func (s State) bounds(offset int) (int, int, error) {
	n := len(s.Manifest.IDs)
	if offset < 0 || offset%PageSize != 0 || (n == 0 && offset != 0) || (n > 0 && offset >= n) {
		return 0, 0, ErrInvalid
	}
	end := offset + PageSize
	if end > n {
		end = n
	}
	return offset / PageSize, end, nil
}

func (s State) Validate() error {
	if err := validateManifest(s.Manifest); err != nil {
		return err
	}
	if s.Version != Version || s.Revision == 0 || s.Revision == math.MaxUint64 || !validTime(s.LastAt) || s.LastAt.Before(s.Manifest.CreatedAt) ||
		s.LastAt.After(s.Manifest.CreatedAt.Add(s.Manifest.Policy.RoundTTL+s.Manifest.Policy.FeedbackGrace)) || len(s.Pages) != pageCount(len(s.Manifest.IDs)) {
		return ErrInvalid
	}
	for i, p := range s.Pages {
		n := len(s.Manifest.IDs) - i*PageSize
		if n > PageSize {
			n = PageSize
		}
		bits := pageBits(n)
		if (p.Delivered|p.Omitted|p.Consumed)&^bits != 0 || p.Consumed&^p.Delivered != 0 {
			return ErrInvalid
		}
		if p.FirstAt.IsZero() {
			if p.Delivered|p.Omitted|p.Consumed != 0 {
				return ErrInvalid
			}
			continue
		}
		if !validTime(p.FirstAt) || p.FirstAt.Before(s.Manifest.CreatedAt) || !p.FirstAt.Before(s.Manifest.CreatedAt.Add(s.Manifest.Policy.RoundTTL)) || p.FirstAt.After(s.LastAt) ||
			p.Delivered|p.Omitted != bits {
			return ErrInvalid
		}
	}
	return nil
}

func (h History) Validate() error {
	if !h.Account.Valid() || h.Revision == 0 || h.Revision == math.MaxUint64 || !validTime(h.LastAt) || h.Deliveries == nil || h.Exposures == nil || len(h.Deliveries) > MaxDeliveryHistory || len(h.Exposures) > MaxExposureHistory {
		return ErrInvalid
	}
	var previous ID
	for _, d := range h.Deliveries {
		if !d.ID.Valid() || d.ID <= previous || !validTime(d.At) || !validTime(d.Until) || !d.Until.After(d.At) || d.Until.Sub(d.At) > 5*time.Minute || d.At.After(h.LastAt) {
			return ErrInvalid
		}
		previous = d.ID
	}
	previous = ""
	for _, e := range h.Exposures {
		if !e.ID.Valid() || e.ID <= previous || !validTime(e.Until) || e.Until.After(h.LastAt.Add(72*time.Hour)) {
			return ErrInvalid
		}
		previous = e.ID
	}
	return nil
}

func (r Record) Validate() error {
	if err := r.State.Validate(); err != nil {
		return err
	}
	if err := r.History.Validate(); err != nil {
		return err
	}
	if r.History.Account != r.State.Manifest.Binding.Account {
		return ErrScope
	}
	return nil
}

func (r Record) check(c Context, rev Revisions, feedback bool) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !validTime(c.Now) {
		return ErrInvalid
	}
	if c.Binding != r.State.Manifest.Binding {
		return ErrScope
	}
	if rev.Round != r.State.Revision || rev.History != r.History.Revision {
		return ErrConflict
	}
	if c.Now.Before(r.State.LastAt) || c.Now.Before(r.History.LastAt) {
		return ErrInvalid
	}
	until := r.State.Manifest.CreatedAt.Add(r.State.Manifest.Policy.RoundTTL)
	if feedback {
		until = until.Add(r.State.Manifest.Policy.FeedbackGrace)
	}
	if !c.Now.Before(until) {
		return ErrExpired
	}
	return nil
}

func clone(r Record) Record {
	r.State.Manifest.IDs = append([]ID{}, r.State.Manifest.IDs...)
	r.State.Pages = append([]Page{}, r.State.Pages...)
	r.History.Deliveries = append([]Delivery{}, r.History.Deliveries...)
	r.History.Exposures = append([]Exposure{}, r.History.Exposures...)
	return r
}

func sortHistory(h *History) {
	sort.Slice(h.Deliveries, func(i, j int) bool { return h.Deliveries[i].ID < h.Deliveries[j].ID })
	sort.Slice(h.Exposures, func(i, j int) bool { return h.Exposures[i].ID < h.Exposures[j].ID })
}
