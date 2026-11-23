package history

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

// t0 = 22:00 UTC; the tests read as wall-clock times from there.
var t0 = time.Date(2026, 11, 21, 22, 0, 0, 0, time.UTC)

func at(h, m, s int) time.Time {
	return time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)
}

func tp(t time.Time) *time.Time { return &t }

func TestAnnotateEndInGap(t *testing.T) {
	end := at(33, 0, 30) // 09:00:30 next day
	o := store.Outage{Start: t0, End: tp(end)}
	gaps := []store.Gap{{Start: at(23, 0, 0), End: tp(at(33, 0, 0)), Reason: store.GapAppNotRunning}}
	b := Annotate(o, gaps, end.Add(time.Hour), end.Add(time.Hour), true)
	if !b.EndApprox || b.NotAfter == nil || !b.NotAfter.Equal(end) {
		t.Fatalf("EndApprox/NotAfter = %v/%v, want true/%v", b.EndApprox, b.NotAfter, end)
	}
	if b.Known != time.Hour+30*time.Second {
		t.Fatalf("Known = %v, want 1h0m30s", b.Known)
	}
	if !b.AtLeast || b.StartApprox || b.Frozen || !b.AsOf.Equal(end) {
		t.Fatalf("bounds = %+v", b)
	}
}

func TestAnnotateStartAfterGap(t *testing.T) {
	o := store.Outage{Start: at(27, 0, 40), End: tp(at(28, 0, 0))}
	gaps := []store.Gap{{Start: at(25, 0, 0), End: tp(at(27, 0, 0)), Reason: store.GapPCSleep}}
	b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true)
	if !b.StartApprox || b.EndApprox || b.NotAfter != nil {
		t.Fatalf("bounds = %+v, want StartApprox only", b)
	}
	if b.Known != 59*time.Minute+20*time.Second || !b.AtLeast {
		t.Fatalf("Known/AtLeast = %v/%v, want 59m20s/true", b.Known, b.AtLeast)
	}
}

// Ledger Task 1 minor: a start that lies inside a gap (the gap began before it
// and ended after it) is approximate too; a start at the gap's own start (the
// last observation before it) is exact.
func TestAnnotateStartInsideGap(t *testing.T) {
	o := store.Outage{Start: at(26, 0, 0), End: tp(at(28, 0, 0))}
	gaps := []store.Gap{{Start: at(25, 0, 0), End: tp(at(27, 0, 0)), Reason: store.GapNoLink}}
	if b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true); !b.StartApprox || !b.AtLeast || b.EndApprox {
		t.Fatalf("bounds = %+v, want StartApprox", b)
	}
	// An open gap ends at now: still around the start.
	open := []store.Gap{{Start: at(25, 0, 0), Reason: store.GapNoLink}}
	if b := Annotate(store.Outage{Start: at(26, 0, 0)}, open, at(27, 0, 0), at(25, 0, 0), false); !b.StartApprox {
		t.Fatalf("bounds = %+v, want StartApprox in an open gap", b)
	}
	gaps[0].Start = o.Start
	if b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true); b.StartApprox {
		t.Fatalf("bounds = %+v, a start at the gap's start is observed", b)
	}
}

