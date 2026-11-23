package monitor

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/forecast"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// gapCardMin: a startup gap shorter than this gets no card.
const gapCardMin = 5 * time.Minute

// Forecasts are the time rings. nil = unknown; Warming = "calculating…"
// (less than 60 s of smoothed power in the current battery mode).
type Forecasts struct {
	ToEmpty, ToFull, LastsFor *time.Duration
	EmptyAt, FullAt           *time.Time
	Warming                   bool
}

// Snapshot is everything the UI shows, recomputed after each sample and tick.
type Snapshot struct {
	// Seq grows by one with every publication, so a consumer that receives
	// snapshots out of order (a bound call racing an event) keeps the newest.
	Seq       uint64
	Station   store.Station
	Profile   model.Profile
	Link      link.State
	Diagnosis link.Diagnosis
	Last      *model.Sample
	Age       time.Duration
	Grid      detect.GridState
	// GridPending is an observed but not yet confirmed grid change (the 30 s
	// debounce is running); GridUnknownState = nothing pending. Only while the
	// link is LIVE. GridPendingSince is its first observation.
	GridPending      detect.GridState
	GridPendingSince *time.Time
	Mode             detect.BatteryMode
	F                Forecasts
	// NoLightToday is the observed off time today (history.OffKnown), a lower
	// bound when NoLightAtLeast; GapsToday is today's unobserved time.
	NoLightToday   time.Duration
	NoLightAtLeast bool
	GapsToday      time.Duration
	OutagesToday   int
	Current        *store.Outage
	Previous       *store.Outage
	// What was actually observed of Current / Previous; nil without one.
	CurrentBounds, PreviousBounds *history.Bounds
	GapCard                       *store.Gap // gap ≥ 5 min at startup, not yet dismissed by the user
	Fills                         struct{ SOC, Power, Time, NoLight float64 }
	ObservedSince                 *time.Time
	// LowThreshold is the low-charge threshold, percent (Settings).
	LowThreshold float64
	// NewEvents: journal warnings since the user last opened it; JournalSeenAt:
	// when that was (nil = never).
	NewEvents     int
	JournalSeenAt *time.Time
	// Battery: the estimated cycles and health for the overview's «Батарея»
	// row; refreshed every 10 min.
	Battery BatteryState
}

// BatteryState is the snapshot's «Батарея» (Task 14): Cycles, the estimated
// cycles (nil = model without a rated cycle size); HealthPct, the health
// percent (nil = not estimated yet); HealthApprox, preliminary or clamped
// (health.Summary.Approx, shown with «≈»).
type BatteryState struct {
	Cycles       *float64
	HealthPct    *float64
	HealthApprox bool
}

// SeriesPoint is one chart point; SOC nil breaks the line.
type SeriesPoint struct {
	At  time.Time
	SOC *float64
}

// Band is a shaded chart range; Label is a copy key.
type Band struct {
	Start, End time.Time
	Kind       string // "outage" | "gap"
	Label      string
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Min(1, math.Max(0, v))
}

func localMidnight(now time.Time) time.Time {
	l := now.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}

// snapshot builds the current Snapshot. Caller holds m.mu.
func (m *Monitor) snapshot(now time.Time) Snapshot {
	st := m.machine.State()
	s := Snapshot{Station: m.station, Profile: m.profile, Link: st, Diagnosis: m.machine.Diagnosis(),
		LowThreshold: m.d.Notify.LowThreshold, NewEvents: m.newEvents, Battery: m.battery}
	if m.seenAt != nil {
		t := *m.seenAt
		s.JournalSeenAt = &t
	}
	live := st == link.Live
	if m.last != nil {
		cp := *m.last
		s.Last, s.Age = &cp, now.Sub(cp.At)
	}
	s.Grid = m.tracker.State(live)
	s.GridPending = detect.GridUnknownState
	if ps, since, ok := m.tracker.PendingState(); ok && live {
		s.GridPending, s.GridPendingSince = ps, &since
	}
	// Mode and forecasts only from fresh data: a stale reading is not extended.
	fresh := (live || st == link.Resyncing) && m.last != nil
	s.Mode = detect.ModeUnknown
	if fresh {
		s.Mode = detect.Mode(m.last.BatteryW, m.last.SOC, m.profile.MinSOC)
	}

	if since, ok, err := m.d.Store.ObservedSince(m.station.ID); err != nil {
		slog.Warn("monitor: observed since", "err", err)
	} else if ok {
		s.ObservedSince = &since
	}
	m.fillOutages(&s, now, live)
	if g := m.startupGap; g != nil && !m.gapDismissed {
		end := now
		if g.End != nil {
			end = *g.End
		}
		if end.Sub(g.Start) >= gapCardMin {
			c := *g
			s.GapCard = &c
		}
	}
	if fresh {
		m.fillForecasts(&s, now)
	}
	return s
}

