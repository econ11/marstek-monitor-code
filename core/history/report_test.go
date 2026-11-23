package history

import (
	"math"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Kyiv on any machine

	"github.com/econ11/marstek-monitor-code/core/store"
)

// kyiv sets time.Local to Europe/Kyiv for the test (days start at local midnight).
func kyiv(t *testing.T) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

// lt is a local wall-clock time.
func lt(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.Local)
}

func mustPeriod(t *testing.T, from, to string, now time.Time) Period {
	t.Helper()
	p, err := PeriodFor(from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPeriodFor(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	p := mustPeriod(t, "2026-11-15", "2026-11-21", now)
	if !p.From.Equal(lt(2026, 11, 15, 0, 0)) || !p.To.Equal(now) || p.Weeks {
		t.Fatalf("period = %+v, want 15.11 00:00 .. now, days", p)
	}
	past := mustPeriod(t, "2026-11-01", "2026-11-02", now)
	if !past.To.Equal(lt(2026, 11, 3, 0, 0)) {
		t.Fatalf("To = %v, want the midnight after the last day", past.To)
	}
	for _, c := range [][2]string{{"2026-11-22", "2026-11-22"}, {"2026-11-21", "2026-11-20"}, {"21.11.2026", "2026-11-21"}, {"2026-11-21", ""}} {
		if _, err := PeriodFor(c[0], c[1], now); err == nil {
			t.Fatalf("PeriodFor(%q, %q) = nil error", c[0], c[1])
		}
	}
	if p := mustPeriod(t, "2026-10-22", "2026-11-21", now); !p.Weeks {
		t.Fatal("31 days: want weeks")
	}
	if p := mustPeriod(t, "2026-10-23", "2026-11-21", now); p.Weeks {
		t.Fatal("30 days: want days")
	}
	// Weeks counts only the days up to today: 25.10..31.12 is 28 days so far.
	if p := mustPeriod(t, "2026-10-25", "2026-12-31", now); p.Weeks || !p.To.Equal(now) {
		t.Fatalf("future toDay: %+v, want days up to now", p)
	}
}

func closed(s, e time.Time) store.Outage { return store.Outage{Start: s, End: tp(e)} }

func TestReportCountsAndRings(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	p := mustPeriod(t, "2026-11-20", "2026-11-21", now)
	since := lt(2026, 11, 1, 0, 0)
	died := closed(lt(2026, 11, 20, 8, 0), lt(2026, 11, 20, 12, 0)) // 4h
	died.DiedAt = tp(lt(2026, 11, 20, 11, 0))
	outs := []store.Outage{
		closed(lt(2026, 11, 20, 1, 0), lt(2026, 11, 20, 2, 0)), // 1h
		died,
		closed(lt(2026, 11, 21, 6, 0), lt(2026, 11, 21, 8, 0)), // 2h
		{Start: lt(2026, 11, 21, 13, 0)},                       // ongoing, 1h so far
	}
	var asked [][2]time.Time
	minSOC := func(from, to time.Time) *float64 {
		asked = append(asked, [2]time.Time{from, to})
		v := 40.0
		return &v
	}
	r := BuildReport(outs, nil, minSOC, &since, p, now, now, true)
	if r.Count != 4 || r.RanOut != 1 {
		t.Fatalf("Count/RanOut = %d/%d, want 4/1", r.Count, r.RanOut)
	}
	if r.Coverage == nil || math.Abs(*r.Coverage-200.0/3) > 0.01 {
		t.Fatalf("Coverage = %v, want 66.67", r.Coverage)
	}
	if r.Off != 8*time.Hour || r.Observed != 38*time.Hour {
		t.Fatalf("Off/Observed = %v/%v, want 8h/38h", r.Off, r.Observed)
	}
	want := float64(30) / 38 * 100
	if r.GridPct == nil || math.Abs(*r.GridPct-want) > 1e-9 {
		t.Fatalf("GridPct = %v, want %v", r.GridPct, want)
	}
	if r.Longest != 4*time.Hour || r.LongestAt == nil || !r.LongestAt.Equal(died.Start) || r.Average != 2*time.Hour {
		t.Fatalf("Longest/At/Average = %v/%v/%v", r.Longest, r.LongestAt, r.Average)
	}
	if r.Uncertain != 0 || r.Gaps != 0 {
		t.Fatalf("Uncertain/Gaps = %d/%v", r.Uncertain, r.Gaps)
	}
	if r.OffAtLeast || r.CoverageApprox || r.Buckets[0].OffAtLeast || r.Buckets[1].OffAtLeast {
		t.Fatalf("exact outages: OffAtLeast/CoverageApprox = %v/%v, buckets %v/%v; want false",
			r.OffAtLeast, r.CoverageApprox, r.Buckets[0].OffAtLeast, r.Buckets[1].OffAtLeast)
	}
	if len(r.Rows) != 4 || !r.Rows[0].Outage.Start.Equal(outs[3].Start) || !r.Rows[3].Outage.Start.Equal(outs[0].Start) {
		t.Fatalf("rows not newest first: %+v", r.Rows)
	}
	if len(r.Buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(r.Buckets))
	}
	d0, d1 := r.Buckets[0], r.Buckets[1]
	if d0.Count != 2 || d0.Off != 5*time.Hour || !d0.RanOut || d0.Ongoing || d0.Longest == nil || d0.Longest.Bounds.Known != 4*time.Hour {
		t.Fatalf("day 20 = %+v", d0)
	}
	if d1.Count != 2 || d1.Off != 3*time.Hour || d1.RanOut || !d1.Ongoing || !d1.End.Equal(now) || d1.Observed != 14*time.Hour {
		t.Fatalf("day 21 = %+v", d1)
	}
	if d0.MinSOC == nil || *d0.MinSOC != 40 {
		t.Fatalf("MinSOC = %v, want 40", d0.MinSOC)
	}
	// minSOC is asked only for the outage parts inside each bucket.
	for _, a := range asked {
		if a[0].Before(p.From) || a[1].After(now) || !a[1].After(a[0]) {
			t.Fatalf("minSOC asked for %v", a)
		}
	}

	// A single outage in a bucket has no separate Longest; no closed outage → nil coverage.
	r = BuildReport(outs[3:], nil, nil, &since, p, now, now, true)
	if r.Coverage != nil || r.Buckets[1].Longest != nil || r.Buckets[1].MinSOC != nil {
		t.Fatalf("coverage/longest/minSOC = %v/%v/%v, want nil", r.Coverage, r.Buckets[1].Longest, r.Buckets[1].MinSOC)
	}
}

func TestReportLowerBoundsAcrossGaps(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	p := mustPeriod(t, "2026-11-20", "2026-11-21", now)
	since := lt(2026, 11, 1, 0, 0)
	// 22:00–09:00 with the PC off 23:00–09:00: known 1h, end "not later than".
	outs := []store.Outage{
		closed(lt(2026, 11, 20, 22, 0), lt(2026, 11, 21, 9, 0)),
		closed(lt(2026, 11, 21, 10, 0), lt(2026, 11, 21, 13, 0)), // exact 3h
	}
	gaps := []store.Gap{{Start: lt(2026, 11, 20, 23, 0), End: tp(lt(2026, 11, 21, 9, 0)), Reason: store.GapAppNotRunning}}
	r := BuildReport(outs, gaps, nil, &since, p, now, now, true)
	if r.Uncertain != 1 {
		t.Fatalf("Uncertain = %d, want 1", r.Uncertain)
	}
	if r.Average != 2*time.Hour || r.Longest != 3*time.Hour || r.Off != 4*time.Hour {
		t.Fatalf("Average/Longest/Off = %v/%v/%v, want 2h/3h/4h (known time)", r.Average, r.Longest, r.Off)
	}
	if r.Gaps != 10*time.Hour || r.Observed != 28*time.Hour {
		t.Fatalf("Gaps/Observed = %v/%v, want 10h/28h", r.Gaps, r.Observed)
	}
	if v := r.Rows[1].Bounds; !v.AtLeast || !v.EndApprox || v.NotAfter == nil {
		t.Fatalf("row bounds = %+v, want a lower bound with NotAfter", v)
	}
	if b := r.Buckets[0]; b.Gaps != time.Hour || b.Observed != 23*time.Hour || b.Off != time.Hour {
		t.Fatalf("day 20 = %+v", b)
	}
	// «≥» for the totals and for each day's tooltip («≥ 1 год» on 20.11).
	if !r.OffAtLeast || !r.Buckets[0].OffAtLeast || !r.Buckets[1].OffAtLeast {
		t.Fatalf("OffAtLeast = %v, buckets %v/%v; want all true", r.OffAtLeast, r.Buckets[0].OffAtLeast, r.Buckets[1].OffAtLeast)
	}
	// The first outage ended in the gap: a death there could not be seen, so
	// it is left out of coverage, which becomes approximate.
	if r.Coverage == nil || *r.Coverage != 100 || !r.CoverageApprox {
		t.Fatalf("Coverage/Approx = %v/%v, want 100/true", r.Coverage, r.CoverageApprox)
	}
	// Only the end-in-gap outage: nothing to count, coverage nil but approximate.
	r = BuildReport(outs[:1], gaps, nil, &since, p, now, now, true)
	if r.Coverage != nil || !r.CoverageApprox {
		t.Fatalf("Coverage/Approx = %v/%v, want nil/true", r.Coverage, r.CoverageApprox)
	}
	// A dead outage whose end lies in a gap is still counted: the death was seen.
	died := closed(lt(2026, 11, 20, 22, 0), lt(2026, 11, 21, 9, 0))
	died.DiedAt = tp(lt(2026, 11, 20, 23, 30))
	r = BuildReport([]store.Outage{died}, gaps, nil, &since, p, now, now, true)
	if r.Coverage == nil || *r.Coverage != 0 || r.CoverageApprox {
		t.Fatalf("Coverage/Approx = %v/%v, want 0/false", r.Coverage, r.CoverageApprox)
	}

	// A row uncertain only outside the window: its start (right after a gap
	// on 20.11) is approximate, but its part on 21.11 is exact.
	late := []store.Outage{closed(lt(2026, 11, 20, 5, 0), lt(2026, 11, 21, 2, 0))}
	early := []store.Gap{{Start: lt(2026, 11, 20, 3, 0), End: tp(lt(2026, 11, 20, 4, 59)), Reason: store.GapPCSleep}}
	r = BuildReport(late, early, nil, &since, mustPeriod(t, "2026-11-21", "2026-11-21", now), now, now, true)
	if r.Uncertain != 1 || r.OffAtLeast || r.Buckets[0].OffAtLeast || r.Off != 2*time.Hour {
		t.Fatalf("Uncertain/OffAtLeast/bucket/Off = %d/%v/%v/%v, want 1/false/false/2h",
			r.Uncertain, r.OffAtLeast, r.Buckets[0].OffAtLeast, r.Off)
	}
}

func TestReportDayBucketsAcrossDST(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 1, 12, 0)
	since := lt(2026, 10, 1, 0, 0)
	p := mustPeriod(t, "2026-10-24", "2026-10-26", now)
	r := BuildReport(nil, nil, nil, &since, p, now, now, true)
	if len(r.Buckets) != 3 {
		t.Fatalf("buckets = %d, want 3", len(r.Buckets))
	}
	lens := []time.Duration{24 * time.Hour, 25 * time.Hour, 24 * time.Hour}
	for i, b := range r.Buckets {
		l := b.Start.In(time.Local)
		if l.Hour() != 0 || l.Minute() != 0 || l.Day() != 24+i {
			t.Fatalf("bucket %d starts %v, want local midnight of %d.10", i, l, 24+i)
		}
		if b.End.Sub(b.Start) != lens[i] || b.Observed != lens[i] || b.NoData || b.Count != 0 {
			t.Fatalf("bucket %d = %+v, want length/observed %v", i, b, lens[i])
		}
	}
	if r.Observed != 73*time.Hour {
		t.Fatalf("Observed = %v, want 73h", r.Observed)
	}
}

