package notify

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/model"
)

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func f(v float64) *float64 { return &v }

func settings() Settings {
	return Settings{LowThreshold: 20, QuietEnabled: true, QuietStart: "23:00", QuietEnd: "07:00"}
}

// newPolicy starts well before t0 so the post-start link quiet period is over.
func newPolicy(s Settings) *Policy { return NewPolicy(s, t0.Add(-time.Hour)) }

func lost(at time.Time) detect.PowerEvent {
	return detect.PowerEvent{Kind: detect.GridLost, At: at, SOC: f(60)}
}
func back(at time.Time) detect.PowerEvent {
	return detect.PowerEvent{Kind: detect.GridBack, At: at, SOC: f(60)}
}

func sample(soc float64, at time.Time) model.Sample {
	return model.Sample{StationID: "st1", At: at, SOC: f(soc)}
}

func TestNoToastWhenNotLiveOrOld(t *testing.T) {
	p := newPolicy(settings())
	if got := p.OnPower(lost(t0), 1, false, t0.Add(time.Second)); len(got) != 0 {
		t.Fatalf("not live: %v", got)
	}
	if got := p.OnPower(lost(t0), 1, true, t0.Add(3*time.Minute)); len(got) != 0 {
		t.Fatalf("old event: %v", got)
	}
	if got := p.OnSample(sample(10, t0), detect.Discharging, false, t0); len(got) != 0 {
		t.Fatalf("not live sample: %v", got)
	}
	if got := p.OnPower(lost(t0), 1, true, t0.Add(time.Minute)); len(got) != 1 {
		t.Fatalf("fresh live event should toast: %v", got)
	}
}

func TestLowChargeHysteresis(t *testing.T) {
	p := newPolicy(settings())
	n := func(soc float64, m detect.BatteryMode) int {
		return len(p.OnSample(sample(soc, t0), m, true, t0))
	}
	if n(21, detect.Discharging) != 0 {
		t.Fatal("21 should not toast")
	}
	if n(19, detect.Discharging) != 1 {
		t.Fatal("21->19 should toast once")
	}
	if n(19, detect.Discharging) != 0 {
		t.Fatal("repeat 19 should not toast")
	}
	if n(21, detect.Discharging) != 0 || n(19, detect.Discharging) != 0 {
		t.Fatal("19->21->19 must not re-toast")
	}
	if n(25, detect.Charging) != 0 {
		t.Fatal("rearm sample should not toast")
	}
	if n(19, detect.Discharging) != 1 {
		t.Fatal("after >=25 must toast again")
	}
}

func TestLowChargeOnlyWhileDischarging(t *testing.T) {
	p := newPolicy(settings())
	if got := p.OnSample(sample(10, t0), detect.Charging, true, t0); len(got) != 0 {
		t.Fatalf("charging: %v", got)
	}
	if got := p.OnSample(sample(10, t0), detect.ModeUnknown, true, t0); len(got) != 0 {
		t.Fatalf("unknown mode: %v", got)
	}
}

func TestLowChargeBypassesQuietAndDND(t *testing.T) {
	night := time.Date(2026, 11, 22, 2, 0, 0, 0, time.UTC)
	p := newPolicy(settings())
	until := night.Add(time.Hour)
	p.SetDND(&until)
	got := p.OnSample(sample(10, night), detect.Discharging, true, night)
	if len(got) != 1 || got[0].Kind != LowCharge || got[0].Silent {
		t.Fatalf("got %+v", got)
	}
	if got[0].StationID != "st1" {
		t.Fatalf("station: %q", got[0].StationID)
	}
}

func TestQuietHoursAcrossMidnightAreSilent(t *testing.T) {
	for _, c := range []struct {
		h      int
		silent bool
	}{{2, true}, {12, false}, {23, true}, {7, false}, {6, true}} {
		now := time.Date(2026, 11, 22, c.h, 0, 0, 0, time.UTC)
		p := NewPolicy(settings(), now.Add(-time.Hour))
		got := p.OnPower(lost(now), 1, true, now)
		if len(got) != 1 || got[0].Silent != c.silent {
			t.Fatalf("hour %d: got %+v want silent=%v", c.h, got, c.silent)
		}
	}
	s := settings()
	s.QuietEnabled = false
	now := time.Date(2026, 11, 22, 2, 0, 0, 0, time.UTC)
	got := NewPolicy(s, now.Add(-time.Hour)).OnPower(lost(now), 1, true, now)
	if got[0].Silent {
		t.Fatal("quiet disabled must not silence")
	}
}

