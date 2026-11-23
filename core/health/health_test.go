package health

import (
	"math"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var t0 = time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)

func f(v float64) *float64 { return &v }

// runEvery appends samples every `every` from `at` for d (plus one at at+d),
// the SOC going linearly from soc0 to soc1 and read in whole percent as the
// station reports it, at a constant power w.
func runEvery(out []model.Sample, at time.Time, d, every time.Duration, soc0, soc1, w float64) []model.Sample {
	steps := int(d / every)
	for i := 0; i <= steps; i++ {
		t := at.Add(time.Duration(i) * every)
		if i == steps {
			t = at.Add(d)
		}
		soc := soc0 + (soc1-soc0)*float64(t.Sub(at))/float64(d)
		out = append(out, model.Sample{At: t, SOC: f(math.Round(soc)), BatteryW: f(w)})
	}
	return out
}

func run(out []model.Sample, at time.Time, d time.Duration, soc0, soc1, w float64) []model.Sample {
	return runEvery(out, at, d, 5*time.Second, soc0, soc1, w)
}

// deep is 90 % → 30 % at exactly −1000 W for 3 h 4 min 19 s: 3072 Wh over 60
// points, a 5120 Wh battery.
const deep = 3*time.Hour + 4*time.Minute + 19*time.Second

// Measured tick to tick: from the first reading of 89 to the first of 30,
// 59 whole steps, 5120 Wh.
func TestFindDischargesDeepRun(t *testing.T) {
	es, maxDelta := FindDischarges(run(nil, t0, deep, 90, 30, -1000))
	if len(es) != 1 {
		t.Fatalf("estimates = %+v, want one", es)
	}
	e := es[0]
	if math.Abs(e.Wh-5120) > 5 || e.DeltaSOC != 59 || maxDelta != 59 {
		t.Fatalf("estimate = %+v, maxDelta %v; want 5120 Wh over 59 steps", e, maxDelta)
	}
	if e.Start.Before(t0) || e.Start.After(t0.Add(2*time.Minute)) || e.End.After(t0.Add(deep)) || e.End.Before(t0.Add(deep-2*time.Minute)) {
		t.Fatalf("span = %v … %v", e.Start, e.End)
	}
}

// Review fix: 30 days resting at 90 % with a −5 W idle offset, the deep run,
// then 2 days at 30 % at −5 W. The plateaus add neither energy (3.6 kWh
// would make it ≈ 11 kWh) nor time (Start a month early, wrong month).
func TestFindDischargesIgnoresPlateaus(t *testing.T) {
	from := t0.Add(-30 * 24 * time.Hour)
	s := runEvery(nil, from, 30*24*time.Hour, 20*time.Second, 90, 90, -5)
	s = run(s, t0.Add(20*time.Second), deep, 90, 30, -1000)
	end := t0.Add(20*time.Second + deep)
	s = runEvery(s, end.Add(20*time.Second), 48*time.Hour, 20*time.Second, 30, 30, -5)
	es, _ := FindDischarges(s)
	if len(es) != 1 || math.Abs(es[0].Wh-5120) > 5 {
		t.Fatalf("estimates = %+v, want one of 5120 Wh", es)
	}
	if e := es[0]; e.Start.Before(t0) || e.Start.After(t0.Add(3*time.Minute)) || e.End.After(end) {
		t.Fatalf("span = %v … %v, want inside the discharge (%v … %v)", e.Start, e.End, t0, end)
	}
}

