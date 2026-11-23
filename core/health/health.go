// Package health estimates the battery's actual capacity and its cycle count
// from the stored samples and energy (spec §7.5): the station reports
// neither. Every figure is an estimate; without enough data there is none,
// never a made-up one.
package health

import (
	"math"
	"sort"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/energy"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// MinDeltaSOC: a discharge run must drop the SOC by at least this many points
// to give a capacity estimate; over a shorter run the 1-point SOC steps of
// the station make the ratio too coarse.
const MinDeltaSOC = 50.0 // п.п.

// minMonthCount: a month with fewer estimates is shown as approximate.
const minMonthCount = 3

// maxBridge: one missed poll inside a run is bridged (the step's power is
// held over it, left rectangle); a longer hole ends the run. Discharges
// happen mostly during outages, when the Wi-Fi is least reliable.
const maxBridge = 2 * energy.MaxStep

// maxBounce: the BMS may read one point back up under a load change
// (55 → 56 → 55); a rise of more than this above the run's lowest SOC ends
// the run.
const maxBounce = 1.0

// armDrop: until a run has dropped this many points below its start, a
// reading back at the plateau SOC restarts it (a resting battery that
// jitters 90 ↔ 89 for weeks opens no run); the bounce tolerance applies only
// after that.
const armDrop = 3.0

// maxStall: a run with no new SOC low for longer than this is closed at its
// last low (a pause mid-discharge, or a plateau held open by jitter); what
// follows is judged as a new run.
const maxStall = time.Hour

// Finder finds discharge runs in samples fed in ascending order, in any
// number of chunks (the monitor reads weeks of samples a few hours at a
// time). The zero value is ready.
//
// Candidate stretch: samples with a known SOC and power, no hole longer than
// maxBridge between two of them, no charge (BatteryW ≤ +20 W) and an SOC that
// never rises more than maxBounce above its lowest so far (before the first
// drop a rise just moves the plateau; before the run is armDrop deep, a
// reading back at the plateau restarts it). Idle (|P| ≤ 20 W) inside is
// allowed, but a run without a new low for maxStall ends at its last low.
//
// The run is measured SOC tick to tick, not over the whole stretch: it starts
// at the first sample below the plateau SOC and ends at the sample that first
// reaches the lowest SOC. Idle before the first drop (days at 100 % on grid)
// and after the last one adds neither energy nor time, and with whole-percent
// SOC readings both ends sit right after a 1-point step, so ΔSOC is an exact
// count of steps. Energy is the discharge between those two samples by left
// rectangles (step i→i+1 adds −P_i·Δt when P_i < 0: idle discharge inside
// counts, deadband "charge" noise does not); Wh = energy / (ΔSOC/100).
//
// Bias: BatteryW is the station's power balance (ongrid + offgrid − PV, i.e.
// measured on the AC side), while ΔSOC is the BMS's figure for the cells.
// Inverter losses are on the cell side of that, so the estimate runs low by
// the conversion loss (a few per cent): a new battery can read below 100 %.
type Finder struct {
	open     bool
	prev     model.Sample // the stretch's newest sample
	top      float64      // the plateau SOC before the first drop
	dropping bool         // the SOC has dropped below top
	low      float64      // lowest SOC so far (once dropping)
	startAt  time.Time    // first sample below top
	startSOC float64
	endAt    time.Time // first sample at low
	wh       float64   // discharge since startAt
	whAtLow  float64   // wh at endAt
	es       []store.CapacityEstimate
	maxDelta float64
}

// Add feeds the next sample.
func (f *Finder) Add(s model.Sample) {
	ok := s.SOC != nil && s.BatteryW != nil && *s.BatteryW <= detect.BatteryDeadbandW
	if f.open && ok && f.dropping && s.At.Sub(f.endAt) > maxStall {
		f.close() // stalled: keep the run up to its last low
		f.open, f.prev, f.top, f.dropping = true, s, *s.SOC, false
		return
	}
	if f.open && ok {
		dt := s.At.Sub(f.prev.At)
		soc := *s.SOC
		if dt > 0 && dt <= maxBridge && (!f.dropping || soc <= f.low+maxBounce) {
			f.step(s, dt, soc)
			return
		}
	}
	f.close()
	if ok {
		f.open, f.prev, f.top, f.dropping = true, s, *s.SOC, false
	}
}

// step extends the open stretch by s.
func (f *Finder) step(s model.Sample, dt time.Duration, soc float64) {
	defer func() { f.prev = s }()
	if !f.dropping {
		if soc >= f.top {
			f.top = soc // idle or a rise on the plateau: nothing measured yet
			return
		}
		f.dropping, f.low, f.startAt, f.startSOC = true, soc, s.At, soc
		f.endAt, f.wh, f.whAtLow = s.At, 0, 0
		return
	}
	if f.startSOC-f.low < armDrop && soc >= f.top {
		f.dropping, f.top = false, soc // jitter on the plateau: start over
		f.endAt, f.wh, f.whAtLow = s.At, 0, 0
		return
	}
	if p := *f.prev.BatteryW; p < 0 {
		f.wh += -p * dt.Hours()
	}
	if soc < f.low {
		f.low, f.endAt, f.whAtLow = soc, s.At, f.wh
	}
}

// close ends the open stretch, keeping its run when it is deep enough.
func (f *Finder) close() {
	if !f.open {
		return
	}
	f.open = false
	if !f.dropping {
		return
	}
	d := f.startSOC - f.low
	f.maxDelta = math.Max(f.maxDelta, d)
	if d >= MinDeltaSOC {
		f.es = append(f.es, store.CapacityEstimate{Start: f.startAt, End: f.endAt, Wh: f.whAtLow / (d / 100), DeltaSOC: d})
	}
}

// Result ends the open run (it counts as it is so far) and returns the
// estimates (only runs of ΔSOC ≥ MinDeltaSOC) and the largest ΔSOC of any
// run. The Finder is not to be fed after it.
func (f *Finder) Result() ([]store.CapacityEstimate, float64) {
	f.close()
	return f.es, f.maxDelta
}

// FindDischarges finds the discharge runs in samples (ascending by At); see
// Finder. es holds only runs with ΔSOC ≥ MinDeltaSOC; maxDelta is the largest
// ΔSOC of any run.
func FindDischarges(samples []model.Sample) (es []store.CapacityEstimate, maxDelta float64) {
	var f Finder
	for _, s := range samples {
		f.Add(s)
	}
	return f.Result()
}

// Month is the median capacity estimate of one local month (Month = its
// first midnight). Approx: fewer than 3 estimates.
type Month struct {
	Month    time.Time
	MedianWh float64
	Count    int
	Approx   bool
}

// Monthly groups the estimates by the local month of their start, ascending.
func Monthly(es []store.CapacityEstimate) []Month {
	by := map[time.Time][]float64{}
	for _, e := range es {
		l := e.Start.Local()
		k := time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, time.Local)
		by[k] = append(by[k], e.Wh)
	}
	out := make([]Month, 0, len(by))
	for k, whs := range by {
		out = append(out, Month{Month: k, MedianWh: median(whs), Count: len(whs), Approx: len(whs) < minMonthCount})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month.Before(out[j].Month) })
	return out
}