func TestDNDSuppressesAsSilent(t *testing.T) {
	p := newPolicy(settings())
	until := t0.Add(time.Hour)
	p.SetDND(&until)
	if p.DNDUntil() == nil || !p.DNDUntil().Equal(until) {
		t.Fatal("DNDUntil")
	}
	got := p.OnPower(lost(t0), 1, true, t0)
	if len(got) != 1 || !got[0].Silent {
		t.Fatalf("during DND: %+v", got)
	}
	later := t0.Add(2 * time.Hour)
	p2 := newPolicy(settings())
	p2.SetDND(&until)
	got = p2.OnPower(lost(later), 2, true, later)
	if got[0].Silent {
		t.Fatal("expired DND must not silence")
	}
	p.SetDND(nil)
	if p.DNDUntil() != nil {
		t.Fatal("DND not cleared")
	}
}

func TestFlappingCollapsesToasts(t *testing.T) {
	p := newPolicy(settings())
	e1 := p.OnPower(lost(t0), 1, true, t0)
	e2 := p.OnPower(back(t0.Add(time.Minute)), 1, true, t0.Add(time.Minute))
	if len(e1) != 1 || e1[0].Kind != GridLost || len(e2) != 1 || e2[0].Kind != GridBack {
		t.Fatalf("first two: %v %v", e1, e2)
	}
	t3 := t0.Add(2 * time.Minute)
	e3 := p.OnPower(lost(t3), 2, true, t3)
	if len(e3) != 1 || e3[0].Kind != GridFlapping {
		t.Fatalf("3rd: %v", e3)
	}
	t4 := t0.Add(3 * time.Minute)
	if got := p.OnPower(back(t4), 2, true, t4); len(got) != 0 {
		t.Fatalf("4th must be silent: %v", got)
	}
	// still unstable: a transition 9 min after the last one stays silent
	t5 := t4.Add(9 * time.Minute)
	if got := p.OnPower(lost(t5), 3, true, t5); len(got) != 0 {
		t.Fatalf("within 10 min of last transition: %v", got)
	}
	// 10 min of stability, then toasts resume
	t6 := t5.Add(10 * time.Minute)
	got := p.OnPower(back(t6), 3, true, t6)
	if len(got) != 2 || got[0].Kind != GridLost || got[0].Tag != "3" || got[1].Kind != GridBack {
		t.Fatalf("after stability: want pending lost then back, got %v", got)
	}
}

func TestFlappingThenStayOffReportsLostOnTick(t *testing.T) {
	p := newPolicy(settings())
	p.OnPower(lost(t0), 1, true, t0)
	p.OnPower(back(t0.Add(time.Minute)), 1, true, t0.Add(time.Minute))
	t3 := t0.Add(2 * time.Minute)
	if e := p.OnPower(lost(t3), 2, true, t3); len(e) != 1 || e[0].Kind != GridFlapping {
		t.Fatalf("3rd: %v", e)
	}
	if got := p.Tick(t3.Add(9 * time.Minute)); len(got) != 0 {
		t.Fatalf("too early: %v", got)
	}
	got := p.Tick(t3.Add(10 * time.Minute))
	if len(got) != 1 || got[0].Kind != GridLost || got[0].Tag != "2" || !got[0].At.Equal(t3) {
		t.Fatalf("tick: %v", got)
	}
	if got := p.Tick(t3.Add(11 * time.Minute)); len(got) != 0 {
		t.Fatalf("must emit once: %v", got)
	}
	tb := t3.Add(2 * time.Hour)
	b := p.OnPower(back(tb), 2, true, tb)
	if len(b) != 1 || b[0].Kind != GridBack || b[0].Tag != "2" {
		t.Fatalf("back pairs with lost: %v", b)
	}
}

func TestFlappingEndingWithGridBackStaysSilentOnTick(t *testing.T) {
	p := newPolicy(settings())
	p.OnPower(back(t0), 1, true, t0)
	p.OnPower(lost(t0.Add(time.Minute)), 2, true, t0.Add(time.Minute))
	t3 := t0.Add(2 * time.Minute)
	p.OnPower(back(t3), 2, true, t3)
	if got := p.Tick(t3.Add(10 * time.Minute)); len(got) != 0 {
		t.Fatalf("grid is on, nothing to report: %v", got)
	}
}

