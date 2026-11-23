// Package energy integrates battery power into per-minute energy and builds
// the energy report of a period from the stored hours.
package energy

import (
	"sort"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// MaxStep = 3 × monitor.CalmPoll (20 s): samples come every 5 s (Active) or
// 20 s (Calm). A longer step between two samples is a hole: it is neither
// integrated nor counted as covered.
const MaxStep = 60 * time.Second

// minEfficiencyWh: below this much charged or discharged energy the ratio
// says nothing.
const minEfficiencyWh = 500

// Rollup integrates the samples (ascending by At) into the whole minutes of
// [from, to) by left rectangles: the step i→i+1 (Δt ≤ MaxStep) adds P_i·Δt to
// the minute the step started in, even when it ends in the next one. A step
// with unknown P_i adds nothing and is not covered (never read as 0 W);
// |P_i| ≤ 20 W is 0 Wh but covered. A discharge step that starts during an
// outage also counts in DisOutageWh; PVChgWh = min(the step's charge,
// PV_i·Δt). Minutes without a covered step are left out.
func Rollup(samples []model.Sample, outages []store.Outage, from, to time.Time) []store.EnergyMinute {
	byMin := map[int64]*store.EnergyMinute{}
	for i := 0; i+1 < len(samples); i++ {
		a := samples[i]
		if a.At.Before(from) || !a.At.Before(to) || a.BatteryW == nil {
			continue
		}
		dt := samples[i+1].At.Sub(a.At)
		if dt <= 0 || dt > MaxStep {
			continue
		}
		key := a.At.UnixMilli() / 60000 * 60000
		m := byMin[key]
		if m == nil {
			m = &store.EnergyMinute{At: time.UnixMilli(key).UTC()}
			byMin[key] = m
		}
		m.Covered += dt
		p, h := *a.BatteryW, dt.Hours()
		switch {
		case p > detect.BatteryDeadbandW:
			wh := p * h
			m.ChgWh += wh
			if a.PVW != nil && *a.PVW > 0 {
				m.PVChgWh += min(wh, *a.PVW*h)
			}
		case p < -detect.BatteryDeadbandW:
			m.DisWh += -p * h
			if inOutage(a.At, outages) {
				m.DisOutageWh += -p * h
			}
			if m.PeakDisW == nil || -p > *m.PeakDisW {
				w, at := -p, a.At
				m.PeakDisW, m.PeakAt = &w, &at
			}
		}
	}
	out := make([]store.EnergyMinute, 0, len(byMin))
	for _, m := range byMin {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// inOutage: t lies in [Start, End) of an outage (an open one has no end).
func inOutage(t time.Time, outages []store.Outage) bool {
	for _, o := range outages {
		if !t.Before(o.Start) && (o.End == nil || t.Before(*o.End)) {
			return true
		}
	}
	return false
}

// Bucket is one day (or week) of the report. Span is its length (23, 24 or
// 25 h for a day; clipped to now for today). NoData: nothing covered in a
// bucket that is not before ObservedSince.
type Bucket struct {
	Start, End                                    time.Time
	ChargedWh, DischargedWh, OutageWh, PVChargeWh float64
	PeakW                                         *float64
	PeakAt                                        *time.Time
	Covered, Span                                 time.Duration
	NoData                                        bool
}

// Report is the energy of one period.
type Report struct {
	Period                            history.Period
	ChargedWh, DischargedWh, OutageWh float64
	Efficiency                        *float64 // Discharged/Charged·100; nil if either is < 500 Wh
	Coverage                          float64  // Covered / observed time (period ∩ [ObservedSince, now]), 0..1
	HasPV                             bool
	Buckets                           []Bucket
	// EnergySince is the first rolled-up minute: energy counting starts
	// there. A NoData day ending before it lies before counting began (on
	// an upgraded database, older than the 30 days of raw samples), not in a
	// gap. Nil until the first minute is stored. Set by the monitor.
	EnergySince *time.Time
	// Backfilling: the rollup is still catching up on past days, so recent
	// totals may grow. Set by the monitor.
	Backfilling bool
}

// BuildReport sums the stored hours (store.EnergyHours) of p into its days,
// or weeks from Monday when p.Weeks (history.BucketBounds, the same columns
// as the outages tab). Hours outside p are ignored.
func BuildReport(hours []store.EnergyMinute, observedSince *time.Time, p history.Period, now time.Time) Report {
	r := Report{Period: p}
	var covered time.Duration
	for _, iv := range history.BucketBounds(p) {
		b := Bucket{Start: iv.Start, End: iv.End, Span: iv.End.Sub(iv.Start)}
		for _, h := range hours {
			if h.At.Before(b.Start) || !h.At.Before(b.End) {
				continue
			}
			b.ChargedWh += h.ChgWh
			b.DischargedWh += h.DisWh
			b.OutageWh += h.DisOutageWh
			b.PVChargeWh += h.PVChgWh
			b.Covered += h.Covered
			if h.PeakDisW != nil && (b.PeakW == nil || *h.PeakDisW > *b.PeakW) {
				w := *h.PeakDisW
				b.PeakW, b.PeakAt = &w, nil
				if h.PeakAt != nil {
					at := *h.PeakAt
					b.PeakAt = &at
				}
			}
		}
		b.NoData = b.Covered == 0 && observedSince != nil && b.End.After(*observedSince)
		r.ChargedWh += b.ChargedWh
		r.DischargedWh += b.DischargedWh
		r.OutageWh += b.OutageWh
		r.HasPV = r.HasPV || b.PVChargeWh > 0
		covered += b.Covered
		r.Buckets = append(r.Buckets, b)
	}
	if r.ChargedWh >= minEfficiencyWh && r.DischargedWh >= minEfficiencyWh {
		e := r.DischargedWh / r.ChargedWh * 100
		r.Efficiency = &e
	}
	if observedSince != nil {
		from, to := p.From, p.To
		if observedSince.After(from) {
			from = *observedSince
		}
		if now.Before(to) {
			to = now
		}
		if obs := to.Sub(from); obs > 0 {
			r.Coverage = max(0, min(1, float64(covered)/float64(obs)))
		}
	}
	return r
}