// A hole longer than one missed poll (150 s), a charging sample, a rise of
// 2 points or an unknown power splits the run into two parts under 50 steps:
// no estimate, the larger part (89 → 55, 34 steps) is maxDelta.
func TestFindDischargesBreaksOnHoleAndCharge(t *testing.T) {
	first := run(nil, t0, 2*time.Hour, 90, 55, -1000)
	hole := run(first, t0.Add(2*time.Hour+150*time.Second), time.Hour, 55, 35, -1000)
	if es, maxDelta := FindDischarges(hole); len(es) != 0 || maxDelta != 34 {
		t.Fatalf("hole: estimates %+v, maxDelta %v; want none, 34", es, maxDelta)
	}

	charge := run(nil, t0, 2*time.Hour, 90, 55, -1000)
	charge = append(charge, model.Sample{At: t0.Add(2*time.Hour + 5*time.Second), SOC: f(55), BatteryW: f(300)})
	charge = run(charge, t0.Add(2*time.Hour+10*time.Second), time.Hour, 55, 35, -1000)
	if es, maxDelta := FindDischarges(charge); len(es) != 0 || maxDelta != 34 {
		t.Fatalf("charge: estimates %+v, maxDelta %v; want none, 34", es, maxDelta)
	}

	rise := run(nil, t0, 2*time.Hour, 90, 55, -1000)
	rise = run(rise, t0.Add(2*time.Hour+5*time.Second), time.Hour, 57, 35, -1000)
	if es, maxDelta := FindDischarges(rise); len(es) != 0 || maxDelta != 34 {
		t.Fatalf("rise: estimates %+v, maxDelta %v; want none, 34", es, maxDelta)
	}
	unknown := run(nil, t0, 2*time.Hour, 90, 55, -1000)
	unknown = append(unknown, model.Sample{At: t0.Add(2*time.Hour + 5*time.Second), SOC: f(55)})
	unknown = run(unknown, t0.Add(2*time.Hour+10*time.Second), time.Hour, 55, 35, -1000)
	if es, maxDelta := FindDischarges(unknown); len(es) != 0 || maxDelta != 34 {
		t.Fatalf("unknown power: estimates %+v, maxDelta %v; want none, 34", es, maxDelta)
	}
}

// Real data (controller ruling): one missed poll (a 90 s step) and a 1-point
// BMS bounce (55 → 56 → 55) inside the run keep it whole.
func TestFindDischargesToleratesMissedPollAndBounce(t *testing.T) {
	all := run(nil, t0, deep, 90, 30, -1000)
	var missed []model.Sample
	hole := t0.Add(time.Hour)
	for _, s := range all {
		if s.At.After(hole) && s.At.Before(hole.Add(90*time.Second)) {
			continue
		}
		missed = append(missed, s)
	}
	if es, _ := FindDischarges(missed); len(es) != 1 || math.Abs(es[0].Wh-5120) > 5 || es[0].DeltaSOC != 59 {
		t.Fatalf("missed poll: estimates %+v, want one of 5120 Wh", es)
	}

	bounced := append([]model.Sample(nil), all...)
	for i := range bounced {
		if at := bounced[i].At; !at.Before(t0.Add(2*time.Hour)) && at.Before(t0.Add(2*time.Hour+time.Minute)) {
			bounced[i].SOC = f(*bounced[i].SOC + 1)
		}
	}
	if es, _ := FindDischarges(bounced); len(es) != 1 || math.Abs(es[0].Wh-5120) > 5 || es[0].DeltaSOC != 59 {
		t.Fatalf("bounce: estimates %+v, want one of 5120 Wh", es)
	}
}

// Idle inside the run (|P| ≤ 20 W, 50 min: under maxStall) keeps it whole.
// Idle discharge counts as energy, idle "charge" (+15 W, noise) does not:
// compared with the same run idling at 0 W, −15 W adds 15 × 50/60 Wh (and
// its 5 s join) over 59 steps, +15 W adds nothing. 90 → 70 at −2000 W in 1 h
// and 70 → 30 at −6000 W in 40 min: 100 Wh per point.
func TestFindDischargesAllowsIdleInside(t *testing.T) {
	build := func(idleW float64) store.CapacityEstimate {
		t.Helper()
		s := run(nil, t0, time.Hour, 90, 70, -2000)
		s = run(s, t0.Add(time.Hour+5*time.Second), 50*time.Minute, 70, 70, idleW)
		s = run(s, t0.Add(time.Hour+50*time.Minute+10*time.Second), 40*time.Minute, 70, 30, -6000)
		es, maxDelta := FindDischarges(s)
		if len(es) != 1 || maxDelta != 59 || es[0].DeltaSOC != 59 {
			t.Fatalf("idle %v W: estimates %+v, maxDelta %v; want one over 59 steps", idleW, es, maxDelta)
		}
		return es[0]
	}
	base := build(0)
	if math.Abs(base.Wh-10000) > 25 {
		t.Fatalf("base = %+v, want ≈ 10000 Wh", base)
	}
	if d := build(15).Wh - base.Wh; math.Abs(d) > 1e-6 {
		t.Fatalf("+15 W idle added %v Wh, want 0", d)
	}
	want := 15 * (50*60 + 5) / 3600.0 / 0.59
	if d := build(-15).Wh - base.Wh; math.Abs(d-want) > 1e-6 {
		t.Fatalf("−15 W idle added %v Wh, want %v", d, want)
	}
}

