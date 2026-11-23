package monitor

import (
	"bytes"
	"database/sql"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// insertDischarge stores a run sampled every 20 s (Calm polling; fewer synced
// inserts) from `at`: SOC 90 → 30 at −1000 W for 3 h 4 min 19 s (3072 Wh over
// 60 points: 5120 Wh), the SOC read in whole percent as the station does
// (measured tick to tick: 89 → 30, 59 steps).
func insertDischarge(t *testing.T, st *store.Store, at time.Time) {
	t.Helper()
	d := 3*time.Hour + 4*time.Minute + 19*time.Second
	w := -1000.0
	for i := 0; ; i++ {
		ts := at.Add(time.Duration(i) * 20 * time.Second)
		if ts.After(at.Add(d)) {
			ts = at.Add(d)
		}
		soc := math.Round(90 - 60*float64(ts.Sub(at))/float64(d))
		if err := st.InsertSample(model.Sample{StationID: testMac, At: ts, Source: model.SourceLAN, SOC: &soc, BatteryW: &w}); err != nil {
			t.Fatal(err)
		}
		if ts.Equal(at.Add(d)) {
			return
		}
	}
}

// A deep discharge 29 days ago is estimated at start. Two days later the
// daily pass runs before Compact deletes those raw samples: the estimate,
// the largest ΔSOC and the overview numbers all stay, and a restart (a new
// pass over what is left) keeps them too.
func TestMonitorEstimatesSurviveCompaction(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	from := t0.Add(-29 * 24 * time.Hour)
	insertDischarge(t, st, from)
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	if b := m.Latest().Battery; b.HealthPct != nil {
		t.Fatalf("estimated in New: %+v; start must not wait for it", b)
	}
	stop := run(t, m, clk)

	check := func(when string, m *Monitor) {
		t.Helper()
		s, err := m.Battery()
		if err != nil {
			t.Fatalf("%s: %v", when, err)
		}
		if s.ActualWh == nil || math.Abs(*s.ActualWh-5120) > 5 || s.HealthPct == nil || math.Abs(*s.HealthPct-100) > 0.1 || s.MaxDelta != 59 {
			t.Fatalf("%s: summary = %+v, want 5120 Wh ≈ 100 %%, max 59 points", when, s)
		}
		if !s.Approx || len(s.Months) != 1 || s.CycleLife != 6000 || s.NominalWh != 5120 {
			t.Fatalf("%s: summary = %+v", when, s)
		}
		if b := m.Latest().Battery; b.HealthPct == nil || math.Abs(*b.HealthPct-*s.HealthPct) > 1e-9 || b.HealthApprox != s.Approx {
			t.Fatalf("%s: snapshot battery = %+v, want %v %%", when, b, *s.HealthPct)
		}
		if v, ok, err := st.GetSetting(SettingMaxDelta); err != nil || !ok || v != "59" {
			t.Fatalf("%s: %s = %q %v %v", when, SettingMaxDelta, v, ok, err)
		}
	}
	check("at start", m)

	// The daily pass: the loop wakes two days later.
	clk.Advance(48 * time.Hour)
	clk.waitIdle(t)
	if left, err := st.Samples(testMac, from, from.Add(4*time.Hour)); err != nil || len(left) != 0 {
		t.Fatalf("raw samples of 31 days ago: %d, %v; want compacted", len(left), err)
	}
	check("after compaction", m)

	stop()
	m2, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m2, clk)
	check("after restart", m2)
}

// The cycles come from the discharged energy of all stored minutes, one per
// 90 % DoD equivalent (4608 Wh for Venus E 3.0), and the snapshot carries
// them (the overview rounds them to whole cycles).
func TestMonitorBatteryCycles(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	if err := st.UpsertEnergyMinutes(testMac, []store.EnergyMinute{
		{At: t0.Add(-48 * time.Hour), DisWh: 9216, Covered: time.Minute}}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Battery()
	if err != nil || s.Cycles == nil || math.Abs(*s.Cycles-2) > 1e-9 || s.HealthPct != nil || s.MaxDelta != 0 {
		t.Fatalf("summary = %+v, %v; want 2 cycles, no health yet", s, err)
	}
	if b := m.Latest().Battery; b.Cycles == nil || math.Abs(*b.Cycles-2) > 1e-9 || b.HealthPct != nil {
		t.Fatalf("snapshot battery = %+v, want 2 cycles", b)
	}
}

// After Compact has deleted the head of a stored run, a pass finds only its
// tail, which starts elsewhere: it must not count as a second estimate. The
// same run found again, longer, replaces the stored one (same start).
func TestOverlapsOther(t *testing.T) {
	stored := []store.CapacityEstimate{{Start: t0, End: t0.Add(3 * time.Hour), Wh: 5100, DeltaSOC: 60}}
	tail := store.CapacityEstimate{Start: t0.Add(time.Hour), End: t0.Add(3 * time.Hour), Wh: 5000, DeltaSOC: 50}
	longer := store.CapacityEstimate{Start: t0, End: t0.Add(4 * time.Hour), Wh: 5050, DeltaSOC: 70}
	later := store.CapacityEstimate{Start: t0.Add(4 * time.Hour), End: t0.Add(7 * time.Hour), Wh: 5050, DeltaSOC: 70}
	if !overlapsOther(stored, tail) || overlapsOther(stored, longer) || overlapsOther(stored, later) {
		t.Fatal("want: tail dropped, longer and later kept")
	}
}

// A broken estimates table does not stop the monitor: it keeps polling, the
// snapshot keeps no health, and the failure is logged once per batteryEvery
// (plus once in New and once at the start pass), not every tick.
func TestBatteryStoreFailureIsLoggedRarely(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	db, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE capacity_estimates RENAME TO capacity_gone`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m, clk)
	clk.step(t, 30, nil)
	for i := 0; i < 2; i++ {
		clk.Advance(batteryEvery)
		clk.waitIdle(t)
	}
	if _, err := m.Battery(); err == nil {
		t.Fatal("Battery() without the table: want an error")
	}
	if s := m.Latest(); s.Battery.HealthPct != nil || s.Last == nil {
		t.Fatalf("snapshot = battery %+v, last %v; want polling on, no health", s.Battery, s.Last)
	}
	if n := strings.Count(buf.String(), "no such table: capacity_estimates"); n != 4 {
		t.Fatalf("warnings = %d, want 4:\n%s", n, buf.String())
	}
}
