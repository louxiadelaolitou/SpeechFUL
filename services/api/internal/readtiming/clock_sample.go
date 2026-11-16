package readtiming

import "time"

// ClockSample brackets a RAW clock read with Go monotonic reads. Benchmarks
// compare intervals, including scheduling uncertainty, rather than attributing
// a pause in Trace.Snapshot's allocation to clock frequency drift.
type ClockSample struct {
	Tick          int64
	Before, After time.Time
	Valid         bool
}

func SampleClock() ClockSample {
	before := time.Now()
	tick, valid := clockNow()
	return ClockSample{Tick: tick, Before: before, After: time.Now(), Valid: valid}
}

// ElapsedClock rejects invalid clocks, backward time, imprecise read brackets
// and >1%/>1ms disagreement. It does not adjust or normalize measured time.
func ElapsedClock(start, end ClockSample) (int64, bool) {
	elapsed := end.Tick - start.Tick
	if !start.Valid || !end.Valid || elapsed <= 0 {
		return elapsed, false
	}
	lower := end.Before.Sub(start.After).Nanoseconds()
	upper := end.After.Sub(start.Before).Nanoseconds()
	tolerance := elapsed / 100
	if tolerance < 1_000_000 {
		tolerance = 1_000_000
	}
	uncertainty := start.After.Sub(start.Before) + end.After.Sub(end.Before)
	return elapsed, lower >= 0 && uncertainty <= time.Millisecond && elapsed >= lower-tolerance && elapsed <= upper+tolerance
}
