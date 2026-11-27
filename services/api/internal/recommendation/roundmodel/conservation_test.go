package roundmodel

import (
	"errors"
	"testing"
	"time"
)

func TestConservedCapacityServiceCurveBoundary(t *testing.T) {
	now := testTime()
	p := ConservedCapacity{Profile: testDigest(7), MeasuredAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Minute), MinUnitsPerSecond: 2, MaxPause: time.Second, WorstGrowthBytesPerMutation: 8192}
	if e := p.Validate(now, p.Profile, 599, 8192); e != nil {
		t.Fatal(e)
	}
	if e := p.Validate(now, p.Profile, 600, 8192); !errors.Is(e, ErrCapacity) {
		t.Fatal("service pause omitted", e)
	}
	if e := p.Validate(now, testDigest(8), 1, 8192); e == nil {
		t.Fatal("wrong profile")
	}
	if e := p.Validate(p.ValidUntil, p.Profile, 1, 8192); e == nil {
		t.Fatal("stale evidence")
	}
	if e := p.Validate(now, p.Profile, 1, 4096); e == nil {
		t.Fatal("unreserved file growth")
	}
	for _, change := range []func(*ConservedCapacity){func(p *ConservedCapacity) { p.MinUnitsPerSecond = 0 }, func(p *ConservedCapacity) { p.MaxPause = 10 * time.Minute }, func(p *ConservedCapacity) { p.MeasuredAt = now.Add(time.Second) }, func(p *ConservedCapacity) { p.ValidUntil = now.Add(2 * time.Hour) }} {
		next := p
		change(&next)
		if e := next.Validate(now, next.Profile, 1, 8192); e == nil {
			t.Fatal("unmeasured capacity admitted")
		}
	}
}

func TestConservedCapacityExpandedCeilingRequiresMeasuredService(t *testing.T) {
	now := testTime()
	p := ConservedCapacity{Profile: testDigest(7), MeasuredAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Minute), MinUnitsPerSecond: 2, MaxPause: time.Second, WorstGrowthBytesPerMutation: 8192}
	const simultaneousFreshRounds = 300 * 1252
	if err := p.Validate(now, p.Profile, simultaneousFreshRounds, 8192); !errors.Is(err, ErrCapacity) {
		t.Fatal("expanded schema ceiling admitted an inadequate service profile", err)
	}
	p.MinUnitsPerSecond = 20000
	if err := p.Validate(now, p.Profile, simultaneousFreshRounds, 8192); err != nil {
		t.Fatal("valid service curve rejected above old ceiling", err)
	}
	if err := p.Validate(now, p.Profile, MaxConservedUnits+1, 8192); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbounded operator quota accepted", err)
	}
}