func TestReportNoDataAndZeroDays(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	since := lt(2026, 11, 1, 0, 0)
	p := mustPeriod(t, "2026-11-19", "2026-11-20", now)
	gaps := []store.Gap{{Start: lt(2026, 11, 18, 20, 0), End: tp(lt(2026, 11, 20, 0, 0)), Reason: store.GapPCSleep}}
	r := BuildReport(nil, gaps, nil, &since, p, now, now, true)
	gap, seen := r.Buckets[0], r.Buckets[1]
	if !gap.NoData || gap.Observed != 0 || gap.Gaps != 24*time.Hour || gap.BeforeObs {
		t.Fatalf("day in a gap = %+v, want NoData", gap)
	}
	if seen.NoData || seen.Count != 0 || seen.Observed != 24*time.Hour {
		t.Fatalf("observed day = %+v, want 0 outages with data", seen)
	}
	if r.GridPct == nil || *r.GridPct != 100 || r.Count != 0 {
		t.Fatalf("GridPct/Count = %v/%d, want 100/0", r.GridPct, r.Count)
	}

	// Nothing observed in the whole period: no percentage, not 100%.
	r = BuildReport(nil, gaps, nil, &since, mustPeriod(t, "2026-11-19", "2026-11-19", now), now, now, true)
	if r.GridPct != nil || r.Observed != 0 {
		t.Fatalf("GridPct/Observed = %v/%v, want nil/0", r.GridPct, r.Observed)
	}
}