func median(v []float64) float64 {
	sort.Float64s(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}

// The health percentage is clamped for display. A single month's median can
// read above 100 % from estimate noise (whole-percent SOC, a BMS that
// recalibrates) even though the AC-side bias (Finder) pulls it low; a new
// battery is about 100 %, so anything far above is noise, not health.
// Above healthApproxPct the value is marked approximate; it is capped at
// healthCapPct. ActualWh and the months stay as measured.
const (
	healthApproxPct = 105.0
	healthCapPct    = 110.0
)

// Summary is the "Battery" tab. HealthPct, ActualWh and Approx come from the
// newest month with estimates; nil without any (the UI says «ще рахуємо»).
// HealthPct is also nil for a model without a nominal capacity.
type Summary struct {
	HealthPct *float64
	ActualWh  *float64
	Approx    bool
	// Cycles = discharged Wh / (RatedDoD × NominalWh): equivalents of the
	// rated cycle (Venus E 3.0: over 6000 at 90 % DoD, so 4608 Wh each). An
	// estimate from energy throughput (the station does not report cycles);
	// lifetime: Task 16's pruning keeps the total. nil for a model without
	// NominalWh or RatedDoD (the UI shows «—», never 0). CycleLife is the
	// rated count at RatedDoD, from the profile.
	Cycles    *float64
	CycleLife int
	RatedDoD  float64
	NominalWh float64
	// MaxDelta: the largest ΔSOC of any run so far, for «найбільший розряд
	// поки N п.п.».
	MaxDelta float64
	Months   []Month
}

// Summarize builds the summary from the stored estimates, the discharged
// energy and the largest ΔSOC so far, for profile p.
func Summarize(es []store.CapacityEstimate, dischargedWh, maxDelta float64, p model.Profile) Summary {
	s := Summary{CycleLife: p.CycleLife, RatedDoD: p.RatedDoD, NominalWh: p.NominalWh, MaxDelta: maxDelta}
	if len(es) > 0 {
		s.Months = Monthly(es)
	}
	if cycleWh := p.RatedDoD * p.NominalWh; cycleWh > 0 {
		c := dischargedWh / cycleWh
		s.Cycles = &c
	}
	if n := len(s.Months); n > 0 {
		last := s.Months[n-1]
		wh := last.MedianWh
		s.ActualWh, s.Approx = &wh, last.Approx
		if p.NominalWh > 0 {
			pct := wh / p.NominalWh * 100
			if pct > healthApproxPct {
				s.Approx = true
			}
			pct = math.Min(pct, healthCapPct)
			s.HealthPct = &pct
		}
	}
	return s
}
