package link

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var t0 = time.Date(2026, 11, 21, 0, 40, 0, 0, time.UTC)

var venus = model.Profile{Model: "VenusE 3.0", NominalWh: 5120, MaxChargeW: 2500, MaxDischargeW: 2500, MinSOC: 10, Tested: true}

func f(v float64) *float64 { return &v }

func sample(at time.Time) model.Sample {
	return model.Sample{StationID: "st1", At: at, Source: model.SourceLAN, SOC: f(60), BatteryW: f(0), Grid: model.GridPresent}
}

func find(effs []Effect, k EffectKind) (Effect, bool) {
	for _, e := range effs {
		if e.Kind == k {
			return e, true
		}
	}
	return Effect{}, false
}

func count(effs []Effect, k EffectKind) int {
	n := 0
	for _, e := range effs {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func wantState(t *testing.T, m *Machine, want State) {
	t.Helper()
	if got := m.State(); got != want {
		t.Fatalf("state = %s, want %s", got, want)
	}
}

// liveAt returns a machine that went LIVE on a fresh sample taken at `at`.
func liveAt(t *testing.T, at time.Time, s model.Sample) *Machine {
	t.Helper()
	m := NewMachine(DefaultConfig(), nil, venus, at, false)
	s.At = at
	m.Observe(s, at)
	wantState(t, m, Live)
	return m
}

func TestDefaultConfig(t *testing.T) {
	want := Config{Fresh: 15 * time.Second, StaleToNoLink: 5 * time.Minute, ConnectTimeout: 30 * time.Second,
		BootConnectTimeout: 90 * time.Second, ResyncMax: 30 * time.Second}
	if got := DefaultConfig(); got != want {
		t.Fatalf("got %+v", got)
	}
}

func TestConnectingToLiveOnFreshSample(t *testing.T) {
	m := NewMachine(DefaultConfig(), nil, venus, t0, false)
	wantState(t, m, Connecting)
	effs := m.Observe(sample(t0.Add(2*time.Second)), t0.Add(2*time.Second))
	wantState(t, m, Live)
	sc, ok := find(effs, StateChanged)
	if !ok || sc.From != Connecting || sc.To != Live {
		t.Fatalf("effects %+v", effs)
	}
	if count(effs, OpenGap)+count(effs, CloseGap) != 0 {
		t.Fatalf("unexpected gap effects %+v", effs)
	}
}

func TestStaleSampleDoesNotMakeLive(t *testing.T) {
	m := NewMachine(DefaultConfig(), nil, venus, t0, false)
	if effs := m.Observe(sample(t0.Add(-time.Minute)), t0); len(effs) != 0 {
		t.Fatalf("effects %+v", effs)
	}
	wantState(t, m, Connecting)
}

func TestLiveToStaleAfter15s(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	if effs := m.Tick(t0.Add(15*time.Second), true); len(effs) != 0 {
		t.Fatalf("at 15s: %+v", effs)
	}
	wantState(t, m, Live)
	effs := m.Tick(t0.Add(16*time.Second), true)
	wantState(t, m, Stale)
	if sc, ok := find(effs, StateChanged); !ok || sc.From != Live || sc.To != Stale {
		t.Fatalf("effects %+v", effs)
	}
}

func TestStaleRecoversWithoutGap(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Tick(t0.Add(16*time.Second), true)
	wantState(t, m, Stale)
	at := t0.Add(2 * time.Minute)
	effs := m.Observe(sample(at), at)
	wantState(t, m, Live)
	d, ok := find(effs, LogDelay)
	if !ok || !d.At.Equal(t0) {
		t.Fatalf("LogDelay missing or wrong At: %+v", effs)
	}
	if count(effs, OpenGap)+count(effs, CloseGap) != 0 {
		t.Fatalf("delay must not be a gap: %+v", effs)
	}
}

func TestStaleToNoLinkOpensGapAfter5m(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Tick(t0.Add(16*time.Second), true)
	if effs := m.Tick(t0.Add(5*time.Minute+15*time.Second), true); len(effs) != 0 {
		t.Fatalf("still within 5m of STALE: %+v", effs)
	}
	wantState(t, m, Stale)
	effs := m.Tick(t0.Add(5*time.Minute+16*time.Second), true)
	wantState(t, m, NoLink)
	g, ok := find(effs, OpenGap)
	if !ok || g.Reason != store.GapNoLink || !g.At.Equal(t0) {
		t.Fatalf("OpenGap %+v in %+v", g, effs)
	}
	if m.Diagnosis() != DiagLANSilent {
		t.Fatalf("diagnosis %q", m.Diagnosis())
	}
	m.Tick(t0.Add(6*time.Minute), false)
	if m.Diagnosis() != DiagPCOffline {
		t.Fatalf("diagnosis %q", m.Diagnosis())
	}
	if effs := m.Tick(t0.Add(7*time.Minute), false); count(effs, OpenGap) != 0 {
		t.Fatalf("gap reopened: %+v", effs)
	}
}

func TestNoLinkToLikelyDead(t *testing.T) {
	s := sample(t0)
	s.Grid, s.SOC, s.BatteryW = model.GridAbsent, f(12), f(-300)
	m := liveAt(t, t0, s)
	m.Tick(t0.Add(16*time.Second), true)
	effs := m.Tick(t0.Add(5*time.Minute+16*time.Second), false)
	wantState(t, m, LikelyDead)
	g, ok := find(effs, OpenGap)
	if !ok || g.Reason != store.GapLikelyDead || !g.At.Equal(t0) {
		t.Fatalf("OpenGap %+v in %+v", g, effs)
	}
	if count(effs, OpenGap) != 1 {
		t.Fatalf("want one gap: %+v", effs)
	}
	if _, ok := find(effs, EnteredLikelyDead); !ok {
		t.Fatalf("no EnteredLikelyDead: %+v", effs)
	}
	if m.Diagnosis() != "" {
		t.Fatalf("diagnosis outside NO_LINK: %q", m.Diagnosis())
	}
}

func TestNoLinkBecomesLikelyDeadWhenForecastPasses(t *testing.T) {
	// SOC 40% of 5120 Wh, min 10% → 1536 Wh left; at 1536 W → empty after 1 h.
	s := sample(t0)
	s.Grid, s.SOC, s.BatteryW = model.GridAbsent, f(40), f(-1536)
	m := liveAt(t, t0, s)
	m.Tick(t0.Add(16*time.Second), true)
	m.Tick(t0.Add(6*time.Minute), true)
	wantState(t, m, NoLink)
	if effs := m.Tick(t0.Add(59*time.Minute), true); len(effs) != 0 {
		t.Fatalf("before forecast: %+v", effs)
	}
	effs := m.Tick(t0.Add(61*time.Minute), true)
	wantState(t, m, LikelyDead)
	d, ok := find(effs, EnteredLikelyDead)
	if !ok || !d.At.Equal(t0.Add(time.Hour)) {
		t.Fatalf("EnteredLikelyDead %+v in %+v", d, effs)
	}
	if count(effs, OpenGap) != 0 {
		t.Fatalf("gap already open, must not reopen: %+v", effs)
	}
}

func TestResyncThenLiveClosesGap(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Tick(t0.Add(16*time.Second), true)
	m.Tick(t0.Add(6*time.Minute), true)
	wantState(t, m, NoLink)
	back := t0.Add(20 * time.Minute)
	effs := m.Observe(sample(back), back)
	wantState(t, m, Resyncing)
	c, ok := find(effs, CloseGap)
	if !ok || !c.At.Equal(back) {
		t.Fatalf("CloseGap %+v in %+v", c, effs)
	}
	m.Observe(sample(back.Add(5*time.Second)), back.Add(5*time.Second))
	if effs := m.Tick(back.Add(10*time.Second), true); len(effs) != 0 {
		t.Fatalf("resync ended early: %+v", effs)
	}
	wantState(t, m, Resyncing)
	m.Observe(sample(back.Add(28*time.Second)), back.Add(28*time.Second))
	effs = m.Tick(back.Add(30*time.Second), true)
	wantState(t, m, Live)
	if sc, ok := find(effs, StateChanged); !ok || sc.From != Resyncing || sc.To != Live {
		t.Fatalf("effects %+v", effs)
	}
	if count(effs, CloseGap) != 0 {
		t.Fatalf("gap closed twice: %+v", effs)
	}
}

func TestSuspendOpensPcSleepGap(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Observe(sample(t0.Add(5*time.Second)), t0.Add(5*time.Second))
	sleep := t0.Add(7 * time.Second)
	effs := m.Suspend(sleep)
	wantState(t, m, Suspended)
	g, ok := find(effs, OpenGap)
	if !ok || g.Reason != store.GapPCSleep || !g.At.Equal(t0.Add(5*time.Second)) {
		t.Fatalf("OpenGap %+v in %+v", g, effs)
	}
	if effs := m.Tick(sleep.Add(time.Hour), false); len(effs) != 0 {
		t.Fatalf("tick while suspended: %+v", effs)
	}
	wake := t0.Add(2 * time.Hour)
	effs = m.Resume(wake)
	wantState(t, m, Connecting)
	if sc, ok := find(effs, StateChanged); !ok || sc.From != Suspended || sc.To != Connecting {
		t.Fatalf("effects %+v", effs)
	}
	// After resume the normal 30 s timeout applies, not the boot one.
	if m.Tick(wake.Add(20*time.Second), true); m.State() != Connecting {
		t.Fatalf("state %s", m.State())
	}
	first := wake.Add(25 * time.Second)
	effs = m.Observe(sample(first), first)
	wantState(t, m, Resyncing)
	c, ok := find(effs, CloseGap)
	if !ok || !c.At.Equal(first) {
		t.Fatalf("CloseGap %+v in %+v", c, effs)
	}
}

func TestResumeUsesNormalConnectTimeout(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Suspend(t0.Add(time.Second))
	wake := t0.Add(time.Hour)
	m.Resume(wake)
	effs := m.Tick(wake.Add(31*time.Second), true)
	// Old data exists and is far beyond 5 min; pc_sleep gap is already open.
	wantState(t, m, NoLink)
	if count(effs, OpenGap) != 0 {
		t.Fatalf("pc_sleep gap already open: %+v", effs)
	}
}

func TestShortSleepThenStaleClosesPcSleepGap(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Suspend(t0.Add(time.Second))
	wake := t0.Add(time.Minute)
	m.Resume(wake)
	m.Tick(wake.Add(31*time.Second), true)
	wantState(t, m, Stale) // data 91 s old: within 5 min, but the pc_sleep gap is open
	back := wake.Add(40 * time.Second)
	effs := m.Observe(sample(back), back)
	wantState(t, m, Resyncing)
	if c, ok := find(effs, CloseGap); !ok || !c.At.Equal(back) {
		t.Fatalf("effects %+v", effs)
	}
	if count(effs, LogDelay) != 0 {
		t.Fatalf("a gap is not a delay: %+v", effs)
	}
}

func TestBootUses90sConnectTimeout(t *testing.T) {
	m := NewMachine(DefaultConfig(), nil, venus, t0, true)
	if effs := m.Tick(t0.Add(60*time.Second), true); len(effs) != 0 {
		t.Fatalf("at 60s: %+v", effs)
	}
	wantState(t, m, Connecting)
	effs := m.Tick(t0.Add(91*time.Second), false)
	wantState(t, m, NoLink)
	if count(effs, OpenGap) != 0 {
		t.Fatalf("no data ever — nothing to gap: %+v", effs)
	}
	if m.Diagnosis() != DiagPCOffline {
		t.Fatalf("diagnosis %q", m.Diagnosis())
	}
	// Next poll tick with still no data ever: must not panic, stays NO_LINK.
	if effs := m.Tick(t0.Add(96*time.Second), true); len(effs) != 0 {
		t.Fatalf("second tick: %+v", effs)
	}
	wantState(t, m, NoLink)
}

func TestNonBootUses30sConnectTimeout(t *testing.T) {
	m := NewMachine(DefaultConfig(), nil, venus, t0, false)
	m.Tick(t0.Add(29*time.Second), true)
	wantState(t, m, Connecting)
	m.Tick(t0.Add(31*time.Second), true)
	wantState(t, m, NoLink)
	if effs := m.Tick(t0.Add(36*time.Second), true); len(effs) != 0 {
		t.Fatalf("second tick: %+v", effs)
	}
	wantState(t, m, NoLink)
}

func TestConnectTimeoutWithRecentOldDataGoesStale(t *testing.T) {
	// PC rebooted: last sample 1 min old, no fresh data within the boot window.
	last := sample(t0.Add(-time.Minute))
	m := NewMachine(DefaultConfig(), &last, venus, t0, true)
	effs := m.Tick(t0.Add(91*time.Second), true)
	wantState(t, m, Stale)
	if count(effs, OpenGap) != 0 {
		t.Fatalf("effects %+v", effs)
	}
	// Gap opens once the data is 5 min past fresh, measured from the last sample.
	effs = m.Tick(last.At.Add(5*time.Minute+16*time.Second), true)
	wantState(t, m, NoLink)
	if g, ok := find(effs, OpenGap); !ok || g.Reason != store.GapNoLink || !g.At.Equal(last.At) {
		t.Fatalf("effects %+v", effs)
	}
}

func TestStartupAfterShortAbsenceHasNoGap(t *testing.T) {
	last := sample(t0.Add(-2 * time.Minute))
	m := NewMachine(DefaultConfig(), &last, venus, t0, true)
	effs := m.Tick(t0.Add(time.Second), true)
	effs = append(effs, m.Observe(sample(t0.Add(10*time.Second)), t0.Add(10*time.Second))...)
	wantState(t, m, Live)
	if count(effs, OpenGap)+count(effs, CloseGap) != 0 {
		t.Fatalf("effects %+v", effs)
	}
}

func TestStartupAfterLongAbsenceOpensAppNotRunningGap(t *testing.T) {
	last := sample(t0.Add(-3 * time.Hour))
	m := NewMachine(DefaultConfig(), &last, venus, t0, false)
	effs := m.Tick(t0.Add(time.Second), true)
	g, ok := find(effs, OpenGap)
	if !ok || g.Reason != store.GapAppNotRunning || !g.At.Equal(last.At) {
		t.Fatalf("OpenGap %+v in %+v", g, effs)
	}
	if _, ok := find(effs, EnteredLikelyDead); ok {
		t.Fatalf("grid was present — not dead: %+v", effs)
	}
	wantState(t, m, Connecting)
	if effs := m.Tick(t0.Add(2*time.Second), true); len(effs) != 0 {
		t.Fatalf("startup effects repeated: %+v", effs)
	}
	effs = m.Observe(sample(t0.Add(5*time.Second)), t0.Add(5*time.Second))
	wantState(t, m, Resyncing)
	if c, ok := find(effs, CloseGap); !ok || !c.At.Equal(t0.Add(5*time.Second)) {
		t.Fatalf("effects %+v", effs)
	}
}

func TestStartupPendingEffectsDrainOnObserve(t *testing.T) {
	last := sample(t0.Add(-3 * time.Hour))
	m := NewMachine(DefaultConfig(), &last, venus, t0, false)
	effs := m.Observe(sample(t0.Add(time.Second)), t0.Add(time.Second))
	if count(effs, OpenGap) != 1 || count(effs, CloseGap) != 1 {
		t.Fatalf("effects %+v", effs)
	}
	if effs[0].Kind != OpenGap {
		t.Fatalf("OpenGap must precede CloseGap: %+v", effs)
	}
}

func TestStartupRetroLikelyDead(t *testing.T) {
	last := sample(t0.Add(-3 * time.Hour))
	last.Grid, last.SOC, last.BatteryW = model.GridAbsent, f(11), f(-200)
	m := NewMachine(DefaultConfig(), &last, venus, t0, true)
	effs := m.Tick(t0.Add(time.Second), true)
	g, ok := find(effs, OpenGap)
	if !ok || g.Reason != store.GapLikelyDead || !g.At.Equal(last.At) {
		t.Fatalf("OpenGap %+v in %+v", g, effs)
	}
	// 1% above min of 5120 Wh = 51.2 Wh at 200 W → ≈ 15m22s after the last sample.
	d, ok := find(effs, EnteredLikelyDead)
	want := last.At.Add(time.Duration(51.2 / 200 * float64(time.Hour)))
	if diff := d.At.Sub(want); !ok || diff < -time.Second || diff > time.Second {
		t.Fatalf("EnteredLikelyDead %+v (want At %v) in %+v", d, want, effs)
	}
	wantState(t, m, Connecting)
}

func TestDeadCondition(t *testing.T) {
	now := t0.Add(time.Hour)
	before, after := t0.Add(30*time.Minute), t0.Add(2*time.Hour)
	cases := []struct {
		name    string
		grid    model.GridSignal
		soc     *float64
		emptyAt *time.Time
		want    bool
	}{
		{"absent low soc", model.GridAbsent, f(15), nil, true},
		{"absent soc above band", model.GridAbsent, f(15.5), nil, false},
		{"absent forecast passed", model.GridAbsent, f(50), &before, true},
		{"absent forecast exactly now", model.GridAbsent, f(50), &now, true},
		{"absent forecast ahead", model.GridAbsent, f(50), &after, false},
		{"absent soc unknown", model.GridAbsent, nil, nil, false},
		{"present low soc", model.GridPresent, f(5), &before, false},
		{"unknown grid low soc", model.GridUnknown, f(5), &before, false},
	}
	for _, c := range cases {
		s := sample(t0)
		s.Grid, s.SOC = c.grid, c.soc
		if got := DeadCondition(s, venus, c.emptyAt, now); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestSuspendFromNoLinkKeepsExistingGap(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.Tick(t0.Add(16*time.Second), true)
	m.Tick(t0.Add(6*time.Minute), true)
	effs := m.Suspend(t0.Add(7 * time.Minute))
	wantState(t, m, Suspended)
	if count(effs, OpenGap) != 0 {
		t.Fatalf("no_link gap is already open: %+v", effs)
	}
}

func TestOutOfOrderSampleIgnored(t *testing.T) {
	m := liveAt(t, t0.Add(10*time.Second), sample(t0))
	m.Observe(sample(t0.Add(5*time.Second)), t0.Add(11*time.Second))
	// Data age must still count from t0+10s, not the late t0+5s sample.
	m.Tick(t0.Add(24*time.Second), true)
	wantState(t, m, Live)
}

func TestStartupRetroLikelyDeadEmittedOnce(t *testing.T) {
	last := sample(t0.Add(-3 * time.Hour))
	last.Grid, last.SOC, last.BatteryW = model.GridAbsent, f(11), f(-200)
	m := NewMachine(DefaultConfig(), &last, venus, t0, true)
	effs := m.Tick(t0.Add(time.Second), true)
	effs = append(effs, m.Tick(t0.Add(91*time.Second), true)...)
	effs = append(effs, m.Tick(t0.Add(96*time.Second), true)...)
	wantState(t, m, LikelyDead)
	if n := count(effs, EnteredLikelyDead); n != 1 {
		t.Fatalf("EnteredLikelyDead x%d: %+v", n, effs)
	}
	if n := count(effs, OpenGap); n != 1 {
		t.Fatalf("OpenGap x%d: %+v", n, effs)
	}
}

func TestStartupClockJumpedBackAcceptsFreshData(t *testing.T) {
	// Stored sample is "from the future": the wall clock went back across a restart.
	last := sample(t0)
	now := t0.Add(-time.Hour)
	m := NewMachine(DefaultConfig(), &last, venus, now, true)
	m.Observe(sample(now.Add(time.Second)), now.Add(time.Second))
	wantState(t, m, Live)
	m.Tick(now.Add(20*time.Second), true)
	wantState(t, m, Stale)
	m.Observe(sample(now.Add(2*time.Minute)), now.Add(2*time.Minute))
	wantState(t, m, Live)
}

func TestStartupClockJumpedBackStillTimesOut(t *testing.T) {
	// Future stored sample and no new data: age must not stay negative and block NO_LINK.
	last := sample(t0)
	now := t0.Add(-time.Hour)
	m := NewMachine(DefaultConfig(), &last, venus, now, false)
	m.Tick(now.Add(31*time.Second), true)
	wantState(t, m, Stale)
	effs := m.Tick(now.Add(6*time.Minute), true)
	wantState(t, m, NoLink)
	if g, ok := find(effs, OpenGap); !ok || g.At.After(now.Add(6*time.Minute)) {
		t.Fatalf("gap must not start in the future: %+v", effs)
	}
}

// I-4: a reboot shorter than 5 min is not a gap, but the first sample after
// start must report the hole (At = the last sample before it).
func TestStartupAfterShortAbsenceReportsStartDelay(t *testing.T) {
	last := sample(t0.Add(-2 * time.Minute))
	m := NewMachine(DefaultConfig(), &last, venus, t0, true)
	at := t0.Add(10 * time.Second)
	effs := m.Observe(sample(at), at)
	wantState(t, m, Live)
	d, ok := find(effs, LogStartDelay)
	if !ok || !d.At.Equal(last.At) {
		t.Fatalf("LogStartDelay missing or wrong At: %+v", effs)
	}
	if count(effs, OpenGap)+count(effs, CloseGap)+count(effs, LogDelay) != 0 {
		t.Fatalf("a start delay is neither a gap nor a plain delay: %+v", effs)
	}
	// Only the first sample reports it.
	if effs := m.Observe(sample(at.Add(5*time.Second)), at.Add(5*time.Second)); len(effs) != 0 {
		t.Fatalf("second sample: %+v", effs)
	}
}

// The same hole when the station answers only after the connect timeout
// (CONNECTING to STALE to LIVE) is still a start delay, reported once.
func TestStartDelayThroughStale(t *testing.T) {
	last := sample(t0.Add(-2 * time.Minute))
	m := NewMachine(DefaultConfig(), &last, venus, t0, false)
	m.Tick(t0.Add(31*time.Second), true)
	wantState(t, m, Stale)
	at := t0.Add(40 * time.Second)
	effs := m.Observe(sample(at), at)
	wantState(t, m, Live)
	if d, ok := find(effs, LogStartDelay); !ok || !d.At.Equal(last.At) || count(effs, LogDelay) != 0 {
		t.Fatalf("effects %+v, want one LogStartDelay from the stored sample", effs)
	}
}

func TestNoStartDelayForQuickRestartOrFirstRun(t *testing.T) {
	// Restart within the fresh threshold: no hole.
	last := sample(t0.Add(-5 * time.Second))
	m := NewMachine(DefaultConfig(), &last, venus, t0, false)
	at := t0.Add(5 * time.Second)
	if effs := m.Observe(sample(at), at); count(effs, LogStartDelay) != 0 {
		t.Fatalf("10 s hole: %+v", effs)
	}
	// No stored sample: nothing before the hole.
	m = NewMachine(DefaultConfig(), nil, venus, t0, false)
	if effs := m.Observe(sample(at), at); count(effs, LogStartDelay) != 0 {
		t.Fatalf("first run: %+v", effs)
	}
	// A gap (5 min or more) is reported as a gap, not as a delay.
	old := sample(t0.Add(-3 * time.Hour))
	m = NewMachine(DefaultConfig(), &old, venus, t0, false)
	if effs := m.Observe(sample(at), at); count(effs, LogStartDelay) != 0 || count(effs, CloseGap) != 1 {
		t.Fatalf("long absence: %+v", effs)
	}
	// A delay while running stays a plain delay.
	m = liveAt(t, t0, sample(t0))
	m.Tick(t0.Add(16*time.Second), true)
	back := t0.Add(2 * time.Minute)
	if effs := m.Observe(sample(back), back); count(effs, LogStartDelay) != 0 || count(effs, LogDelay) != 1 {
		t.Fatalf("running delay: %+v", effs)
	}
}

// SetFresh moves the LIVE → STALE threshold (3 × the current poll interval)
// and with it STALE → NO_LINK (Fresh + StaleToNoLink); StaleToNoLink stays.
func TestSetFreshMovesStaleAndNoLink(t *testing.T) {
	m := liveAt(t, t0, sample(t0))
	m.SetFresh(60 * time.Second)
	m.Tick(t0.Add(60*time.Second), true)
	wantState(t, m, Live)
	m.Tick(t0.Add(61*time.Second), true)
	wantState(t, m, Stale)
	m.Tick(t0.Add(6*time.Minute), true)
	wantState(t, m, Stale)
	effs := m.Tick(t0.Add(6*time.Minute+time.Second), true)
	wantState(t, m, NoLink)
	if g, ok := find(effs, OpenGap); !ok || !g.At.Equal(t0) || g.Reason != store.GapNoLink {
		t.Fatalf("effects %+v", effs)
	}

	// Back to 15 s: a sample 20 s old is no longer fresh.
	m2 := liveAt(t, t0, sample(t0))
	m2.SetFresh(60 * time.Second)
	m2.SetFresh(15 * time.Second)
	m2.Tick(t0.Add(16*time.Second), true)
	wantState(t, m2, Stale)
}
