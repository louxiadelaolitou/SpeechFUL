package roundmodel

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
)

func candidateFixture(t *testing.T, n int) (CandidateQuery, CandidateSnapshot, Manifest) {
	t.Helper()
	q := CandidateQuery{testTime(), 42, testBinding(), []string{string(testID(100))}}
	in := recommendation.Input{Now: q.At, Seed: q.Seed, Channels: append([]string{}, q.Channels...)}
	for i := 0; i < n; i++ {
		id := string(testID(i + 1))
		in.Candidates = append(in.Candidates, recommendation.Candidate{ID: id, Channel: q.Channels[0], PublishedAt: q.At.Add(-time.Duration(i+1) * time.Minute), Ordinal: int64(i + 1), Eligible: true})
		in.Pools[i%int(recommendation.SourceCount)] = append(in.Pools[i%int(recommendation.SourceCount)], id)
	}
	s := CandidateSnapshot{in, Work{BaseKeys: n}}
	hash, e := SelectorConfigDigest(recommendation.ExperimentalConfig())
	if e != nil {
		t.Fatal(e)
	}
	m := Manifest{testID(2000), q.Binding, hash, q.Seed, q.At, ExperimentalPolicy(), []ID{}}
	return q, s, m
}

func TestCandidateBoundaryAndUnchangedSelectorIntegration(t *testing.T) {
	for _, n := range []int{0, 7, 100, 400} {
		q, s, m := candidateFixture(t, n)
		before, e := jsonCopy(s)
		if e != nil {
			t.Fatal(e)
		}
		state, e := BuildManifest(q, s, recommendation.ExperimentalConfig(), m)
		if e != nil {
			t.Fatal(n, e)
		}
		if len(state.Manifest.IDs) != min(n, 100) {
			t.Fatal("unbounded or incomplete manifest", n, len(state.Manifest.IDs))
		}
		again, e := BuildManifest(q, s, recommendation.ExperimentalConfig(), m)
		if e != nil || !reflect.DeepEqual(again, state) {
			t.Fatal("nondeterministic fixed inputs", e)
		}
		if !reflect.DeepEqual(before, s) {
			t.Fatal("candidate input mutated")
		}
	}
}

// Repository signature can be implemented by a scoped adapter. This fake only
// establishes the replaceable interface and cancellation contract, not PG IO.
type fakeRepository struct{ snapshot CandidateSnapshot }

func (f fakeRepository) ReadCandidates(ctx context.Context, _ CandidateQuery) (CandidateSnapshot, error) {
	if e := ctx.Err(); e != nil {
		return CandidateSnapshot{}, e
	}
	return f.snapshot, nil
}

var _ CandidateRepository = fakeRepository{}

func TestCandidateCancellationAndBoundaryFailures(t *testing.T) {
	q, s, m := candidateFixture(t, 4)
	var repo CandidateRepository = fakeRepository{s}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := repo.ReadCandidates(ctx, q); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	for _, change := range []func(*CandidateQuery, *CandidateSnapshot){
		func(q *CandidateQuery, s *CandidateSnapshot) { q.Seed++ },
		func(q *CandidateQuery, s *CandidateSnapshot) { q.Channels = []string{"bad"} },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Input.Now = s.Input.Now.Add(time.Second) },
		func(q *CandidateQuery, s *CandidateSnapshot) {
			s.Input.State.RoundSelected = []string{string(testID(1))}
		},
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Input.Candidates[0].ID = "not-a-uuid" },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Input.Pools[0] = make([]string, 101) },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Work.BaseKeys = 2001 },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Work.DiscussionFacts = 3073 },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Work.HeatFacts = 4001 },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Work.InterestFacts = -1 },
		func(q *CandidateQuery, s *CandidateSnapshot) { s.Work.BaseKeys = 1 },
		func(q *CandidateQuery, s *CandidateSnapshot) {
			s.Input.Signals = []recommendation.InterestSignal{{Channel: q.Channels[0], At: q.At, Weight: 1}}
		},
	} {
		next, e := jsonCopy(s)
		if e != nil {
			t.Fatal(e)
		}
		query := q
		query.Channels = append([]string{}, q.Channels...)
		change(&query, &next)
		if e := ValidateCandidates(query, next); !errors.Is(e, ErrInvalid) {
			t.Fatal("bad snapshot accepted", e)
		}
	}
	config := recommendation.ExperimentalConfig()
	config.PageSize = 20
	if _, e := BuildManifest(q, s, config, m); e == nil {
		t.Fatal("wrong page size", e)
	}
	m.Seed++
	if _, e := BuildManifest(q, s, recommendation.ExperimentalConfig(), m); !errors.Is(e, ErrInvalid) {
		t.Fatal("metadata mismatch", e)
	}
	m.Seed = q.Seed
	m.ConfigHash = testDigest(9)
	if _, e := BuildManifest(q, s, recommendation.ExperimentalConfig(), m); !errors.Is(e, ErrInvalid) {
		t.Fatal("unbound selector config", e)
	}
	m.ConfigHash, _ = SelectorConfigDigest(recommendation.ExperimentalConfig())
	m.Policy.ExposureWindow = 24 * time.Hour
	if _, e := BuildManifest(q, s, recommendation.ExperimentalConfig(), m); !errors.Is(e, ErrInvalid) {
		t.Fatal("mismatched history policy", e)
	}
	m.Policy = ExperimentalPolicy()
	// Canonical ID metadata can still have invalid features. B1 is the sole
	// feature validator and rejects the adapter fact before any manifest exists.
	m.Seed = q.Seed
	s.Input.Candidates[0].Ordinal = 0
	if _, e := BuildManifest(q, s, recommendation.ExperimentalConfig(), m); e == nil {
		t.Fatal("invalid B1 fact accepted")
	}
}
