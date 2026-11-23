package monitor

import (
	"bytes"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"

	_ "modernc.org/sqlite"
)

// Three minutes of polling a station that discharges 300 W: the minutes that
// ended more than energy.MaxStep ago (12:00 and 12:01) are rolled up; a
// restart neither duplicates nor changes them.
func TestMonitorRollsUpMinutes(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.set(60, 300) // ongrid 0, offgrid 300 W: BatteryW −300
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	stop := run(t, m, clk)
	clk.step(t, 180, nil) // polls t0 … t0+3m

	check := func(when string) {
		t.Helper()
		hours, err := st.EnergyHours(testMac, t0, t0.Add(time.Hour))
		if err != nil || len(hours) != 1 {
			t.Fatalf("%s: hours = %+v, %v", when, hours, err)
		}
		if h := hours[0]; h.Covered != 2*time.Minute || math.Abs(h.DisWh-10) > 1e-9 || h.ChgWh != 0 {
			t.Fatalf("%s: hour = %+v, want 2 min covered and 10 Wh discharged", when, h)
		}
		last, ok, err := st.LastEnergyMinute(testMac)
		if err != nil || !ok || !last.Equal(t0.Add(time.Minute)) {
			t.Fatalf("%s: last minute = %v %v %v, want 12:01", when, last, ok, err)
		}
	}
	check("first run")
	day := t0.Local().Format(time.DateOnly)
	r, err := m.EnergyReport(day, day)
	if err != nil || math.Abs(r.DischargedWh-10) > 1e-9 || len(r.Buckets) != 1 {
		t.Fatalf("report = %+v, %v", r, err)
	}
	// Counting started at the first rolled-up minute, and it has caught up.
	if r.EnergySince == nil || !r.EnergySince.Equal(t0) || r.Backfilling {
		t.Fatalf("energy since = %v, backfilling = %v; want 12:00, false", r.EnergySince, r.Backfilling)
	}

	stop()
	m2, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m2, clk)
	clk.step(t, 5, nil)
	check("after restart")
}

