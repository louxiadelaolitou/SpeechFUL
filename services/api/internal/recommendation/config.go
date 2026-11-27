// Package recommendation contains bounded, deterministic experimental feed rules.
// It does not authorize reads or persist rounds, deliveries, or impressions.
package recommendation

import (
	"errors"
	"math"
	"time"
)

var (
	ErrConfig = errors.New("invalid recommendation experiment configuration")
	ErrInput  = errors.New("invalid or oversized recommendation input")
)

type Source uint8

const (
	Recent Source = iota
	Interest
	Discussion
	Explore
	SourceCount
)

// Config contains experiment values, not a production policy or platform facts.
// Version identifies the complete configuration; adapters must also bind its hash.
type Config struct {
	Version               string
	PageSize              int
	Quotas                [SourceCount]int
	Weights               [3]float64 // freshness, interest, recent interaction strength
	MaxPerSource          int
	MaxCandidates         int
	MaxRoundIDs           int
	MaxHistoryIDs         int // each of delivery and exposure summaries
	MaxChannels           int
	MaxSignals            int
	MaxEventsPerCandidate int
	MaxInteractionEvents  int
	MaxPerChannel         int
	MaxPerAuthor          int
	FreshnessHalfLife     time.Duration
	InterestHalfLife      time.Duration
	InterestWindow        time.Duration
	InterestPrior         float64
	HeatWindow            time.Duration
	HeatActorCap          float64
	HeatSaturation        float64
	InteractionWeights    [3]float64 // like, favorite, comment
	DeliveryCooldown      time.Duration
	ExposureCooldown      time.Duration
}

func ExperimentalConfig() Config {
	return Config{
		Version: "b1-experiment-v1", PageSize: 10,
		Quotas: [SourceCount]int{4, 3, 2, 1}, Weights: [3]float64{0.45, 0.30, 0.25},
		MaxPerSource: 100, MaxCandidates: 400, MaxRoundIDs: 400,
		MaxHistoryIDs: 2048, MaxChannels: 10, MaxSignals: 1024,
		MaxEventsPerCandidate: 64, MaxInteractionEvents: 4096,
		MaxPerChannel: 4, MaxPerAuthor: 2,
		FreshnessHalfLife: 24 * time.Hour, InterestHalfLife: 7 * 24 * time.Hour,
		InterestWindow: 30 * 24 * time.Hour, InterestPrior: 3,
		HeatWindow: 7 * 24 * time.Hour, HeatActorCap: 2, HeatSaturation: 20,
		InteractionWeights: [3]float64{1, 2, 1},
		DeliveryCooldown:   5 * time.Minute, ExposureCooldown: 72 * time.Hour,
	}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (c Config) Validate() error {
	if !validKey(c.Version) || c.PageSize < 1 || c.PageSize > 50 ||
		c.MaxPerSource < 1 || c.MaxPerSource > 1024 ||
		c.MaxCandidates < c.PageSize || c.MaxCandidates > 4096 || c.MaxCandidates > int(SourceCount)*c.MaxPerSource ||
		c.MaxRoundIDs < c.PageSize || c.MaxRoundIDs > 4096 ||
		c.MaxHistoryIDs < 1 || c.MaxHistoryIDs > 4096 ||
		c.MaxChannels < 1 || c.MaxChannels > 64 || c.MaxSignals < 1 || c.MaxSignals > 4096 ||
		c.MaxEventsPerCandidate < 1 || c.MaxEventsPerCandidate > 256 ||
		c.MaxInteractionEvents < 1 || c.MaxInteractionEvents > 65536 ||
		c.MaxPerChannel < 1 || c.MaxPerChannel > c.PageSize ||
		c.MaxPerAuthor < 1 || c.MaxPerAuthor > c.PageSize {
		return ErrConfig
	}
	quotaSum := 0
	for _, q := range c.Quotas {
		if q < 0 || q > c.PageSize {
			return ErrConfig
		}
		quotaSum += q
	}
	weightSum := 0.0
	for _, w := range c.Weights {
		if !finite(w) || w < 0 || w > 1 {
			return ErrConfig
		}
		weightSum += w
	}
	if quotaSum != c.PageSize || math.Abs(weightSum-1) > 1e-9 {
		return ErrConfig
	}
	for _, w := range c.InteractionWeights {
		if !finite(w) || w <= 0 || w > 10 {
			return ErrConfig
		}
	}
	for _, d := range []time.Duration{c.FreshnessHalfLife, c.InterestHalfLife, c.InterestWindow, c.HeatWindow, c.DeliveryCooldown, c.ExposureCooldown} {
		if d <= 0 || d > 365*24*time.Hour {
			return ErrConfig
		}
	}
	if c.InterestHalfLife > c.InterestWindow || c.DeliveryCooldown > c.ExposureCooldown ||
		!finite(c.InterestPrior) || c.InterestPrior <= 0 || c.InterestPrior > 1e6 ||
		!finite(c.HeatActorCap) || c.HeatActorCap <= 0 || c.HeatActorCap > 100 ||
		!finite(c.HeatSaturation) || c.HeatSaturation < c.HeatActorCap || c.HeatSaturation > 1e6 {
		return ErrConfig
	}
	return nil
}
