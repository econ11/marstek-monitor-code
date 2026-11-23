package detect

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func smp(at time.Time, g model.GridSignal) model.Sample {
	soc := 50.0
	return model.Sample{At: at, Grid: g, SOC: &soc}
}

// feed observes g once per second at start+from .. start+to (inclusive) and
// returns the events keyed by the second offset at which they were emitted.
func feed(tr *Tracker, start time.Time, g model.GridSignal, from, to int) map[int][]PowerEvent {
	out := map[int][]PowerEvent{}
	for i := from; i <= to; i++ {
		if ev := tr.Observe(smp(start.Add(time.Duration(i)*time.Second), g), true); len(ev) > 0 {
			out[i] = ev
		}
	}
	return out
}

func TestGridLostAfterDebounce(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	ev := feed(tr, t0, model.GridAbsent, 0, 40)
	if len(ev) != 1 || len(ev[30]) != 1 {
		t.Fatalf("want exactly one event at +30s, got %v", ev)
	}
	e := ev[30][0]
	if e.Kind != GridLost || !e.At.Equal(t0) {
		t.Fatalf("event = %+v, want GridLost at t0", e)
	}
	if e.SOC == nil || *e.SOC != 50 {
		t.Fatalf("SOC = %v", e.SOC)
	}
	if got := tr.State(true); got != GridOff {
		t.Fatalf("State = %v", got)
	}
}

func TestDebounceIgnoresShortFlicker(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	if ev := feed(tr, t0, model.GridAbsent, 0, 19); len(ev) != 0 {
		t.Fatalf("events during flicker: %v", ev)
	}
	if ev := feed(tr, t0, model.GridPresent, 20, 120); len(ev) != 0 {
		t.Fatalf("events after flicker: %v", ev)
	}
	if got := tr.State(true); got != GridOn {
		t.Fatalf("State = %v", got)
	}
}

func TestGridBackClosesOutage(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	feed(tr, t0, model.GridAbsent, 0, 100)
	back := t0.Add(200 * time.Second)
	ev := feed(tr, back, model.GridPresent, 0, 40)
	if len(ev) != 1 || len(ev[30]) != 1 {
		t.Fatalf("want one event at +30s, got %v", ev)
	}
	if e := ev[30][0]; e.Kind != GridBack || !e.At.Equal(back) {
		t.Fatalf("event = %+v", e)
	}
	if tr.State(true) != GridOn {
		t.Fatal("not GridOn")
	}
}

func TestUnknownWhenNotLive(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0, model.GridPresent, 0, 60)
	if tr.State(true) != GridOn {
		t.Fatal("want GridOn while live")
	}
	if got := tr.State(false); got != GridUnknownState {
		t.Fatalf("State(false) = %v", got)
	}
}

func TestRestartDuringOutageKeepsSingleOpenOutage(t *testing.T) {
	tr := NewTracker(30*time.Second, &store.Outage{Start: t0.Add(-time.Hour)})
	if tr.State(true) != GridUnknownState {
		t.Fatal("restart with open outage must start unknown")
	}
	if ev := feed(tr, t0, model.GridAbsent, 0, 120); len(ev) != 0 {
		t.Fatalf("duplicate GridLost: %v", ev)
	}
	ev := feed(tr, t0.Add(200*time.Second), model.GridPresent, 0, 40)
	if len(ev) != 1 || ev[30][0].Kind != GridBack || ev[30][0].AcrossGap {
		t.Fatalf("want GridBack at +30s, got %v", ev)
	}
}

func TestUnknownSamplesNeitherConfirmNorReset(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	tr.Observe(smp(t0, model.GridAbsent), true)
	// An unknown sample in the middle must not reset the pending change...
	if ev := tr.Observe(smp(t0.Add(10*time.Second), model.GridUnknown), true); len(ev) != 0 {
		t.Fatalf("unknown confirmed: %v", ev)
	}
	// ...the next Absent sample at +35 s (irregular polling) confirms, and At
	// is still the first observation.
	ev := tr.Observe(smp(t0.Add(35*time.Second), model.GridAbsent), true)
	if len(ev) != 1 || ev[0].Kind != GridLost || !ev[0].At.Equal(t0) {
		t.Fatalf("got %v", ev)
	}
}