// Samples stored before the energy table existed (an upgraded v1 database)
// are rolled up after start, a bounded chunk per tick: start is not blocked.
func TestMonitorBackfillsEnergyInChunks(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	from := t0.Add(-2 * 24 * time.Hour)
	w := 360.0 // charging: 0.5 Wh per 5 s step
	for at := from; at.Before(from.Add(time.Hour)); at = at.Add(5 * time.Second) {
		if err := st.InsertSample(model.Sample{StationID: testMac, At: at, Source: model.SourceLAN, BatteryW: &w}); err != nil {
			t.Fatal(err)
		}
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.LastEnergyMinute(testMac); ok {
		t.Fatal("rolled up in New: start must not wait for the backfill")
	}
	day := t0.Local().Format(time.DateOnly)
	if r, err := m.EnergyReport(day, day); err != nil || !r.Backfilling || r.EnergySince != nil {
		t.Fatalf("before the rollup: backfilling = %v, since = %v, %v; want true, nil", r.Backfilling, r.EnergySince, err)
	}
	run(t, m, clk)
	clk.step(t, 2, nil)
	if r, err := m.EnergyReport(day, day); err != nil || !r.Backfilling {
		t.Fatalf("two days behind: backfilling = %v, %v; want true", r.Backfilling, err)
	}
	clk.step(t, int(2*24*time.Hour/energyChunk), nil)
	if r, err := m.EnergyReport(day, day); err != nil || r.Backfilling || r.EnergySince == nil || !r.EnergySince.Equal(from) {
		t.Fatalf("caught up: backfilling = %v, since = %v, %v; want false, %v", r.Backfilling, r.EnergySince, err, from)
	}
	hours, err := st.EnergyHours(testMac, from, from.Add(time.Hour))
	if err != nil || len(hours) != 1 {
		t.Fatalf("hours = %+v, %v", hours, err)
	}
	// 720 samples, 719 steps (the last one has no successor).
	if h := hours[0]; h.Covered != 719*5*time.Second || math.Abs(h.ChgWh-719*0.5) > 1e-6 {
		t.Fatalf("hour = %+v, want 719 steps", h)
	}
}

// A rollup that keeps failing is not «backfilling»: it makes no progress, so
// the report must not keep promising past days (the tab would poll and say
// «Дораховуємо…» forever). Both a failing read and a failing write.
func TestEnergyBackfillingFalseWhileRollupFails(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"reads fail", `ALTER TABLE outages RENAME TO outages_gone`},
		{"writes fail", `CREATE TRIGGER energy_fail BEFORE INSERT ON energy_minute BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st, err := store.Open(filepath.Join(dir, "monitor.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			fs := startStation(t, testMac)
			seedStation(t, st, fs.Addr.IP.String())
			from := t0.Add(-2 * 24 * time.Hour)
			w := 360.0
			for at := from; at.Before(from.Add(time.Hour)); at = at.Add(5 * time.Second) {
				if err := st.InsertSample(model.Sample{StationID: testMac, At: at, Source: model.SourceLAN, BatteryW: &w}); err != nil {
					t.Fatal(err)
				}
			}
			clk := newClock(t0)
			m, err := New(deps(st, newClient(t), fs.Addr, clk))
			if err != nil {
				t.Fatal(err)
			}
			// Break the store behind the monitor's back, through a second connection.
			db, err := sql.Open("sqlite", filepath.Join(dir, "monitor.db")+"?_pragma=busy_timeout(5000)")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			db.Close()
			run(t, m, clk)
			clk.step(t, 5, nil)
			day := t0.Local().Format(time.DateOnly)
			r, err := m.EnergyReport(day, day)
			if err != nil {
				t.Fatal(err)
			}
			if r.Backfilling {
				t.Fatal("backfilling while every rollup fails: want false")
			}
			if _, ok, _ := st.LastEnergyMinute(testMac); ok {
				t.Fatal("a minute was stored: the store was not broken")
			}
		})
	}
}

// A backfill chunk does its store work without m.mu: the snapshot, the UI
// calls and the poll handling are not blocked behind it.
func TestMonitorRollupDoesNotHoldTheLock(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	w := 360.0
	for at := t0.Add(-time.Hour); at.Before(t0); at = at.Add(5 * time.Second) {
		if err := st.InsertSample(model.Sample{StationID: testMac, At: at, Source: model.SourceLAN, BatteryW: &w}); err != nil {
			t.Fatal(err)
		}
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	var chunks, locked int
	rollupHook = func() {
		chunks++
		if !m.mu.TryLock() {
			locked++
			return
		}
		m.mu.Unlock()
	}
	t.Cleanup(func() { rollupHook = nil })
	stop := run(t, m, clk)
	clk.step(t, 3, nil)
	stop()
	if chunks == 0 || locked != 0 {
		t.Fatalf("chunks %d, with m.mu held %d; want ≥ 1 and 0", chunks, locked)
	}
	if _, ok, _ := st.LastEnergyMinute(testMac); !ok {
		t.Fatal("nothing rolled up")
	}
}

// A store that keeps failing logs one rollup warning per energyWarnEvery,
// not one per tick.
func TestEnergyWarnIsRateLimited(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	m := &Monitor{}
	boom := errors.New("disk I/O error")
	for s := 0; s <= 1200; s++ { // 20 min of ticks
		m.energyWarn(t0.Add(time.Duration(s)*time.Second), "store energy", boom)
	}
	if n := strings.Count(buf.String(), "disk I/O error"); n != 3 {
		t.Fatalf("warnings = %d, want 3 (at 0, 10 and 20 min)", n)
	}
}

// The battery power chart of a day: per minute, from the raw samples, with
// the day's discharge peak; a period over energy.MaxPowerDays is clipped,
// not refused, and never read whole.
func TestMonitorPowerSeries(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.set(60, 300) // BatteryW −300
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m, clk)
	clk.step(t, 180, nil)

	day := t0.Local().Format(time.DateOnly)
	s, err := m.PowerSeries(day, day)
	if err != nil {
		t.Fatal(err)
	}
	if s.Step != time.Minute || len(s.Points) < 3 {
		t.Fatalf("series: step %v, %d points", s.Step, len(s.Points))
	}
	for _, p := range s.Points[:3] {
		if p.W == nil || math.Abs(*p.W+300) > 1e-9 {
			t.Fatalf("point %v = %v, want −300 W", p.At, p.W)
		}
	}
	if s.PeakW == nil || *s.PeakW != 300 || s.PeakAt == nil {
		t.Fatalf("peak = %v at %v, want 300 W", s.PeakW, s.PeakAt)
	}
	from := t0.Local().AddDate(0, 0, -59).Format(time.DateOnly)
	long, err := m.PowerSeries(from, day)
	if err != nil || !long.Clipped || long.Step != time.Hour || len(long.Points) != 1 {
		t.Fatalf("60 days: clipped %v, step %v, %d points, %v", long.Clipped, long.Step, len(long.Points), err)
	}
	// The hour comes from the rollup: the two rolled-up minutes at −300 W.
	if w := long.Points[0].W; w == nil || math.Abs(*w+300) > 1e-6 {
		t.Fatalf("hour = %v, want −300 W", w)
	}
}
