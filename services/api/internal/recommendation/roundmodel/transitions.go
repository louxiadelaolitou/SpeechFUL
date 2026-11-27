package roundmodel

import (
	"sort"
	"time"
)

type View struct {
	Offset  int
	End     int
	Next    *int
	IDs     []ID
	Slots   []int // page-local slot for each ID, INTERNAL only
	FirstAt time.Time
}

func view(s State, offset int) (View, error) {
	i, end, err := s.bounds(offset)
	if err != nil {
		return View{}, err
	}
	v := View{Offset: offset, End: end, IDs: []ID{}, Slots: []int{}, FirstAt: s.Pages[i].FirstAt}
	if end < len(s.Manifest.IDs) {
		next := end
		v.Next = &next
	}
	for slot := 0; slot < end-offset; slot++ {
		if s.Pages[i].Omitted&(1<<slot) == 0 {
			v.IDs = append(v.IDs, s.Manifest.IDs[offset+slot])
			v.Slots = append(v.Slots, slot)
		}
	}
	return v, nil
}

// PageInput returns only the current fixed page's unomitted IDs. Its caller must
// project them with A's current authorized scope loader, never replay old body.
func PageInput(r Record, c Context, rev Revisions, offset int) (View, error) {
	if err := r.check(c, rev, false); err != nil {
		return View{}, err
	}
	return view(r.State, offset)
}

// Project records A's INTERNAL Unavailable InputIndex values relative to
// PageInput.IDs (not absolute manifest positions). Every other offered slot is
// assumed to have passed the external current projection. It proves no Gate.
// Both returned values must be persisted atomically; errors expose no changes.
func Project(r Record, c Context, rev Revisions, offset int, unavailable []int) (Record, View, error) {
	v, err := PageInput(r, c, rev, offset)
	if err != nil {
		return Record{}, View{}, err
	}
	if len(unavailable) > len(v.IDs) {
		return Record{}, View{}, ErrInvalid
	}
	mask := uint16(0)
	for _, idx := range unavailable {
		if idx < 0 || idx >= len(v.IDs) || mask&(1<<v.Slots[idx]) != 0 {
			return Record{}, View{}, ErrInvalid
		}
		mask |= 1 << v.Slots[idx]
	}
	next := clone(r)
	index := offset / PageSize
	p := &next.State.Pages[index]
	changed := mask&^p.Omitted != 0 || p.FirstAt.IsZero()
	p.Omitted |= mask
	if p.FirstAt.IsZero() {
		p.FirstAt = c.Now
		p.Delivered = pageBits(v.End-offset) &^ p.Omitted
		for slot := 0; slot < v.End-offset; slot++ {
			if p.Delivered&(1<<slot) == 0 {
				continue
			}
			id := next.State.Manifest.IDs[offset+slot]
			found := false
			for j := range next.History.Deliveries {
				if next.History.Deliveries[j].ID == id {
					found = true
					if c.Now.After(next.History.Deliveries[j].At) {
						until := c.Now.Add(next.State.Manifest.Policy.DeliveryWindow)
						if next.History.Deliveries[j].Until.After(until) {
							until = next.History.Deliveries[j].Until
						}
						next.History.Deliveries[j] = Delivery{id, c.Now, until}
					}
					break
				}
			}
			if !found {
				if len(next.History.Deliveries) >= next.State.Manifest.Policy.DeliveryLimit {
					return Record{}, View{}, ErrCapacity
				}
				next.History.Deliveries = append(next.History.Deliveries, Delivery{id, c.Now, c.Now.Add(next.State.Manifest.Policy.DeliveryWindow)})
			}
		}
		if p.Delivered != 0 {
			next.History.Revision++
			next.History.LastAt = c.Now
			sortHistory(&next.History)
		}
	}
	if changed {
		next.State.Revision++
		next.State.LastAt = c.Now
	}
	if err := next.Validate(); err != nil {
		return Record{}, View{}, err
	}
	v, err = view(next.State, offset)
	return next, v, err
}