func TestReportDaysBeforeObservation(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	since := lt(2026, 11, 18, 0, 0)
	r := BuildReport(nil, nil, nil, &since, mustPeriod(t, "2026-11-15", "2026-11-21", now), now, now, true)
	if len(r.Buckets) != 7 {
		t.Fatalf("buckets = %d, want 7", len(r.Buckets))
	}
	for i, b := range r.Buckets {
		before := i < 3
		if b.BeforeObs != before || b.NoData || (before && b.Observed != 0) || (!before && b.Observed == 0) {
			t.Fatalf("bucket %d (%v) = %+v, want BeforeObs %v", i, b.Start, b, before)
		}
	}
	if r.Observed != 3*24*time.Hour+14*time.Hour {
		t.Fatalf("Observed = %v, want 86h (from 18.11)", r.Observed)
	}
	if r.ObservedSince == nil || !r.ObservedSince.Equal(since) {
		t.Fatalf("ObservedSince = %v", r.ObservedSince)
	}

	// Never observed: every bucket is before observation, nothing is "no data".
	r = BuildReport(nil, nil, nil, nil, mustPeriod(t, "2026-11-20", "2026-11-21", now), now, now, true)
	for i, b := range r.Buckets {
		if !b.BeforeObs || b.NoData || b.Observed != 0 {
			t.Fatalf("bucket %d = %+v, want BeforeObs", i, b)
		}
	}
	if r.GridPct != nil {
		t.Fatalf("GridPct = %v, want nil", *r.GridPct)
	}
}