func TestFlappingExpiryFromOnSample(t *testing.T) {
	p := newPolicy(settings())
	p.OnPower(lost(t0), 1, true, t0)
	p.OnPower(back(t0.Add(time.Minute)), 1, true, t0.Add(time.Minute))
	t3 := t0.Add(2 * time.Minute)
	p.OnPower(lost(t3), 2, true, t3)
	now := t3.Add(10 * time.Minute)
	got := p.OnSample(sample(50, now), detect.Idle, true, now)
	if len(got) != 1 || got[0].Kind != GridLost || got[0].Tag != "2" {
		t.Fatalf("got %v", got)
	}
}

func TestGridBackSharesTagWithLost(t *testing.T) {
	p := newPolicy(settings())
	l := p.OnPower(lost(t0), 42, true, t0)
	b := p.OnPower(back(t0.Add(time.Minute)), 42, true, t0.Add(time.Minute))
	if l[0].Tag != "42" || b[0].Tag != "42" {
		t.Fatalf("tags %q %q", l[0].Tag, b[0].Tag)
	}
}

func TestFullRearmsBelow95(t *testing.T) {
	p := newPolicy(settings())
	n := func(soc float64) int { return len(p.OnSample(sample(soc, t0), detect.Charging, true, t0)) }
	if n(99) != 0 {
		t.Fatal("99 no toast")
	}
	if n(100) != 1 {
		t.Fatal("100 toast")
	}
	if n(100) != 0 {
		t.Fatal("repeat")
	}
	if n(96) != 0 || n(100) != 0 {
		t.Fatal("96 must not rearm")
	}
	if n(94) != 0 {
		t.Fatal("rearm sample")
	}
	if n(100) != 1 {
		t.Fatal("after <95 must toast again")
	}
}

func TestLikelyDeadOnlyWhenDetectedLive(t *testing.T) {
	p := newPolicy(settings())
	if got := p.OnLikelyDead(t0, f(5), false, t0); len(got) != 0 {
		t.Fatalf("not live: %v", got)
	}
	got := p.OnLikelyDead(t0, f(5), true, t0)
	if len(got) != 1 || got[0].Kind != LikelyDead {
		t.Fatalf("got %v", got)
	}
}

func TestNoLinkToastsForTwoMinutesAfterStartAndResume(t *testing.T) {
	p := NewPolicy(settings(), t0)
	if got := p.OnLikelyDead(t0, nil, true, t0.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("after start: %v", got)
	}
	if got := p.OnLikelyDead(t0, nil, true, t0.Add(2*time.Minute)); len(got) != 1 {
		t.Fatalf("after quiet period: %v", got)
	}
	r := t0.Add(time.Hour)
	p.OnResume(r)
	if got := p.OnLikelyDead(r, nil, true, r.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("after resume: %v", got)
	}
}

func TestEnabledMapDisablesKind(t *testing.T) {
	s := settings()
	s.Enabled = map[Kind]bool{GridLost: false, GridBack: true}
	p := newPolicy(s)
	if got := p.OnPower(lost(t0), 1, true, t0); len(got) != 0 {
		t.Fatalf("disabled kind: %v", got)
	}
}

func TestDNDOptionMorning(t *testing.T) {
	now := time.Date(2026, 11, 22, 22, 10, 0, 0, time.UTC)
	s := settings()
	s.QuietEnabled = false
	got := DNDOption("morning", now, s)
	if want := time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC); got == nil || !got.Equal(want) {
		t.Fatalf("disabled: got %v want %v", got, want)
	}
	s.QuietEnabled, s.QuietStart, s.QuietEnd = true, "23:00", "06:30"
	got = DNDOption("morning", now, s)
	if want := time.Date(2026, 11, 23, 6, 30, 0, 0, time.UTC); got == nil || !got.Equal(want) {
		t.Fatalf("enabled: got %v want %v", got, want)
	}
}

func TestDNDOptionDurationsAndManual(t *testing.T) {
	s := settings()
	for opt, d := range map[string]time.Duration{"1h": time.Hour, "3h": 3 * time.Hour, "8h": 8 * time.Hour} {
		got := DNDOption(opt, t0, s)
		if got == nil || !got.Equal(t0.Add(d)) {
			t.Fatalf("%s: %v", opt, got)
		}
	}
	m := DNDOption("manual", t0, s)
	if m == nil || !m.After(t0.AddDate(50, 0, 0)) {
		t.Fatalf("manual: %v", m)
	}
	if DNDOption("bogus", t0, s) != nil {
		t.Fatal("unknown option must be nil")
	}
}

