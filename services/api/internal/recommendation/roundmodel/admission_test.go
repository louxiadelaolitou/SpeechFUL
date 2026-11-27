package roundmodel

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func jsonCopy[T any](in T) (T, error) {
	var out T
	b, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(b, &out)
	return out, e
}
func admissionFixture() Admission {
	now := testTime()
	return Admission{Now: now, Profile: testDigest(4), Evidence: CapacityEvidence{testDigest(4), now.Add(-time.Minute), now.Add(time.Minute), 204}, SafetyPermille: 500, MaxPause: 30 * time.Second, ExposureLimit: 1024, DeliveryLimit: 1024, Jobs: []CleanupJob{}}
}
func certificate(t *testing.T, in Admission, p AdmissionPlan) {
	t.Helper()
	totals := make([]int64, len(in.Jobs))
	last := in.Now
	if p.EffectiveRowsPerSecond != in.Evidence.MinRowsPerSecond*in.SafetyPermille/1000 {
		t.Fatal("wrong rate")
	}
	for _, a := range p.Allocations {
		if a.Job < 0 || a.Job >= len(in.Jobs) {
			t.Fatal("bad job")
		}
		j := in.Jobs[a.Job]
		release := j.ReleaseAt
		if release.Before(in.Now) {
			release = in.Now
		}
		release = release.Add(in.MaxPause)
		if a.Start.Before(last) || a.Start.Before(release) || !a.End.After(a.Start) || a.End.After(j.Deadline) || a.Rows < 1 || a.Rows > a.End.Sub(a.Start).Microseconds()*p.EffectiveRowsPerSecond/1000000 {
			t.Fatal("invalid schedule certificate", a)
		}
		last = a.End
		totals[a.Job] += a.Rows
	}
	for i, j := range in.Jobs {
		if totals[i] != j.Rows {
			t.Fatal("unreserved cleanup work", i, totals[i], j.Rows)
		}
	}
}

func TestAdmissionConcentratedExpiryAndFeedbackReservations(t *testing.T) {
	for _, n := range []int{25, 26, 100} {
		in := admissionFixture()
		for i := 0; i < n; i++ {
			in.Jobs = append(in.Jobs, CleanupJob{in.Now, in.Now.Add(10 * time.Minute), 2289})
		}
		p, e := Admit(in)
		if n == 25 {
			if e != nil {
				t.Fatal(e)
			}
			certificate(t, in, p)
		} else if !errors.Is(e, ErrCapacity) {
			t.Fatal("concentrated expiry should fail", n, e)
		}
	}
	in := admissionFixture()
	in.OccupiedExposure = 1000
	in.ReservedExposure = 24
	in.NewExposure = 1
	in.Jobs = []CleanupJob{{in.Now, in.Now.Add(time.Minute), 1}}
	if _, e := Admit(in); !errors.Is(e, ErrCapacity) {
		t.Fatal("feedback reservation stolen", e)
	}
	in.NewExposure = 0
	if _, e := Admit(in); e != nil {
		t.Fatal("exact reserved limit", e)
	}
	in.OccupiedDelivery = 1024
	in.NewDelivery = 1
	if _, e := Admit(in); !errors.Is(e, ErrCapacity) {
		t.Fatal("delivery over capacity", e)
	}
	in = admissionFixture()
	in.NewExposure = 10
	if _, e := Admit(in); !errors.Is(e, ErrInvalid) {
		t.Fatal("new reservation without cleanup budget", e)
	}
}

func TestAdmissionReleaseTimeCounterexampleAndPreemption(t *testing.T) {
	in := admissionFixture()
	in.MaxPause = 0
	// A prefix count from now would see 1h of capacity. This future burst only
	// has one second after release and therefore cannot clean 1000 rows.
	in.Jobs = []CleanupJob{{in.Now.Add(time.Hour), in.Now.Add(time.Hour + time.Second), 1000}}
	if _, e := Admit(in); !errors.Is(e, ErrCapacity) {
		t.Fatal("future idle capacity incorrectly banked", e)
	}
	in.Jobs = []CleanupJob{{in.Now, in.Now.Add(time.Minute), 4000}, {in.Now.Add(time.Second), in.Now.Add(3 * time.Second), 100}}
	before, _ := jsonCopy(in)
	plan, e := Admit(in)
	if e != nil {
		t.Fatal(e)
	}
	certificate(t, in, plan)
	if len(plan.Allocations) < 3 || !reflect.DeepEqual(in, before) {
		t.Fatal("preemption or immutable inputs")
	}
}