// fillOutages sets today's time without light and gaps, and the current and
// previous outage with what was observed of them (core/history): time that
// lies in a data gap is unknown, not "without light".
func (m *Monitor) fillOutages(s *Snapshot, now time.Time, live bool) {
	dayStart := localMidnight(now)
	lastAt := now
	if m.last != nil {
		lastAt = m.last.At
	}
	// Gaps from BoundarySlack before the earliest outage shown: a gap that
	// ended just before an outage start makes that start approximate.
	from := dayStart
	for _, o := range []*store.Outage{m.cur, m.prev} {
		if o != nil && o.Start.Before(from) {
			from = o.Start
		}
	}
	gaps, err := m.d.Store.Gaps(m.station.ID, from.Add(-history.BoundarySlack), now.Add(time.Second))
	if err != nil {
		slog.Warn("monitor: gaps", "err", err)
	}
	outs, err := m.d.Store.Outages(m.station.ID, dayStart, now.Add(time.Second))
	if err != nil {
		slog.Warn("monitor: outages today", "err", err)
	}
	s.NoLightToday, s.OutagesToday, s.NoLightAtLeast = history.OffKnown(outs, gaps, dayStart, now, now, lastAt, live)
	s.GapsToday = history.Unobserved(gaps, s.ObservedSince, dayStart, now, now)
	s.Fills.NoLight = forecast.FillNoLight(s.NoLightToday, now.Sub(dayStart))

	if m.cur != nil {
		c := *m.cur
		b := history.Annotate(c, gaps, now, lastAt, live)
		s.Current, s.CurrentBounds = &c, &b
	}
	if m.prev != nil {
		p := *m.prev
		b := history.Annotate(p, gaps, now, lastAt, live)
		s.Previous, s.PreviousBounds = &p, &b
	}
}

// fillForecasts sets F and the SOC/Power/Time fills (spec §5.0) per mode.
func (m *Monitor) fillForecasts(s *Snapshot, now time.Time) {
	last, p := m.last, m.profile
	if last.SOC == nil {
		return
	}
	soc := *last.SOC
	s.Fills.SOC = clamp01(soc / 100)
	var bat float64
	if last.BatteryW != nil {
		bat = math.Abs(*last.BatteryW)
	}
	w, _, warm := m.smoother.Value(now)
	// "Calculating…" also while a new mode is taking over the forecast.
	warm = warm && !m.takingOver(s.Mode)
	switch s.Mode {
	case detect.Discharging:
		if p.MaxDischargeW > 0 {
			s.Fills.Power = clamp01(bat / p.MaxDischargeW)
		}
		s.Fills.Time = forecast.FillToEmpty(soc, p)
		if !warm {
			s.F.Warming = true
		} else if d, ok := forecast.ToEmpty(soc, p, -w); ok {
			at := now.Add(d)
			s.F.ToEmpty, s.F.EmptyAt = &d, &at
		}
	case detect.Charging:
		if p.MaxChargeW > 0 {
			s.Fills.Power = clamp01(bat / p.MaxChargeW)
		}
		s.Fills.Time = forecast.FillToFull(soc)
		if !warm {
			s.F.Warming = true
		} else if d, ok := forecast.ToFull(soc, p, w); ok {
			at := now.Add(d)
			s.F.ToFull, s.F.FullAt = &d, &at
		}
	case detect.Idle, detect.Cutoff:
		s.Fills.Time = forecast.FillToEmpty(soc, p)
		if d, ok := forecast.LastsFor(soc, p, m.avgOutageW); ok {
			s.F.LastsFor = &d
		}
	}
}

// gapLabel maps a gap reason to its chart copy key (copy.md §3.1, flows D4).
func gapLabel(r store.GapReason) string {
	switch r {
	case store.GapAppNotRunning:
		return "gap.label.appOff"
	case store.GapPCSleep:
		return "gap.label.pcSleep"
	case store.GapNoLink:
		return "gap.label.noLink"
	case store.GapLikelyDead:
		return "gap.label.likelyOut"
	case store.GapAppUpdate:
		return "gap.label.update"
	}
	return "gap.label"
}