func TestMissedGridBackWhileNotLiveDoesNotRevivePendingLost(t *testing.T) {
	p := newPolicy(settings())
	p.OnPower(lost(t0), 1, true, t0)
	p.OnPower(back(t0.Add(time.Minute)), 1, true, t0.Add(time.Minute))
	t3 := t0.Add(2 * time.Minute)
	if e := p.OnPower(lost(t3), 2, true, t3); len(e) != 1 || e[0].Kind != GridFlapping {
		t.Fatalf("3rd: %v", e)
	}
	// link drops; the grid returns while not live, the event is ignored for toasts
	tb := t3.Add(3 * time.Minute)
	if got := p.OnPower(back(tb), 2, false, tb); len(got) != 0 {
		t.Fatalf("not live: %v", got)
	}
	// link live again after the stability window
	now := tb.Add(11 * time.Minute)
	if got := p.Tick(now); len(got) != 0 {
		t.Fatalf("false grid_lost on Tick: %v", got)
	}
	if got := p.OnSample(sample(50, now), detect.Idle, true, now); len(got) != 0 {
		t.Fatalf("false grid_lost on OnSample: %v", got)
	}
}

// I-1: a change first seen at t0 and confirmed live 135 s later (the station
// was silent in between) is a live observation, not catch-up: it is toasted,
// and the toast keeps the time of the first observation.
func TestLateConfirmationIsNotCatchUp(t *testing.T) {
	p := newPolicy(settings())
	confirmed := t0.Add(135 * time.Second)
	ev := detect.PowerEvent{Kind: detect.GridLost, At: t0, ConfirmedAt: confirmed, SOC: f(60)}
	got := p.OnPower(ev, 7, true, confirmed)
	if len(got) != 1 || got[0].Kind != GridLost || got[0].Tag != "7" || !got[0].At.Equal(t0) {
		t.Fatalf("got %+v, want one grid_lost toast at t0", got)
	}
	// Reported long after it was confirmed: that is catch-up.
	p = newPolicy(settings())
	if got := p.OnPower(ev, 7, true, confirmed.Add(2*time.Minute)); len(got) != 0 {
		t.Fatalf("old confirmation toasted: %+v", got)
	}
	bk := detect.PowerEvent{Kind: detect.GridBack, At: t0, ConfirmedAt: confirmed, SOC: f(60)}
	p = newPolicy(settings())
	if got := p.OnPower(bk, 7, true, confirmed); len(got) != 1 || got[0].Kind != GridBack {
		t.Fatalf("got %+v, want one grid_back toast", got)
	}
}

// I-2: "full" means the charge REACHED 100%. A station already at 100% on
// the first reading after start or resume only disarms the toast.
func TestFullDoesNotFireWhenAlreadyFullAtStartOrResume(t *testing.T) {
	p := newPolicy(settings())
	n := func(soc float64, live bool) int {
		return len(p.OnSample(sample(soc, t0), detect.Idle, live, t0))
	}
	if n(100, true) != 0 || n(100, true) != 0 {
		t.Fatal("already full at start must not toast")
	}
	if n(94, true) != 0 || n(100, true) != 1 {
		t.Fatal("after dropping below 95 it must toast on reaching 100")
	}

	// The first reading may come while the link is still resyncing.
	p = newPolicy(settings())
	if n(100, false) != 0 || n(100, true) != 0 {
		t.Fatal("already full on a not-live first reading must not toast later")
	}

	// Below 100 at start: reaching 100 is observed, so it toasts.
	p = newPolicy(settings())
	if n(97, true) != 0 || n(100, true) != 1 {
		t.Fatal("97 at start, then 100: want one toast")
	}

	// Charged to 100 while the PC slept: not observed, no toast.
	p = newPolicy(settings())
	if n(80, true) != 0 {
		t.Fatal("80 no toast")
	}
	p.OnResume(t0)
	if n(100, true) != 0 || n(100, true) != 0 {
		t.Fatal("already full at resume must not toast")
	}
}