func TestUnknownAloneNeverConfirms(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	tr.Observe(smp(t0, model.GridAbsent), true)
	if ev := tr.Observe(smp(t0.Add(60*time.Second), model.GridUnknown), true); len(ev) != 0 {
		t.Fatalf("unknown sample confirmed a change: %v", ev)
	}
}

func TestIrregularSamplesDebounceByTime(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	tr.Observe(smp(t0.Add(-time.Minute), model.GridPresent), true)
	tr.Observe(smp(t0.Add(-50*time.Second), model.GridPresent), true)
	tr.Observe(smp(t0, model.GridAbsent), true)
	if ev := tr.Observe(smp(t0.Add(9*time.Second), model.GridAbsent), true); len(ev) != 0 {
		t.Fatalf("early: %v", ev)
	}
	ev := tr.Observe(smp(t0.Add(31*time.Second), model.GridAbsent), true)
	if len(ev) != 1 || !ev[0].At.Equal(t0) {
		t.Fatalf("got %v", ev)
	}
}

func TestNotLiveObservationNeitherConfirmsNorResets(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	tr.Observe(smp(t0, model.GridAbsent), true)
	if ev := tr.Observe(smp(t0.Add(40*time.Second), model.GridAbsent), false); len(ev) != 0 {
		t.Fatalf("event while link not live: %v", ev)
	}
	ev := tr.Observe(smp(t0.Add(50*time.Second), model.GridAbsent), true)
	if len(ev) != 1 || ev[0].Kind != GridLost || !ev[0].At.Equal(t0) {
		t.Fatalf("got %v, want GridLost at t0", ev)
	}
}

func TestPendingExpiresWithoutLiveSamples(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	tr.Observe(smp(t0, model.GridAbsent), true)
	tr.Observe(smp(t0.Add(time.Minute), model.GridAbsent), false)
	late := t0.Add(PendingExpiry + time.Second)
	if ev := tr.Observe(smp(late, model.GridAbsent), true); len(ev) != 0 {
		t.Fatalf("stale evidence confirmed: %v", ev)
	}
	ev := tr.Observe(smp(late.Add(30*time.Second), model.GridAbsent), true)
	if len(ev) != 1 || !ev[0].At.Equal(late) {
		t.Fatalf("got %v, want GridLost at %v", ev, late)
	}
}

func TestInitialPresentIsSilent(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	if ev := feed(tr, t0, model.GridPresent, 0, 120); len(ev) != 0 {
		t.Fatalf("events: %v", ev)
	}
	if tr.State(true) != GridOn {
		t.Fatal("not GridOn")
	}
}

func fp(v float64) *float64 { return &v }

func TestModeDeadband(t *testing.T) {
	cases := []struct {
		name string
		p    *float64
		soc  *float64
		want BatteryMode
	}{
		{"15 idle", fp(15), fp(50), Idle},
		{"20 idle edge", fp(20), fp(50), Idle},
		{"25 charging", fp(25), fp(50), Charging},
		{"-25 discharging", fp(-25), fp(50), Discharging},
		{"-20 idle edge", fp(-20), fp(50), Idle},
		{"cutoff", fp(0), fp(10), Cutoff},
		{"below min cutoff", fp(0), fp(5), Cutoff},
		{"charging at min is charging", fp(500), fp(10), Charging},
		{"nil power unknown", nil, fp(50), ModeUnknown},
		{"nil soc not cutoff", fp(0), nil, Idle},
	}
	for _, c := range cases {
		if got := Mode(c.p, c.soc, 10); got != c.want {
			t.Errorf("%s: Mode = %v, want %v", c.name, got, c.want)
		}
	}
}

// I-1: the event says when the change was confirmed, not only when it was
// first seen; the two differ by more than the debounce when the station was
// silent in between.
func TestEventCarriesConfirmationTime(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	tr.Observe(smp(t0, model.GridAbsent), true)
	confirmed := t0.Add(135 * time.Second)
	ev := tr.Observe(smp(confirmed, model.GridAbsent), true)
	if len(ev) != 1 || ev[0].Kind != GridLost || !ev[0].At.Equal(t0) || !ev[0].ConfirmedAt.Equal(confirmed) {
		t.Fatalf("got %+v, want GridLost at t0 confirmed at +135 s", ev)
	}
	back := confirmed.Add(time.Minute)
	tr.Observe(smp(back, model.GridPresent), true)
	ev = tr.Observe(smp(back.Add(40*time.Second), model.GridPresent), true)
	if len(ev) != 1 || ev[0].Kind != GridBack || !ev[0].At.Equal(back) || !ev[0].ConfirmedAt.Equal(back.Add(40*time.Second)) {
		t.Fatalf("got %+v, want GridBack at %v confirmed 40 s later", ev, back)
	}
}

