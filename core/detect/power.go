// Package detect holds pure detection logic over normalized samples.
package detect

import (
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// GridState is the confirmed state of the "power" axis (flows spec 2.5).
type GridState int

const (
	GridOn GridState = iota
	GridOff
	GridUnknownState
)

// BatteryMode is the battery behaviour prop.
type BatteryMode int

const (
	// ModeUnknown is returned when battery power is unknown (nil, never 0).
	ModeUnknown BatteryMode = iota
	Idle
	Charging
	Discharging
	Cutoff
)

// BatteryDeadbandW is the +/- noise band around zero treated as idle.
const BatteryDeadbandW = 20

// Mode classifies battery power (+ charge, - discharge). |P| <= 20 W is Idle;
// SOC at or below minSOC with P <= 20 W is Cutoff (discharge forbidden).
func Mode(batteryW, soc *float64, minSOC float64) BatteryMode {
	if batteryW == nil {
		return ModeUnknown
	}
	p := *batteryW
	if soc != nil && *soc <= minSOC && p <= BatteryDeadbandW {
		return Cutoff
	}
	switch {
	case p > BatteryDeadbandW:
		return Charging
	case p < -BatteryDeadbandW:
		return Discharging
	}
	return Idle
}

type PowerEventKind int

const (
	GridLost PowerEventKind = iota
	GridBack
)

// PowerEvent reports a confirmed grid change. At is when the new state was
// FIRST observed (not when the debounce elapsed); SOC is the reading then.
// ConfirmedAt is the time of the live sample that confirmed it: At is what
// gets stored and shown, ConfirmedAt tells a live observation from catch-up
// (the two are far apart when the station was silent in between).
//
// AcrossGap: the state before the change was not observed up to it (startup,
// or after Unconfirm), so At is only the first observation of the new state;
// the real change happened at some unknown time before it.
//
// GapFrom (AcrossGap only): the last observation before the gap, so the
// change happened between GapFrom and At. The tracker does not know it (it
// sees no gaps); the caller fills it in. Zero when nothing was observed
// before (the first run ever).
type PowerEvent struct {
	Kind        PowerEventKind
	At          time.Time
	ConfirmedAt time.Time
	SOC         *float64
	AcrossGap   bool
	GapFrom     time.Time
}

// Tracker derives the grid axis from samples with a time-based debounce.
// Pure logic: it never touches the store. Not safe for concurrent use.
type Tracker struct {
	debounce time.Duration
	state    GridState // confirmed; GridUnknownState until first confirmation
	// open: an outage is open (in the DB) while the state is unknown. The
	// first confirmed Present then closes it (GridBack across the gap) and the
	// first confirmed Absent continues it silently.
	open bool

	pending    GridState
	hasPending bool
	since      time.Time
	pendingSOC *float64
	lastLive   time.Time // timestamp of the last sample seen with the link live
}

// PendingExpiry is how long a pending change survives without any live sample.
// It equals the STALE to NO_LINK threshold (5 min): beyond it the evidence for
// the first observation is too old to anchor PowerEvent.At.
const PendingExpiry = 5 * time.Minute

// NewTracker starts in GridUnknownState: nothing was observed before the
// start. open (an outage still open in the DB after a restart, End == nil) is
// remembered: the first confirmed Absent continues it silently (no duplicate
// GridLost) and the first confirmed Present is GridBack{AcrossGap: true}.
// Without it the first Present is silent and the first Absent is
// GridLost{AcrossGap: true}.
func NewTracker(debounce time.Duration, open *store.Outage) *Tracker {
	return &Tracker{debounce: debounce, state: GridUnknownState, open: open != nil && open.End == nil}
}

// Unconfirm is called after a data gap: the state becomes unknown and a
// pending change is dropped (its evidence predates the gap). An open outage
// is remembered, so the next confirmed change is reported across the gap.
func (t *Tracker) Unconfirm() {
	switch t.state {
	case GridOff:
		t.open = true
	case GridOn:
		t.open = false
	}
	t.state, t.hasPending = GridUnknownState, false
}

// Assume continues state s without an event, after a restart shorter than a
// gap (nothing was missed). A pending change is dropped.
func (t *Tracker) Assume(s GridState) {
	t.state, t.hasPending = s, false
	if s != GridUnknownState {
		t.open = s == GridOff
	}
}

// State is the confirmed state. When the link is not LIVE it is always
// GridUnknownState: the last known state is deliberately not extended.
func (t *Tracker) State(linkLive bool) GridState {
	if !linkLive {
		return GridUnknownState
	}
	return t.state
}

// Observe feeds one sample. A change is reported once the new signal has held
// for at least the debounce, measured on sample timestamps (polling is
// irregular), with At = time of its first observation.
//
// GridUnknown samples and samples taken while the link is not live carry no
// information: they neither confirm nor reset a pending change (an ATS
// switch-over typically makes the API time out right as an outage starts).
// A pending change is dropped only when a live sample contradicts it, or when
// no live sample was seen for longer than PendingExpiry. Confirmation always
// needs a live sample showing the new state at least the debounce after the
// first observation. From GridUnknownState (see NewTracker, Unconfirm) the
// change is reported with AcrossGap; a change from a known state is not.
func (t *Tracker) Observe(s model.Sample, linkLive bool) []PowerEvent {
	if !linkLive {
		return nil
	}
	if t.hasPending && s.At.Sub(t.lastLive) > PendingExpiry {
		t.hasPending = false
	}
	t.lastLive = s.At
	var target GridState
	switch s.Grid {
	case model.GridPresent:
		target = GridOn
	case model.GridAbsent:
		target = GridOff
	default:
		return nil
	}
	if target == t.state {
		t.hasPending = false
		return nil
	}
	if !t.hasPending || t.pending != target {
		t.pending, t.hasPending, t.since = target, true, s.At
		t.pendingSOC = nil
		if s.SOC != nil {
			v := *s.SOC
			t.pendingSOC = &v
		}
	}
	if s.At.Sub(t.since) < t.debounce {
		return nil
	}
	prev, wasOpen := t.state, t.open
	across := prev == GridUnknownState
	t.state, t.hasPending, t.open = target, false, target == GridOff
	ev := PowerEvent{At: t.since, ConfirmedAt: s.At, SOC: t.pendingSOC, AcrossGap: across}
	switch {
	case target == GridOff && !(across && wasOpen):
		ev.Kind = GridLost
	case target == GridOn && (prev == GridOff || across && wasOpen):
		ev.Kind = GridBack
	default:
		return nil // unknown to On without an open outage, or Off continued
	}
	return []PowerEvent{ev}
}

// Pending reports an observed grid change that is not confirmed yet (it has
// not held for the debounce): the poller watches it closely meanwhile.
func (t *Tracker) Pending() bool { return t.hasPending }

// PendingState is the unconfirmed change (its state and the time of its first
// observation); ok is false when nothing is pending. It says nothing about the
// link: the caller shows it only while the link is LIVE.
func (t *Tracker) PendingState() (state GridState, since time.Time, ok bool) {
	if !t.hasPending {
		return GridUnknownState, time.Time{}, false
	}
	return t.pending, t.since, true
}