// Owner 2026-11-23: a start that lies in a gap names the earliest possible
// start, the gap's start (the last observation before it), so the UI can say
// «між {from} і {to}». Known/AtLeast maths are unchanged.
func TestAnnotateStartFromGapStart(t *testing.T) {
	gaps := []store.Gap{{Start: at(25, 0, 0), End: tp(at(27, 0, 0)), Reason: store.GapAppNotRunning}}
	// Found right after the gap (the owner's case: Partial, first seen at the gap end).
	o := store.Outage{Start: at(27, 0, 40), End: tp(at(28, 0, 0)), Partial: true}
	b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true)
	if b.StartFrom == nil || !b.StartFrom.Equal(at(25, 0, 0)) || !b.StartApprox || !b.AtLeast {
		t.Fatalf("bounds = %+v, want StartFrom 01:00", b)
	}
	if b.Known != 59*time.Minute+20*time.Second {
		t.Fatalf("Known = %v, want 59m20s (unchanged)", b.Known)
	}
	// Strictly inside the gap.
	o = store.Outage{Start: at(26, 0, 0), End: tp(at(28, 0, 0))}
	if b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom == nil || !b.StartFrom.Equal(at(25, 0, 0)) {
		t.Fatalf("bounds = %+v, want StartFrom 01:00 for a start inside the gap", b)
	}
	// Overlapping gaps chain back to the earliest start.
	chained := append([]store.Gap{{Start: at(24, 0, 0), End: tp(at(25, 30, 0)), Reason: store.GapPCSleep}}, gaps...)
	if b := Annotate(o, chained, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom == nil || !b.StartFrom.Equal(at(24, 0, 0)) {
		t.Fatalf("bounds = %+v, want StartFrom 00:00 through the overlapping gaps", b)
	}
	// A gap that only touches it ends at an observation: the window starts there.
	touching := append([]store.Gap{{Start: at(24, 0, 0), End: tp(at(25, 0, 0)), Reason: store.GapAppNotRunning}}, gaps...)
	if b := Annotate(o, touching, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom == nil || !b.StartFrom.Equal(at(25, 0, 0)) {
		t.Fatalf("bounds = %+v, want StartFrom 01:00: a touching gap ends at an observation", b)
	}
	// Two gaps just before the start: the later one, the earlier ended at an observation.
	o2 := store.Outage{Start: at(27, 0, 40), Partial: true}
	two := []store.Gap{{Start: at(26, 0, 0), End: tp(at(26, 59, 0)), Reason: store.GapNoLink}, {Start: at(26, 59, 30), End: tp(at(27, 0, 20)), Reason: store.GapNoLink}}
	if b := Annotate(o2, two, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom == nil || !b.StartFrom.Equal(at(26, 59, 30)) {
		t.Fatalf("bounds = %+v, want StartFrom 02:59:30, the latest gap", b)
	}
	if got := WindowStart(chained, at(25, 0, 0), at(29, 0, 0)); !got.Equal(at(24, 0, 0)) {
		t.Fatalf("WindowStart = %v, want 00:00 through the overlap", got)
	}
	if got := WindowStart(touching, at(25, 0, 0), at(29, 0, 0)); !got.Equal(at(25, 0, 0)) {
		t.Fatalf("WindowStart = %v, want 01:00: touching is not overlapping", got)
	}
	// A start at the gap's own start is exact: no window.
	o = store.Outage{Start: at(25, 0, 0), End: tp(at(28, 0, 0))}
	if b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom != nil || b.StartApprox {
		t.Fatalf("bounds = %+v, want an exact start", b)
	}
	// Ongoing across a later gap (restart while it went on): the start was seen.
	o = store.Outage{Start: at(24, 0, 0)}
	if b := Annotate(o, gaps, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom != nil || b.StartApprox {
		t.Fatalf("bounds = %+v, want an exact start", b)
	}
	// Partial without any gap (first run, grid already off): no window.
	o = store.Outage{Start: at(27, 0, 0), Partial: true}
	if b := Annotate(o, nil, at(29, 0, 0), at(29, 0, 0), true); b.StartFrom != nil || !b.StartApprox {
		t.Fatalf("bounds = %+v, want approx without a window", b)
	}
}

func TestAnnotateExact(t *testing.T) {
	o := store.Outage{Start: at(14, 5, 0), End: tp(at(15, 17, 0))}
	b := Annotate(o, nil, at(16, 0, 0), at(16, 0, 0), true)
	if b.StartApprox || b.EndApprox || b.NotAfter != nil || b.Frozen || b.AtLeast {
		t.Fatalf("bounds = %+v, want all flags false", b)
	}
	if b.Known != time.Hour+12*time.Minute || !b.AsOf.Equal(at(15, 17, 0)) {
		t.Fatalf("Known/AsOf = %v/%v", b.Known, b.AsOf)
	}
}

func TestAnnotateFrozenWhenNotLive(t *testing.T) {
	o := store.Outage{Start: at(14, 5, 0)}
	b := Annotate(o, nil, at(15, 0, 0), at(14, 30, 0), false)
	if !b.Frozen || !b.AsOf.Equal(at(14, 30, 0)) || !b.AtLeast {
		t.Fatalf("bounds = %+v, want Frozen at 14:30, AtLeast", b)
	}
	if b.Known != 25*time.Minute {
		t.Fatalf("Known = %v, want 25m", b.Known)
	}
	// Live: AsOf is now and nothing is frozen.
	b = Annotate(o, nil, at(15, 0, 0), at(14, 59, 55), true)
	if b.Frozen || !b.AsOf.Equal(at(15, 0, 0)) || b.AtLeast || b.Known != 55*time.Minute {
		t.Fatalf("live bounds = %+v", b)
	}
}

func TestAnnotateOpenGapEndsAtNow(t *testing.T) {
	// Ongoing outage, a NO_LINK gap still open: the gap runs to now.
	o := store.Outage{Start: at(14, 0, 0)}
	gaps := []store.Gap{{Start: at(14, 30, 0), Reason: store.GapNoLink}}
	b := Annotate(o, gaps, at(15, 0, 0), at(14, 29, 0), false)
	if !b.Frozen || b.Known != 29*time.Minute || !b.AtLeast {
		t.Fatalf("bounds = %+v, want Frozen, Known 29m", b)
	}
}

func TestOverlapMergesAndClips(t *testing.T) {
	gaps := []store.Gap{
		{Start: at(10, 0, 0), End: tp(at(11, 0, 0))},
		{Start: at(10, 30, 0), End: tp(at(11, 30, 0))}, // overlaps the first
		{Start: at(12, 0, 0)},                          // open: ends at now
	}
	if got := Overlap(gaps, at(10, 15, 0), at(13, 0, 0), at(12, 20, 0)); got != 75*time.Minute+20*time.Minute {
		t.Fatalf("Overlap = %v, want 1h35m", got)
	}
}

func TestOffKnownClipsAtMidnightAndGaps(t *testing.T) {
	outs := []store.Outage{{Start: at(23, 0, 0), End: tp(at(25, 0, 0))}}
	gaps := []store.Gap{{Start: at(23, 30, 0), End: tp(at(24, 30, 0))}}
	off, n, atLeast := OffKnown(outs, gaps, at(24, 0, 0), at(48, 0, 0), at(30, 0, 0), at(30, 0, 0), true)
	if off != 30*time.Minute || n != 1 || !atLeast {
		t.Fatalf("OffKnown = %v, %d, %v; want 30m, 1, true", off, n, atLeast)
	}
}

func TestOffKnownExactDayIsNotAtLeast(t *testing.T) {
	outs := []store.Outage{
		{Start: at(14, 5, 0), End: tp(at(15, 17, 0))},
		{Start: at(23, 50, 0), End: tp(at(24, 10, 0))}, // half in the next day
		{Start: at(25, 0, 0), End: tp(at(26, 0, 0))},   // next day only
	}
	off, n, atLeast := OffKnown(outs, nil, at(0, 0, 0), at(24, 0, 0), at(30, 0, 0), at(30, 0, 0), true)
	if off != time.Hour+22*time.Minute || n != 2 || atLeast {
		t.Fatalf("OffKnown = %v, %d, %v; want 1h22m, 2, false", off, n, atLeast)
	}
}

func TestUnobservedBeforeFirstObservation(t *testing.T) {
	if got := Unobserved(nil, tp(at(9, 0, 0)), at(0, 0, 0), at(24, 0, 0), at(10, 0, 0)); got != 9*time.Hour {
		t.Fatalf("Unobserved = %v, want 9h", got)
	}
}

func TestUnobservedUnionsGapsAndPrefix(t *testing.T) {
	gaps := []store.Gap{
		{Start: at(8, 0, 0), End: tp(at(9, 30, 0))}, // overlaps the prefix by 1 h
		{Start: at(9, 50, 0)},                       // open: ends at now
	}
	if got := Unobserved(gaps, tp(at(9, 0, 0)), at(0, 0, 0), at(24, 0, 0), at(10, 0, 0)); got != 9*time.Hour+40*time.Minute {
		t.Fatalf("Unobserved = %v, want 9h40m", got)
	}
}

// Review fix: an ongoing outage frozen before the window (since yesterday
// 22:00, last measurement 23:00, link not live) still touches today's window
// and makes the figure a lower bound, never an exact 0.
func TestOffKnownCountsFrozenOutageAfterLastMeasurement(t *testing.T) {
	outs := []store.Outage{{Start: at(22, 0, 0)}}
	off, n, atLeast := OffKnown(outs, nil, at(24, 0, 0), at(48, 0, 0), at(34, 0, 0), at(23, 0, 0), false)
	if off != 0 || n != 1 || !atLeast {
		t.Fatalf("OffKnown = %v, %d, %v; want 0, 1, true", off, n, atLeast)
	}
}

// Review fix: no first observation means nothing was observed up to now.
func TestUnobservedWithoutObservationIsWholeWindowUpToNow(t *testing.T) {
	if got := Unobserved(nil, nil, at(0, 0, 0), at(24, 0, 0), at(10, 0, 0)); got != 10*time.Hour {
		t.Fatalf("Unobserved = %v, want 10h", got)
	}
	if got := Unobserved(nil, nil, at(0, 0, 0), at(24, 0, 0), at(30, 0, 0)); got != 24*time.Hour {
		t.Fatalf("past window: Unobserved = %v, want 24h", got)
	}
}
