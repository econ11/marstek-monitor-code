package history

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

// weeksOver: a period of more than this many days is shown by weeks.
const weeksOver = 30

// Period is a report range of whole local days: From is the local midnight of
// the first day, To the local midnight after the last day, clipped to now.
// Weeks: more than 30 days, so the buckets are weeks starting on Monday.
type Period struct {
	From, To time.Time
	Weeks    bool
}

// PeriodFor builds the period for the days fromDay..toDay ("2006-01-02",
// inclusive, in time.Local). fromDay after toDay or after today is an error.
func PeriodFor(fromDay, toDay string, now time.Time) (Period, error) {
	from, err := time.ParseInLocation(time.DateOnly, fromDay, time.Local)
	if err != nil {
		return Period{}, fmt.Errorf("history: from day: %w", err)
	}
	to, err := time.ParseInLocation(time.DateOnly, toDay, time.Local)
	if err != nil {
		return Period{}, fmt.Errorf("history: to day: %w", err)
	}
	if to.Before(from) {
		return Period{}, errors.New("history: from day is after to day")
	}
	if from.After(now) {
		return Period{}, errors.New("history: from day is in the future")
	}
	end := nextDay(to)
	if end.After(now) {
		end = now
	}
	// Weeks counts only the days up to today: a future toDay is clipped.
	if l := now.In(time.Local); to.After(l) {
		to = l
	}
	return Period{From: from, To: end, Weeks: dayCount(from, to) > weeksOver}, nil
}

// View is one outage with what was observed of it.
type View struct {
	Outage store.Outage
	Bounds Bounds
}

// Bucket is one day (or week) of the report: a chart column and its tooltip.
type Bucket struct {
	Start, End time.Time
	Off        time.Duration // sum of Known within the bucket
	OffAtLeast bool          // Off is a lower bound «≥» (OffKnown's atLeast for the bucket)
	Count      int           // outages that touch the bucket
	// Ongoing: an ongoing outage touches the bucket. It is set in every bucket
	// the outage spans (its start day too), meaning "continues past this bucket".
	Ongoing  bool
	Longest  *View    // only if Count > 1
	MinSOC   *float64 // lowest charge during outages in the bucket
	RanOut   bool     // an outage's DiedAt lies in the bucket (bucket-scoped, unlike Report.RanOut)
	Observed time.Duration
	Gaps     time.Duration
	// NoData: Observed == 0 and the bucket is not before ObservedSince. It wins
	// over Count: an outage spanning a whole-day gap, or a frozen ongoing one,
	// can touch a NoData bucket, which is still shown as "no data".
	NoData    bool
	BeforeObs bool // the bucket lies entirely before ObservedSince
}

// Report is the "Outages" tab for one period.
type Report struct {
	Period        Period
	ObservedSince *time.Time
	Observed      time.Duration // period ∩ [ObservedSince, now] minus gaps
	Off           time.Duration // sum of Known within the period
	OffAtLeast    bool          // Off is a lower bound «≥» (OffKnown's atLeast for the period)
	GridPct       *float64      // (Observed−Off)/Observed·100; nil when Observed == 0
	// RanOut counts rows with DiedAt, wherever DiedAt lies (consistent with
	// Rows); Bucket.RanOut is scoped to its bucket.
	Count, RanOut int
	// Coverage = closed without DiedAt / closed ·100 over closed outages whose
	// end was observed: a closed outage without DiedAt whose end lies in a data
	// gap (EndApprox) is left out, since a death there could not be seen.
	// CoverageApprox: some were left out («≈»). nil without counted outages.
	Coverage       *float64
	CoverageApprox bool
	// Average and Longest use Known (the lower bound of a "≥" row), over the
	// whole outage, ongoing ones included.
	Average, Longest time.Duration
	LongestAt        *time.Time
	Uncertain        int // rows with AtLeast
	Gaps             time.Duration
	Buckets          []Bucket
	Rows             []View // newest first
	// GapSpans are the gaps that touch the period or end just before a row's
	// start (within BoundarySlack), oldest first, with their original bounds
	// (not clipped to the period): the tab names their reasons.
	GapSpans []store.Gap
}

