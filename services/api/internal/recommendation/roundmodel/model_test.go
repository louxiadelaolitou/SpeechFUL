package roundmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func testTime() time.Time      { return time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC) }
func testID(i int) ID          { return ID(fmt.Sprintf("%08x-1111-4111-8111-111111111111", i)) }
func testDigest(i byte) Digest { var d Digest; d[0] = i; return d }
func testBinding() Binding     { return Binding{testID(1000), testDigest(1), 1, 1, 1, 0, testDigest(2)} }
func fixture(t *testing.T, n int) Record {
	t.Helper()
	ids := make([]ID, n)
	for i := range ids {
		ids[i] = testID(i + 1)
	}
	s, e := New(Manifest{testID(2000), testBinding(), testDigest(3), 42, testTime(), ExperimentalPolicy(), ids})
	if e != nil {
		t.Fatal(e)
	}
	return Record{s, History{testBinding().Account, 1, testTime(), []Delivery{}, []Exposure{}}}
}
func rev(r Record) Revisions { return Revisions{r.State.Revision, r.History.Revision} }
func at(r Record, d time.Duration) Context {
	return Context{testTime().Add(d), r.State.Manifest.Binding}
}
func mustProject(t *testing.T, r Record, d time.Duration, offset int, missing ...int) (Record, View) {
	t.Helper()
	next, v, e := Project(r, at(r, d), rev(r), offset, missing)
	if e != nil {
		t.Fatal(e)
	}
	return next, v
}

func TestGoldenFixedPageAndFeedback(t *testing.T) {
	var golden struct {
		Name                                                                   string
		Count                                                                  int
		FirstMissing, RetryMissing, FirstSlots, RetrySlots, AfterFeedbackSlots []int
		Next                                                                   int
		FeedbackSlots, FeedbackUnavailable                                     []int
		ExposureIDs                                                            []int
	}
	b, e := os.ReadFile("testdata/fixed-page.json")
	if e != nil || json.Unmarshal(b, &golden) != nil {
		t.Fatal("invalid golden fixture")
	}
	r := fixture(t, golden.Count)
	initial := clone(r)
	r, v := mustProject(t, r, 0, 0, golden.FirstMissing...)
	if !reflect.DeepEqual(v.Slots, golden.FirstSlots) || v.Next == nil || *v.Next != golden.Next {
		t.Fatal("first fixed page", v)
	}
	firstAt := v.FirstAt
	delivered := append([]Delivery{}, r.History.Deliveries...)
	r, v = mustProject(t, r, time.Minute, 0, golden.RetryMissing...)
	if !reflect.DeepEqual(v.Slots, golden.RetrySlots) || *v.Next != golden.Next || v.FirstAt != firstAt || !reflect.DeepEqual(delivered, r.History.Deliveries) {
		t.Fatal("retry moved/renewed fixed slots")
	}
	before := clone(r)
	r, e = Feedback(r, at(r, 2*time.Minute), rev(r), 0, golden.FeedbackSlots, golden.FeedbackUnavailable)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.History.Exposures) != len(golden.ExposureIDs) {
		t.Fatal("exposure count")
	}
	for i, id := range golden.ExposureIDs {
		if r.History.Exposures[i].ID != testID(id) || r.History.Exposures[i].Until != firstAt.Add(72*time.Hour) {
			t.Fatal("exposure anchor")
		}
	}
	duplicate, e := Feedback(r, at(r, 3*time.Minute), rev(r), 0, golden.FeedbackSlots, nil)
	if e != nil || !reflect.DeepEqual(duplicate, r) {
		t.Fatal("duplicate extended signal")
	}
	if !reflect.DeepEqual(initial, fixture(t, golden.Count)) || before.State.Pages[0].Consumed != 0 {
		t.Fatal("input mutated")
	}
	_, v = mustProject(t, r, 4*time.Minute, 0)
	if !reflect.DeepEqual(v.Slots, golden.AfterFeedbackSlots) {
		t.Fatal("omitted slot resurrected")
	}
}

func TestEmptyShortAndTailPageBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, 7, 10, 11, 99, 100} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			r := fixture(t, n)
			seen := map[ID]bool{}
			offset := 0
			for pages := 0; ; pages++ {
				if pages >= 10 {
					t.Fatal("unbounded page loop")
				}
				var v View
				r, v = mustProject(t, r, 0, offset)
				if v.End != min(offset+PageSize, n) {
					t.Fatal("boundary")
				}
				for _, id := range v.IDs {
					if seen[id] {
						t.Fatal("round duplicate")
					}
					seen[id] = true
				}
				if v.Next == nil {
					break
				}
				offset = *v.Next
			}
			if len(seen) != n {
				t.Fatal("lost IDs")
			}
			for _, bad := range []int{-1, 1, 101, 110} {
				if _, e := PageInput(r, at(r, 0), rev(r), bad); !errors.Is(e, ErrInvalid) {
					t.Fatal("bad offset", bad, e)
				}
			}
		})
	}
	r := fixture(t, 20)
	r, v := mustProject(t, r, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	if len(v.IDs) != 0 || v.Next == nil || *v.Next != 10 {
		t.Fatal("empty is not global EOF")
	}
	if _, e := Feedback(r, at(r, time.Second), rev(r), 0, []int{0}, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal("omitted member accepted")
	}
}

func TestBindingRevisionTimeAndMalformedProjection(t *testing.T) {
	r := fixture(t, 20)
	baseline := clone(r)
	for _, change := range []func(*Binding){func(b *Binding) { b.Account = testID(999) }, func(b *Binding) { b.Session = testDigest(9) }, func(b *Binding) { b.SessionGeneration++ }, func(b *Binding) { b.AccountEpoch++ }, func(b *Binding) { b.SecurityEpoch++ }, func(b *Binding) { b.FilterRevision++ }, func(b *Binding) { b.Scope = testDigest(9) }} {
		c := at(r, 0)
		change(&c.Binding)
		if _, _, e := Project(r, c, rev(r), 0, nil); !errors.Is(e, ErrScope) {
			t.Fatal("scope change", e)
		}
	}
	for _, bad := range [][]int{{-1}, {10}, {0, 0}, make([]int, 11)} {
		if _, _, e := Project(r, at(r, 0), rev(r), 0, bad); !errors.Is(e, ErrInvalid) {
			t.Fatal("bad unavailable", bad, e)
		}
	}
	if _, _, e := Project(r, at(r, 0), Revisions{0, 1}, 0, nil); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, _, e := Project(r, at(r, 30*time.Minute), rev(r), 0, nil); !errors.Is(e, ErrExpired) {
		t.Fatal(e)
	}
	if _, _, e := Project(r, at(r, -time.Microsecond), rev(r), 0, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(r, baseline) {
		t.Fatal("failed transition mutated input")
	}
	r, _ = mustProject(t, r, 2*time.Minute, 0)
	if _, e := Feedback(r, at(r, time.Minute), rev(r), 0, []int{0}, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal("clock rewind", e)
	}
}

func TestFeedbackEligibilityDeadlinesAndAtomicCapacity(t *testing.T) {
	r := fixture(t, 20)
	if _, e := Feedback(r, at(r, 0), rev(r), 10, []int{0}, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal("undelivered page")
	}
	r, _ = mustProject(t, r, 0, 0, 1)
	for _, c := range []struct{ slots, missing []int }{{[]int{1}, nil}, {[]int{0, 0}, nil}, {[]int{10}, nil}, {[]int{0}, []int{2}}, {[]int{0}, []int{0, 0}}} {
		if _, e := Feedback(r, at(r, time.Second), rev(r), 0, c.slots, c.missing); !errors.Is(e, ErrInvalid) {
			t.Fatal("forged feedback", e)
		}
	}
	if _, e := Feedback(r, at(r, 10*time.Minute), rev(r), 0, []int{0}, nil); !errors.Is(e, ErrExpired) {
		t.Fatal("deadline inclusive", e)
	}
	r.State.Manifest.Policy.ExposureLimit = 1
	before := clone(r)
	if _, e := Feedback(r, at(r, time.Second), rev(r), 0, []int{0, 2}, nil); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, r) {
		t.Fatal("partial capacity write")
	}
	accepted, e := Feedback(r, at(r, time.Second), rev(r), 0, []int{0, 2}, []int{2})
	if e != nil || len(accepted.History.Exposures) != 1 || accepted.State.Pages[0].Consumed != 5 {
		t.Fatal("unavailable should consume without signal", e)
	}
	r.State.Manifest.Policy.DeliveryLimit = 1
	r.State.Pages = make([]Page, 2)
	r.State.LastAt = testTime()
	r.History = fixture(t, 20).History
	before = clone(r)
	if _, _, e := Project(r, at(r, 0), rev(r), 0, nil); !errors.Is(e, ErrCapacity) {
		t.Fatal("delivery capacity", e)
	}
	if !reflect.DeepEqual(before, r) {
		t.Fatal("delivery failure partially changed input")
	}
}

