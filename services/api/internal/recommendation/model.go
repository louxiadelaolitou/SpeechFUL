package recommendation

import (
	"strings"
	"time"
)

type InteractionKind uint8

const (
	Like InteractionKind = iota
	Favorite
	Comment
)

// Interaction is an already eligible, timestamped fact. ActorKey is an adapter-
// private grouping key, never a public identity or a wire field. B1 uses synthetic
// actors only. Missing historical like timestamps must not be fabricated.
type Interaction struct {
	ActorKey string
	At       time.Time
	Kind     InteractionKind
	Eligible bool
}

type Candidate struct {
	ID            string
	Channel       string
	AuthorGroup   string // optional opaque PUBLIC-identity grouping, never nickname
	OwnerActorKey string // optional private actor key to exclude self contributions
	PublishedAt   time.Time
	Ordinal       int64
	Eligible      bool // supplied by adapter; NOT proof of final authorization
	Interactions  []Interaction
}

// InterestSignal is a bounded, positive input to a synthetic channel profile.
// Real ingestion, deduplication, withdrawal and collection policy belong to B2+.
type InterestSignal struct {
	Channel string
	At      time.Time
	Weight  float64
}

type TimedID struct {
	ID string
	At time.Time
}

type State struct {
	RoundSelected []string // exact IDs already selected in the current bounded round
	Delivered     []TimedID
	Exposed       []TimedID // separate from delivery; media prefetch never adds here
}

type Input struct {
	Now        time.Time
	Seed       uint64
	Channels   []string
	Candidates []Candidate
	Pools      [SourceCount][]string // independently recalled source memberships
	Signals    []InterestSignal
	State      State
}

type Features struct {
	Freshness    float64
	Interest     float64
	Heat         float64
	Participants int
	Score        float64
}

type Relaxation string

const (
	Strict           Relaxation = "strict"
	DiversityRelaxed Relaxation = "diversity_relaxed"
	DeliveryRecycled Relaxation = "delivery_recycled"
)

// Selection and Result are INTERNAL experiment output, not public response DTOs.
// They contain no content, account selectors, cursors or authorization decisions.
type Selection struct {
	ID         string
	Source     Source
	Features   Features
	Relaxation Relaxation
}

type Result struct {
	Items              []Selection
	SourceCounts       [SourceCount]int
	UniquePoolIDs      int
	Shortfall          bool
	RoundLimitReached  bool
	HasMoreWithinInput bool // describes this bounded input only, never global EOF
}

func validKey(s string) bool {
	if len(s) == 0 || len(s) > 128 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:-", r)) {
			return false
		}
	}
	return true
}

func validAt(at, now time.Time) bool { return !at.IsZero() && !at.After(now) }

func validateInput(c Config, in Input) error {
	if in.Now.IsZero() || len(in.Channels) < 1 || len(in.Channels) > c.MaxChannels ||
		len(in.Candidates) > c.MaxCandidates || len(in.Signals) > c.MaxSignals ||
		len(in.State.RoundSelected) > c.MaxRoundIDs || len(in.State.Delivered) > c.MaxHistoryIDs || len(in.State.Exposed) > c.MaxHistoryIDs {
		return ErrInput
	}
	for _, pool := range in.Pools {
		if len(pool) > c.MaxPerSource {
			return ErrInput
		}
	}
	channels := make(map[string]bool, len(in.Channels))
	for _, channel := range in.Channels {
		if !validKey(channel) || channels[channel] {
			return ErrInput
		}
		channels[channel] = true
	}
	ids := make(map[string]bool, len(in.Candidates))
	totalEvents := 0
	for _, p := range in.Candidates {
		if !validKey(p.ID) || ids[p.ID] || !channels[p.Channel] || !validAt(p.PublishedAt, in.Now) || p.Ordinal < 1 ||
			p.AuthorGroup != "" && !validKey(p.AuthorGroup) || p.OwnerActorKey != "" && !validKey(p.OwnerActorKey) ||
			len(p.Interactions) > c.MaxEventsPerCandidate {
			return ErrInput
		}
		ids[p.ID] = true
		totalEvents += len(p.Interactions)
		if totalEvents > c.MaxInteractionEvents {
			return ErrInput
		}
		for _, event := range p.Interactions {
			if !validKey(event.ActorKey) || !validAt(event.At, in.Now) || event.At.Before(p.PublishedAt) || event.Kind > Comment {
				return ErrInput
			}
		}
	}
	for _, pool := range in.Pools {
		for _, id := range pool {
			if !ids[id] {
				return ErrInput
			}
		}
	}
	for _, signal := range in.Signals {
		if !channels[signal.Channel] || !validAt(signal.At, in.Now) || !finite(signal.Weight) || signal.Weight <= 0 || signal.Weight > 10 {
			return ErrInput
		}
	}
	roundIDs := make(map[string]bool, len(in.State.RoundSelected))
	for _, id := range in.State.RoundSelected {
		if !validKey(id) || roundIDs[id] {
			return ErrInput
		}
		roundIDs[id] = true
	}
	for _, entries := range [][]TimedID{in.State.Delivered, in.State.Exposed} {
		for _, e := range entries {
			if !validKey(e.ID) || !validAt(e.At, in.Now) {
				return ErrInput
			}
		}
	}
	return nil
}
