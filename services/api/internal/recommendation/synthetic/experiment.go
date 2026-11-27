// Package synthetic supplies fictional inputs only. It never reads user data.
package synthetic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	r "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
)

type Metrics struct {
	Selected         int                `json:"selected"`
	UniqueChannels   int                `json:"uniqueChannels"`
	Sources          [r.SourceCount]int `json:"sources"`
	DiversityRelaxed int                `json:"diversityRelaxed"`
	DeliveryRecycled int                `json:"deliveryRecycled"`
	MeanFreshness    float64            `json:"meanFreshness"`
	MeanInterest     float64            `json:"meanInterest"`
	MeanHeat         float64            `json:"meanHeat"`
	IDs              []string           `json:"ids"`
}

type Case struct {
	Scenario                    string   `json:"scenario"`
	Variant                     string   `json:"variant"`
	Config                      r.Config `json:"config"`
	ConfigSHA256                string   `json:"configSHA256"`
	InputSHA256                 string   `json:"inputSHA256"`
	Candidates                  int      `json:"candidates"`
	First                       Metrics  `json:"first"`
	ChronologicalIDs            []string `json:"chronologicalIDs"`
	FirstVsChronologicalOverlap int      `json:"firstVsChronologicalOverlap"`
	RetryIdentical              bool     `json:"identicalInputRetryIdentical"`
	NextNewRound                Metrics  `json:"nextNewRound"`
	AdjacentOverlap             int      `json:"adjacentOverlap"`
	OverlapDenominator          int      `json:"overlapDenominator"`
	CandidateShortfall          bool     `json:"candidateShortfall"`
}