func TestReportWeeksOver30Days(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	since := lt(2026, 9, 1, 0, 0)
	p := mustPeriod(t, "2026-10-01", "2026-11-14", now) // 45 days, Thu .. Sat
	if !p.Weeks {
		t.Fatal("45 days: want weeks")
	}
	r := BuildReport(nil, nil, nil, &since, p, now, now, true)
	if n := len(r.Buckets); n != 7 {
		t.Fatalf("buckets = %d, want 7 (Thu–Sun, 5 full weeks, Mon–Sat)", n)
	}
	first, last := r.Buckets[0], r.Buckets[len(r.Buckets)-1]
	if !first.Start.Equal(p.From) || !first.End.Equal(lt(2026, 10, 5, 0, 0)) {
		t.Fatalf("first week = %v..%v, want 01.10..05.10 (partial)", first.Start, first.End)
	}
	if !last.Start.Equal(lt(2026, 11, 9, 0, 0)) || !last.End.Equal(lt(2026, 11, 15, 0, 0)) {
		t.Fatalf("last week = %v..%v, want 09.11..15.11 (partial)", last.Start, last.End)
	}
	for i, b := range r.Buckets[1:] {
		l := b.Start.In(time.Local)
		if l.Weekday() != time.Monday || l.Hour() != 0 {
			t.Fatalf("bucket %d starts %v, want Monday 00:00", i+1, l)
		}
		if i+1 < len(r.Buckets)-1 && b.End.Sub(b.Start) < 7*24*time.Hour {
			t.Fatalf("middle week %d is %v long", i+1, b.End.Sub(b.Start))
		}
	}
	// The week with 25.10 has 7 days and 1 hour.
	if b := r.Buckets[3]; !b.Start.Equal(lt(2026, 10, 19, 0, 0)) || b.End.Sub(b.Start) != 7*24*time.Hour+time.Hour {
		t.Fatalf("DST week = %v..%v", b.Start, b.End)
	}
}