// I-3 (c): after a restart the grid is unknown until observed; the open
// outage is continued silently by the first confirmed Absent.
func TestNewTrackerWithOpenOutageStartsUnknown(t *testing.T) {
	tr := NewTracker(30*time.Second, &store.Outage{})
	if got := tr.State(true); got != GridUnknownState {
		t.Fatalf("State = %v, want unknown", got)
	}
	if ev := feed(tr, t0, model.GridAbsent, 0, 30); len(ev) != 0 {
		t.Fatalf("events: %v", ev)
	}
	if got := tr.State(true); got != GridOff {
		t.Fatalf("State = %v, want GridOff", got)
	}
}

func TestGridBackAfterRestartIsAcrossGap(t *testing.T) {
	tr := NewTracker(30*time.Second, &store.Outage{Start: t0.Add(-time.Hour)})
	ev := feed(tr, t0, model.GridPresent, 0, 30)
	if len(ev) != 1 || len(ev[30]) != 1 {
		t.Fatalf("want one event at +30s, got %v", ev)
	}
	e := ev[30][0]
	if e.Kind != GridBack || !e.AcrossGap || !e.At.Equal(t0) {
		t.Fatalf("event = %+v, want GridBack{AcrossGap} at t0", e)
	}
}

func TestUnconfirmMakesNextChangeAcrossGap(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	if tr.State(true) != GridOn {
		t.Fatal("GridOn not confirmed")
	}
	tr.Unconfirm()
	if got := tr.State(true); got != GridUnknownState {
		t.Fatalf("State after Unconfirm = %v", got)
	}
	ev := feed(tr, t0, model.GridAbsent, 0, 30)
	if len(ev) != 1 || len(ev[30]) != 1 || ev[30][0].Kind != GridLost || !ev[30][0].AcrossGap {
		t.Fatalf("want GridLost{AcrossGap} at +30s, got %v", ev)
	}
}

func TestAssumeContinuesWithoutEvent(t *testing.T) {
	tr := NewTracker(30*time.Second, &store.Outage{Start: t0.Add(-time.Hour)})
	tr.Assume(GridOff)
	if got := tr.State(true); got != GridOff {
		t.Fatalf("State = %v, want GridOff", got)
	}
	ev := feed(tr, t0, model.GridPresent, 0, 30)
	if len(ev) != 1 || len(ev[30]) != 1 || ev[30][0].Kind != GridBack || ev[30][0].AcrossGap {
		t.Fatalf("want GridBack{AcrossGap:false} at +30s, got %v", ev)
	}
}

func TestContiguousChangeIsNotAcrossGap(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	ev := feed(tr, t0, model.GridAbsent, 0, 30)
	if len(ev) != 1 || len(ev[30]) != 1 || ev[30][0].Kind != GridLost || ev[30][0].AcrossGap {
		t.Fatalf("want GridLost{AcrossGap:false} at +30s, got %v", ev)
	}
}

// Without an open outage the first confirmed Absent is GridLost across the gap.
func TestFirstAbsentWithoutOpenOutageIsAcrossGap(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	ev := feed(tr, t0, model.GridAbsent, 0, 30)
	if len(ev) != 1 || len(ev[30]) != 1 || ev[30][0].Kind != GridLost || !ev[30][0].AcrossGap {
		t.Fatalf("want GridLost{AcrossGap} at +30s, got %v", ev)
	}
}

// Unconfirm drops a pending change (its evidence predates the gap) and keeps
// the open outage, so the next confirmed Present still closes it.
func TestUnconfirmDropsPendingAndKeepsOpenOutage(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	feed(tr, t0, model.GridAbsent, 0, 30) // GridLost: outage open
	tr.Observe(smp(t0.Add(40*time.Second), model.GridPresent), true)
	tr.Unconfirm()
	after := t0.Add(50 * time.Second)
	ev := feed(tr, after, model.GridPresent, 0, 30)
	if len(ev) != 1 || len(ev[30]) != 1 || ev[30][0].Kind != GridBack || !ev[30][0].AcrossGap || !ev[30][0].At.Equal(after) {
		t.Fatalf("want GridBack{AcrossGap} at %v, confirmed at +30s, got %v", after, ev)
	}
}