// Task 1: a grid return whose start lies in a data gap is not toasted, but
// the state is remembered so the next change counts correctly.
func TestGridBackAcrossGapIsNotToasted(t *testing.T) {
	p := newPolicy(settings())
	ev := back(t0)
	ev.AcrossGap = true
	if got := p.OnPower(ev, 1, true, t0.Add(time.Second)); got != nil {
		t.Fatalf("toasted across gap: %+v", got)
	}
	at := t0.Add(5 * time.Minute)
	got := p.OnPower(lost(at), 2, true, at.Add(time.Second))
	if len(got) != 1 || got[0].Kind != GridLost {
		t.Fatalf("want one grid_lost (not flapping), got %+v", got)
	}

	// p.last must be updated: flapping that ends with an across-gap GridBack
	// must not report the already-ended outage when it expires.
	p = newPolicy(settings())
	p.OnPower(lost(t0), 1, true, t0.Add(time.Second))
	p.OnPower(back(t0.Add(time.Minute)), 1, true, t0.Add(time.Minute+time.Second))
	p.OnPower(lost(t0.Add(2*time.Minute)), 2, true, t0.Add(2*time.Minute+time.Second)) // flapping
	ev = back(t0.Add(3 * time.Minute))
	ev.AcrossGap = true
	if got := p.OnPower(ev, 2, true, t0.Add(3*time.Minute+time.Second)); got != nil {
		t.Fatalf("toasted across gap: %+v", got)
	}
	if got := p.Tick(t0.Add(13 * time.Minute)); len(got) != 0 {
		t.Fatalf("flapping expiry reported an ended outage: %+v", got)
	}
}

func TestGridLostAcrossGapIsApprox(t *testing.T) {
	p := newPolicy(settings())
	ev := lost(t0)
	ev.AcrossGap = true
	got := p.OnPower(ev, 1, true, t0.Add(time.Second))
	if len(got) != 1 || got[0].Kind != GridLost || got[0].Args["approx"] != "1" {
		t.Fatalf("want one approx grid_lost, got %+v", got)
	}
	if got := newPolicy(settings()).OnPower(lost(t0), 1, true, t0.Add(time.Second)); len(got) != 1 || got[0].Args["approx"] != "" {
		t.Fatalf("contiguous grid_lost must not be approx: %+v", got)
	}
}

// Owner acceptance A2: a grid_lost found across a gap names the window the
// grid went off in: from the last observation before the gap (ev.GapFrom) to
// the first observation of the outage (ev.At), RFC 3339 in now's zone. A live
// outage carries neither; a gap without a known start carries only "to".
func TestGridLostAcrossGapCarriesFromTo(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	from := t0.Add(-12 * time.Hour)
	ev := lost(t0)
	ev.AcrossGap, ev.GapFrom = true, from
	got := newPolicy(settings()).OnPower(ev, 1, true, t0.Add(time.Second).In(kyiv))
	if len(got) != 1 || got[0].Args["approx"] != "1" ||
		got[0].Args["from"] != from.In(kyiv).Format(time.RFC3339) || got[0].Args["to"] != t0.In(kyiv).Format(time.RFC3339) {
		t.Fatalf("want approx grid_lost with from/to, got %+v", got)
	}

	ev.GapFrom = time.Time{}
	got = newPolicy(settings()).OnPower(ev, 1, true, t0.Add(time.Second))
	if len(got) != 1 || got[0].Args["from"] != "" || got[0].Args["to"] != t0.Format(time.RFC3339) {
		t.Fatalf("unknown gap start: want only to, got %+v", got)
	}

	live := lost(t0)
	live.GapFrom = from // ignored: not across a gap
	got = newPolicy(settings()).OnPower(live, 1, true, t0.Add(time.Second))
	if len(got) != 1 || got[0].Args["approx"] != "" || got[0].Args["from"] != "" || got[0].Args["to"] != "" {
		t.Fatalf("live grid_lost must stay exact: %+v", got)
	}
}

