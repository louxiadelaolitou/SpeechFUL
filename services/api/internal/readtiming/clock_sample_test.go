package readtiming

import (
	"testing"
	"time"
)

func TestElapsedClockRejectsDriftAndImpreciseSamples(t *testing.T) {
	base := time.Now()
	first := ClockSample{Tick: 10, Before: base, After: base, Valid: true}
	for _, test := range []struct {
		name                 string
		raw, goTime, bracket time.Duration
		valid                bool
	}{
		{"matching", 100 * time.Millisecond, 100 * time.Millisecond, 0, true},
		{"frequency_drift", 100 * time.Millisecond, 91 * time.Millisecond, 0, false},
		{"scheduler_uncertainty", 100 * time.Millisecond, 100 * time.Millisecond, 2 * time.Millisecond, false},
		{"backward_raw", -time.Millisecond, time.Millisecond, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			last := ClockSample{Tick: first.Tick + test.raw.Nanoseconds(), Before: base.Add(test.goTime), After: base.Add(test.goTime + test.bracket), Valid: true}
			elapsed, valid := ElapsedClock(first, last)
			if valid != test.valid || elapsed != test.raw.Nanoseconds() {
				t.Fatal("invalid clock acceptance", elapsed, valid)
			}
		})
	}
}