func TestAdmissionRejectsUnmeasuredStaleAndInvalidCapacity(t *testing.T) {
	for _, change := range []func(*Admission){
		func(a *Admission) { a.Evidence = CapacityEvidence{} },
		func(a *Admission) { a.Profile = testDigest(9) },
		func(a *Admission) { a.Evidence.ValidUntil = a.Now },
		func(a *Admission) { a.Evidence.MeasuredAt = a.Now.Add(time.Second) },
		func(a *Admission) { a.Evidence.ValidUntil = a.Now.Add(2 * time.Hour) },
		func(a *Admission) { a.Evidence.MinRowsPerSecond = 1000001 },
		func(a *Admission) { a.SafetyPermille = 501 },
		func(a *Admission) { a.MaxPause = time.Minute + time.Microsecond },
		func(a *Admission) { a.ExposureLimit = MaxExposureHistory + 1 },
		func(a *Admission) { a.ReservedExposure = -1 },
		func(a *Admission) { a.Jobs = make([]CleanupJob, 129) },
		func(a *Admission) { a.Jobs = []CleanupJob{{a.Now, a.Now, 1}} },
		func(a *Admission) { a.Jobs = []CleanupJob{{a.Now, a.Now.Add(time.Minute), 0}} },
		func(a *Admission) { a.Jobs = []CleanupJob{{a.Now, a.Now.Add(8 * 24 * time.Hour), 1}} },
	} {
		in := admissionFixture()
		change(&in)
		if _, e := Admit(in); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid measurement admitted", e)
		}
	}
	in := admissionFixture()
	in.ExpiryRowsPerSecond = 103
	if _, e := Admit(in); !errors.Is(e, ErrCapacity) {
		t.Fatal("arrival rate exceeds effective capacity", e)
	}
	in = admissionFixture()
	in.Evidence.MinRowsPerSecond = 1
	if _, e := Admit(in); !errors.Is(e, ErrCapacity) {
		t.Fatal("floor capacity zero", e)
	}
	in = admissionFixture()
	in.Jobs = []CleanupJob{{in.Now, in.Now.Add(29 * time.Second), 1}}
	if _, e := Admit(in); !errors.Is(e, ErrCapacity) {
		t.Fatal("pause leaves no capacity", e)
	}
}

func TestSeededAdmissionCertificates(t *testing.T) {
	rng := rand.New(rand.NewSource(20261026))
	accepted, rejected := 0, 0
	for trial := 0; trial < 300; trial++ {
		in := admissionFixture()
		in.MaxPause = time.Second
		in.Evidence.MinRowsPerSecond = 100
		for i := 0; i < 1+rng.Intn(20); i++ {
			start := in.Now.Add(time.Duration(rng.Intn(120)) * time.Second)
			in.Jobs = append(in.Jobs, CleanupJob{start, start.Add(time.Duration(5+rng.Intn(30)) * time.Second), int64(1 + rng.Intn(1000))})
		}
		plan, e := Admit(in)
		if e == nil {
			accepted++
			certificate(t, in, plan)
		} else if errors.Is(e, ErrCapacity) {
			rejected++
		} else {
			t.Fatal(e)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatal("property generator missed a branch")
	}
}

func TestUnknownDeliveryTimeNeedsConservativeExpiryBuckets(t *testing.T) {
	first, last := testTime(), testTime().Add(30*time.Minute)
	jobs, e := ReserveExpiryWindow(first, last, 72*time.Hour, 10*time.Minute, time.Minute, 100)
	if e != nil || len(jobs) != 30 {
		t.Fatal("bounded future reservations", e)
	}
	// No single common interval can cover every possible expiry: latest release
	// is 30m later, while the earliest deadline is only 10m later.
	if !last.Add(72 * time.Hour).After(first.Add(72*time.Hour + 10*time.Minute)) {
		t.Fatal("counterexample missing")
	}
	for second := 0; second < 30*60; second++ {
		event := first.Add(time.Duration(second) * time.Second)
		expiry := event.Add(72 * time.Hour)
		j := jobs[second/60]
		if j.ReleaseAt.Before(expiry) || j.Deadline.After(expiry.Add(10*time.Minute)) || !j.Deadline.After(j.ReleaseAt) {
			t.Fatal("bucket borrowed unavailable time")
		}
	}
	in := admissionFixture()
	in.Jobs = jobs
	p, e := Admit(in)
	if e != nil {
		t.Fatal(e)
	}
	certificate(t, in, p)
	for _, x := range []struct {
		retention, lag, quantum time.Duration
		rows                    int64
	}{{0, time.Minute, time.Second, 1}, {time.Hour, time.Minute, time.Minute, 1}, {time.Hour, time.Minute, time.Microsecond, 1}, {time.Hour, time.Minute, time.Second, 101}} {
		if _, e := ReserveExpiryWindow(first, last, x.retention, x.lag, x.quantum, x.rows); e == nil {
			t.Fatal("bad expiry reservation accepted")
		}
	}
}