type Report struct {
	Schema   string    `json:"schema"`
	FixedNow time.Time `json:"fixedNow"`
	Seed     uint64    `json:"seed"`
	Warning  string    `json:"warning"`
	Cases    []Case    `json:"cases"`
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func dataset(n int, overlap, singleChannel, cold bool) r.Input {
	now := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	in := r.Input{Now: now, Seed: 20261025, Channels: []string{"ch-0", "ch-1", "ch-2", "ch-3", "ch-4", "ch-5", "ch-6"}}
	for i := 0; i < n; i++ {
		channel := in.Channels[i%len(in.Channels)]
		if singleChannel {
			channel = in.Channels[0]
		}
		age := time.Duration(1+(i*13)%168) * time.Hour
		p := r.Candidate{ID: fmt.Sprintf("synthetic-post-%03d", i), Channel: channel, AuthorGroup: fmt.Sprintf("public-mask-%02d", i%19),
			OwnerActorKey: fmt.Sprintf("private-owner-%d", i), PublishedAt: now.Add(-age), Ordinal: int64(n - i), Eligible: true}
		// All timestamps are explicitly fictional; no unknown historical time is
		// replaced with now. Heat is independent of the post's age inside its window.
		for actor := 0; actor < i%6; actor++ {
			p.Interactions = append(p.Interactions, r.Interaction{ActorKey: fmt.Sprintf("fictional-actor-%d", actor), At: now.Add(-30 * time.Minute), Kind: r.InteractionKind(actor % 3), Eligible: true})
		}
		in.Candidates = append(in.Candidates, p)
		if overlap {
			for s := r.Recent; s < r.SourceCount; s++ {
				in.Pools[s] = append(in.Pools[s], p.ID)
			}
		} else {
			s := i % int(r.SourceCount)
			in.Pools[s] = append(in.Pools[s], p.ID)
		}
	}
	if !cold {
		in.Signals = []r.InterestSignal{{Channel: "ch-0", At: now.Add(-time.Hour), Weight: 3}, {Channel: "ch-0", At: now.Add(-24 * time.Hour), Weight: 2}, {Channel: "ch-3", At: now.Add(-72 * time.Hour), Weight: 1}}
	}
	return in
}

func metrics(in r.Input, result r.Result) Metrics {
	m := Metrics{Selected: len(result.Items), Sources: result.SourceCounts, IDs: make([]string, 0, len(result.Items))}
	channels := map[string]string{}
	for _, p := range in.Candidates {
		channels[p.ID] = p.Channel
	}
	seen := map[string]bool{}
	for _, item := range result.Items {
		m.IDs = append(m.IDs, item.ID)
		seen[channels[item.ID]] = true
		m.MeanFreshness += item.Features.Freshness
		m.MeanInterest += item.Features.Interest
		m.MeanHeat += item.Features.Heat
		if item.Relaxation == r.DiversityRelaxed {
			m.DiversityRelaxed++
		}
		if item.Relaxation == r.DeliveryRecycled {
			m.DeliveryRecycled++
		}
	}
	m.UniqueChannels = len(seen)
	if m.Selected > 0 {
		d := float64(m.Selected)
		m.MeanFreshness /= d
		m.MeanInterest /= d
		m.MeanHeat /= d
	}
	return m
}

func overlapCount(a, b []string) int {
	seen := map[string]bool{}
	for _, id := range a {
		seen[id] = true
	}
	n := 0
	for _, id := range b {
		if seen[id] {
			n++
		}
	}
	return n
}

func chronological(in r.Input, limit int) []string {
	posts := append([]r.Candidate(nil), in.Candidates...)
	sort.Slice(posts, func(i, j int) bool {
		if !posts[i].PublishedAt.Equal(posts[j].PublishedAt) {
			return posts[i].PublishedAt.After(posts[j].PublishedAt)
		}
		return posts[i].Ordinal > posts[j].Ordinal
	})
	ids := make([]string, 0, limit)
	for _, p := range posts {
		if p.Eligible {
			ids = append(ids, p.ID)
			if len(ids) == limit {
				break
			}
		}
	}
	return ids
}

// Run is a finite sensitivity experiment, not a benchmark or an effectiveness
// evaluation. No wall-clock measurements, real users, storage, or network exist.
func Run() (Report, error) {
	report := Report{Schema: "B1_SYNTHETIC_ONLY/v1", FixedNow: time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC), Seed: 20261025,
		Warning: "Fictional inputs only. Selection changes and overlap are not real exposure, user benefit, production latency, or capacity evidence."}
	for _, scenario := range []string{"disjoint-400", "overlap-64", "small-7", "single-channel-40", "cold-start-200"} {
		for _, variant := range []string{"initial", "freshness-heavy", "interest-heavy", "exploration-two", "source-cap-20", "short-interest-half-life"} {
			var in r.Input
			switch scenario {
			case "disjoint-400":
				in = dataset(400, false, false, false)
			case "overlap-64":
				in = dataset(64, true, false, false)
			case "small-7":
				in = dataset(7, true, false, false)
			case "single-channel-40":
				in = dataset(40, false, true, false)
			case "cold-start-200":
				in = dataset(200, false, false, true)
			}
			c := r.ExperimentalConfig()
			c.Version = "b1-" + variant
			switch variant {
			case "freshness-heavy":
				c.Weights = [3]float64{.70, .15, .15}
			case "interest-heavy":
				c.Weights = [3]float64{.25, .60, .15}
			case "exploration-two":
				c.Quotas = [r.SourceCount]int{3, 3, 2, 2}
			case "short-interest-half-life":
				c.InterestHalfLife = 24 * time.Hour
			case "source-cap-20":
				c.MaxPerSource = 20
				c.MaxCandidates = 80
				used := map[string]bool{}
				for s := range in.Pools {
					if len(in.Pools[s]) > 20 {
						in.Pools[s] = in.Pools[s][:20]
					}
					for _, id := range in.Pools[s] {
						used[id] = true
					}
				}
				bounded := make([]r.Candidate, 0, len(used))
				for _, p := range in.Candidates {
					if used[p.ID] {
						bounded = append(bounded, p)
					}
				}
				in.Candidates = bounded
			}
			first, err := r.Select(c, in)
			if err != nil {
				return report, fmt.Errorf("%s/%s: %w", scenario, variant, err)
			}
			retry, err := r.Select(c, in)
			if err != nil {
				return report, err
			}
			entry := Case{Scenario: scenario, Variant: variant, Config: c, ConfigSHA256: digest(c), InputSHA256: digest(in), Candidates: len(in.Candidates),
				First: metrics(in, first), RetryIdentical: reflect.DeepEqual(first, retry), CandidateShortfall: first.Shortfall, ChronologicalIDs: chronological(in, c.PageSize)}
			entry.FirstVsChronologicalOverlap = overlapCount(entry.First.IDs, entry.ChronologicalIDs)
			// An explicit new round after only the first TWO selected posts gained
			// fictional exposure. Other delivered IDs remain eligible for recycling.
			for i, item := range first.Items {
				in.State.Delivered = append(in.State.Delivered, r.TimedID{ID: item.ID, At: in.Now})
				if i < 2 {
					in.State.Exposed = append(in.State.Exposed, r.TimedID{ID: item.ID, At: in.Now})
				}
			}
			next, err := r.Select(c, in)
			if err != nil {
				return report, err
			}
			entry.NextNewRound = metrics(in, next)
			entry.AdjacentOverlap = overlapCount(entry.First.IDs, entry.NextNewRound.IDs)
			entry.OverlapDenominator = len(entry.First.IDs)
			if len(entry.NextNewRound.IDs) < entry.OverlapDenominator {
				entry.OverlapDenominator = len(entry.NextNewRound.IDs)
			}
			report.Cases = append(report.Cases, entry)
		}
	}
	return report, nil
}