// BuildReport computes the report for p from the outages and gaps around it
// (rows outside p are ignored). minSOC returns the lowest charge in [from, to)
// or nil; it may be nil itself. A day with no observation is "no data", never
// "0 outages": see Bucket.NoData and BeforeObs.
func BuildReport(outs []store.Outage, gaps []store.Gap, minSOC func(from, to time.Time) *float64,
	observedSince *time.Time, p Period, now, lastAt time.Time, live bool) Report {
	r := Report{Period: p}
	if observedSince != nil {
		since := *observedSince
		r.ObservedSince = &since
	}
	var views []View
	for _, o := range outs {
		if _, n, _ := OffKnown([]store.Outage{o}, gaps, p.From, p.To, now, lastAt, live); n == 1 {
			views = append(views, View{Outage: o, Bounds: Annotate(o, gaps, now, lastAt, live)})
		}
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].Outage.Start.After(views[j].Outage.Start) })
	r.Rows = views

	r.Observed, r.Gaps = observed(gaps, observedSince, p.From, p.To, now)
	r.Off, r.Count, r.OffAtLeast = OffKnown(outages(views), gaps, p.From, p.To, now, lastAt, live)
	if r.Observed > 0 {
		pct := clampPct(float64(r.Observed-r.Off) / float64(r.Observed) * 100)
		r.GridPct = &pct
	}
	var closedN, lasted int
	var sum time.Duration
	for _, v := range views {
		if v.Outage.DiedAt != nil {
			r.RanOut++
		}
		switch {
		case v.Outage.End == nil: // ongoing: not counted
		case v.Outage.DiedAt == nil && v.Bounds.EndApprox:
			r.CoverageApprox = true // end in a gap: a death could not be observed
		default:
			closedN++
			if v.Outage.DiedAt == nil {
				lasted++
			}
		}
		if v.Bounds.AtLeast {
			r.Uncertain++
		}
		sum += v.Bounds.Known
		if r.LongestAt == nil || v.Bounds.Known > r.Longest {
			start := v.Outage.Start
			r.Longest, r.LongestAt = v.Bounds.Known, &start
		}
	}
	if len(views) > 0 {
		r.Average = sum / time.Duration(len(views))
	}
	if closedN > 0 {
		c := float64(lasted) / float64(closedN) * 100
		r.Coverage = &c
	}
	for _, b := range BucketBounds(p) {
		r.Buckets = append(r.Buckets, bucket(b, views, gaps, minSOC, observedSince, now, lastAt, live))
	}
	r.GapSpans = gapSpans(gaps, views, p, now)
	return r
}

// gapSpans keeps the gaps that overlap [lo, p.To), lo being the earlier of
// p.From and the oldest row's start, less BoundarySlack; sorted by start.
func gapSpans(gaps []store.Gap, views []View, p Period, now time.Time) []store.Gap {
	lo := p.From
	for _, v := range views {
		lo = minTime(lo, v.Outage.Start)
	}
	lo = lo.Add(-BoundarySlack)
	var out []store.Gap
	for _, g := range gaps {
		if gapEnd(g, now).After(lo) && g.Start.Before(p.To) {
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

func bucket(iv Span, views []View, gaps []store.Gap, minSOC func(from, to time.Time) *float64,
	observedSince *time.Time, now, lastAt time.Time, live bool) Bucket {
	b := Bucket{Start: iv.Start, End: iv.End}
	b.Observed, b.Gaps = observed(gaps, observedSince, iv.Start, iv.End, now)
	b.BeforeObs = observedSince == nil || !iv.End.After(*observedSince)
	b.NoData = b.Observed == 0 && !b.BeforeObs
	var longest *View
	for i, v := range views {
		off, n, atLeast := OffKnown([]store.Outage{v.Outage}, gaps, iv.Start, iv.End, now, lastAt, live)
		if n == 0 {
			continue
		}
		b.Count++
		b.Off += off
		b.OffAtLeast = b.OffAtLeast || atLeast
		if v.Outage.End == nil {
			b.Ongoing = true
		}
		if d := v.Outage.DiedAt; d != nil && !d.Before(iv.Start) && d.Before(iv.End) {
			b.RanOut = true
		}
		if longest == nil || v.Bounds.Known > longest.Bounds.Known {
			longest = &views[i]
		}
		s, e := maxTime(v.Outage.Start, iv.Start), minTime(v.Bounds.AsOf, iv.End)
		if minSOC != nil && e.After(s) {
			if soc := minSOC(s, e); soc != nil && (b.MinSOC == nil || *soc < *b.MinSOC) {
				x := *soc
				b.MinSOC = &x
			}
		}
	}
	if b.Count > 1 {
		l := *longest
		b.Longest = &l
	}
	return b
}

// observed is the observed time in [from, to) (after observedSince, minus
// gaps) and the gap time after observedSince.
func observed(gaps []store.Gap, observedSince *time.Time, from, to, now time.Time) (obs, gap time.Duration) {
	if !to.After(from) {
		return 0, 0
	}
	obs = to.Sub(from) - Unobserved(gaps, observedSince, from, to, now)
	if observedSince != nil {
		gap = Overlap(gaps, maxTime(from, *observedSince), to, now)
	}
	return obs, gap
}

// Span is one report bucket: [Start, End).
type Span struct{ Start, End time.Time }

// BucketBounds splits p into local days, or into weeks from Monday when
// p.Weeks (the first and last week may be partial). Days are stepped by
// calendar date, never by 24 h, so a DST day keeps its 23 or 25 hours.
func BucketBounds(p Period) []Span {
	var out []Span
	s := p.From
	for s.Before(p.To) {
		var e time.Time
		if p.Weeks {
			l := s.In(time.Local)
			back := (int(l.Weekday()) + 6) % 7 // days since Monday
			e = time.Date(l.Year(), l.Month(), l.Day()-back+7, 0, 0, 0, 0, time.Local)
		} else {
			e = nextDay(s)
		}
		out = append(out, Span{s, minTime(e, p.To)})
		s = e
	}
	return out
}

// nextDay is the local midnight after the local day of t.
func nextDay(t time.Time) time.Time {
	l := t.In(time.Local)
	return time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, time.Local)
}

// dayCount is the number of calendar days from a to b inclusive.
func dayCount(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da)/(24*time.Hour)) + 1
}

func outages(vs []View) []store.Outage {
	out := make([]store.Outage, len(vs))
	for i, v := range vs {
		out[i] = v.Outage
	}
	return out
}

func clampPct(v float64) float64 {
	return max(0, min(100, v))
}