func TestLatePageAndOutOfOrderRounds(t *testing.T) {
	a := fixture(t, 1)
	a, _ = mustProject(t, a, 0, 0)
	b := fixture(t, 1)
	b.State.Manifest.RoundID = testID(2001)
	b.State.Manifest.CreatedAt = testTime().Add(time.Minute)
	b.State.LastAt = b.State.Manifest.CreatedAt
	b.History = a.History
	b, _ = mustProject(t, b, time.Minute, 0)
	var e error
	b, e = Feedback(b, at(b, 2*time.Minute), rev(b), 0, []int{0}, nil)
	if e != nil {
		t.Fatal(e)
	}
	a.History = b.History
	a, e = Feedback(a, at(a, 3*time.Minute), rev(a), 0, []int{0}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if a.History.Exposures[0].Until != testTime().Add(time.Minute+72*time.Hour) {
		t.Fatal("old signal overwrote newer expiry")
	}
	late := fixture(t, 1)
	late, _ = mustProject(t, late, 29*time.Minute, 0)
	if _, e := Feedback(late, at(late, 38*time.Minute), rev(late), 0, []int{0}, nil); e != nil {
		t.Fatal("grace after round expiry", e)
	}
	if _, e := Feedback(late, at(late, 39*time.Minute), rev(late), 0, []int{0}, nil); !errors.Is(e, ErrExpired) {
		t.Fatal("page grace must end", e)
	}
}

func TestExpiredRowsNeedExplicitBoundedPrune(t *testing.T) {
	r := fixture(t, 1)
	r.State.Manifest.Policy.DeliveryLimit = 1
	r.History.Deliveries = []Delivery{{testID(999), testTime().Add(-10 * time.Minute), testTime().Add(-5 * time.Minute)}}
	if _, _, e := Project(r, at(r, 0), rev(r), 0, nil); !errors.Is(e, ErrCapacity) {
		t.Fatal("expired physical row silently evicted")
	}
	h, n, e := PruneExpired(r.History, testTime(), 1)
	if e != nil || n != 1 {
		t.Fatal(e)
	}
	r.History = h
	r, _ = mustProject(t, r, 0, 0)
	if len(r.History.Deliveries) != 1 || r.History.Deliveries[0].ID != testID(1) {
		t.Fatal("prune/insertion")
	}
	h = History{testBinding().Account, 1, testTime(), []Delivery{}, []Exposure{}}
	for i := 0; i < 200; i++ {
		h.Exposures = append(h.Exposures, Exposure{testID(i + 1), testTime()})
	}
	next, n, e := PruneExpired(h, testTime(), 128)
	if e != nil || n != 128 || len(next.Exposures) != 72 || len(h.Exposures) != 200 {
		t.Fatal("bounded prune", n, e)
	}
	if _, _, e := PruneExpired(h, testTime(), 129); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	// Older exposure work cannot starve behind a stream of more recent expired
	// delivery rows. Row type is not the primary cleanup ordering key.
	h.Deliveries = []Delivery{{testID(999), testTime().Add(-time.Minute), testTime()}}
	h.Exposures = []Exposure{{testID(998), testTime().Add(-time.Second)}}
	next, n, e = PruneExpired(h, testTime(), 1)
	if e != nil || n != 1 || len(next.Exposures) != 0 || len(next.Deliveries) != 1 {
		t.Fatal("oldest-expiry fairness", e)
	}
}

func TestImmutableParallelEvaluationAndCASCounterexample(t *testing.T) {
	r := fixture(t, 20)
	before := clone(r)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, e := Project(r, at(r, 0), rev(r), 0, nil)
			if e != nil || a.State.Revision != 2 {
				t.Error("pure parallel evaluation", e)
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(r, before) {
		t.Fatal("shared input mutated")
	}
	// The model deliberately cannot commit. Simulate one store CAS and reject
	// another candidate produced from the same old pair of revisions.
	a, _, e := Project(r, at(r, 0), rev(r), 0, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Project(a, at(a, time.Second), rev(r), 10, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("lost-update fence")
	}
	noFence := r
	other, _, e := Project(noFence, at(noFence, 0), rev(noFence), 10, nil)
	if e != nil || len(other.History.Deliveries) != 10 {
		t.Fatal(e)
	}
	if reflect.DeepEqual(a.History, other.History) {
		t.Fatal("counterexample must show distinct writes")
	}
	// Persisting 'other' over 'a' without CAS would lose page0's deliveries.
}

func TestSeededMonotoneSubsequenceProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(20261025))
	for trial := 0; trial < 300; trial++ {
		n := 1 + rng.Intn(100)
		r := fixture(t, n)
		offset := rng.Intn(pageCount(n)) * PageSize
		last := []int(nil)
		first := time.Time{}
		for iteration := 0; iteration < 6; iteration++ {
			input, e := PageInput(r, at(r, time.Duration(iteration)*time.Second), rev(r), offset)
			if e != nil {
				t.Fatal(e)
			}
			miss := []int{}
			for i := range input.IDs {
				if rng.Intn(4) == 0 {
					miss = append(miss, i)
				}
			}
			var v View
			r, v = mustProject(t, r, time.Duration(iteration)*time.Second, offset, miss...)
			if iteration == 0 {
				first = v.FirstAt
			} else {
				for _, slot := range v.Slots {
					found := false
					for _, old := range last {
						if slot == old {
							found = true
						}
					}
					if !found {
						t.Fatal("slot resurrected")
					}
				}
			}
			if v.End != min(offset+10, n) || v.FirstAt != first {
				t.Fatal("boundary or first time changed")
			}
			for i, id := range v.IDs {
				if id != r.State.Manifest.IDs[offset+v.Slots[i]] {
					t.Fatal("order")
				}
			}
			last = append([]int{}, v.Slots...)
		}
	}
}

func TestNewPolicyCannotShortenStoredAbsoluteDeliveryUntil(t *testing.T) {
	old := fixture(t, 1)
	old, _ = mustProject(t, old, 0, 0)
	next := fixture(t, 1)
	next.State.Manifest.RoundID = testID(2001)
	next.State.Manifest.CreatedAt = testTime().Add(time.Minute)
	next.State.LastAt = next.State.Manifest.CreatedAt
	next.State.Manifest.Policy.DeliveryWindow = time.Minute
	next.History = old.History
	next, _ = mustProject(t, next, time.Minute, 0)
	if next.History.Deliveries[0].Until != testTime().Add(5*time.Minute) {
		t.Fatal("new shorter config shortened prior absolute window")
	}
}
