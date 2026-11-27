package recommendation

import (
	"math"
	"sort"
)

// profile sorts a copy so floating point accumulation is independent of input
// order. Unknown/insufficient history tends toward the explicit neutral 0.5.
func profile(c Config, in Input) map[string]float64 {
	signals := append([]InterestSignal(nil), in.Signals...)
	sort.Slice(signals, func(i, j int) bool {
		a, b := signals[i], signals[j]
		if a.Channel != b.Channel {
			return a.Channel < b.Channel
		}
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		return a.Weight < b.Weight
	})
	mass := make(map[string]float64, len(in.Channels))
	for _, s := range signals {
		age := in.Now.Sub(s.At)
		if age >= c.InterestWindow {
			continue
		}
		mass[s.Channel] += s.Weight * math.Exp2(-float64(age)/float64(c.InterestHalfLife))
	}
	channels := append([]string(nil), in.Channels...)
	sort.Strings(channels)
	total := 0.0
	for _, channel := range channels {
		total += mass[channel]
	}
	result := make(map[string]float64, len(channels))
	for _, channel := range channels {
		other := 0.0
		if len(channels) > 1 {
			other = (total - mass[channel]) / float64(len(channels)-1)
		}
		result[channel] = (c.InterestPrior*0.5 + mass[channel]) / (c.InterestPrior + mass[channel] + other)
	}
	return result
}

func score(c Config, in Input, p Candidate, interest float64) Features {
	f := Features{
		Freshness: math.Exp2(-float64(in.Now.Sub(p.PublishedAt)) / float64(c.FreshnessHalfLife)),
		Interest:  interest,
	}
	// Each actor contributes at most one like, one favorite, and capped comments.
	// Grouping actors across kinds avoids counting three identities as three people.
	type actorFacts struct {
		liked, saved bool
		comments     int
	}
	actors := make(map[string]actorFacts)
	for _, event := range p.Interactions {
		if !event.Eligible || event.ActorKey == p.OwnerActorKey || in.Now.Sub(event.At) >= c.HeatWindow {
			continue
		}
		a := actors[event.ActorKey]
		switch event.Kind {
		case Like:
			a.liked = true
		case Favorite:
			a.saved = true
		case Comment:
			a.comments++
		}
		actors[event.ActorKey] = a
	}
	keys := make([]string, 0, len(actors))
	for key := range actors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	strength := 0.0
	for _, key := range keys {
		a := actors[key]
		s := float64(a.comments) * c.InteractionWeights[Comment]
		if a.liked {
			s += c.InteractionWeights[Like]
		}
		if a.saved {
			s += c.InteractionWeights[Favorite]
		}
		strength += math.Min(s, c.HeatActorCap)
	}
	f.Participants = len(actors)
	f.Heat = math.Log1p(math.Min(strength, c.HeatSaturation)) / math.Log1p(c.HeatSaturation)
	f.Score = c.Weights[0]*f.Freshness + c.Weights[1]*f.Interest + c.Weights[2]*f.Heat
	return f
}