// breakAt puts a point without a value (a line break) before the first point
// measured after each restart hole (a link.delayed event from
// eventSourceStart, logged at the time of that first measurement), so the
// line is not drawn across the time the app was not running. Other delays
// (station silent under 5 min while the app ran) stay joined, per flows 2.2.
func breakAt(pts []SeriesPoint, delays []store.Event, bucket time.Duration) []SeriesPoint {
	out := pts[:0:0]
	i := 0
	for _, e := range delays {
		if e.Source != eventSourceStart {
			continue
		}
		resumed := e.At.Truncate(bucket)
		for ; i < len(pts) && pts[i].At.Before(resumed); i++ {
			out = append(out, pts[i])
		}
		// Nothing to separate at either end of the series.
		if i < len(pts) && len(out) > 0 && out[len(out)-1].SOC != nil {
			out = append(out, SeriesPoint{At: pts[i].At})
		}
	}
	return append(out, pts[i:]...)
}

// SOCSeries returns the charge chart for "24h" (raw samples), "7d" (per
// minute) or "30d" (per hour), plus outage and gap bands clipped to the range.
// A point with a nil SOC breaks the line.
func (m *Monitor) SOCSeries(rangeKey string) ([]SeriesPoint, []Band, error) {
	var span, bucket time.Duration
	switch rangeKey {
	case "24h":
		span = 24 * time.Hour
	case "7d":
		span, bucket = 7*24*time.Hour, time.Minute
	case "30d":
		span, bucket = 30*24*time.Hour, time.Hour
	default:
		return nil, nil, fmt.Errorf("monitor: unknown range %q", rangeKey)
	}
	m.mu.Lock()
	id := m.station.ID
	m.mu.Unlock()
	now := m.clock.Now()
	from, to := now.Add(-span), now.Add(time.Second)

	var pts []SeriesPoint
	if bucket == 0 {
		raw, err := m.d.Store.PlainSamples(id, from, to)
		if err != nil {
			return nil, nil, err
		}
		pts = make([]SeriesPoint, 0, len(raw))
		for _, s := range raw {
			pts = append(pts, SeriesPoint{At: s.At, SOC: s.SOC})
		}
	} else {
		// Averaged in the store over raw samples and aggregated minutes:
		// neither the raw JSON nor every row is read for weeks of data.
		bs, err := m.d.Store.SOCBuckets(id, from, to, bucket)
		if err != nil {
			return nil, nil, err
		}
		pts = make([]SeriesPoint, 0, len(bs))
		for _, b := range bs {
			pts = append(pts, SeriesPoint{At: b.At, SOC: b.SOC})
		}
	}
	if bucket < time.Hour { // an hour bucket cannot show a hole under 5 min
		delays, err := m.d.Store.Events(id, "link.delayed", from, to)
		if err != nil {
			return nil, nil, err
		}
		pts = breakAt(pts, delays, bucket)
	}

	var bands []Band
	clip := func(a time.Time, e *time.Time) (time.Time, time.Time) {
		end := now
		if e != nil && e.Before(now) {
			end = *e
		}
		if a.Before(from) {
			a = from
		}
		return a, end
	}
	outs, err := m.d.Store.Outages(id, from, to)
	if err != nil {
		return nil, nil, err
	}
	for _, o := range outs {
		a, e := clip(o.Start, o.End)
		label := "overview.chart.band.outage"
		if o.End == nil {
			label = "overview.chart.band.outage.ongoing"
		}
		bands = append(bands, Band{Start: a, End: e, Kind: "outage", Label: label})
	}
	gaps, err := m.d.Store.Gaps(id, from, to)
	if err != nil {
		return nil, nil, err
	}
	for _, g := range gaps {
		a, e := clip(g.Start, g.End)
		bands = append(bands, Band{Start: a, End: e, Kind: "gap", Label: gapLabel(g.Reason)})
	}
	return pts, bands, nil
}

// PendingGrid is the unconfirmed grid change, ok false when none. It needs
// GridPendingSince too: the zero GridState is GridOn, so a Snapshot built
// without the field must not read as "grid coming back".
func (s Snapshot) PendingGrid() (detect.GridState, bool) {
	if s.GridPendingSince == nil || (s.GridPending != detect.GridOn && s.GridPending != detect.GridOff) {
		return detect.GridUnknownState, false
	}
	return s.GridPending, true
}

// PendingKind names the unconfirmed grid change for the texts: "" none,
// "Unknown" when the confirmed state is unknown (nothing "came back", it is
// just not known yet), else "On" (coming back) or "Off" (going off).
func (s Snapshot) PendingKind() string {
	p, ok := s.PendingGrid()
	switch {
	case !ok:
		return ""
	case s.Grid == detect.GridUnknownState:
		return "Unknown"
	case p == detect.GridOn:
		return "On"
	}
	return "Off"
}