func TestPolicySetSettingsKeepsState(t *testing.T) {
	run := func(reconfigure bool) Kind {
		p := newPolicy(settings())
		p.OnPower(lost(t0), 1, true, t0)
		if reconfigure {
			s := settings()
			s.LowThreshold = 30
			p.SetSettings(s)
		}
		p.OnPower(back(t0.Add(time.Minute)), 1, true, t0.Add(time.Minute))
		e := p.OnPower(lost(t0.Add(2*time.Minute)), 2, true, t0.Add(2*time.Minute))
		if len(e) != 1 {
			t.Fatalf("reconfigure=%v: want one toast, got %+v", reconfigure, e)
		}
		return e[0].Kind
	}
	without, with := run(false), run(true)
	if without != GridFlapping || with != without {
		t.Fatalf("third transition: without SetSettings %q, with %q; want both %q", without, with, GridFlapping)
	}
	// The new threshold is in effect.
	p := newPolicy(settings())
	s := settings()
	s.LowThreshold = 30
	p.SetSettings(s)
	if got := p.OnSample(sample(25, t0), detect.Discharging, true, t0); len(got) != 1 || got[0].Kind != LowCharge {
		t.Fatalf("25%% under new 30%% threshold: %+v", got)
	}
}

func TestSetSettingsClonesEnabled(t *testing.T) {
	p := newPolicy(settings())
	s := settings()
	s.Enabled = map[Kind]bool{}
	p.SetSettings(s)
	s.Enabled[GridLost] = false // caller edits its map afterwards
	if got := p.OnPower(lost(t0), 1, true, t0); len(got) != 1 {
		t.Fatalf("caller's map edit leaked into the policy: %+v", got)
	}
}

// noLinkSettings: the no_link toast on, after 10 min.
func noLinkSettings() Settings {
	s := settings()
	s.NoLinkAfter = 10 * time.Minute
	s.Enabled = map[Kind]bool{NoLink: true}
	return s
}

func TestNoLinkAfterThreshold(t *testing.T) {
	p := newPolicy(noLinkSettings())
	since, last := t0, t0.Add(-5*time.Minute)
	if got := p.OnLink(true, since, last, f(60), since.Add(9*time.Minute)); got != nil {
		t.Fatalf("+9m: %+v", got)
	}
	got := p.OnLink(true, since, last, f(60), since.Add(10*time.Minute))
	if len(got) != 1 || got[0].Kind != NoLink {
		t.Fatalf("+10m: want one no_link, got %+v", got)
	}
	tt := got[0]
	if tt.Tag != "link-"+strconv.FormatInt(since.Unix(), 10) || tt.Silent {
		t.Fatalf("toast = %+v", tt)
	}
	if tt.Args["duration"] != (15*time.Minute).String() || tt.Args["time"] != last.Format("15:04") {
		t.Fatalf("args = %+v, want duration 15m since the last data at %s", tt.Args, last.Format("15:04"))
	}
	if got := p.OnLink(true, since, last, f(60), since.Add(20*time.Minute)); got != nil {
		t.Fatalf("+20m: once per episode, got %+v", got)
	}
}

func TestNoLinkHeldInQuietPeriodAfterStart(t *testing.T) {
	p := NewPolicy(noLinkSettings(), t0)
	if got := p.OnLink(true, t0.Add(-20*time.Minute), t0.Add(-25*time.Minute), nil, t0.Add(time.Minute)); got != nil {
		t.Fatalf("within 2 min of start: %+v", got)
	}
	if got := p.OnLink(true, t0.Add(-20*time.Minute), t0.Add(-25*time.Minute), nil, t0.Add(2*time.Minute)); len(got) != 1 {
		t.Fatalf("after the quiet period: %+v", got)
	}
}

func TestNoLinkRestoredReplacesSameTag(t *testing.T) {
	p := newPolicy(noLinkSettings())
	since, last := t0, t0.Add(-5*time.Minute)
	lost := p.OnLink(true, since, last, f(60), since.Add(10*time.Minute))
	if len(lost) != 1 {
		t.Fatalf("no_link: %+v", lost)
	}
	// Suspended / connecting again: not a no-link state, but no new data
	// either. Nothing is restored yet.
	if got := p.OnLink(false, time.Time{}, last, f(60), since.Add(11*time.Minute)); got != nil {
		t.Fatalf("restored without new data: %+v", got)
	}
	back := since.Add(30 * time.Minute)
	got := p.OnLink(false, time.Time{}, back, f(55), back.Add(time.Second))
	if len(got) != 1 || got[0].Kind != NoLinkRestored || got[0].Tag != lost[0].Tag {
		t.Fatalf("want no_link_restored with tag %q, got %+v", lost[0].Tag, got)
	}
	if got[0].Args["duration"] != (35*time.Minute).String() || got[0].Args["soc"] != "55" {
		t.Fatalf("args = %+v", got[0].Args)
	}
	if got := p.OnLink(false, time.Time{}, back, f(55), back.Add(2*time.Second)); got != nil {
		t.Fatalf("restored twice: %+v", got)
	}
	// A new episode toasts again with its own tag.
	since2 := back.Add(time.Hour)
	again := p.OnLink(true, since2, back.Add(55*time.Minute), f(50), since2.Add(10*time.Minute))
	if len(again) != 1 || again[0].Tag == lost[0].Tag {
		t.Fatalf("second episode: %+v", again)
	}
}

