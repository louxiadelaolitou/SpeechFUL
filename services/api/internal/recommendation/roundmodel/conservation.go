package roundmodel

import "time"

// MaxConservedUnits is a schema ceiling, not a deployable capacity. Validate
// still derives the allowed installed cap from the measured cleanup service.
const MaxConservedUnits int64 = 4194304

// ConservedCapacity is a sufficient-condition model for a global potential-work
// cap. Units include real rows, cascades, index/transaction/lock overhead and all
// promised future work. A profile must bound SERVICE, not just average arrivals.
type ConservedCapacity struct {
	Profile                     Digest
	MeasuredAt, ValidUntil      time.Time
	MinUnitsPerSecond           int64
	MaxPause                    time.Duration
	WorstGrowthBytesPerMutation int64
}

func (p ConservedCapacity) Validate(now time.Time, profile Digest, maxUnits, growthBytes int64) error {
	if !validTime(now) || p.Profile != profile || !profile.valid() || !validTime(p.MeasuredAt) || !validTime(p.ValidUntil) || p.MeasuredAt.After(now) || !now.Before(p.ValidUntil) ||
		p.ValidUntil.Sub(p.MeasuredAt) > time.Hour || p.MinUnitsPerSecond < 2 || p.MinUnitsPerSecond > 1000000 || p.MaxPause < 0 || p.MaxPause >= 10*time.Minute || p.WorstGrowthBytesPerMutation < 8192 ||
		growthBytes < p.WorstGrowthBytesPerMutation || growthBytes > 1048576 || maxUnits < 1 || maxUnits > MaxConservedUnits {
		return ErrInvalid
	}
	safe := p.MinUnitsPerSecond / 2
	// floor capacity (rather than rounded-up average-rate arithmetic).
	if maxUnits > (10*time.Minute-p.MaxPause).Microseconds()*safe/1000000 {
		return ErrCapacity
	}
	return nil
}