func est(at time.Time, wh float64) store.CapacityEstimate {
	return store.CapacityEstimate{Start: at, End: at.Add(3 * time.Hour), Wh: wh, DeltaSOC: 60}
}

func TestMonthlyMedianAndApprox(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })

	oct := time.Date(2026, 10, 5, 12, 0, 0, 0, loc)
	nov := time.Date(2026, 11, 3, 12, 0, 0, 0, loc)
	// 31.10 23:30 UTC is already 1 November in Kyiv.
	edge := time.Date(2026, 10, 31, 23, 30, 0, 0, time.UTC)
	ms := Monthly([]store.CapacityEstimate{est(nov, 5000), est(oct, 5100), est(edge, 4900), est(oct.AddDate(0, 0, 3), 5050), est(oct.AddDate(0, 0, 1), 4800)})
	if len(ms) != 2 {
		t.Fatalf("months = %+v", ms)
	}
	if o := ms[0]; !o.Month.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, loc)) || o.Count != 3 || o.Approx || o.MedianWh != 5050 {
		t.Fatalf("October = %+v, want 3 estimates, median 5050, not approximate", o)
	}
	if n := ms[1]; !n.Month.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, loc)) || n.Count != 2 || !n.Approx || n.MedianWh != 4950 {
		t.Fatalf("November = %+v, want 2 estimates, median 4950, approximate", n)
	}
	if ms := Monthly(nil); len(ms) != 0 {
		t.Fatalf("no estimates: %+v", ms)
	}
}

var venus = model.ProfileFor("VenusE 3.0")

// One cycle is a 90 % DoD equivalent: 0.9 × 5120 = 4608 Wh given out, so
// 9216 Wh is 2 cycles of the rated 6000 (10240 Wh would be ≈ 2.22).
func TestSummarizeCycles(t *testing.T) {
	s := Summarize([]store.CapacityEstimate{est(t0, 4800), est(t0.Add(-24*time.Hour), 4600)}, 9216, 62, venus)
	if s.Cycles == nil || math.Abs(*s.Cycles-2) > 1e-9 || s.RatedDoD != 0.9 || s.CycleLife != 6000 || s.NominalWh != 5120 || s.MaxDelta != 62 {
		t.Fatalf("summary = %+v, want 2 cycles of the rated 6000 at 90 %% DoD", s)
	}
	if s.ActualWh == nil || *s.ActualWh != 4700 || s.HealthPct == nil || math.Abs(*s.HealthPct-4700.0/5120*100) > 1e-9 || !s.Approx {
		t.Fatalf("health = %v %v %v, want 4700 Wh ≈ 91.8 %%, approximate", s.ActualWh, s.HealthPct, s.Approx)
	}
	if len(s.Months) != 1 {
		t.Fatalf("months = %+v", s.Months)
	}
	// An unknown model has no nominal capacity or rated DoD: cycles and
	// percentage are unknown (nil, «—»), never 0 or a division by zero.
	u := Summarize([]store.CapacityEstimate{est(t0, 4800)}, 10240, 62, model.ProfileFor("Other"))
	if u.Cycles != nil || u.HealthPct != nil || u.ActualWh == nil || *u.ActualWh != 4800 {
		t.Fatalf("unknown model = %+v", u)
	}
}

func TestSummarizeNoEstimates(t *testing.T) {
	s := Summarize(nil, 460.8, 34, venus)
	if s.HealthPct != nil || s.ActualWh != nil || s.Approx || len(s.Months) != 0 {
		t.Fatalf("summary = %+v, want no health yet", s)
	}
	if s.MaxDelta != 34 || s.Cycles == nil || math.Abs(*s.Cycles-0.1) > 1e-9 {
		t.Fatalf("summary = %+v, want maxDelta 34 and 0.1 cycles", s)
	}
}