func TestNoLinkRestoredWithoutToastIsSilent(t *testing.T) {
	p := newPolicy(noLinkSettings())
	since, last := t0, t0.Add(-5*time.Minute)
	if got := p.OnLink(true, since, last, f(60), since.Add(5*time.Minute)); got != nil {
		t.Fatalf("before the threshold: %+v", got)
	}
	back := since.Add(6 * time.Minute)
	if got := p.OnLink(false, time.Time{}, back, f(60), back); got != nil {
		t.Fatalf("restored without a no_link toast: %+v", got)
	}
	// Disabled: no no_link toast, so no restore either.
	s := noLinkSettings()
	s.Enabled[NoLink] = false
	p = newPolicy(s)
	if got := p.OnLink(true, since, last, f(60), since.Add(time.Hour)); got != nil {
		t.Fatalf("disabled: %+v", got)
	}
	if got := p.OnLink(false, time.Time{}, back.Add(time.Hour), f(60), back.Add(time.Hour)); got != nil {
		t.Fatalf("disabled, restored: %+v", got)
	}
}

func TestNoLinkSilentInQuietHours(t *testing.T) {
	night := time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC)
	p := NewPolicy(noLinkSettings(), night.Add(-time.Hour))
	got := p.OnLink(true, night.Add(-10*time.Minute), night.Add(-15*time.Minute), nil, night)
	if len(got) != 1 || !got[0].Silent {
		t.Fatalf("want one silent no_link, got %+v", got)
	}
	got = p.OnLink(false, time.Time{}, night.Add(time.Minute), f(40), night.Add(time.Minute))
	if len(got) != 1 || got[0].Kind != NoLinkRestored || !got[0].Silent {
		t.Fatalf("want one silent no_link_restored, got %+v", got)
	}
}

func TestFlappingFollowsGridLostToggle(t *testing.T) {
	flap := func(s Settings) (kinds []Kind) {
		p := newPolicy(s)
		for i := 0; i < 5; i++ {
			at := t0.Add(time.Duration(i) * time.Minute)
			ev := lost(at)
			if i%2 == 1 {
				ev = back(at)
			}
			for _, tt := range p.OnPower(ev, 1, true, at) {
				kinds = append(kinds, tt.Kind)
			}
		}
		return kinds
	}
	s := settings()
	s.Enabled = map[Kind]bool{GridLost: false}
	// grid_lost off: no grid_flapping, the grid_back toasts are not collapsed.
	if got := flap(s); !slices.Equal(got, []Kind{GridBack, GridBack}) {
		t.Fatalf("grid_lost off: %v", got)
	}
	// grid_lost on: flapping collapses whatever grid_flapping's own entry says.
	s.Enabled = map[Kind]bool{GridLost: true, GridFlapping: false}
	if got := flap(s); !slices.Equal(got, []Kind{GridLost, GridBack, GridFlapping}) {
		t.Fatalf("grid_lost on: %v", got)
	}
}

// Review fix: the threshold counts awake time without a link, from the
// noLinkSince passed in (the monitor re-enters NO_LINK after resume); the
// episode and its Tag survive the sleep.
func TestNoLinkThresholdRestartsAfterResume(t *testing.T) {
	p := newPolicy(noLinkSettings())
	since1, last := t0, t0.Add(-5*time.Minute)
	if got := p.OnLink(true, since1, last, f(60), since1.Add(3*time.Minute)); got != nil {
		t.Fatalf("+3m: %+v", got)
	}
	// Asleep for the night: no link state, no new data.
	if got := p.OnLink(false, since1, last, f(60), since1.Add(4*time.Minute)); got != nil {
		t.Fatalf("suspended: %+v", got)
	}
	r := since1.Add(8 * time.Hour)
	p.OnResume(r)
	since2 := r.Add(30 * time.Second) // NO_LINK again after the connect timeout
	if got := p.OnLink(true, since2, last, f(60), r.Add(3*time.Minute)); got != nil {
		t.Fatalf("2.5 min awake without link: %+v", got)
	}
	got := p.OnLink(true, since2, last, f(60), since2.Add(10*time.Minute))
	if len(got) != 1 || got[0].Tag != "link-"+strconv.FormatInt(since1.Unix(), 10) {
		t.Fatalf("after 10 awake minutes: want one toast with the episode's first tag, got %+v", got)
	}
}