// Feedback operates on page-local slots from an already delivered batch. The
// unavailable subset is supplied by the future current-visibility adapter.
// Consuming unavailable members is a no-signal no-op, not a public reason code.
func Feedback(r Record, c Context, rev Revisions, offset int, slots, unavailable []int) (Record, error) {
	if err := r.check(c, rev, true); err != nil {
		return Record{}, err
	}
	i, end, err := r.State.bounds(offset)
	if err != nil {
		return Record{}, err
	}
	p := r.State.Pages[i]
	if p.FirstAt.IsZero() {
		return Record{}, ErrInvalid
	}
	if !c.Now.Before(p.FirstAt.Add(r.State.Manifest.Policy.FeedbackGrace)) {
		return Record{}, ErrExpired
	}
	if len(slots) > PageSize || len(unavailable) > len(slots) {
		return Record{}, ErrInvalid
	}
	requested, missing := uint16(0), uint16(0)
	for _, slot := range slots {
		if slot < 0 || slot >= end-offset || requested&(1<<slot) != 0 || p.Delivered&(1<<slot) == 0 {
			return Record{}, ErrInvalid
		}
		requested |= 1 << slot
	}
	for _, slot := range unavailable {
		if slot < 0 || slot >= end-offset || requested&(1<<slot) == 0 || missing&(1<<slot) != 0 {
			return Record{}, ErrInvalid
		}
		missing |= 1 << slot
	}
	fresh := requested &^ p.Consumed
	next := clone(r)
	if fresh == 0 && missing&^p.Omitted == 0 {
		return next, nil
	}
	for slot := 0; slot < end-offset; slot++ {
		if fresh&(1<<slot) == 0 || missing&(1<<slot) != 0 || p.Omitted&(1<<slot) != 0 {
			continue
		}
		id := r.State.Manifest.IDs[offset+slot]
		until := p.FirstAt.Add(r.State.Manifest.Policy.ExposureWindow)
		found := false
		for j := range next.History.Exposures {
			if next.History.Exposures[j].ID == id {
				found = true
				if until.After(next.History.Exposures[j].Until) {
					next.History.Exposures[j].Until = until
				}
				break
			}
		}
		if !found {
			if len(next.History.Exposures) >= r.State.Manifest.Policy.ExposureLimit {
				return Record{}, ErrCapacity
			}
			next.History.Exposures = append(next.History.Exposures, Exposure{id, until})
		}
	}
	next.State.Pages[i].Consumed |= fresh
	// A confirmed current miss from the feedback adapter is also monotone;
	// restoring visibility later must not resurrect this slot in this round.
	next.State.Pages[i].Omitted |= missing
	next.State.Revision++
	next.State.LastAt = c.Now
	// Even a no-signal consumed batch advances the account fence: otherwise two
	// rounds could overwrite each other's account snapshot at one revision.
	next.History.Revision++
	next.History.LastAt = c.Now
	sortHistory(&next.History)
	if err := next.Validate(); err != nil {
		return Record{}, err
	}
	return next, nil
}

// PruneExpired is an explicit bounded model transition. Expired physical rows
// keep occupying capacity until this returned history is actually committed.
func PruneExpired(h History, now time.Time, limit int) (History, int, error) {
	if err := h.Validate(); err != nil {
		return History{}, 0, err
	}
	if !validTime(now) || now.Before(h.LastAt) || limit < 1 || limit > 128 {
		return History{}, 0, ErrInvalid
	}
	type expired struct {
		delivery bool
		index    int
		until    time.Time
		id       ID
	}
	entries := make([]expired, 0, len(h.Deliveries)+len(h.Exposures))
	for i, d := range h.Deliveries {
		if !d.Until.After(now) {
			entries = append(entries, expired{true, i, d.Until, d.ID})
		}
	}
	for i, e := range h.Exposures {
		if !e.Until.After(now) {
			entries = append(entries, expired{false, i, e.Until, e.ID})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if !a.until.Equal(b.until) {
			return a.until.Before(b.until)
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return !a.delivery && b.delivery
	})
	removed := len(entries)
	if removed > limit {
		removed = limit
	}
	dropD, dropE := map[int]bool{}, map[int]bool{}
	for _, x := range entries[:removed] {
		if x.delivery {
			dropD[x.index] = true
		} else {
			dropE[x.index] = true
		}
	}
	next := h
	next.Deliveries = []Delivery{}
	next.Exposures = []Exposure{}
	for i, d := range h.Deliveries {
		if !dropD[i] {
			next.Deliveries = append(next.Deliveries, d)
		}
	}
	for i, e := range h.Exposures {
		if !dropE[i] {
			next.Exposures = append(next.Exposures, e)
		}
	}
	if removed > 0 {
		next.Revision++
		next.LastAt = now
	}
	if err := next.Validate(); err != nil {
		return History{}, 0, err
	}
	return next, removed, nil
}