// One finder fed in chunks gives the same as one pass over everything (the
// monitor reads the samples a few hours at a time).
func TestFinderAcrossChunks(t *testing.T) {
	d := 3*time.Hour + 4*time.Minute + 19*time.Second
	all := run(nil, t0, d, 90, 30, -1000)
	var fd Finder
	for i := 0; i < len(all); i += 100 {
		for _, s := range all[i:min(i+100, len(all))] {
			fd.Add(s)
		}
	}
	got, gotMax := fd.Result()
	want, wantMax := FindDischarges(all)
	if len(got) != 1 || got[0] != want[0] || gotMax != wantMax {
		t.Fatalf("chunked = %+v %v, one pass = %+v %v", got, gotMax, want, wantMax)
	}
}

// A known nominal capacity without a rated DoD gives no cycle size either.
func TestSummarizeNoRatedDoD(t *testing.T) {
	p := venus
	p.RatedDoD = 0
	if s := Summarize(nil, 9216, 0, p); s.Cycles != nil {
		t.Fatalf("cycles = %v, want nil", *s.Cycles)
	}
}

// Health above 105 % is noise, marked approximate; it is shown capped at
// 110 %. ActualWh stays as measured.
func TestSummarizeHealthClamp(t *testing.T) {
	three := func(wh float64) []store.CapacityEstimate {
		return []store.CapacityEstimate{est(t0, wh), est(t0.Add(-time.Hour), wh), est(t0.Add(-2*time.Hour), wh)}
	}
	for _, tc := range []struct {
		wh, pct float64
		approx  bool
	}{
		{5300, 5300.0 / 5120 * 100, false}, // 103.5 %
		{5500, 5500.0 / 5120 * 100, true},  // 107.4 %
		{5900, 110, true},                  // 115.2 %
	} {
		s := Summarize(three(tc.wh), 0, 60, venus)
		if s.HealthPct == nil || math.Abs(*s.HealthPct-tc.pct) > 1e-9 || s.Approx != tc.approx || *s.ActualWh != tc.wh {
			t.Errorf("%v Wh: health %v approx %v actual %v; want %.1f %v", tc.wh, *s.HealthPct, s.Approx, *s.ActualWh, tc.pct, tc.approx)
		}
	}
}

// Re-review: a resting battery whose SOC jitters 90 ↔ 89 for 30 days (at
// −5 W) opens no run that the bounce tolerance keeps open: the later 90 → 30
// discharge alone is measured.
func TestFindDischargesIgnoresPlateauJitter(t *testing.T) {
	from := t0.Add(-30 * 24 * time.Hour)
	var s []model.Sample
	for at := from; at.Before(t0); at = at.Add(20 * time.Second) {
		soc := 90.0
		if at.Sub(from)/(10*time.Minute)%2 == 1 {
			soc = 89
		}
		s = append(s, model.Sample{At: at, SOC: f(soc), BatteryW: f(-5)})
	}
	s = run(s, t0.Add(20*time.Second), deep, 90, 30, -1000)
	es, _ := FindDischarges(s)
	if len(es) != 1 || math.Abs(es[0].Wh-5120) > 5 {
		t.Fatalf("estimates = %+v, want one of 5120 Wh", es)
	}
	if e := es[0]; e.Start.Before(t0) || e.Start.After(t0.Add(3*time.Minute)) {
		t.Fatalf("start = %v, want the discharge start %v", e.Start, t0)
	}
}

// A 2 h pause at 40 % mid-discharge (−15 W idle) closes the run at its last
// low; the rest is a new run, judged on its own. 95 → 40 at −1000 W (94 → 40,
// 54 steps) is an estimate without the pause's energy; 40 → 20 is not.
func TestFindDischargesSplitsOnPause(t *testing.T) {
	d1 := deep * 55 / 60
	s := run(nil, t0, d1, 95, 40, -1000)
	p := t0.Add(d1 + 5*time.Second)
	s = run(s, p, 2*time.Hour, 40, 40, -15)
	s = run(s, p.Add(2*time.Hour+5*time.Second), deep*20/60, 40, 20, -1000)
	es, maxDelta := FindDischarges(s)
	if len(es) != 1 || es[0].DeltaSOC != 54 || math.Abs(es[0].Wh-5120) > 5 || es[0].End.After(p) || maxDelta != 54 {
		t.Fatalf("estimates %+v, maxDelta %v; want one of 5120 Wh over 54 steps, ending before the pause", es, maxDelta)
	}
}