func TestNoLinkZeroSinceIsIgnored(t *testing.T) {
	p := newPolicy(noLinkSettings())
	if got := p.OnLink(true, time.Time{}, t0.Add(-time.Hour), nil, t0); got != nil {
		t.Fatalf("zero noLinkSince: %+v", got)
	}
}

// The last-data time comes as RFC 3339 too, so the toast can say which day.
func TestNoLinkArgsLastAt(t *testing.T) {
	p := newPolicy(noLinkSettings())
	last := t0.Add(-15 * time.Minute)
	got := p.OnLink(true, t0, last, nil, t0.Add(10*time.Minute))
	if len(got) != 1 || got[0].Args["lastAt"] != last.Format(time.RFC3339) {
		t.Fatalf("args = %+v", got)
	}
}

func TestCrossingsIndependentOfToastSwitch(t *testing.T) {
	s := settings()
	s.Enabled = map[Kind]bool{LowCharge: false}
	p := newPolicy(s)
	if got := p.OnSample(sample(19, t0), detect.Discharging, true, t0); len(got) != 0 {
		t.Fatalf("toasts = %v, want none (switched off)", got)
	}
	if got := p.Crossings(); !slices.Equal(got, []Kind{LowCharge}) {
		t.Fatalf("Crossings = %v, want [low_charge]", got)
	}
	if got := p.Crossings(); len(got) != 0 {
		t.Fatalf("second Crossings = %v, want none (cleared)", got)
	}
	p.OnSample(sample(18, t0.Add(time.Second)), detect.Discharging, true, t0.Add(time.Second))
	if got := p.Crossings(); len(got) != 0 {
		t.Fatalf("Crossings below the threshold again = %v, want none (not re-armed)", got)
	}
	p.OnSample(sample(26, t0.Add(2*time.Second)), detect.Charging, true, t0.Add(2*time.Second))
	p.OnSample(sample(19, t0.Add(3*time.Second)), detect.Discharging, true, t0.Add(3*time.Second))
	if got := p.Crossings(); !slices.Equal(got, []Kind{LowCharge}) {
		t.Fatalf("Crossings after re-arm = %v, want [low_charge]", got)
	}
	// Not from stale data.
	p.OnSample(sample(26, t0.Add(4*time.Second)), detect.Charging, true, t0.Add(4*time.Second))
	p.OnSample(sample(19, t0.Add(5*time.Second)), detect.Discharging, false, t0.Add(5*time.Second))
	if got := p.Crossings(); len(got) != 0 {
		t.Fatalf("Crossings from a non-live sample = %v, want none", got)
	}
}

// Plan 3 Task 4: the update toast has no switch (not in Kinds, an Enabled
// entry does not stop it) but quiet hours and "do not disturb" make it silent.
func TestUpdateToast(t *testing.T) {
	if slices.Contains(Kinds, Update) {
		t.Fatal("Update must not be a user switch")
	}
	s := settings()
	s.Enabled = map[Kind]bool{Update: false}
	day := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	got := UpdateToast("1.2.0", s, nil, day)
	if got.Kind != Update || got.Silent || got.Args["version"] != "1.2.0" || !got.At.Equal(day) {
		t.Fatalf("day: %+v", got)
	}
	night := time.Date(2026, 11, 22, 2, 0, 0, 0, time.UTC)
	if got := UpdateToast("1.2.0", s, nil, night); !got.Silent {
		t.Fatal("quiet hours must silence the update toast")
	}
	until := day.Add(time.Hour)
	if got := UpdateToast("1.2.0", s, &until, day); !got.Silent {
		t.Fatal("DND must silence the update toast")
	}
	if got := UpdateToast("1.2.0", s, &until, day.Add(2*time.Hour)); got.Silent {
		t.Fatal("an expired DND must not silence")
	}
}
