package roundmodel

import (
	"sort"
	"time"
)

const MaxCleanupJobs = 128

// CapacityEvidence values MUST come from a trusted measured deployment profile.
// This pure model authenticates no evidence and performs no benchmark. Missing
// or stale measurements reject admission; tests inject synthetic values only.
type CapacityEvidence struct {
	Profile          Digest
	MeasuredAt       time.Time
	ValidUntil       time.Time
	MinRowsPerSecond int64
}
type CleanupJob struct {
	ReleaseAt, Deadline time.Time
	Rows                int64
}
type Admission struct {
	Now                                             time.Time
	Profile                                         Digest
	Evidence                                        CapacityEvidence
	SafetyPermille                                  int64
	MaxPause                                        time.Duration
	ExpiryRowsPerSecond                             int64
	Jobs                                            []CleanupJob // includes existing backlog AND worst-case new reserved work
	OccupiedExposure, ReservedExposure, NewExposure int
	OccupiedDelivery, ReservedDelivery, NewDelivery int
	ExposureLimit, DeliveryLimit                    int
}
type Allocation struct {
	Job        int
	Start, End time.Time
	Rows       int64
}
type AdmissionPlan struct {
	EffectiveRowsPerSecond int64
	Allocations            []Allocation
}

// ReserveExpiryWindow over-reserves every possible event-time bucket. Up to
// rows events may occur anywhere in [earliest,latest); reserving rows in EACH
// bucket is conservative. Real delivery/feedback selects its bucket and the
// store may then release the unused reservations atomically.
func ReserveExpiryWindow(earliest, latest time.Time, retention, lag, quantum time.Duration, rows int64) ([]CleanupJob, error) {
	if !validTime(earliest) || !validTime(latest) || !latest.After(earliest) || latest.Sub(earliest) > 30*time.Minute || retention <= 0 || retention > 72*time.Hour ||
		lag <= 0 || lag > 10*time.Minute || quantum <= 0 || quantum >= lag || rows < 1 || rows > MaxIDs || retention%time.Microsecond != 0 || lag%time.Microsecond != 0 || quantum%time.Microsecond != 0 {
		return nil, ErrInvalid
	}
	if (latest.Sub(earliest)+quantum-1)/quantum > MaxCleanupJobs {
		return nil, ErrCapacity
	}
	jobs := []CleanupJob{}
	for start := earliest; start.Before(latest); start = start.Add(quantum) {
		end := start.Add(quantum)
		if end.After(latest) {
			end = latest
		}
		jobs = append(jobs, CleanupJob{end.Add(retention), start.Add(retention + lag), rows})
	}
	return jobs, nil
}

// Admit constructs a conservative preemptive EDF schedule certificate, with a
// pause allowance applied to every job release. A concrete schedule is a
// sufficient admission condition; rejection does not prove no schedule exists.
// The future store must reserve feedback slots and this work atomically before
// returning success. No TTL extension or eviction is an admission fallback.
func Admit(in Admission) (AdmissionPlan, error) {
	e := in.Evidence
	if !validTime(in.Now) || !in.Profile.valid() || in.Profile != e.Profile || !validTime(e.MeasuredAt) || !validTime(e.ValidUntil) || e.MeasuredAt.After(in.Now) ||
		!in.Now.Before(e.ValidUntil) || e.ValidUntil.Sub(e.MeasuredAt) > time.Hour || e.MinRowsPerSecond < 1 || e.MinRowsPerSecond > 1000000 ||
		in.SafetyPermille < 1 || in.SafetyPermille > 500 || in.MaxPause < 0 || in.MaxPause > time.Minute || in.MaxPause%time.Microsecond != 0 ||
		len(in.Jobs) > MaxCleanupJobs || in.ExpiryRowsPerSecond < 0 {
		return AdmissionPlan{}, ErrInvalid
	}
	rate := e.MinRowsPerSecond * in.SafetyPermille / 1000
	if rate < 1 || in.ExpiryRowsPerSecond > rate {
		return AdmissionPlan{}, ErrCapacity
	}
	for _, x := range [][5]int{{in.OccupiedExposure, in.ReservedExposure, in.NewExposure, in.ExposureLimit, MaxExposureHistory}, {in.OccupiedDelivery, in.ReservedDelivery, in.NewDelivery, in.DeliveryLimit, MaxDeliveryHistory}} {
		if x[3] < 1 || x[3] > x[4] {
			return AdmissionPlan{}, ErrInvalid
		}
		var sum int64
		for _, n := range x[:3] {
			if n < 0 || n > x[4] {
				return AdmissionPlan{}, ErrInvalid
			}
			sum += int64(n)
		}
		if sum > int64(x[3]) {
			return AdmissionPlan{}, ErrCapacity
		}
	}
	type task struct {
		index             int
		release, deadline time.Time
		remaining         int64
	}
	tasks := make([]task, len(in.Jobs))
	var totalRows int64
	for i, j := range in.Jobs {
		if !validTime(j.ReleaseAt) || !validTime(j.Deadline) || !j.Deadline.After(j.ReleaseAt) || !j.Deadline.After(in.Now) || j.Deadline.After(in.Now.Add(7*24*time.Hour)) || j.Rows < 1 || j.Rows > 1000000000 {
			return AdmissionPlan{}, ErrInvalid
		}
		r := j.ReleaseAt
		if r.Before(in.Now) {
			r = in.Now
		}
		r = r.Add(in.MaxPause)
		if !r.Before(j.Deadline) {
			return AdmissionPlan{}, ErrCapacity
		}
		tasks[i] = task{i, r, j.Deadline, j.Rows}
		totalRows += j.Rows
	}
	if totalRows < int64(in.NewExposure)+int64(in.NewDelivery) {
		return AdmissionPlan{}, ErrInvalid
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].deadline.Equal(tasks[j].deadline) {
			return tasks[i].index < tasks[j].index
		}
		return tasks[i].deadline.Before(tasks[j].deadline)
	})
	plan := AdmissionPlan{rate, []Allocation{}}
	now := in.Now
	remaining := len(tasks)
	// At most one completion or a previously unprocessed release per iteration.
	for steps := 0; remaining > 0; steps++ {
		if steps > 3*MaxCleanupJobs {
			return AdmissionPlan{}, ErrCapacity
		}
		pick := -1
		var nextRelease time.Time
		for i, t := range tasks {
			if t.remaining == 0 {
				continue
			}
			if !now.Before(t.deadline) {
				return AdmissionPlan{}, ErrCapacity
			}
			if !t.release.After(now) {
				if pick < 0 {
					pick = i
				}
			} else if nextRelease.IsZero() || t.release.Before(nextRelease) {
				nextRelease = t.release
			}
		}
		if pick < 0 {
			now = nextRelease
			continue
		}
		t := &tasks[pick]
		end := t.deadline
		if !nextRelease.IsZero() && nextRelease.Before(end) {
			end = nextRelease
		}
		micros := end.Sub(now).Microseconds()
		capacity := micros * rate / 1000000
		if capacity < 1 {
			now = end
			continue
		}
		rows := t.remaining
		if rows > capacity {
			rows = capacity
		}
		usedMicros := (rows*1000000 + rate - 1) / rate
		finish := now.Add(time.Duration(usedMicros) * time.Microsecond)
		plan.Allocations = append(plan.Allocations, Allocation{t.index, now, finish, rows})
		t.remaining -= rows
		if t.remaining == 0 {
			remaining--
		}
		now = finish
		if rows == capacity && t.remaining > 0 {
			now = end
		}
	}
	return plan, nil
}
