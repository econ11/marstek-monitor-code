// Package history derives what the stored outages and gaps say about the
// past: how much of an outage was actually observed and which of its bounds
// are only known approximately because they lie next to a data gap (final
// review I-3). Pure logic over store rows; time comes only from parameters.
package history

import (
	"sort"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

// BoundarySlack is the longest delay from a gap end to the first live
// observation (resync up to 30 s plus the debounce). An outage bound this
// close after a gap end was only first seen then, not necessarily caused then.
const BoundarySlack = 2 * time.Minute

// atLeastOverlap: gap overlap from this much on makes the duration a lower bound.
const atLeastOverlap = time.Minute

// Bounds annotates one outage with what was actually observed of it.
type Bounds struct {
	StartApprox bool          // Partial, or Start inside a gap or <= BoundarySlack after its end
	EndApprox   bool          // a gap inside the outage ended <= BoundarySlack before End
	NotAfter    *time.Time    // EndApprox: the grid was back by then (= End)
	Frozen      bool          // ongoing and link not live: AsOf = last measurement
	AsOf        time.Time     // End, else now (live) or lastAt (not live)
	Known       time.Duration // [Start, AsOf] minus overlap with gaps: observed off time
	AtLeast     bool          // StartApprox || EndApprox || Frozen || gap overlap >= 1 min
	// StartFrom: the start lies in a gap (inside it or <= BoundarySlack after
	// its end), so the outage began between StartFrom (the gap's start, the
	// last observation before it; see WindowStart) and
	// Start (its first observation). nil when the start was observed, or is
	// approximate without a gap (Partial on the very first run).
	StartFrom *time.Time
}

// Annotate computes the bounds of o. A gap with End == nil ends at now.
func Annotate(o store.Outage, gaps []store.Gap, now, lastAt time.Time, live bool) Bounds {
	var b Bounds
	switch {
	case o.End != nil:
		b.AsOf = *o.End
	case live:
		b.AsOf = now
	default:
		b.Frozen, b.AsOf = true, lastAt
	}
	b.StartApprox = o.Partial
	// in: the start of the latest gap the start lies in. An earlier one ended
	// at an observation before that gap began.
	var in *time.Time
	lies := func(g store.Gap) {
		if in == nil || g.Start.After(*in) {
			s := g.Start
			in = &s
		}
	}
	for _, g := range gaps {
		ge := gapEnd(g, now)
		if !ge.After(o.Start) && o.Start.Sub(ge) <= BoundarySlack {
			b.StartApprox = true
			lies(g)
		}
		// The start lies inside the gap: it was not observed either. A start
		// at the gap's own start is its last observation, so it is exact.
		if g.Start.Before(o.Start) && ge.After(o.Start) {
			b.StartApprox = true
			lies(g)
		}
		if o.End != nil && ge.After(o.Start) && !ge.After(*o.End) && o.End.Sub(ge) <= BoundarySlack {
			b.EndApprox = true
		}
	}
	if in != nil {
		s := WindowStart(gaps, *in, now)
		b.StartFrom = &s
	}
	if b.EndApprox {
		end := *o.End
		b.NotAfter = &end
	}
	if b.AsOf.After(o.Start) {
		ov := Overlap(gaps, o.Start, b.AsOf, now)
		b.Known = b.AsOf.Sub(o.Start) - ov
		b.AtLeast = ov >= atLeastOverlap
	}
	b.AtLeast = b.AtLeast || b.StartApprox || b.EndApprox || b.Frozen
	return b
}

// WindowStart is the earliest start of the window that begins at from, the
// start of a gap (the last observation before it): gaps that overlap from
// (begin before it, end after it; an open one ends at now) reach further back,
// followed transitively. A gap that only touches from ended at an observation,
// so it does not. The toast ({from}, core/monitor) and Bounds.StartFrom share it.
func WindowStart(gaps []store.Gap, from, now time.Time) time.Time {
	for changed := true; changed; {
		changed = false
		for _, g := range gaps {
			if g.Start.Before(from) && gapEnd(g, now).After(from) {
				from, changed = g.Start, true
			}
		}
	}
	return from
}

// Overlap is the total time the gaps cover within [from, to); overlapping
// gaps are counted once. An open gap (End == nil) ends at now.
func Overlap(gaps []store.Gap, from, to, now time.Time) time.Duration {
	ivs := make([]interval, 0, len(gaps))
	for _, g := range gaps {
		ivs = append(ivs, interval{g.Start, gapEnd(g, now)})
	}
	return union(ivs, from, to)
}

// OffKnown sums the observed off time (Bounds.Known) of outs clipped to
// [from, to). count is the number of outages that touch the window; atLeast
// reports that the sum is a lower bound: an approximate bound inside the
// window, a frozen outage, or gap overlap of at least 1 min inside it.
func OffKnown(outs []store.Outage, gaps []store.Gap, from, to, now, lastAt time.Time, live bool) (off time.Duration, count int, atLeast bool) {
	for _, o := range outs {
		b := Annotate(o, gaps, now, lastAt, live)
		s, e := maxTime(o.Start, from), minTime(b.AsOf, to)
		// A frozen ongoing outage still touches every window after its last
		// measurement: its off time there is unknown, not zero.
		if !o.Start.Before(to) || (!e.After(s) && o.Start.Before(from) && !b.Frozen) {
			continue // outside the window
		}
		count++
		if e.After(s) {
			ov := Overlap(gaps, s, e, now)
			off += e.Sub(s) - ov
			if ov >= atLeastOverlap {
				atLeast = true
			}
		}
		if b.Frozen || b.StartApprox && !o.Start.Before(from) || b.EndApprox && b.NotAfter.Before(to) {
			atLeast = true
		}
	}
	return off, count, atLeast
}

// Unobserved is the time in [from, to) that was not observed: the gaps plus
// [from, observedSince) (before the first observation ever). A nil
// observedSince means nothing was ever observed: the whole window up to now
// is unobserved (never "fully observed by default"). Overlaps are counted
// once; an open gap ends at now.
func Unobserved(gaps []store.Gap, observedSince *time.Time, from, to, now time.Time) time.Duration {
	since := now
	if observedSince != nil {
		since = *observedSince
	}
	ivs := make([]interval, 0, len(gaps)+1)
	for _, g := range gaps {
		ivs = append(ivs, interval{g.Start, gapEnd(g, now)})
	}
	ivs = append(ivs, interval{from, since})
	return union(ivs, from, to)
}

type interval struct{ s, e time.Time }

// union is the length of the union of ivs clipped to [from, to).
func union(ivs []interval, from, to time.Time) time.Duration {
	clipped := ivs[:0:0]
	for _, iv := range ivs {
		s, e := maxTime(iv.s, from), minTime(iv.e, to)
		if e.After(s) {
			clipped = append(clipped, interval{s, e})
		}
	}
	sort.Slice(clipped, func(i, j int) bool { return clipped[i].s.Before(clipped[j].s) })
	var total time.Duration
	var cur interval
	for i, iv := range clipped {
		switch {
		case i == 0:
			cur = iv
		case !iv.s.After(cur.e):
			cur.e = maxTime(cur.e, iv.e)
		default:
			total += cur.e.Sub(cur.s)
			cur = iv
		}
	}
	if len(clipped) > 0 {
		total += cur.e.Sub(cur.s)
	}
	return total
}

func gapEnd(g store.Gap, now time.Time) time.Time {
	if g.End != nil {
		return *g.End
	}
	return now
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