func TestReportMidnightSplit(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	since := lt(2026, 11, 1, 0, 0)
	outs := []store.Outage{closed(lt(2026, 11, 19, 23, 0), lt(2026, 11, 20, 1, 0))}
	r := BuildReport(outs, nil, nil, &since, mustPeriod(t, "2026-11-19", "2026-11-20", now), now, now, true)
	for i, b := range r.Buckets {
		if b.Off != time.Hour || b.Count != 1 {
			t.Fatalf("day %d = %+v, want 1h and 1 outage", i, b)
		}
	}
	if r.Count != 1 || r.Off != 2*time.Hour || len(r.Rows) != 1 {
		t.Fatalf("Count/Off/Rows = %d/%v/%d", r.Count, r.Off, len(r.Rows))
	}
	// Outages outside the period (the monitor fetches a day of margin) are ignored.
	outs = append(outs, closed(lt(2026, 11, 18, 10, 0), lt(2026, 11, 18, 11, 0)), closed(lt(2026, 11, 21, 1, 0), lt(2026, 11, 21, 2, 0)))
	r = BuildReport(outs, nil, nil, &since, mustPeriod(t, "2026-11-19", "2026-11-20", now), now, now, true)
	if r.Count != 1 || len(r.Rows) != 1 {
		t.Fatalf("Count/Rows = %d/%d, want only the outage in the period", r.Count, len(r.Rows))
	}
}

func TestReportGapSpans(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 21, 14, 0)
	p := mustPeriod(t, "2026-11-20", "2026-11-21", now)
	since := lt(2026, 11, 1, 0, 0)
	// The outage started on 19.11 right after a PC-off gap: that gap is kept
	// for its reason, though it lies before the period.
	outs := []store.Outage{{Start: lt(2026, 11, 19, 22, 1), End: tp(lt(2026, 11, 20, 1, 0)), Partial: true}}
	gaps := []store.Gap{
		{ID: 1, Start: lt(2026, 11, 18, 1, 0), End: tp(lt(2026, 11, 18, 2, 0)), Reason: store.GapAppNotRunning}, // too early
		{ID: 3, Start: lt(2026, 11, 21, 3, 0), End: tp(lt(2026, 11, 21, 5, 0)), Reason: store.GapPCSleep},
		{ID: 2, Start: lt(2026, 11, 19, 20, 0), End: tp(lt(2026, 11, 19, 22, 0)), Reason: store.GapAppNotRunning},
		{ID: 4, Start: lt(2026, 11, 21, 13, 0), Reason: store.GapNoLink}, // open
	}
	r := BuildReport(outs, gaps, nil, &since, p, now, now, true)
	var ids []int64
	for _, g := range r.GapSpans {
		ids = append(ids, g.ID)
	}
	if len(ids) != 3 || ids[0] != 2 || ids[1] != 3 || ids[2] != 4 {
		t.Fatalf("GapSpans ids = %v, want [2 3 4]", ids)
	}
}
