package energy

import (
	"math"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Kyiv on any machine

	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var t0 = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

func f(v float64) *float64 { return &v }

func tp(t time.Time) *time.Time { return &t }

// smp is a sample at t0+sec with battery power w (+ charge, − discharge) and PV pv.
func smp(sec float64, w *float64, pv *float64) model.Sample {
	return model.Sample{StationID: "st", At: t0.Add(time.Duration(sec * float64(time.Second))), BatteryW: w, PVW: pv}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func sumMinutes(ms []store.EnergyMinute) (m store.EnergyMinute) {
	for _, x := range ms {
		m.ChgWh += x.ChgWh
		m.DisWh += x.DisWh
		m.DisOutageWh += x.DisOutageWh
		m.PVChgWh += x.PVChgWh
		m.Covered += x.Covered
	}
	return m
}

func TestRollupLeftRectangles(t *testing.T) {
	ss := []model.Sample{smp(0, f(-400), nil), smp(5, f(-400), nil), smp(10, f(-400), nil)}
	got := Rollup(ss, nil, t0, t0.Add(time.Minute))
	if len(got) != 1 || !got[0].At.Equal(t0) {
		t.Fatalf("minutes = %+v, want one at %v", got, t0)
	}
	m := got[0]
	if !near(m.DisWh, 400.0*10/3600) || m.ChgWh != 0 || m.Covered != 10*time.Second {
		t.Fatalf("DisWh/ChgWh/Covered = %v/%v/%v, want 1.111/0/10s", m.DisWh, m.ChgWh, m.Covered)
	}
	if m.PeakDisW == nil || *m.PeakDisW != 400 || m.PeakAt == nil || !m.PeakAt.Equal(t0) {
		t.Fatalf("peak = %v at %v, want 400 W at t0", m.PeakDisW, m.PeakAt)
	}
}

func TestRollupSkipsHoles(t *testing.T) {
	// 90 s: a hole, nothing integrated or covered.
	hole := Rollup([]model.Sample{smp(0, f(-400), nil), smp(90, f(-400), nil)}, nil, t0, t0.Add(2*time.Minute))
	if s := sumMinutes(hole); s.DisWh != 0 || s.Covered != 0 {
		t.Fatalf("hole: DisWh/Covered = %v/%v, want 0/0", s.DisWh, s.Covered)
	}
	// 20 s (Calm polling): integrated.
	calm := Rollup([]model.Sample{smp(0, f(360), nil), smp(20, f(360), nil)}, nil, t0, t0.Add(time.Minute))
	if s := sumMinutes(calm); !near(s.ChgWh, 2) || s.Covered != 20*time.Second {
		t.Fatalf("calm: ChgWh/Covered = %v/%v, want 2/20s", s.ChgWh, s.Covered)
	}
	// Exactly MaxStep is still a step; unknown power contributes nothing.
	edge := Rollup([]model.Sample{smp(0, f(360), nil), smp(60, nil, nil), smp(70, f(360), nil)}, nil, t0, t0.Add(2*time.Minute))
	if s := sumMinutes(edge); !near(s.ChgWh, 6) || s.Covered != 60*time.Second {
		t.Fatalf("edge: ChgWh/Covered = %v/%v, want 6/60s (the nil step is not covered)", s.ChgWh, s.Covered)
	}
}

func TestRollupDeadband(t *testing.T) {
	got := Rollup([]model.Sample{smp(0, f(-15), nil), smp(5, f(15), nil), smp(10, f(0), nil)}, nil, t0, t0.Add(time.Minute))
	s := sumMinutes(got)
	if s.DisWh != 0 || s.ChgWh != 0 || s.Covered != 10*time.Second {
		t.Fatalf("DisWh/ChgWh/Covered = %v/%v/%v, want 0/0/10s", s.DisWh, s.ChgWh, s.Covered)
	}
	if len(got) != 1 || got[0].PeakDisW != nil {
		t.Fatalf("minutes = %+v, want one without a peak", got)
	}
}

func TestRollupOutageAndPV(t *testing.T) {
	outs := []store.Outage{{Start: t0.Add(10 * time.Second)}} // open
	ss := []model.Sample{smp(0, f(-300), nil), smp(10, f(-300), nil), smp(20, f(-300), nil)}
	s := sumMinutes(Rollup(ss, outs, t0, t0.Add(time.Minute)))
	if !near(s.DisWh, 300.0*20/3600) || !near(s.DisOutageWh, 300.0*10/3600) {
		t.Fatalf("DisWh/DisOutageWh = %v/%v, want 1.667/0.833", s.DisWh, s.DisOutageWh)
	}
	in := sumMinutes(Rollup(ss[1:], outs, t0, t0.Add(time.Minute)))
	if in.DisOutageWh != in.DisWh || in.DisWh == 0 {
		t.Fatalf("all in outage: DisOutageWh %v != DisWh %v", in.DisOutageWh, in.DisWh)
	}
	closed := []store.Outage{{Start: t0, End: tp(t0.Add(10 * time.Second))}}
	if c := sumMinutes(Rollup(ss, closed, t0, t0.Add(time.Minute))); !near(c.DisOutageWh, 300.0*10/3600) {
		t.Fatalf("closed outage: DisOutageWh = %v, want 0.833 (end excluded)", c.DisOutageWh)
	}

	pv := sumMinutes(Rollup([]model.Sample{smp(0, f(1000), f(300)), smp(5, f(1000), f(300))}, nil, t0, t0.Add(time.Minute)))
	if pv.ChgWh == 0 || !near(pv.PVChgWh/pv.ChgWh, 0.3) {
		t.Fatalf("PVChgWh/ChgWh = %v/%v, want ratio 0.3", pv.PVChgWh, pv.ChgWh)
	}
	small := sumMinutes(Rollup([]model.Sample{smp(0, f(100), f(300)), smp(5, f(100), f(300))}, nil, t0, t0.Add(time.Minute)))
	if small.PVChgWh != small.ChgWh {
		t.Fatalf("PV above the charge: PVChgWh %v, want = ChgWh %v", small.PVChgWh, small.ChgWh)
	}
}

func TestRollupStepBelongsToItsStartMinute(t *testing.T) {
	ss := []model.Sample{smp(57, f(-720), nil), smp(62, f(-720), nil)} // 10:00:57 → 10:01:02
	got := Rollup(ss, nil, t0, t0.Add(2*time.Minute))
	if len(got) != 1 || !got[0].At.Equal(t0) || got[0].Covered != 5*time.Second || !near(got[0].DisWh, 1) {
		t.Fatalf("minutes = %+v, want only 10:00 with 5 s and 1 Wh", got)
	}
	// Only whole minutes of [from, to): the step is outside [10:01, 10:02).
	if got := Rollup(ss, nil, t0.Add(time.Minute), t0.Add(2*time.Minute)); len(got) != 0 {
		t.Fatalf("minutes = %+v, want none", got)
	}
	// A step without a successor is not integrated.
	if got := Rollup(ss[:1], nil, t0, t0.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("lone sample: %+v, want none", got)
	}
}

// kyiv sets time.Local to Europe/Kyiv for the test.
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

func lt(y int, mo time.Month, d, h int) time.Time { return time.Date(y, mo, d, h, 0, 0, 0, time.Local) }

func period(t *testing.T, from, to string, now time.Time) history.Period {
	t.Helper()
	p, err := history.PeriodFor(from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func hour(at time.Time, chg, dis float64) store.EnergyMinute {
	return store.EnergyMinute{At: at, ChgWh: chg, DisWh: dis, Covered: time.Hour}
}

func TestEnergyReportEfficiencyNeedsHalfKWh(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 22, 12)
	since := lt(2026, 11, 22, 0)
	p := period(t, "2026-11-22", "2026-11-22", now)
	r := BuildReport([]store.EnergyMinute{hour(lt(2026, 11, 22, 1), 400, 300)}, &since, p, now)
	if r.Efficiency != nil || r.ChargedWh != 400 || r.DischargedWh != 300 {
		t.Fatalf("Efficiency/Charged/Discharged = %v/%v/%v, want nil/400/300", r.Efficiency, r.ChargedWh, r.DischargedWh)
	}
	r = BuildReport([]store.EnergyMinute{hour(lt(2026, 11, 22, 1), 5000, 0), hour(lt(2026, 11, 22, 2), 0, 4500)}, &since, p, now)
	if r.Efficiency == nil || !near(*r.Efficiency, 90) {
		t.Fatalf("Efficiency = %v, want 90", r.Efficiency)
	}
	if !near(r.Coverage, 2.0/12) {
		t.Fatalf("Coverage = %v, want 2/12 (2 h covered of 12 h observed)", r.Coverage)
	}
}

func TestEnergyReportDaysAcrossDST(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 1, 12)
	since := lt(2026, 10, 1, 0)
	p := period(t, "2026-10-24", "2026-10-26", now)
	peak := 900.0
	hours := []store.EnergyMinute{
		hour(lt(2026, 10, 24, 23), 100, 0), // last hour of the 24th
		{At: lt(2026, 10, 25, 0), DisWh: 50, Covered: time.Hour, PeakDisW: &peak, PeakAt: tp(lt(2026, 10, 25, 0).Add(7 * time.Minute))},
		hour(lt(2026, 10, 26, 0), 0, 20), // first hour of the 26th
	}
	// 25.10 is 25 h long: its last hour starts at 23:00 local = 21:00 UTC.
	hours = append(hours, hour(lt(2026, 10, 26, 0).Add(-time.Hour), 7, 0))
	r := BuildReport(hours, &since, p, now)
	if len(r.Buckets) != 3 {
		t.Fatalf("buckets = %d, want 3", len(r.Buckets))
	}
	spans := []time.Duration{24 * time.Hour, 25 * time.Hour, 24 * time.Hour}
	chg := []float64{100, 7, 0}
	dis := []float64{0, 50, 20}
	for i, b := range r.Buckets {
		l := b.Start.In(time.Local)
		if l.Hour() != 0 || l.Day() != 24+i || b.Span != spans[i] || b.End.Sub(b.Start) != spans[i] {
			t.Fatalf("bucket %d = %v..%v span %v, want local midnight of %d.10 and %v", i, b.Start, b.End, b.Span, 24+i, spans[i])
		}
		if b.ChargedWh != chg[i] || b.DischargedWh != dis[i] || b.NoData {
			t.Fatalf("bucket %d = %+v, want charged %v discharged %v", i, b, chg[i], dis[i])
		}
	}
	if b := r.Buckets[1]; b.PeakW == nil || *b.PeakW != 900 || b.PeakAt == nil || !b.PeakAt.Equal(lt(2026, 10, 25, 0).Add(7*time.Minute)) {
		t.Fatalf("peak = %v at %v", b.PeakW, b.PeakAt)
	}
}

func TestEnergyReportNoDataDay(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 22, 12)
	since := lt(2026, 11, 20, 12)
	p := period(t, "2026-11-19", "2026-11-22", now)
	r := BuildReport([]store.EnergyMinute{hour(lt(2026, 11, 20, 13), 10, 0), hour(lt(2026, 11, 22, 3), 0, 5)}, &since, p, now)
	if len(r.Buckets) != 4 {
		t.Fatalf("buckets = %d, want 4", len(r.Buckets))
	}
	want := []bool{false, false, true, false} // 19th before ObservedSince, 21st uncovered
	for i, b := range r.Buckets {
		if b.NoData != want[i] {
			t.Fatalf("bucket %d NoData = %v, want %v", i, b.NoData, want[i])
		}
	}
	if r.Buckets[3].Span != 12*time.Hour {
		t.Fatalf("today's span = %v, want 12h (clipped to now)", r.Buckets[3].Span)
	}
	if r.HasPV {
		t.Fatal("HasPV without PV charge")
	}
	r = BuildReport([]store.EnergyMinute{{At: lt(2026, 11, 22, 3), ChgWh: 5, PVChgWh: 1, Covered: time.Minute}}, &since, p, now)
	if !r.HasPV {
		t.Fatal("HasPV = false with PV charge")
	}
}