func TestPendingReportsUnconfirmedChange(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	if tr.State(true) != GridOn || tr.Pending() {
		t.Fatalf("after confirmed Present: state %v, Pending %v", tr.State(true), tr.Pending())
	}
	feed(tr, t0, model.GridAbsent, 0, 10)
	if !tr.Pending() {
		t.Fatal("Absent for 10 s: Pending() = false, want true")
	}
	ev := feed(tr, t0, model.GridAbsent, 11, 30)
	if len(ev[30]) != 1 || ev[30][0].Kind != GridLost {
		t.Fatalf("events = %v, want GridLost at +30s", ev)
	}
	if tr.Pending() {
		t.Fatal("after confirmation: Pending() = true, want false")
	}
}

func TestCutoffWinsAtFloor(t *testing.T) {
	p, soc := -15.0, 10.0
	if got := Mode(&p, &soc, 10); got != Cutoff {
		t.Fatalf("Mode = %v, want Cutoff", got)
	}
}

func TestFlickerOffPresentOffKeepsOneOutage(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	if ev := feed(tr, t0, model.GridAbsent, 0, 40); len(ev) != 1 || len(ev[30]) != 1 || ev[30][0].Kind != GridLost {
		t.Fatalf("want GridLost at +30s, got %v", ev)
	}
	if ev := feed(tr, t0, model.GridPresent, 41, 60); len(ev) != 0 {
		t.Fatalf("events during a 20 s Present: %v", ev)
	}
	if ev := feed(tr, t0, model.GridAbsent, 61, 180); len(ev) != 0 {
		t.Fatalf("events after the flicker: %v", ev)
	}
	if got := tr.State(true); got != GridOff {
		t.Fatalf("State = %v", got)
	}
}

func TestRestartWithShortPresentKeepsOutage(t *testing.T) {
	tr := NewTracker(30*time.Second, &store.Outage{Start: t0.Add(-time.Hour)})
	if ev := feed(tr, t0, model.GridPresent, 0, 9); len(ev) != 0 {
		t.Fatalf("events during a 10 s Present: %v", ev)
	}
	if ev := feed(tr, t0, model.GridAbsent, 10, 120); len(ev) != 0 {
		t.Fatalf("want no events (outage continues), got %v", ev)
	}
	if got := tr.State(true); got != GridOff {
		t.Fatalf("State = %v", got)
	}
}

func TestPendingStateLifecycle(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59) // confirmed On
	if _, _, ok := tr.PendingState(); ok {
		t.Fatal("pending before any contradicting sample")
	}
	// First contradicting live sample: pending Off since t0.
	tr.Observe(smp(t0, model.GridAbsent), true)
	st, since, ok := tr.PendingState()
	if !ok || st != GridOff || !since.Equal(t0) {
		t.Fatalf("PendingState = %v %v %v, want Off since t0", st, since, ok)
	}
	// Still pending (and since unchanged) mid-debounce.
	tr.Observe(smp(t0.Add(10*time.Second), model.GridAbsent), true)
	if _, s2, ok := tr.PendingState(); !ok || !s2.Equal(t0) {
		t.Fatalf("pending lost or since moved: %v %v", s2, ok)
	}
	// A contradicting sample (grid back) clears it.
	tr.Observe(smp(t0.Add(11*time.Second), model.GridPresent), true)
	if _, _, ok := tr.PendingState(); ok {
		t.Fatal("pending survived a contradicting sample")
	}
	// Confirmation clears it.
	feed(tr, t0.Add(20*time.Second), model.GridAbsent, 0, 31)
	if _, _, ok := tr.PendingState(); ok {
		t.Fatal("pending survived confirmation")
	}
}

func TestPendingStateFromUnknown(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	tr.Observe(smp(t0, model.GridPresent), true)
	if st, _, ok := tr.PendingState(); !ok || st != GridOn {
		t.Fatalf("PendingState = %v %v, want On pending from unknown", st, ok)
	}
}

func TestPendingStateNotLiveNeverStarts(t *testing.T) {
	tr := NewTracker(30*time.Second, nil)
	feed(tr, t0.Add(-60*time.Second), model.GridPresent, 0, 59)
	tr.Observe(smp(t0, model.GridAbsent), false)
	if _, _, ok := tr.PendingState(); ok {
		t.Fatal("pending started from a non-live sample")
	}
}
