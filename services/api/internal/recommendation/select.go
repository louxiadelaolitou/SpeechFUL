package recommendation

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"time"
)

type ranked struct {
	post        Candidate
	features    Features
	exploration uint64
}

// Select makes one bounded batch. The caller supplies fixed time, seed, pools and
// finite state. Determinism is NOT a durable retry/authorization guarantee: B2+
// must store round page boundaries and recheck visibility during projection.
func Select(c Config, in Input) (Result, error) {
	r := Result{Items: make([]Selection, 0)}
	if err := c.Validate(); err != nil {
		return r, err
	}
	if err := validateInput(c, in); err != nil {
		return r, err
	}
	prefs := profile(c, in)
	catalog := make(map[string]ranked, len(in.Candidates))
	for _, p := range in.Candidates {
		catalog[p.ID] = ranked{p, score(c, in, p, prefs[p.Channel]), explorationKey(in.Seed, p.ID)}
	}
	var pools [SourceCount][]ranked
	union := make(map[string]bool)
	for s, ids := range in.Pools {
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			union[id] = true
			if seen[id] {
				continue
			}
			seen[id] = true
			if catalog[id].post.Eligible {
				pools[s] = append(pools[s], catalog[id])
			}
		}
		source := Source(s)
		sort.Slice(pools[s], func(i, j int) bool {
			a, b := pools[s][i], pools[s][j]
			if source == Explore && a.exploration != b.exploration {
				return a.exploration < b.exploration
			}
			if a.features.Score != b.features.Score {
				return a.features.Score > b.features.Score
			}
			if a.post.Ordinal != b.post.Ordinal {
				return a.post.Ordinal > b.post.Ordinal
			}
			return a.post.ID < b.post.ID
		})
	}
	r.UniquePoolIDs = len(union)
	selected := make(map[string]bool, len(in.State.RoundSelected)+c.PageSize)
	for _, id := range in.State.RoundSelected {
		selected[id] = true
	}
	delivered := activeIDs(in.State.Delivered, in.Now, c.DeliveryCooldown)
	exposed := activeIDs(in.State.Exposed, in.Now, c.ExposureCooldown)
	channelCounts, authorCounts := make(map[string]int), make(map[string]int)
	quotaLeft := c.Quotas
	schedule := make([]Source, 0, c.PageSize)
	for len(schedule) < c.PageSize {
		for s := Recent; s < SourceCount; s++ {
			if quotaLeft[s] > 0 {
				schedule = append(schedule, s)
				quotaLeft[s]--
			}
		}
	}
	for _, preferred := range schedule {
		if len(selected) >= c.MaxRoundIDs {
			break
		}
		found := false
		for tier := 0; tier < 3 && !found; tier++ {
			for offset := Source(0); offset < SourceCount && !found; offset++ {
				s := (preferred + offset) % SourceCount
				for _, p := range pools[s] {
					if selected[p.post.ID] || exposed[p.post.ID] || tier < 2 && delivered[p.post.ID] {
						continue
					}
					if tier == 0 && (channelCounts[p.post.Channel] >= c.MaxPerChannel || p.post.AuthorGroup != "" && authorCounts[p.post.AuthorGroup] >= c.MaxPerAuthor) {
						continue
					}
					relaxation := Strict
					if tier == 1 {
						relaxation = DiversityRelaxed
					}
					if tier == 2 {
						relaxation = DeliveryRecycled
					}
					r.Items = append(r.Items, Selection{p.post.ID, s, p.features, relaxation})
					r.SourceCounts[s]++
					selected[p.post.ID] = true
					channelCounts[p.post.Channel]++
					if p.post.AuthorGroup != "" {
						authorCounts[p.post.AuthorGroup]++
					}
					found = true
					break
				}
			}
		}
		if !found {
			break
		}
	}
	spread(r.Items, catalog)
	r.RoundLimitReached = len(selected) >= c.MaxRoundIDs
	r.Shortfall = len(r.Items) < c.PageSize
	if !r.RoundLimitReached {
		for id := range union {
			if catalog[id].post.Eligible && !selected[id] && !exposed[id] {
				r.HasMoreWithinInput = true
				break
			}
		}
	}
	return r, nil
}

// Spread the chosen batch, without changing membership, within each relaxation
// phase. Never promote a recycled delivery ahead of an available fresh item.
// Prefer a different public author and channel; if both are impossible prefer a
// different author, then a different channel, preserving prior order for ties.
func spread(items []Selection, catalog map[string]ranked) {
	lastChannel, lastAuthor := "", ""
	for start := 0; start < len(items); {
		end := start + 1
		for end < len(items) && items[end].Relaxation == items[start].Relaxation {
			end++
		}
		for pos := start; pos < end; pos++ {
			best, bestPenalty := pos, 4
			for j := pos; j < end; j++ {
				p := catalog[items[j].ID].post
				penalty := 0
				if lastChannel != "" && p.Channel == lastChannel {
					penalty++
				}
				if lastAuthor != "" && p.AuthorGroup == lastAuthor {
					penalty += 2
				}
				if penalty < bestPenalty {
					best, bestPenalty = j, penalty
				}
				if penalty == 0 {
					break
				}
			}
			chosen := items[best]
			copy(items[pos+1:best+1], items[pos:best])
			items[pos] = chosen
			p := catalog[chosen.ID].post
			lastChannel, lastAuthor = p.Channel, p.AuthorGroup
		}
		start = end
	}
}

func activeIDs(entries []TimedID, now time.Time, ttl time.Duration) map[string]bool {
	ids := make(map[string]bool, len(entries))
	for _, e := range entries {
		if now.Sub(e.At) < ttl {
			ids[e.ID] = true
		}
	}
	return ids
}

// A version-independent digest tie-break avoids map iteration and global RNG.
// This seed is not a credential, public token, or production fairness proof.
func explorationKey(seed uint64, id string) uint64 {
	var prefix [8]byte
	binary.BigEndian.PutUint64(prefix[:], seed)
	h := sha256.New()
	_, _ = h.Write([]byte("HNUHOLE/RECOMMENDATION/B1-EXPLORE\x00"))
	_, _ = h.Write(prefix[:])
	_, _ = h.Write([]byte(id))
	return binary.BigEndian.Uint64(h.Sum(nil)[:8])
}
