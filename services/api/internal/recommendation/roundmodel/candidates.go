package roundmodel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
)

// CandidateRepository is a bounded read boundary, not an authorization API.
// A future PG adapter is scoped to the caller's existing final transaction; it
// must not open another pool or return cached body. B3a supplies no repository.
type CandidateRepository interface {
	ReadCandidates(context.Context, CandidateQuery) (CandidateSnapshot, error)
}
type CandidateQuery struct {
	At       time.Time
	Seed     uint64
	Binding  Binding
	Channels []string
}
type Work struct{ BaseKeys, DiscussionFacts, HeatFacts, InterestFacts int }
type CandidateSnapshot struct {
	Input recommendation.Input
	Work  Work
}

func SelectorConfigDigest(c recommendation.Config) (Digest, error) {
	if err := c.Validate(); err != nil {
		return Digest{}, err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return Digest{}, ErrInvalid
	}
	return Digest(sha256.Sum256(append([]byte("HNUHOLE/RECO/SELECTOR-CONFIG/V1\x00"), b...))), nil
}

func ValidateCandidates(q CandidateQuery, s CandidateSnapshot) error {
	if !validTime(q.At) || !validTime(s.Input.Now) || !q.Binding.valid() || len(q.Channels) < 1 || len(q.Channels) > 7 || len(s.Input.Channels) != len(q.Channels) || !q.At.Equal(s.Input.Now) || q.Seed != s.Input.Seed ||
		len(s.Input.Candidates) > 400 || len(s.Input.Signals) > 1024 || len(s.Input.State.RoundSelected) != 0 || len(s.Input.State.Delivered) > MaxDeliveryHistory || len(s.Input.State.Exposed) > MaxExposureHistory {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for i, ch := range q.Channels {
		if !ID(ch).Valid() || seen[ch] || s.Input.Channels[i] != ch {
			return ErrInvalid
		}
		seen[ch] = true
	}
	events := 0
	for _, p := range s.Input.Candidates {
		if !ID(p.ID).Valid() {
			return ErrInvalid
		}
		if len(p.Interactions) > 64 {
			return ErrInvalid
		}
		events += len(p.Interactions)
	}
	for _, p := range s.Input.Pools {
		if len(p) > 100 {
			return ErrInvalid
		}
	}
	w := s.Work
	if w.BaseKeys < 0 || w.BaseKeys > 2000 || w.DiscussionFacts < 0 || w.DiscussionFacts > 3072 || w.HeatFacts < 0 || w.HeatFacts > 4000 || w.InterestFacts < 0 || w.InterestFacts > 768 {
		return ErrInvalid
	}
	if len(s.Input.Candidates) > w.BaseKeys || events > w.HeatFacts || len(s.Input.Signals) > w.InterestFacts {
		return ErrInvalid
	}
	return nil
}

// BuildManifest connects a bounded candidate snapshot to the unchanged B1 pure
// selector. Eligibility/Work are adapter assertions, not proof of final scope,
// database scan cost, SQL count, or current projection availability.
func BuildManifest(q CandidateQuery, s CandidateSnapshot, config recommendation.Config, m Manifest) (State, error) {
	if err := ValidateCandidates(q, s); err != nil {
		return State{}, err
	}
	hash, err := SelectorConfigDigest(config)
	if err != nil {
		return State{}, err
	}
	if m.IDs == nil || len(m.IDs) != 0 || m.Binding != q.Binding || !m.CreatedAt.Equal(q.At) || m.Seed != q.Seed || config.PageSize != PageSize || config.MaxRoundIDs < MaxIDs || m.ConfigHash != hash || config.DeliveryCooldown != m.Policy.DeliveryWindow || config.ExposureCooldown != m.Policy.ExposureWindow {
		return State{}, ErrInvalid
	}
	in := s.Input
	in.State.RoundSelected = []string{}
	ids := make([]ID, 0, MaxIDs)
	for page := 0; page < MaxIDs/PageSize; page++ {
		selected, err := recommendation.Select(config, in)
		if err != nil {
			return State{}, err
		}
		if len(selected.Items) == 0 {
			break
		}
		for _, item := range selected.Items {
			ids = append(ids, ID(item.ID))
			in.State.RoundSelected = append(in.State.RoundSelected, item.ID)
		}
		if len(ids) >= MaxIDs {
			break
		}
	}
	m.IDs = ids
	return New(m)
}
