package recommendation

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)

func fixture(n int) Input {
	in := Input{Now: fixedNow, Seed: 29, Channels: []string{"a", "b", "c", "d"}}
	for i := 0; i < n; i++ {
		p := Candidate{ID: fmt.Sprintf("p-%03d", i), Channel: in.Channels[i%4], AuthorGroup: fmt.Sprintf("author-%d", i),
			PublishedAt: fixedNow.Add(-time.Duration(i) * time.Hour), Ordinal: int64(n - i), Eligible: true}
		in.Candidates = append(in.Candidates, p)
		in.Pools[Recent] = append(in.Pools[Recent], p.ID)
	}
	return in
}

func selectOK(t *testing.T, c Config, in Input) Result {
	t.Helper()
	r, err := Select(c, in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestConfigurationRejectsInvalidExperiments(t *testing.T) {
	if err := ExperimentalConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Config){
		"zero page":           func(c *Config) { c.PageSize = 0 },
		"over page":           func(c *Config) { c.PageSize = 51 },
		"quota sum":           func(c *Config) { c.Quotas[Recent]++ },
		"negative quota":      func(c *Config) { c.Quotas[Explore] = -1 },
		"weight sum":          func(c *Config) { c.Weights[0] = .9 },
		"nan":                 func(c *Config) { c.Weights[1] = math.NaN() },
		"infinity":            func(c *Config) { c.HeatActorCap = math.Inf(1) },
		"no half life":        func(c *Config) { c.FreshnessHalfLife = 0 },
		"unbounded window":    func(c *Config) { c.HeatWindow = 366 * 24 * time.Hour },
		"bad cooldowns":       func(c *Config) { c.DeliveryCooldown = c.ExposureCooldown + time.Minute },
		"no prior":            func(c *Config) { c.InterestPrior = 0 },
		"too many candidates": func(c *Config) { c.MaxCandidates = 4097 },
		"source cap":          func(c *Config) { c.MaxPerSource = 1025 },
		"union cap":           func(c *Config) { c.MaxPerSource = 20 },
		"round cap":           func(c *Config) { c.MaxRoundIDs = 9 },
		"history cap":         func(c *Config) { c.MaxHistoryIDs = 4097 },
		"event cap":           func(c *Config) { c.MaxInteractionEvents = 65537 },
		"zero kind weight":    func(c *Config) { c.InteractionWeights[Like] = 0 },
		"author cap":          func(c *Config) { c.MaxPerAuthor = 11 },
		"invalid version":     func(c *Config) { c.Version = "secret token" },
	} {
		t.Run(name, func(t *testing.T) {
			c := ExperimentalConfig()
			change(&c)
			if _, err := Select(c, fixture(0)); err != ErrConfig {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSourceQuotasAndIndependentExploration(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(40)
	in.Pools = [SourceCount][]string{}
	for i, p := range in.Candidates {
		in.Pools[i/10] = append(in.Pools[i/10], p.ID)
	}
	r := selectOK(t, c, in)
	if r.SourceCounts != c.Quotas || len(r.Items) != 10 || !r.HasMoreWithinInput {
		t.Fatalf("%+v", r)
	}
	explored := false
	for _, item := range r.Items {
		if item.Source == Explore {
			explored = true
			if item.ID < "p-030" {
				t.Fatal("exploration was truncated to high-score recent pool")
			}
		}
	}
	if !explored {
		t.Fatal("independent exploration missing")
	}
}

func TestOverlapShortfallAndHardConstraints(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(12)
	for s := Interest; s < SourceCount; s++ {
		in.Pools[s] = append([]string(nil), in.Pools[Recent]...)
	}
	in.Pools[Recent] = append(in.Pools[Recent], "p-000") // duplicate in a source is harmless
	in.Candidates[0].Eligible = false
	in.State.RoundSelected = []string{"p-001"}
	in.State.Exposed = []TimedID{{"p-002", fixedNow}}
	in.State.Delivered = []TimedID{{"p-003", fixedNow}}
	r := selectOK(t, c, in)
	if len(r.Items) != 9 || !r.Shortfall || r.HasMoreWithinInput || r.UniquePoolIDs != 12 {
		t.Fatalf("%+v", r)
	}
	seen := map[string]bool{}
	for _, item := range r.Items {
		if seen[item.ID] || item.ID == "p-000" || item.ID == "p-001" || item.ID == "p-002" {
			t.Fatalf("hard constraint relaxed: %+v", item)
		}
		seen[item.ID] = true
	}
	if last := r.Items[len(r.Items)-1]; last.ID != "p-003" || last.Relaxation != DeliveryRecycled {
		t.Fatalf("delivery must recycle after other unseen content: %+v", last)
	}
}

func TestFallbackDoesNotRelaxDiversityBeforeTryingOtherSources(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(12)
	in.Pools = [SourceCount][]string{}
	for i := range in.Candidates {
		in.Candidates[i].Channel = "a"
		in.Candidates[i].AuthorGroup = "same-public-mask"
		if i < 8 {
			in.Pools[Recent] = append(in.Pools[Recent], in.Candidates[i].ID)
		} else {
			in.Candidates[i].Channel = "b"
			in.Candidates[i].AuthorGroup = fmt.Sprintf("other-%d", i)
			in.Pools[Interest] = append(in.Pools[Interest], in.Candidates[i].ID)
		}
	}
	r := selectOK(t, c, in)
	if len(r.Items) != 10 {
		t.Fatal(r)
	}
	for i := 0; i < 6; i++ {
		if r.Items[i].Relaxation != Strict {
			t.Fatalf("premature relaxation at %d", i)
		}
	}
	if r.Items[6].Relaxation != DiversityRelaxed {
		t.Fatal("single-author pool should then relax softly")
	}
}

func TestDeliveryExposureBoundariesAndNoPretendRead(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(3)
	in.State.Exposed = []TimedID{{"p-000", fixedNow.Add(-c.ExposureCooldown)}}
	in.State.Delivered = []TimedID{{"p-001", fixedNow.Add(-c.DeliveryCooldown)}}
	r := selectOK(t, c, in)
	for _, item := range r.Items {
		if item.Relaxation != Strict {
			t.Fatal("boundary entries should expire")
		}
	}
	in.State.Exposed[0].At = fixedNow.Add(-c.ExposureCooldown + time.Nanosecond)
	in.State.Delivered[0].At = fixedNow
	r = selectOK(t, c, in)
	if len(r.Items) != 2 || r.Items[0].ID != "p-002" || r.Items[1].ID != "p-001" || r.Items[1].Relaxation != DeliveryRecycled {
		t.Fatal(r)
	}
	// Delivery does not permanently eliminate an unexposed post, even if no new
	// candidates exist. Conversely an active exposure is never relaxed to fill 10.
}

func TestSpreadingAlternatesPublicAuthorAndChannelWhenAvailable(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(10)
	for i := range in.Candidates {
		in.Candidates[i].Channel = "a"
		in.Candidates[i].AuthorGroup = "public-mask-a"
		if i >= 5 {
			in.Candidates[i].Channel = "b"
			in.Candidates[i].AuthorGroup = "public-mask-b"
		}
	}
	r := selectOK(t, c, in)
	groups := map[string]string{}
	for _, p := range in.Candidates {
		groups[p.ID] = p.AuthorGroup
	}
	if len(r.Items) != 10 {
		t.Fatal(r)
	}
	for i := 1; i < len(r.Items); i++ {
		if groups[r.Items[i-1].ID] == groups[r.Items[i].ID] {
			t.Fatal("available alternate public identity not spread", r.Items)
		}
	}
}

func TestRoundSummaryBoundsAndContinuation(t *testing.T) {
	c := ExperimentalConfig()
	c.MaxRoundIDs = 10
	in := fixture(20)
	for i := 0; i < 8; i++ {
		in.State.RoundSelected = append(in.State.RoundSelected, in.Candidates[i].ID)
	}
	r := selectOK(t, c, in)
	if len(r.Items) != 2 || !r.RoundLimitReached || !r.Shortfall || r.HasMoreWithinInput {
		t.Fatal(r)
	}
	for _, item := range r.Items {
		in.State.RoundSelected = append(in.State.RoundSelected, item.ID)
	}
	r = selectOK(t, c, in)
	if len(r.Items) != 0 || !r.RoundLimitReached {
		t.Fatal(r)
	}
	// A new explicitly reset round can revisit IDs; this is not lifetime dedup.
	in.State = State{}
	if len(selectOK(t, c, in).Items) != 10 {
		t.Fatal("new round incorrectly blocked")
	}
}

func TestInputBoundsAndInvalidFeaturesFailClosed(t *testing.T) {
	c := ExperimentalConfig()
	for name, mutate := range map[string]func(*Input){
		"unknown pool id":       func(i *Input) { i.Pools[Recent] = []string{"unknown"} },
		"duplicate catalog":     func(i *Input) { i.Candidates = append(i.Candidates, i.Candidates[0]) },
		"future post":           func(i *Input) { i.Candidates[0].PublishedAt = fixedNow.Add(time.Second) },
		"bad channel":           func(i *Input) { i.Candidates[0].Channel = "unknown" },
		"duplicate channel":     func(i *Input) { i.Channels = append(i.Channels, "a") },
		"future state":          func(i *Input) { i.State.Delivered = []TimedID{{"p-000", fixedNow.Add(time.Second)}} },
		"duplicate round":       func(i *Input) { i.State.RoundSelected = []string{"x", "x"} },
		"oversized source":      func(i *Input) { i.Pools[Recent] = make([]string, c.MaxPerSource+1) },
		"oversized catalog":     func(i *Input) { i.Candidates = make([]Candidate, c.MaxCandidates+1) },
		"oversized state":       func(i *Input) { i.State.Exposed = make([]TimedID, c.MaxHistoryIDs+1) },
		"oversized round":       func(i *Input) { i.State.RoundSelected = make([]string, c.MaxRoundIDs+1) },
		"oversized signals":     func(i *Input) { i.Signals = make([]InterestSignal, c.MaxSignals+1) },
		"oversized post events": func(i *Input) { i.Candidates[0].Interactions = make([]Interaction, c.MaxEventsPerCandidate+1) },
		"signal nan":            func(i *Input) { i.Signals = []InterestSignal{{"a", fixedNow, math.NaN()}} },
		"missing like time": func(i *Input) {
			i.Candidates[0].Interactions = []Interaction{{ActorKey: "x", Kind: Like, Eligible: true}}
		},
		"prepublication interaction": func(i *Input) {
			i.Candidates[0].Interactions = []Interaction{{"x", fixedNow.Add(-time.Hour), Like, true}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := fixture(1)
			mutate(&in)
			if r, err := Select(c, in); err != ErrInput || len(r.Items) != 0 {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
	in := fixture(65)
	c.MaxInteractionEvents = 64
	for j := range in.Candidates {
		in.Candidates[j].Interactions = []Interaction{{"x", fixedNow, Like, true}}
	}
	if _, err := Select(c, in); err != ErrInput {
		t.Fatal("total event cap was not checked")
	}
}

func TestFeaturesStableScaleColdStartAndIndependentTimeWindow(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(1)
	in.Candidates[0].PublishedAt = fixedNow.Add(-24 * time.Hour)
	r := selectOK(t, c, in)
	f := r.Items[0].Features
	if f.Freshness != .5 || f.Interest != .5 || f.Heat != 0 || f.Score != .375 {
		t.Fatalf("unexpected cold-start features %+v", f)
	}
	in.Signals = []InterestSignal{{"a", fixedNow, 3}}
	f = selectOK(t, c, in).Items[0].Features
	if f.Interest != .75 {
		t.Fatal(f)
	}
	in.Signals[0].At = fixedNow.Add(-c.InterestWindow)
	if f := selectOK(t, c, in).Items[0].Features; f.Interest != .5 {
		t.Fatal("expired signal did not become neutral", f)
	}
	// Adding an unrelated extreme-score candidate must not renormalize this post.
	in = fixture(1)
	in.Candidates[0].PublishedAt = fixedNow.Add(-24 * time.Hour)
	want := selectOK(t, c, in).Items[0].Features
	in.Candidates = append(in.Candidates, Candidate{ID: "outlier", Channel: "d", PublishedAt: fixedNow.Add(-300 * 24 * time.Hour), Ordinal: 2, Eligible: true})
	in.Pools[Recent] = append(in.Pools[Recent], "outlier")
	if got := selectOK(t, c, in).Items[0].Features; got != want {
		t.Fatalf("batch-dependent normalization: %+v %+v", got, want)
	}
	// H uses interaction time, without another penalty based on this post's age.
	in = fixture(1)
	in.Candidates[0].PublishedAt = fixedNow.Add(-100 * 24 * time.Hour)
	in.Candidates[0].Interactions = []Interaction{{"actor", fixedNow.Add(-time.Hour), Like, true}}
	old := selectOK(t, c, in).Items[0].Features
	in.Candidates[0].PublishedAt = fixedNow.Add(-time.Hour)
	fresh := selectOK(t, c, in).Items[0].Features
	if old.Heat != fresh.Heat || old.Freshness >= fresh.Freshness {
		t.Fatal(old, fresh)
	}
}

func TestIndependentActorsAndContributionCaps(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(1)
	in.Candidates[0].PublishedAt = fixedNow.Add(-30 * 24 * time.Hour)
	in.Candidates[0].OwnerActorKey = "owner"
	add := func(actor string, kind InteractionKind, at time.Time, eligible bool) {
		in.Candidates[0].Interactions = append(in.Candidates[0].Interactions, Interaction{actor, at, kind, eligible})
	}
	add("owner", Like, fixedNow, true)
	add("hidden-comment", Comment, fixedNow, false)
	add("old", Favorite, fixedNow.Add(-c.HeatWindow), true)
	add("person", Like, fixedNow, true)
	add("person", Favorite, fixedNow, true)
	for j := 0; j < 50; j++ {
		add("person", Comment, fixedNow, true)
	}
	f := selectOK(t, c, in).Items[0].Features
	wantHeat := math.Log1p(c.HeatActorCap) / math.Log1p(c.HeatSaturation)
	if f.Participants != 1 || math.Abs(f.Heat-wantHeat) > 1e-12 {
		t.Fatalf("one actor inflated: %+v", f)
	}
	add("second-person", Like, fixedNow, true)
	f2 := selectOK(t, c, in).Items[0].Features
	if f2.Participants != 2 || f2.Heat <= f.Heat {
		t.Fatal(f, f2)
	}
	for j := 0; j < 4; j++ {
		add("second-person", Like, fixedNow, true)
	}
	if got := selectOK(t, c, in).Items[0].Features; got != f2 {
		t.Fatal("duplicate like amplified strength")
	}
}

func TestDeterministicPermutationAndInputOwnership(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(32)
	for s := Interest; s < SourceCount; s++ {
		in.Pools[s] = append([]string(nil), in.Pools[Recent]...)
	}
	in.Signals = []InterestSignal{{"a", fixedNow, 3}, {"d", fixedNow.Add(-time.Hour), .1}, {"a", fixedNow.Add(-3 * time.Hour), .3}}
	in.Candidates[0].Interactions = []Interaction{{"actor-2", fixedNow, Like, true}, {"actor-1", fixedNow, Favorite, true}}
	before, _ := json.Marshal(in)
	want := selectOK(t, c, in)
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("selector mutated caller input")
	}
	rng := rand.New(rand.NewSource(847))
	for j := 0; j < 30; j++ {
		rng.Shuffle(len(in.Candidates), func(a, b int) { in.Candidates[a], in.Candidates[b] = in.Candidates[b], in.Candidates[a] })
		rng.Shuffle(len(in.Signals), func(a, b int) { in.Signals[a], in.Signals[b] = in.Signals[b], in.Signals[a] })
		rng.Shuffle(len(in.Channels), func(a, b int) { in.Channels[a], in.Channels[b] = in.Channels[b], in.Channels[a] })
		for s := range in.Pools {
			rng.Shuffle(len(in.Pools[s]), func(a, b int) { in.Pools[s][a], in.Pools[s][b] = in.Pools[s][b], in.Pools[s][a] })
		}
		if got := selectOK(t, c, in); !reflect.DeepEqual(got, want) {
			t.Fatal("same logical input reordered output")
		}
	}
	in.Seed++
	changed := selectOK(t, c, in)
	if reflect.DeepEqual(changed, want) {
		t.Fatal("exploration seed had no effect in fixture")
	}
}

func TestRandomizedBoundedPoolInvariants(t *testing.T) {
	c := ExperimentalConfig()
	rng := rand.New(rand.NewSource(90210))
	for trial := 0; trial < 300; trial++ {
		in := fixture(rng.Intn(81))
		in.Seed = uint64(trial)
		in.Pools = [SourceCount][]string{}
		for i := range in.Candidates {
			p := &in.Candidates[i]
			p.Eligible = rng.Intn(5) != 0
			p.AuthorGroup = fmt.Sprintf("mask-%d", rng.Intn(4))
			for s := Recent; s < SourceCount; s++ {
				if rng.Intn(3) != 0 {
					in.Pools[s] = append(in.Pools[s], p.ID)
				}
			}
			switch rng.Intn(8) {
			case 0:
				in.State.RoundSelected = append(in.State.RoundSelected, p.ID)
			case 1:
				in.State.Exposed = append(in.State.Exposed, TimedID{p.ID, fixedNow})
			case 2:
				in.State.Delivered = append(in.State.Delivered, TimedID{p.ID, fixedNow})
			}
		}
		before, _ := json.Marshal(in)
		r := selectOK(t, c, in)
		again := selectOK(t, c, in)
		after, _ := json.Marshal(in)
		if string(before) != string(after) || !reflect.DeepEqual(r, again) || len(r.Items) > c.PageSize {
			t.Fatalf("trial %d", trial)
		}
		forbidden := map[string]bool{}
		available := map[string]bool{}
		delivered := map[string]bool{}
		for _, id := range in.State.RoundSelected {
			forbidden[id] = true
		}
		for _, e := range in.State.Exposed {
			forbidden[e.ID] = true
		}
		for _, e := range in.State.Delivered {
			delivered[e.ID] = true
		}
		eligible := map[string]bool{}
		for _, p := range in.Candidates {
			eligible[p.ID] = p.Eligible
		}
		for _, pool := range in.Pools {
			for _, id := range pool {
				if eligible[id] && !forbidden[id] {
					available[id] = true
				}
			}
		}
		seen := map[string]bool{}
		recycling := false
		for _, item := range r.Items {
			if forbidden[item.ID] || seen[item.ID] || !available[item.ID] {
				t.Fatalf("trial %d hard constraint: %+v", trial, r)
			}
			if delivered[item.ID] {
				recycling = true
			} else if recycling {
				t.Fatal("unseen content selected after delivery recycling")
			}
			seen[item.ID] = true
			for _, v := range []float64{item.Features.Freshness, item.Features.Interest, item.Features.Heat, item.Features.Score} {
				if !finite(v) || v < 0 || v > 1 {
					t.Fatalf("feature out of range: %v", v)
				}
			}
		}
		want := len(available)
		if want > c.PageSize {
			want = c.PageSize
		}
		if len(r.Items) != want {
			t.Fatalf("trial %d incomplete eligible fill: got %d want %d", trial, len(r.Items), want)
		}
	}
}

func FuzzSelectorBoundedAndDeterministic(f *testing.F) {
	for _, seed := range []uint64{0, 1, 17, 999999} {
		f.Add(seed, uint8(25))
	}
	f.Fuzz(func(t *testing.T, seed uint64, n uint8) {
		in := fixture(int(n) % 81)
		in.Seed = seed
		for s := Interest; s < SourceCount; s++ {
			in.Pools[s] = append([]string(nil), in.Pools[Recent]...)
		}
		a := selectOK(t, ExperimentalConfig(), in)
		b := selectOK(t, ExperimentalConfig(), in)
		if !reflect.DeepEqual(a, b) || len(a.Items) > 10 {
			t.Fatal("unstable or unbounded result")
		}
	})
}
