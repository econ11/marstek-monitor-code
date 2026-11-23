package monitor

import (
	"fmt"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// The report is built from the stored rows of the period, with a day of
// margin: an outage that started yesterday counts today only for its part.
func TestMonitorOutagesReport(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	l := t0.Local()
	mid := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
	today := mid.Format(time.DateOnly)
	for _, s := range []struct {
		at  time.Time
		soc float64
	}{{mid.Add(-3 * time.Hour), 80}, {mid.Add(30 * time.Minute), 33}, {mid.Add(5 * time.Hour), 90}} {
		soc := s.soc
		if err := st.InsertSample(model.Sample{StationID: testMac, At: s.at, Source: model.SourceLAN, SOC: &soc, Grid: model.GridPresent}); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []store.Outage{
		{StationID: testMac, Start: mid.Add(-50 * time.Hour), End: tp(mid.Add(-49 * time.Hour))},
		{StationID: testMac, Start: mid.Add(-time.Hour), End: tp(mid.Add(time.Hour))},
	} {
		if _, err := st.OpenOutage(o); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.OpenGap(store.Gap{StationID: testMac, Start: mid.Add(2 * time.Hour), End: tp(mid.Add(3 * time.Hour)),
		Reason: store.GapPCSleep}); err != nil {
		t.Fatal(err)
	}
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}

	r, err := m.OutagesReport(today, today)
	if err != nil {
		t.Fatal(err)
	}
	if r.Count != 1 || r.Off != time.Hour || len(r.Rows) != 1 || r.Rows[0].Bounds.Known != 2*time.Hour {
		t.Fatalf("Count/Off/Rows = %d/%v/%+v, want 1/1h/the outage across midnight", r.Count, r.Off, r.Rows)
	}
	if r.Gaps != time.Hour || r.Observed != t0.Sub(mid)-time.Hour || r.ObservedSince == nil || !r.ObservedSince.Equal(mid.Add(-3*time.Hour)) {
		t.Fatalf("Gaps/Observed/ObservedSince = %v/%v/%v", r.Gaps, r.Observed, r.ObservedSince)
	}
	if len(r.Buckets) != 1 || r.Buckets[0].MinSOC == nil || *r.Buckets[0].MinSOC != 33 {
		t.Fatalf("buckets = %+v, want one day with MinSOC 33", r.Buckets)
	}

	for _, c := range [][2]string{{"bad", today}, {mid.AddDate(0, 0, 1).Format(time.DateOnly), mid.AddDate(0, 0, 1).Format(time.DateOnly)}} {
		if _, err := m.OutagesReport(c[0], c[1]); err == nil {
			t.Fatalf("OutagesReport(%q, %q) = nil error", c[0], c[1])
		}
	}
}

func tp(t time.Time) *time.Time { return &t }

// Row numbers use the whole outage, so gaps are read up to now: a gap more
// than a day after the period still shortens Known and makes the end «≥».
func TestMonitorOutagesReportReadsGapsOfLongOutages(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	l := t0.Local()
	day := time.Date(l.Year(), l.Month(), l.Day()-4, 0, 0, 0, 0, time.Local)
	to := day.AddDate(0, 0, 1) // the period's To
	soc := 70.0
	if err := st.InsertSample(model.Sample{StationID: testMac, At: day.Add(-time.Hour), Source: model.SourceLAN, SOC: &soc, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	start, end := day.Add(10*time.Hour), to.Add(48*time.Hour)
	if _, err := st.OpenOutage(store.Outage{StationID: testMac, Start: start, End: tp(end)}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenGap(store.Gap{StationID: testMac, Start: to.Add(36 * time.Hour), End: tp(end), Reason: store.GapAppNotRunning}); err != nil {
		t.Fatal(err)
	}
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	d := day.Format(time.DateOnly)
	r, err := m.OutagesReport(d, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 {
		t.Fatalf("rows = %+v, want the long outage", r.Rows)
	}
	b := r.Rows[0].Bounds
	if want := end.Sub(start) - 12*time.Hour; b.Known != want || !b.AtLeast || !b.EndApprox || r.Longest != want {
		t.Fatalf("bounds = %+v, Longest %v; want Known %v, EndApprox", b, r.Longest, want)
	}
}

// pagedMonitor stores n lanRestored events (info) at the times at(i), one
// warning apiSilent every 10th.
func pagedMonitor(t *testing.T, n int, at func(i int) time.Time) *Monitor {
	t.Helper()
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	for i := 0; i < n; i++ {
		code, level := journal.CodeLANRestored, "info"
		if i%10 == 0 {
			code, level = journal.CodeAPISilent, "warn"
		}
		if err := st.AddEvent(store.Event{StationID: testMac, At: at(i), Level: level, Code: code, Source: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestJournalPageLimitAndMore(t *testing.T) {
	m := pagedMonitor(t, 450, func(i int) time.Time { return t0.Add(-time.Duration(i+1) * time.Minute) })
	from, to := t0.Add(-24*time.Hour), t0
	var cur *journal.Cursor
	var sizes []int
	var mores []bool
	var all []journal.Entry
	for {
		es, more, counts, err := m.JournalPage(from, to, cur, 200, "all", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if counts != (JournalCounts{All: 450, Warnings: 45, Connection: 450, New: 45}) {
			t.Fatalf("counts = %+v", counts)
		}
		sizes, mores, all = append(sizes, len(es)), append(mores, more), append(all, es...)
		if !more {
			break
		}
		c := journal.CursorOf(es[len(es)-1])
		cur = &c
	}
	if fmt.Sprint(sizes) != "[200 200 50]" || fmt.Sprint(mores) != "[true true false]" {
		t.Fatalf("sizes %v mores %v", sizes, mores)
	}
	seen := map[string]bool{}
	for _, e := range all {
		if seen[e.ID] {
			t.Fatalf("%s twice", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestJournalPageTiesAcrossPages(t *testing.T) {
	same := t0.Add(-time.Hour)
	m := pagedMonitor(t, 5, func(int) time.Time { return same })
	from, to := t0.Add(-24*time.Hour), t0
	var cur *journal.Cursor
	var ids []string
	for {
		es, more, _, err := m.JournalPage(from, to, cur, 2, "all", time.Time{}) // boundaries fall inside the tie
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			ids = append(ids, e.ID)
		}
		if !more {
			break
		}
		c := journal.CursorOf(es[len(es)-1])
		cur = &c
	}
	if len(ids) != 5 {
		t.Fatalf("ids = %v, want 5", ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("%s twice in %v", id, ids)
		}
		seen[id] = true
	}
}

func TestJournalSinceOnlyNewer(t *testing.T) {
	m := pagedMonitor(t, 6, func(i int) time.Time { return t0.Add(-time.Duration(i+1) * time.Minute) })
	from, to := t0.Add(-24*time.Hour), t0
	es, _, _, err := m.JournalPage(from, to, nil, 3, "all", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// The cursor is the newest of the first page; a newer event arrives.
	cur := journal.CursorOf(es[0])
	got, _, err := m.JournalSince(from, to, cur, "all", time.Time{})
	if err != nil || len(got) != 0 {
		t.Fatalf("since newest = %v, %v; want none", got, err)
	}
	if err := m.d.Store.AddEvent(store.Event{StationID: testMac, At: t0.Add(-30 * time.Second), Level: "info", Code: journal.CodeIPChanged}); err != nil {
		t.Fatal(err)
	}
	got, counts, err := m.JournalSince(from, to, cur, "all", time.Time{})
	if err != nil || len(got) != 1 || got[0].Key != "conn.event.ipChanged" {
		t.Fatalf("since = %+v, %v; want the new ipChanged", got, err)
	}
	if counts.All != 7 {
		t.Fatalf("counts.All = %d, want 7 (whole period)", counts.All)
	}
	// Older cursor: everything above it, newest first.
	last := journal.CursorOf(es[2])
	got, _, _ = m.JournalSince(from, to, last, "all", time.Time{})
	if len(got) != 3 || got[0].Key != "conn.event.ipChanged" {
		t.Fatalf("since oldest of page = %d entries", len(got))
	}
}

func TestJournalPageFilterOnServer(t *testing.T) {
	// 2 warnings older than 250 info rows: the warnings filter finds both on page 1.
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	add := func(code, level string, ago int) {
		if err := st.AddEvent(store.Event{StationID: testMac, At: t0.Add(-time.Duration(ago) * time.Minute), Level: level, Code: code, Source: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	add(journal.CodeAPISilent, "warn", 400)
	add(journal.CodeAPISilent, "warn", 401)
	for i := 1; i <= 250; i++ {
		add(journal.CodeLANRestored, "info", i)
	}
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	from, to := t0.Add(-24*time.Hour), t0
	es, more, counts, err := m.JournalPage(from, to, nil, 200, "warnings", time.Time{})
	if err != nil || len(es) != 2 || more {
		t.Fatalf("warnings page = %d entries, more %v, %v", len(es), more, err)
	}
	if counts.All != 252 || counts.Warnings != 2 || counts.New != 2 {
		t.Fatalf("counts = %+v (whole period, New = both warnings, never seen)", counts)
	}
	// New counts against the kept time.
	_, _, counts, _ = m.JournalPage(from, to, nil, 200, "all", t0.Add(-400*time.Minute).Add(-30*time.Second))
	if counts.New != 1 {
		t.Fatalf("New = %d, want 1 (only the newer warning)", counts.New)
	}
	// Since honours the filter.
	got, sc, err := m.JournalSince(from, to, journal.Cursor{At: t0.Add(-500 * time.Minute)}, "warnings", time.Time{})
	if err != nil || len(got) != 2 || sc.All != 252 || sc.Warnings != 2 {
		t.Fatalf("since warnings = %d, %v", len(got), err)
	}
	if _, _, _, err := m.JournalPage(from, to, nil, 10, "bogus", time.Time{}); err == nil {
		t.Fatal("unknown filter must fail")
	}
}
