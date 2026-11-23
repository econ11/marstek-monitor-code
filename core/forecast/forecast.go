// Package forecast computes time estimates and ring fill fractions from
// already-loaded data. Pure functions: no store calls, no clocks.
package forecast

import (
	"math"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

const (
	warmup          = 60 * time.Second
	taperFromSOC    = 90.0
	taperFactor     = 0.5
	minOutage       = 15 * time.Minute
	outageLookback  = 30 * 24 * time.Hour
	dischargeFloorW = 20.0
	// minForecastW: below it a power is noise, and the time it gives is years
	// (or overflows time.Duration); there is no forecast.
	minForecastW = 1.0
)

type point struct {
	at time.Time
	w  float64
}

// Smoother is a moving average of power over a trailing window.
type Smoother struct {
	window time.Duration
	pts    []point
}

func NewSmoother(window time.Duration) *Smoother { return &Smoother{window: window} }

func (s *Smoother) Add(at time.Time, w float64) {
	s.pts = append(s.pts, point{at, w})
}

// Value averages the points within the window ending at now. ok is false
// until the retained points span at least 60 s.
func (s *Smoother) Value(now time.Time) (w float64, span time.Duration, ok bool) {
	cut := now.Add(-s.window)
	i := 0
	for i < len(s.pts) && s.pts[i].at.Before(cut) {
		i++
	}
	s.pts = s.pts[i:]
	if len(s.pts) == 0 {
		return 0, 0, false
	}
	var sum float64
	for _, p := range s.pts {
		sum += p.w
	}
	span = s.pts[len(s.pts)-1].at.Sub(s.pts[0].at)
	return sum / float64(len(s.pts)), span, span >= warmup
}

func UsableWh(soc, minSOC, nominalWh float64) float64 {
	return math.Max(0, soc-minSOC) / 100 * nominalWh
}

func validPower(w float64) bool { return w >= minForecastW && !math.IsInf(w, 0) }

func hours(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

func ToEmpty(soc float64, p model.Profile, dischargeW float64) (time.Duration, bool) {
	if p.NominalWh <= 0 || !validPower(dischargeW) || math.IsNaN(soc) {
		return 0, false
	}
	return hours(UsableWh(soc, p.MinSOC, p.NominalWh) / dischargeW), true
}

// ToFull assumes charge power halves above 90% (plan decision, to be refined
// from history).
func ToFull(soc float64, p model.Profile, chargeW float64) (time.Duration, bool) {
	if p.NominalWh <= 0 || !validPower(chargeW) || math.IsNaN(soc) {
		return 0, false
	}
	if soc >= 100 {
		return 0, true
	}
	var h float64
	if soc < taperFromSOC {
		h += (taperFromSOC - soc) / 100 * p.NominalWh / chargeW
		soc = taperFromSOC
	}
	h += (100 - soc) / 100 * p.NominalWh / (chargeW * taperFactor)
	return hours(h), true
}

// LastsFor estimates runtime at the average discharge seen in past outages.
func LastsFor(soc float64, p model.Profile, avgOutageW *float64) (time.Duration, bool) {
	if avgOutageW == nil {
		return 0, false
	}
	return ToEmpty(soc, p, *avgOutageW)
}

// AvgOutageDischargeW is the mean discharge (positive watts) over samples that
// fall inside outages of at least 15 min started within 30 days of the newest
// sample. Open outages end at the newest sample. nil if nothing qualifies.
func AvgOutageDischargeW(samples []model.Sample, outages []store.Outage) *float64 {
	var ref time.Time
	for _, s := range samples {
		if s.At.After(ref) {
			ref = s.At
		}
	}
	if ref.IsZero() {
		return nil
	}
	type span struct{ a, b time.Time }
	var spans []span
	for _, o := range outages {
		end := ref
		if o.End != nil {
			end = *o.End
		}
		if end.Sub(o.Start) < minOutage || o.Start.Before(ref.Add(-outageLookback)) {
			continue
		}
		spans = append(spans, span{o.Start, end})
	}
	var sum float64
	var n int
	for _, s := range samples {
		if s.BatteryW == nil || *s.BatteryW >= -dischargeFloorW {
			continue
		}
		for _, sp := range spans {
			if !s.At.Before(sp.a) && !s.At.After(sp.b) {
				sum -= *s.BatteryW
				n++
				break
			}
		}
	}
	if n == 0 {
		return nil
	}
	avg := sum / float64(n)
	return &avg
}

func clamp01(v float64) float64 { return math.Min(1, math.Max(0, v)) }

// FillToEmpty: usable energy now over usable energy at 100%.
func FillToEmpty(soc float64, p model.Profile) float64 {
	full := UsableWh(100, p.MinSOC, p.NominalWh)
	if full <= 0 {
		return 0
	}
	return clamp01(UsableWh(soc, p.MinSOC, p.NominalWh) / full)
}

func FillToFull(soc float64) float64 { return clamp01(soc / 100) }

func FillNoLight(off, elapsedToday time.Duration) float64 {
	if elapsedToday <= 0 {
		return 0
	}
	return clamp01(float64(off) / float64(elapsedToday))
}
