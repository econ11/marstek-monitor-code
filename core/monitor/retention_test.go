package monitor

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

// seedRetention stores an event and an energy minute (3000 Wh) 400 days
// before t0, and recent ones (2000 Wh).
func seedRetention(t *testing.T, st *store.Store) (old, recent time.Time) {
	t.Helper()
	old, recent = t0.Add(-400*24*time.Hour), t0.Add(-24*time.Hour)
	for i, at := range []time.Time{old, recent} {
		if err := st.AddEvent(store.Event{StationID: testMac, At: at, Level: "warn", Code: "conn.lanLost", Text: "{}"}); err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertEnergyMinutes(testMac, []store.EnergyMinute{{At: at, DisWh: float64(3000 - 1000*i), Covered: time.Minute}}); err != nil {
			t.Fatal(err)
		}
	}
	return old, recent
}

func eventsSince(t *testing.T, st *store.Store) []store.Event {
	t.Helper()
	es, err := st.Events(testMac, "conn.lanLost", time.UnixMilli(0), t0.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func checkCycles(t *testing.T, when string, m *Monitor) {
	t.Helper()
	s, err := m.Battery()
	if err != nil || s.Cycles == nil || math.Abs(*s.Cycles-5000/4608.0) > 1e-9 {
		t.Fatalf("%s: summary = %+v, %v; want 5000 Wh of cycles", when, s, err)
	}
}

// With a retention of 365 days the start compaction prunes what is older
// (the lifetime cycles stay); the daily one prunes again as time moves on.
func TestMonitorCompactAppliesRetention(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	_, recent := seedRetention(t, st)
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	d.RetentionDays = 365
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	checkCycles(t, "before start", m)
	run(t, m, clk)
	if es := eventsSince(t, st); len(es) != 1 || !es[0].At.Equal(recent) {
		t.Fatalf("events after start = %+v; want the recent one", es)
	}
	checkCycles(t, "after start", m)

	// The bound moves with time: an event 364 days old goes at the daily
	// pass two days later.
	edge := t0.Add(-364 * 24 * time.Hour)
	if err := st.AddEvent(store.Event{StationID: testMac, At: edge, Level: "warn", Code: "conn.lanLost", Text: "{}"}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(48 * time.Hour)
	clk.waitIdle(t)
	if es := eventsSince(t, st); len(es) != 1 || !es[0].At.Equal(recent) {
		t.Fatalf("events after the daily pass = %+v; want the recent one", es)
	}
	checkCycles(t, "after the daily pass", m)
}

// Retention 0 keeps everything; PruneNow after Configure shortens it at once.
func TestMonitorPruneNow(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	seedRetention(t, st)
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m, clk)
	if es := eventsSince(t, st); len(es) != 2 {
		t.Fatalf("events with retention 0 = %+v; want both", es)
	}
	if err := m.PruneNow(); err != nil {
		t.Fatal(err)
	}
	if es := eventsSince(t, st); len(es) != 2 {
		t.Fatalf("PruneNow with retention 0 pruned: %+v", es)
	}
	m.Configure(Config{RetentionDays: 365})
	if err := m.PruneNow(); err != nil {
		t.Fatal(err)
	}
	if es := eventsSince(t, st); len(es) != 1 {
		t.Fatalf("events after PruneNow = %+v; want the recent one", es)
	}
	checkCycles(t, "after PruneNow", m)
}

// A corrupt lifetime counter hides only the cycles: the Battery tab still
// loads, and Prune deletes nothing (fail-closed).
func TestBatteryCorruptCounterHidesCycles(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	seedRetention(t, st)
	if err := st.SetSetting(store.SettingDischargedPrunedWh, "{broken"); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	d.RetentionDays = 365
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Battery()
	if err != nil || s.Cycles != nil || s.NominalWh != 5120 {
		t.Fatalf("summary = %+v, %v; want the tab without cycles", s, err)
	}
	if err := m.PruneNow(); err == nil {
		t.Fatal("PruneNow with a corrupt counter: no error")
	}
	if es := eventsSince(t, st); len(es) != 2 {
		t.Fatalf("events = %+v; want nothing pruned", es)
	}
}

// The retention bound is a local midnight N local days back, across DST.
func TestRetentionBoundLocalMidnight(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 30, 0, 0, time.Local)
	if got, want := RetentionBound(now, 365), time.Date(2025, 11, 22, 0, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("bound = %v, want %v", got, want)
	}
	if got, want := RetentionBound(time.Date(2026, 4, 1, 0, 30, 0, 0, time.Local), 365), time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("bound = %v, want %v", got, want)
	}
}

// Task 20c review minor 1: PruneNow from the UI while the loop polls, rolls
// up energy and runs its daily maintenance (with a retention set), and while
// energy minutes are written: under -race, and the lifetime discharged total
// is exact afterwards (a pruned minute moves into the counter once, never
// lost, never counted twice).
func TestPruneNowConcurrentWithMaintenanceKeepsTotal(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	want := 0.0
	var old []store.EnergyMinute
	for i := 0; i < 50; i++ { // older than any retention below
		old = append(old, store.EnergyMinute{At: t0.Add(-400*24*time.Hour + time.Duration(i)*time.Minute), DisWh: 10, Covered: time.Minute})
		want += 10
	}
	if err := st.UpsertEnergyMinutes(testMac, old); err != nil {
		t.Fatal(err)
	}
	// The newest stored minute: the rollup resumes after it, so the minutes
	// written below (older) are never rolled up again.
	if err := st.UpsertEnergyMinutes(testMac, []store.EnergyMinute{{At: t0.Add(-24 * time.Hour), DisWh: 2000, Covered: time.Minute}}); err != nil {
		t.Fatal(err)
	}
	want += 2000
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk)) // retention 0: the start pass prunes nothing
	if err != nil {
		t.Fatal(err)
	}
	run(t, m, clk)
	m.Configure(Config{RetentionDays: 365})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // the user confirms a shorter retention, again and again
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if err := m.PruneNow(); err != nil {
				t.Errorf("PruneNow: %v", err)
				return
			}
		}
	}()
	go func() { // recent energy written meanwhile (kept by the retention)
		defer wg.Done()
		for i := 0; i < 20; i++ {
			at := t0.Add(-48*time.Hour - time.Duration(i)*time.Minute)
			if err := st.UpsertEnergyMinutes(testMac, []store.EnergyMinute{{At: at, DisWh: 5, Covered: time.Minute}}); err != nil {
				t.Errorf("energy minute: %v", err)
				return
			}
		}
	}()
	want += 20 * 5
	for i := 0; i < 10; i++ { // polls and rollups
		clk.Advance(time.Second)
		clk.waitIdle(t)
	}
	clk.Advance(25 * time.Hour) // the daily maintenance (prune with 365 days) is due
	clk.waitIdle(t)
	wg.Wait()

	got, err := st.TotalDischargedWh(testMac)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("lifetime discharged = %v Wh, want %v", got, want)
	}
	first, ok, err := st.FirstEnergyMinute(testMac)
	if err != nil || !ok || first.Before(t0.Add(-300*24*time.Hour)) {
		t.Fatalf("oldest energy minute = %v, %v, %v; want none older than the retention", first, ok, err)
	}
}
