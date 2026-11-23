package monitor

import (
	"math"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/model"
)

// Adaptive polling: the station drops calls under load, and its commonest
// state (full, grid on) changes nothing for hours. While it is calm it is
// polled every CalmPoll; anything that moves switches back to the active
// interval at once (see cadenceFor).
const (
	CalmPoll = 20 * time.Second
	// ActiveWarmup: after start or Resume the poll stays active this long.
	ActiveWarmup = 2 * time.Minute
	// CalmSOCWindow / CalmSOCBand: SOC must have stayed within the band
	// (max − min, percentage points) over the window.
	CalmSOCWindow = time.Minute
	CalmSOCBand   = 1.0
)

// Cadence is the polling regime.
type Cadence int

const (
	Active Cadence = iota
	Calm
)

func (c Cadence) String() string {
	if c == Calm {
		return "calm"
	}
	return "active"
}

// cadenceInput is what the cadence is decided on, at the latest sample.
type cadenceInput struct {
	Live bool
	Grid detect.GridState
	Mode detect.BatteryMode
	// PendingGrid: the tracker has an unconfirmed grid change; PendingMode: a
	// new battery mode is taking over the forecast (Monitor.pending).
	PendingGrid, PendingMode bool
	// SOCSpan is max − min SOC over the last CalmSOCWindow of live samples.
	SOCSpan  float64
	SOCKnown bool
	// SinceStart is the time since the start or the last Resume.
	SinceStart time.Duration
	// BatteryW and LastGrid are the latest sample's readings, so a sample
	// that shows a load or the grid gone switches at once.
	BatteryW *float64
	LastGrid model.GridSignal
	// FloorIdle: the latest sample is at the floor with no power flowing, so
	// its grid reads unknown (Ruling 12) though nothing changes.
	FloorIdle bool
}

// cadenceFor is Calm only when the link is live, the grid is confirmed on and
// the latest sample shows it present (or is FloorIdle), the battery is idle
// (|P| ≤ 20 W; Cutoff counts when FloorIdle: a 1-p.p. bounce at the floor),
// no grid or mode change is pending, SOC held within CalmSOCBand over
// CalmSOCWindow, and the warmup is over. Anything else is Active.
func cadenceFor(in cadenceInput) Cadence {
	calm := in.Live && in.Grid == detect.GridOn &&
		(in.LastGrid == model.GridPresent || in.FloorIdle) &&
		(in.Mode == detect.Idle || in.Mode == detect.Cutoff && in.FloorIdle) &&
		in.BatteryW != nil && math.Abs(*in.BatteryW) <= detect.BatteryDeadbandW &&
		!in.PendingGrid && !in.PendingMode &&
		in.SOCKnown && in.SOCSpan <= CalmSOCBand &&
		in.SinceStart >= ActiveWarmup
	if calm {
		return Calm
	}
	return Active
}

// pollInterval is the wait between successful polls: the active interval, or
// CalmPoll when calm but never faster than the active one.
func (m *Monitor) pollInterval() time.Duration {
	if m.cadence == Calm {
		return max(CalmPoll, m.poll)
	}
	return m.poll
}

type socReading struct {
	at  time.Time
	soc float64
}

// noteSOC keeps the live SOC readings of the last CalmSOCWindow and returns
// their span. Caller holds m.mu.
func (m *Monitor) noteSOC(s model.Sample) (span float64, known bool) {
	if s.SOC == nil {
		return 0, false
	}
	m.socs = append(m.socs, socReading{s.At, *s.SOC})
	keep := 0
	for keep < len(m.socs) && s.At.Sub(m.socs[keep].at) > CalmSOCWindow {
		keep++
	}
	m.socs = m.socs[keep:]
	lo, hi := m.socs[0].soc, m.socs[0].soc
	for _, r := range m.socs[1:] {
		lo, hi = min(lo, r.soc), max(hi, r.soc)
	}
	return hi - lo, true
}

// noFlow: Idle and Cutoff both mean the battery is still; a switch between the
// two (a 1-p.p. bounce at the floor) is not a mode change to poll for.
func noFlow(mode detect.BatteryMode) bool { return mode == detect.Idle || mode == detect.Cutoff }

// updateCadence decides the cadence on sample s and, when it changed, moves
// the link machine's freshness to 3 × the new interval and logs the change.
// Caller holds m.mu.
func (m *Monitor) updateCadence(s model.Sample, mode detect.BatteryMode, live bool) {
	in := cadenceInput{Live: live, Grid: m.tracker.State(live), Mode: mode,
		PendingGrid: m.tracker.Pending(),
		PendingMode: m.pending != nil && !(noFlow(m.pendingMode) && noFlow(m.smoothMode)),
		SinceStart:  s.At.Sub(m.warmFrom), BatteryW: s.BatteryW, LastGrid: s.Grid,
		FloorIdle: s.Grid == model.GridUnknown && atFloorIdle(s, m.profile)}
	if live {
		in.SOCSpan, in.SOCKnown = m.noteSOC(s)
	}
	m.setCadence(cadenceFor(in), s.At)
}

func (m *Monitor) setCadence(c Cadence, at time.Time) {
	if c == m.cadence {
		return
	}
	m.cadence = c
	m.machine.SetFresh(freshFor(m.pollInterval()))
	m.addEvent(at, "monitor.cadence", c.String())
}

// freshFor: a sample is fresh for 3 poll intervals.
func freshFor(poll time.Duration) time.Duration { return 3 * poll }
