package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lantest"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

const testMac = "0a1b2c3d4e5f"

// fakeClock is a manual clock. The monitor loop waits on exactly one After
// channel when idle, so waitIdle can tell when a step has finished.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

func newClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.waiters = append(c.waiters, waiter{c.now.Add(d), ch})
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.now) {
			w.ch <- c.now
			continue
		}
		kept = append(kept, w)
	}
	c.waiters = kept
}

func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

func (c *fakeClock) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for c.pending() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("monitor loop did not become idle")
		}
		time.Sleep(200 * time.Microsecond)
	}
}

// step advances the clock one second at a time, n times, letting the loop settle.
func (c *fakeClock) step(t *testing.T, n int, each func(sec int)) {
	t.Helper()
	for i := 0; i < n; i++ {
		c.Advance(time.Second)
		c.waitIdle(t)
		if each != nil {
			each(i + 1)
		}
	}
}

// station is a fake Venus E answering ES.GetStatus with a settable SOC and
// offgrid power (offgrid > 20 W with ongrid 0 = grid absent, discharging).
type station struct {
	*lantest.FakeStation
	info lan.DeviceInfo

	mu       sync.Mutex
	soc, off float64
	on, pv   float64 // ongrid and PV power (0 unless setFlows)
}

func startStation(t *testing.T, mac string) *station { return startStationAt(t, "127.0.0.1:0", mac) }

// startStationAt starts the fake on a given loopback address. Like a real
// station it reports its bare IP; the port reaches the monitor through the
// discovery targets (deps).
func startStationAt(t *testing.T, addr, mac string) *station {
	t.Helper()
	s := &station{soc: 80}
	s.info = lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: mac, WifiMac: "wifi"}
	s.FakeStation = lantest.StartAt(t, addr, s.info)
	s.info.IP = s.Addr.IP.String()
	s.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { return s.info, nil })
	s.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return map[string]any{"bat_soc": s.soc, "bat_cap": 5120, "pv_power": s.pv,
			"ongrid_power": s.on, "offgrid_power": s.off}, nil
	})
	return s
}

func (s *station) set(soc, off float64) {
	s.mu.Lock()
	s.soc, s.off = soc, off
	s.mu.Unlock()
}

func (s *station) gridOff(on bool) {
	s.mu.Lock()
	s.off = 0
	if on {
		s.off = 300
	}
	s.mu.Unlock()
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func newClient(t *testing.T) *lan.Client {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return lan.NewClient(conn, 100*time.Millisecond)
}

func seedStation(t *testing.T, st *store.Store, ip string) {
	t.Helper()
	if err := st.UpsertStation(store.Station{ID: testMac, Model: "VenusE 3.0", BLEMac: testMac, IP: ip,
		Firmware: 151, CreatedAt: t0.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

func deps(st *store.Store, c *lan.Client, bcast *net.UDPAddr, clk Clock) Deps {
	d := Deps{Store: st, Client: c, Clock: clk, PollInterval: 5 * time.Second,
		Notify: notify.Settings{LowThreshold: 20}, PCOnline: func() bool { return true }, AutoAdd: true}
	if bcast != nil {
		d.Targets = func() []*net.UDPAddr { return []*net.UDPAddr{bcast} }
	}
	return d
}

// start runs m in the background and waits for its first step.
func start(t *testing.T, m *Monitor, clk *fakeClock) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	// Registered before the store's cleanup, so it runs first.
	t.Cleanup(func() { cancel(); <-done })
	clk.waitIdle(t)
}

func init() { discoverWait = 50 * time.Millisecond }

func TestMonitorFirstRunDiscoversAndPersistsStation(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	all, err := st.Stations()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].BLEMac != testMac || all[0].IP != fs.Addr.IP.String() || all[0].Model != "VenusE 3.0" {
		t.Fatalf("stations = %+v", all)
	}
	if got := m.Latest().Station.BLEMac; got != testMac {
		t.Fatalf("snapshot station = %q", got)
	}
	if !m.Latest().Profile.Tested {
		t.Fatalf("profile = %+v, want Venus E 3.0", m.Latest().Profile)
	}
	// A second start uses the stored station without discovering again.
	if _, err := New(deps(st, newClient(t), fs.Addr, clk)); err != nil {
		t.Fatal(err)
	}
	if n := fs.Calls("Marstek.GetDevice"); n != 1 {
		t.Fatalf("GetDevice calls = %d, want 1", n)
	}
	if all, _ := st.Stations(); len(all) != 1 {
		t.Fatalf("stations after restart = %d, want 1", len(all))
	}
}

func TestMonitorFirstRunSkipsDeviceWithoutBLEMac(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, "")
	_, err := New(deps(st, newClient(t), fs.Addr, newClock(t0)))
	if !errors.Is(err, ErrNoStation) {
		t.Fatalf("err = %v, want ErrNoStation", err)
	}
	if all, _ := st.Stations(); len(all) != 0 {
		t.Fatalf("stations = %+v, want none", all)
	}
}

func TestMonitorNoAutoAdd(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	d := deps(st, newClient(t), fs.Addr, newClock(t0))
	d.AutoAdd = false
	if _, err := New(d); !errors.Is(err, ErrNoStation) {
		t.Fatalf("err = %v, want ErrNoStation", err)
	}
	if n := fs.Calls("Marstek.GetDevice"); n != 0 {
		t.Fatalf("GetDevice calls = %d, want 0 (no network without AutoAdd)", n)
	}
	if all, _ := st.Stations(); len(all) != 0 {
		t.Fatalf("stations = %+v, want none", all)
	}
}

func TestMonitorPollsAndStoresSamples(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // poll 1 at t0
	clk.step(t, 10, nil)
	// polls 2 and 3 at t0+5s, t0+10s
	samples, err := st.Samples(testMac, t0.Add(-time.Minute), t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3 {
		t.Fatalf("samples = %d, want 3", len(samples))
	}
	if n := fs.Calls("ES.GetStatus"); n != 3 {
		t.Fatalf("ES.GetStatus calls = %d, want 3", n)
	}
	snap := m.Latest()
	if snap.Link != link.Live {
		t.Fatalf("link = %s, want LIVE", snap.Link)
	}
	if snap.Last == nil || snap.Last.SOC == nil || *snap.Last.SOC != 80 {
		t.Fatalf("last = %+v", snap.Last)
	}
	if snap.Fills.SOC != 0.8 {
		t.Fatalf("Fills.SOC = %v, want 0.8", snap.Fills.SOC)
	}
	select {
	case s := <-m.Snapshots():
		if s.Link != link.Live {
			t.Fatalf("channel snapshot link = %s", s.Link)
		}
	default:
		t.Fatal("no snapshot on channel")
	}
}

func TestMonitorBackoffSequence(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // successful poll at t0
	fs.Silence(true)

	var polls, discovers, all []int
	np, nd := fs.Calls("ES.GetStatus"), fs.Calls("Marstek.GetDevice")
	clk.step(t, 200, func(sec int) {
		if n := fs.Calls("ES.GetStatus"); n != np {
			np = n
			polls = append(polls, sec)
			all = append(all, sec)
		}
		if n := fs.Calls("Marstek.GetDevice"); n != nd {
			nd = n
			discovers = append(discovers, sec)
			all = append(all, sec)
		}
	})
	// First failed request at +5 s, then backoff.
	wantPolls := []int{5, 10, 20, 40, 80, 140, 200}
	if !equalInts(polls, wantPolls) {
		t.Fatalf("poll seconds = %v, want %v", polls, wantPolls)
	}
	wantDisc := []int{65, 125, 185}
	if !equalInts(discovers, wantDisc) {
		t.Fatalf("discover seconds = %v, want %v", discovers, wantDisc)
	}
	for i := 1; i < len(all); i++ {
		if all[i] == all[i-1] {
			t.Fatalf("two calls in the same second: %v", all)
		}
	}
	if s := m.Latest().Link; s != link.Stale {
		t.Fatalf("link = %s, want STALE", s)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMonitorRediscoversMovedStation(t *testing.T) {
	st := openStore(t)
	// Another host: a different loopback IP, the same port.
	moved := startStationAt(t, "127.0.0.2:0", testMac)
	old := startStationAt(t, fmt.Sprintf("127.0.0.1:%d", moved.Addr.Port), testMac)
	// The first rediscovery hears only a different device: it must be ignored.
	var mu sync.Mutex
	replies := 0
	moved.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) {
		mu.Lock()
		defer mu.Unlock()
		replies++
		if replies == 1 {
			other := moved.info
			other.BLEMac = "aaaaaaaaaaaa"
			return other, nil
		}
		return moved.info, nil
	})
	seedStation(t, st, old.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), moved.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	if old.Calls("ES.GetStatus") != 1 {
		t.Fatal("first poll did not reach the old address")
	}
	old.Silence(true)
	clk.step(t, 90, nil) // rediscovery at +65 s hears the stranger only
	if moved.Calls("Marstek.GetDevice") != 1 || moved.Calls("ES.GetStatus") != 0 {
		t.Fatalf("after first rediscovery: discover=%d polls=%d, want 1 and 0",
			moved.Calls("Marstek.GetDevice"), moved.Calls("ES.GetStatus"))
	}
	clk.step(t, 60, nil) // rediscovery at +125 s finds our BLE MAC
	if moved.Calls("ES.GetStatus") == 0 {
		t.Fatal("polling did not continue at the new address")
	}
	all, err := st.Stations()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].IP != moved.Addr.IP.String() {
		t.Fatalf("stations = %+v, want one with IP %s", all, moved.Addr.IP)
	}
	clk.step(t, 40, nil) // resync settles
	snap := m.Latest()
	if snap.Link != link.Live || snap.Station.IP != moved.Addr.IP.String() {
		t.Fatalf("link = %s, station IP = %s", snap.Link, snap.Station.IP)
	}
}

func TestMonitorOutageEndToEnd(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	// Present 0..39 s, absent 40..79 s, present 80..119 s.
	clk.step(t, 125, func(sec int) {
		switch sec {
		case 39:
			fs.gridOff(true)
		case 79:
			fs.gridOff(false)
		}
	})
	outs, err := st.Outages(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].End == nil {
		t.Fatalf("outages = %+v, want one closed", outs)
	}
	o := outs[0]
	if !o.Start.Equal(t0.Add(40*time.Second)) || !o.End.Equal(t0.Add(80*time.Second)) || o.Partial {
		t.Fatalf("outage = %+v, want 40s..80s, not partial", o)
	}
	var lost, back *notify.Toast
	for len(m.Toasts()) > 0 {
		tt := <-m.Toasts()
		switch tt.Kind {
		case notify.GridLost:
			lost = &tt
		case notify.GridBack:
			back = &tt
		}
	}
	if lost == nil || back == nil {
		t.Fatalf("toasts lost=%v back=%v", lost, back)
	}
	if lost.Tag == "" || lost.Tag != back.Tag || lost.StationID != testMac || back.StationID != testMac {
		t.Fatalf("lost = %+v, back = %+v", lost, back)
	}
	snap := m.Latest()
	if snap.Current != nil || snap.Previous == nil || snap.Previous.ID != o.ID {
		t.Fatalf("current = %+v, previous = %+v", snap.Current, snap.Previous)
	}
	if snap.OutagesToday != 1 || snap.NoLightToday != 40*time.Second {
		t.Fatalf("today = %d / %s", snap.OutagesToday, snap.NoLightToday)
	}
}

func TestMonitorCompactsDaily(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 50.0
	old := t0.Add(-31 * 24 * time.Hour)                // compacted at start
	later := t0.Add(-30*24*time.Hour + 30*time.Minute) // compacted only by the next run
	for _, at := range []time.Time{old, later} {
		if err := st.InsertSample(model.Sample{StationID: testMac, At: at, Source: model.SourceLAN, SOC: &soc}); err != nil {
			t.Fatal(err)
		}
	}
	raw := func(at time.Time) int {
		s, err := st.Samples(testMac, at, at.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		return len(s)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	if raw(old) != 0 || raw(later) != 1 {
		t.Fatalf("after start: old=%d later=%d, want 0 and 1", raw(old), raw(later))
	}
	clk.Advance(24*time.Hour - time.Minute)
	clk.waitIdle(t)
	if raw(later) != 1 {
		t.Fatal("compacted before 24 h")
	}
	clk.Advance(time.Minute)
	clk.waitIdle(t)
	if raw(later) != 0 {
		t.Fatal("not compacted after 24 h")
	}
	pts, err := st.MinuteSeries(testMac, later.Add(-time.Minute), later.Add(time.Minute))
	if err != nil || len(pts) != 1 {
		t.Fatalf("minute series = %v, %v", pts, err)
	}
}

func drainToasts(m *Monitor) map[notify.Kind][]notify.Toast {
	out := map[notify.Kind][]notify.Toast{}
	for len(m.Toasts()) > 0 {
		tt := <-m.Toasts()
		out[tt.Kind] = append(out[tt.Kind], tt)
	}
	return out
}

// advance jumps the clock in chunks (one loop step per chunk) for total.
func (c *fakeClock) advance(t *testing.T, chunk, total time.Duration) {
	t.Helper()
	for d := time.Duration(0); d < total; d += chunk {
		c.Advance(chunk)
		c.waitIdle(t)
	}
}

func TestMonitorLikelyDeadMarksOutageAndRelabelsGap(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.set(16, 800) // grid off, discharging 800 W: (16-10)% of 5120 Wh lasts 23 min 2.4 s
	seedStation(t, st, fs.Addr.IP.String())
	oid, err := st.OpenOutage(store.Outage{StationID: testMac, Start: t0.Add(-10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil) // last good sample at +30 s
	fs.Silence(true)
	clk.advance(t, 15*time.Second, 10*time.Minute) // NO_LINK with a no_link gap
	if s := m.Latest().Link; s != link.NoLink {
		t.Fatalf("link = %s, want NO_LINK", s)
	}
	clk.advance(t, 15*time.Second, 15*time.Minute) // forecast empty time passes
	if s := m.Latest().Link; s != link.LikelyDead {
		t.Fatalf("link = %s, want LIKELY_DEAD", s)
	}
	gaps, err := st.Gaps(testMac, t0, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].Reason != store.GapLikelyDead || gaps[0].End != nil {
		t.Fatalf("gaps = %+v, want one open likely_dead gap", gaps)
	}
	o, ok, err := st.CurrentOutage(testMac)
	if err != nil || !ok || o.ID != oid || o.DiedAt == nil {
		t.Fatalf("outage = %+v, %v, %v; want died mark", o, ok, err)
	}
	wantDied := t0.Add(30*time.Second + 23*time.Minute + 2400*time.Millisecond)
	if d := o.DiedAt.Sub(wantDied); d < -time.Second || d > time.Second {
		t.Fatalf("died at %s, want ≈ %s", o.DiedAt, wantDied)
	}
	ts := drainToasts(m)
	if len(ts[notify.LikelyDead]) != 1 || ts[notify.LikelyDead][0].StationID != testMac {
		t.Fatalf("likely_dead toasts = %+v", ts[notify.LikelyDead])
	}
	if len(ts[notify.GridLost]) != 0 {
		t.Fatal("grid_lost toasted for an outage already open at start")
	}
}

func TestMonitorPartialOutageWhenGridAbsentAtStart(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.gridOff(true)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 80, func(sec int) {
		if sec == 39 {
			fs.gridOff(false)
		}
	})
	outs, err := st.Outages(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || !outs[0].Partial || !outs[0].Start.Equal(t0) || outs[0].End == nil {
		t.Fatalf("outages = %+v, want one partial from t0, closed", outs)
	}
	ts := drainToasts(m)
	tag := strconv.FormatInt(outs[0].ID, 10)
	// Ruling 11: still toasted (typical after an ATS-triggered PC reboot), start marked approximate.
	if l := ts[notify.GridLost]; len(l) != 1 || l[0].Tag != tag || l[0].Args["approx"] != "1" || l[0].StationID != testMac {
		t.Fatalf("grid_lost toasts = %+v, want one with Tag %s and approx=1", l, tag)
	}
	// Nothing was observed before (no sample of a previous run): no "from".
	if a := ts[notify.GridLost][0].Args; a["from"] != "" || a["to"] != t0.Local().Format(time.RFC3339) {
		t.Fatalf("grid_lost args = %+v, want only to = t0", a)
	}
	if len(ts[notify.GridBack]) != 1 || ts[notify.GridBack][0].Tag != tag || ts[notify.GridBack][0].Args["approx"] != "" {
		t.Fatalf("grid_back toasts = %+v", ts[notify.GridBack])
	}
}

// Owner acceptance A2: PC off overnight, the grid went off at night, the app
// starts at noon. The toast names the window (last sample of the previous
// run .. first observation of the outage), and the outage's start is shown
// as approximate everywhere (current row, Outages tab), never as exact.
func TestMonitorOutageFoundAtStartNamesTheGapWindow(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.gridOff(true)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 95.0
	before := t0.Add(-12*time.Hour - 20*time.Minute)
	if err := st.InsertSample(model.Sample{StationID: testMac, At: before, Source: model.SourceLAN,
		SOC: &soc, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 90, nil) // resync 30 s, then the 30 s debounce
	o, ok, err := st.CurrentOutage(testMac)
	if err != nil || !ok || !o.Partial {
		t.Fatalf("outage = %+v, %v, %v; want an open partial one", o, ok, err)
	}
	l := drainToasts(m)[notify.GridLost]
	if len(l) != 1 || l[0].Args["approx"] != "1" || l[0].Args["approxReason"] != string(store.GapAppNotRunning) ||
		l[0].Args["from"] != before.Local().Format(time.RFC3339) || l[0].Args["to"] != o.Start.Local().Format(time.RFC3339) {
		t.Fatalf("grid_lost toasts = %+v, want approx from %s to %s", l, before.Local().Format(time.RFC3339), o.Start.Local().Format(time.RFC3339))
	}
	if b := m.Latest().CurrentBounds; b == nil || !b.StartApprox || !b.AtLeast || b.StartFrom == nil || !b.StartFrom.Equal(before) {
		t.Fatalf("current bounds = %+v, want an approximate start from %v and «≥» duration", b, before)
	}
	day := clk.Now().Local().Format(time.DateOnly)
	r, err := m.OutagesReport(day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 || !r.Rows[0].Bounds.StartApprox || !r.Rows[0].Bounds.AtLeast ||
		r.Rows[0].Bounds.StartFrom == nil || !r.Rows[0].Bounds.StartFrom.Equal(before) {
		t.Fatalf("report rows = %+v, want the outage with an approximate start", r.Rows)
	}
}

// Review follow-up: the toast's {from} and the UI's StartFrom come from one
// helper (history.WindowStart). Two touching gaps, app_not_running then
// no_link, meet at an observation (the first sample, grid on): the window
// starts there, not at the earlier gap's start.
func TestMonitorTouchingGapsWindowStartsAtTheObservation(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 95.0
	before := t0.Add(-12 * time.Hour)
	if err := st.InsertSample(model.Sample{StationID: testMac, At: before, Source: model.SourceLAN,
		SOC: &soc, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // the startup gap ends at the first sample, grid on
	fs.Silence(true) // no_link from that sample on
	clk.advance(t, 15*time.Second, 10*time.Minute)
	fs.gridOff(true)
	fs.Silence(false)
	clk.step(t, 90, nil) // resync, then the debounce
	gaps, err := st.Gaps(testMac, before.Add(-time.Minute), clk.Now().Add(time.Second))
	if err != nil || len(gaps) != 2 || gaps[0].Reason != store.GapAppNotRunning || gaps[1].Reason != store.GapNoLink ||
		gaps[0].End == nil || !gaps[0].End.Equal(gaps[1].Start) {
		t.Fatalf("gaps = %+v, %v; want app_not_running touching no_link", gaps, err)
	}
	from := gaps[1].Start
	l := drainToasts(m)[notify.GridLost]
	if len(l) != 1 || l[0].Args["from"] != from.Local().Format(time.RFC3339) {
		t.Fatalf("grid_lost toasts = %+v, want from %s", l, from.Local().Format(time.RFC3339))
	}
	if b := m.Latest().CurrentBounds; b == nil || b.StartFrom == nil || !b.StartFrom.Equal(from) {
		t.Fatalf("current bounds = %+v, want StartFrom %v (the same as the toast)", b, from)
	}
}

func TestMonitorStartupGapCard(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 70.0
	if err := st.InsertSample(model.Sample{StationID: testMac, At: t0.Add(-time.Hour), Source: model.SourceLAN,
		SOC: &soc, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	card := m.Latest().GapCard
	if card == nil || card.Reason != store.GapAppNotRunning || !card.Start.Equal(t0.Add(-time.Hour)) || card.End == nil {
		t.Fatalf("gap card = %+v, want closed app_not_running gap from t0-1h", card)
	}
	if err := m.DismissGapCard(card.ID + 1); err == nil {
		t.Fatal("dismissing an unknown card: want error")
	}
	if err := m.DismissGapCard(card.ID); err != nil {
		t.Fatal(err)
	}
	if c := m.Latest().GapCard; c != nil {
		t.Fatalf("gap card after dismiss = %+v", c)
	}
	clk.step(t, 2, nil)
	if c := m.Latest().GapCard; c != nil {
		t.Fatal("gap card came back")
	}
}

func TestMonitorClosesGapLeftOpenByPreviousRun(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 70.0
	last := t0.Add(-time.Minute)
	if err := st.InsertSample(model.Sample{StationID: testMac, At: last, Source: model.SourceLAN, SOC: &soc}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenGap(store.Gap{StationID: testMac, Start: last, Reason: store.GapPCSleep}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	gaps, err := st.Gaps(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].Reason != store.GapPCSleep || gaps[0].End == nil || !gaps[0].End.Equal(t0) {
		t.Fatalf("gaps = %+v, want the pc_sleep gap closed at t0", gaps)
	}
	if c := m.Latest().GapCard; c != nil {
		t.Fatalf("gap card for a 1 min gap = %+v", c)
	}
}

// Ruling 15a: power smoothed over the previous mode must not leak into the
// forecast of the new one (an idle minute made 400 W look like 130 W).
func TestMonitorForecastRestartsOnModeChange(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	// Idle for just under the 2 min warmup (polling stays at 5 s): the smoother
	// is warm with 0 W.
	clk.step(t, 115, nil)
	fs.set(80, 400)
	clk.step(t, 35, nil) // 30 s of discharge
	snap := m.Latest()
	if snap.Mode != detect.Discharging {
		t.Fatalf("mode = %v, want Discharging", snap.Mode)
	}
	if !snap.F.Warming || snap.F.ToEmpty != nil {
		t.Fatalf("30 s after the mode change: warming = %v, toEmpty = %v; want warming, nil", snap.F.Warming, snap.F.ToEmpty)
	}
	clk.step(t, 45, nil) // 75 s of discharge: warm on the new mode alone
	snap = m.Latest()
	// 80% -> 10% of 5120 Wh at 400 W = 8 h 58 min.
	if snap.F.Warming || snap.F.ToEmpty == nil || *snap.F.ToEmpty < 8*time.Hour+30*time.Minute || *snap.F.ToEmpty > 9*time.Hour+30*time.Minute {
		t.Fatalf("75 s after the mode change: warming = %v, toEmpty = %v; want about 9 h", snap.F.Warming, snap.F.ToEmpty)
	}
}

// Ruling 18b: while the new mode is still on hold the old average must not
// be shown as its forecast (seen in the app: "40 h left" at 400 W).
func TestMonitorForecastWarmsWhileNewModeIsPending(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	// Idle for just under the 2 min warmup (polling stays at 5 s): the smoother
	// is warm with 0 W.
	clk.step(t, 115, nil)
	fs.set(80, 400)
	stale := 0
	clk.step(t, 100, func(sec int) {
		snap := m.Latest()
		if snap.Mode != detect.Discharging || sec < 12 { // from the second discharging sample on
			return
		}
		// 80% -> 10% of 5120 Wh at 400 W = 8 h 58 min; anything far above is the old average.
		if snap.F.ToEmpty != nil && *snap.F.ToEmpty > 10*time.Hour {
			stale++
		}
		if snap.F.ToEmpty == nil && !snap.F.Warming {
			t.Errorf("%d s after the mode change: no forecast and not warming", sec)
		}
	})
	if stale != 0 {
		t.Fatalf("a forecast from the previous mode was shown for %d s", stale)
	}
	if snap := m.Latest(); snap.F.Warming || snap.F.ToEmpty == nil {
		t.Fatalf("100 s after the mode change: warming = %v, toEmpty = %v; want a forecast", snap.F.Warming, snap.F.ToEmpty)
	}
}

// Ruling 15b: discovery asks every target; the station answers on one of them.
func TestMonitorDiscoversThroughAnyTarget(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	deaf, err := net.ListenPacket("udp4", "127.0.0.1:0") // a network with no station
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deaf.Close() })
	d := deps(st, newClient(t), nil, newClock(t0))
	calls := 0
	d.Targets = func() []*net.UDPAddr {
		calls++
		return []*net.UDPAddr{deaf.LocalAddr().(*net.UDPAddr), fs.Addr}
	}
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Latest().Station.BLEMac; got != testMac {
		t.Fatalf("station = %q, want %q", got, testMac)
	}
	if calls == 0 {
		t.Fatal("Targets was not asked")
	}
}

// Ruling 17: power hovering around the ±20 W band flips the mode on every
// sample; that must not restart the forecast each time.
func TestMonitorForecastSurvivesModeFlicker(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	fs.set(80, 15)
	start(t, m, clk)
	discharging, warming := 0, 0
	clk.step(t, 180, func(sec int) {
		if sec%5 == 2 { // between polls: 15 W (idle) and 25 W (discharging) alternate
			if (sec/5)%2 == 0 {
				fs.set(80, 25)
			} else {
				fs.set(80, 15)
			}
		}
		if sec < 90 {
			return // the very first minute is honest warming
		}
		if snap := m.Latest(); snap.Mode == detect.Discharging {
			discharging++
			if snap.F.Warming || snap.F.ToEmpty == nil {
				warming++
			}
		}
	})
	if discharging == 0 {
		t.Fatal("the station never read as discharging: the test does not flicker")
	}
	if warming != 0 {
		t.Fatalf("forecast was warming in %d of %d discharging seconds; want 0", warming, discharging)
	}
}

// A toast must not be readable before the snapshot of the same step.
func TestMonitorToastComesWithItsSnapshot(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 10, nil)
	fs.gridOff(true)
	var lost *notify.Toast
	clk.step(t, 60, func(int) {
		if lost != nil || len(m.Toasts()) == 0 {
			return
		}
		tt := <-m.Toasts()
		if tt.Kind != notify.GridLost {
			return
		}
		lost = &tt
		// The outage the toast is about is already in the snapshot, and the
		// forecast is not the one smoothed over the idle time before it.
		snap := m.Latest()
		if snap.Current == nil || snap.Grid != detect.GridOff {
			t.Errorf("snapshot at toast time: current = %v, grid = %v", snap.Current, snap.Grid)
		}
		if !snap.F.Warming || snap.F.ToEmpty != nil {
			t.Errorf("snapshot at toast time: warming = %v, toEmpty = %v; want warming", snap.F.Warming, snap.F.ToEmpty)
		}
	})
	if lost == nil {
		t.Fatal("no grid_lost toast")
	}
}

// I-1: the station goes silent right after the grid loss is first seen (an
// ATS switch-over) and answers again 140 s later. The confirming sample is a
// live observation: the outage is toasted once, with its true start.
func TestMonitorToastsOutageConfirmedAfterSilence(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	// Absent first seen by the poll at +40 s; polls at +45, 50, 60, 80, 120 s
	// fail; the poll at +180 s confirms.
	clk.step(t, 185, func(sec int) {
		switch sec {
		case 39:
			fs.gridOff(true)
		case 41:
			fs.Silence(true)
		case 121:
			fs.Silence(false)
		}
	})
	outs, err := st.Outages(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || !outs[0].Start.Equal(t0.Add(40*time.Second)) || outs[0].End != nil {
		t.Fatalf("outages = %+v, want one open from +40 s", outs)
	}
	ts := drainToasts(m)
	if l := ts[notify.GridLost]; len(l) != 1 || !l[0].At.Equal(t0.Add(40*time.Second)) {
		t.Fatalf("grid_lost toasts = %+v, want exactly one at +40 s", l)
	}
}

// I-2: the station idles at 100% on grid and the app starts with Windows:
// no "full" toast on start.
func TestMonitorNoFullToastWhenAlreadyFullAtStart(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.set(100, 0)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 20, nil)
	if snap := m.Latest(); snap.Link != link.Live || snap.Last == nil || *snap.Last.SOC != 100 {
		t.Fatalf("link = %s, last = %+v; want LIVE at 100%%", snap.Link, snap.Last)
	}
	if ts := drainToasts(m); len(ts[notify.Full]) != 0 {
		t.Fatalf("full toasts = %+v, want none", ts[notify.Full])
	}
}

func nilPoints(pts []SeriesPoint) []int {
	var at []int
	for i, p := range pts {
		if p.SOC == nil {
			at = append(at, i)
		}
	}
	return at
}

// I-4: a PC reboot shorter than 5 min is not a gap (flows 2.2), but it must
// leave a trace: a link.delayed event, and the chart line broken across the
// hole instead of interpolated.
func TestMonitorShortRestartLogsDelayAndBreaksTheChartLine(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 70.0
	before := t0.Add(-3 * time.Minute)
	if err := st.InsertSample(model.Sample{StationID: testMac, At: before, Source: model.SourceLAN,
		SOC: &soc, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // first sample at t0, 3 min after the stored one
	clk.step(t, 10, nil)

	delays, err := st.Events(testMac, "link.delayed", t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(delays) != 1 || !delays[0].At.Equal(t0) || delays[0].Level != "info" {
		t.Fatalf("link.delayed events = %+v, want one at t0 (info)", delays)
	}
	gaps, err := st.Gaps(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 0 || m.Latest().GapCard != nil {
		t.Fatalf("gaps = %+v, card = %+v; a hole under 5 min is neither", gaps, m.Latest().GapCard)
	}
	for _, r := range []string{"24h", "7d"} {
		pts, _, err := m.SOCSeries(r)
		if err != nil {
			t.Fatal(err)
		}
		// stored sample, break, then the samples of this run
		if n := nilPoints(pts); len(pts) < 3 || len(n) != 1 || n[0] != 1 || !pts[0].At.Before(t0) || pts[2].At.Before(t0) {
			t.Fatalf("%s points = %+v, want one break right after the stored sample", r, pts)
		}
	}

	// A delay while the app is running stays joined (flows 2.2): no new break.
	fs.Silence(true)
	clk.step(t, 60, nil)
	fs.Silence(false)
	clk.step(t, 60, nil)
	delays, err = st.Events(testMac, "link.delayed", t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(delays) != 2 {
		t.Fatalf("link.delayed events = %+v, want the start delay and the running one", delays)
	}
	pts, _, err := m.SOCSeries("24h")
	if err != nil {
		t.Fatal(err)
	}
	if n := nilPoints(pts); len(n) != 1 {
		t.Fatalf("breaks at %v, want only the one after the restart", n)
	}
}

// m-1: an empty or null ES.GetStatus result (a known firmware failure) is a
// failed poll: backoff and no LIVE link with every value unknown.
func TestMonitorEmptyStatusIsAFailedPoll(t *testing.T) {
	for name, result := range map[string]any{"null": nil, "empty object": map[string]any{},
		"only nulls": map[string]any{"bat_soc": nil, "ongrid_power": nil}, "capacity only": map[string]any{"bat_cap": 5120}} {
		t.Run(name, func(t *testing.T) {
			st := openStore(t)
			fs := startStation(t, testMac)
			fs.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) { return result, nil })
			seedStation(t, st, fs.Addr.IP.String())
			clk := newClock(t0)
			m, err := New(deps(st, newClient(t), fs.Addr, clk))
			if err != nil {
				t.Fatal(err)
			}
			start(t, m, clk) // poll at t0
			var polls []int
			n := fs.Calls("ES.GetStatus")
			clk.step(t, 25, func(sec int) {
				if c := fs.Calls("ES.GetStatus"); c != n {
					n = c
					polls = append(polls, sec)
				}
			})
			if want := []int{5, 15}; !equalInts(polls, want) { // backoff 5, 10, 20 s
				t.Fatalf("poll seconds = %v, want %v", polls, want)
			}
			if snap := m.Latest(); snap.Link != link.Connecting || snap.Last != nil {
				t.Fatalf("link = %s, last = %+v; want CONNECTING with no data", snap.Link, snap.Last)
			}
			if s, err := st.Samples(testMac, t0.Add(-time.Hour), t0.Add(time.Hour)); err != nil || len(s) != 0 {
				t.Fatalf("stored samples = %d, %v; want none", len(s), err)
			}
		})
	}
}

// m-2: a discovery reply cannot send polling to another address: the station
// is polled where its reply came from, whatever the payload says.
func TestMonitorDiscoveryCannotRedirectPolling(t *testing.T) {
	for _, lie := range []string{"127.0.0.9", "127.0.0.9:9", "example.invalid:30000", ""} {
		t.Run("rediscovery/"+lie, func(t *testing.T) {
			st := openStore(t)
			fs := startStation(t, testMac)
			fs.info.IP = lie
			var mu sync.Mutex
			failing := true
			fs.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
				mu.Lock()
				defer mu.Unlock()
				if failing {
					return nil, &lan.RPCError{Code: -32603, Message: "busy"}
				}
				return map[string]any{"bat_soc": 80, "ongrid_power": 0, "offgrid_power": 0}, nil
			})
			seedStation(t, st, fs.Addr.IP.String())
			clk := newClock(t0)
			m, err := New(deps(st, newClient(t), fs.Addr, clk))
			if err != nil {
				t.Fatal(err)
			}
			start(t, m, clk)
			clk.step(t, 70, nil) // polls fail; rediscovery at +60 s hears the lie
			if fs.Calls("Marstek.GetDevice") == 0 {
				t.Fatal("no rediscovery happened: the test proves nothing")
			}
			mu.Lock()
			failing = false
			mu.Unlock()
			clk.step(t, 70, nil)
			all, err := st.Stations()
			if err != nil {
				t.Fatal(err)
			}
			if len(all) != 1 || all[0].IP != "127.0.0.1" {
				t.Fatalf("stations = %+v, want the one at 127.0.0.1", all)
			}
			if snap := m.Latest(); snap.Link != link.Live || snap.Station.IP != "127.0.0.1" {
				t.Fatalf("link = %s, station IP = %q; want LIVE at 127.0.0.1", snap.Link, snap.Station.IP)
			}
		})
		t.Run("first run/"+lie, func(t *testing.T) {
			st := openStore(t)
			fs := startStation(t, testMac)
			fs.info.IP = lie
			clk := newClock(t0)
			m, err := New(deps(st, newClient(t), fs.Addr, clk))
			if err != nil {
				t.Fatal(err)
			}
			if all, _ := st.Stations(); len(all) != 1 || all[0].IP != "127.0.0.1" {
				t.Fatalf("stations = %+v, want the one at 127.0.0.1", all)
			}
			start(t, m, clk)
			clk.step(t, 3, nil) // the first poll comes 1 s after the discovery
			if fs.Calls("ES.GetStatus") == 0 || m.Latest().Link != link.Live {
				t.Fatalf("polls = %d, link = %s; want the station polled where it answered", fs.Calls("ES.GetStatus"), m.Latest().Link)
			}
		})
	}
}

// m-2: only a bare IPv4 is a poll address; a stored "host:port" or a name is not.
func TestResolveAcceptsBareIPv4Only(t *testing.T) {
	m := &Monitor{port: 30000}
	if a := m.resolve("192.168.50.50"); a == nil || a.String() != "192.168.50.50:30000" {
		t.Fatalf("bare IPv4 = %v", a)
	}
	for _, bad := range []string{"192.168.50.50:9", "example.invalid", "example.invalid:30000", "::1", "[::1]:30000", ""} {
		if a := m.resolve(bad); a != nil {
			t.Errorf("resolve(%q) = %v, want nil", bad, a)
		}
	}
}

// m-4: when the store fails to close a gap, the monitor must keep it and try
// again, with the time the gap really ended.
func TestMonitorRetriesClosingGapAfterStoreFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	// A second connection breaks and repairs UPDATEs on gaps.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 70.0
	last := t0.Add(-time.Minute)
	if err := st.InsertSample(model.Sample{StationID: testMac, At: last, Source: model.SourceLAN, SOC: &soc}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenGap(store.Gap{StationID: testMac, Start: last, Reason: store.GapPCSleep}); err != nil {
		t.Fatal(err)
	}
	exec(`CREATE TRIGGER gaps_fail BEFORE UPDATE ON gaps BEGIN SELECT RAISE(ABORT, 'simulated write failure'); END`)
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // first sample at t0: the gap ends, the UPDATE fails
	clk.step(t, 6, nil)
	gaps, err := st.Gaps(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].End != nil {
		t.Fatalf("gaps = %+v; the simulated failure did not happen", gaps)
	}
	exec(`DROP TRIGGER gaps_fail`)
	clk.step(t, 6, nil) // the next sample retries
	gaps, err = st.Gaps(testMac, t0.Add(-time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].End == nil || !gaps[0].End.Equal(t0) {
		t.Fatalf("gaps = %+v, want the one gap closed at t0", gaps)
	}
}

// run starts m like start, but returns a stop function: a test can end one
// run of the app and start the next on the same store.
func run(t *testing.T, m *Monitor, clk *fakeClock) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	var once sync.Once
	stop = func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(stop)
	clk.waitIdle(t)
	return stop
}

// RF1 / review focus 1: the grid goes off at 22:00, the PC is shut down at
// 23:00 and started again at 09:00 with the grid back. The outage ended at
// some unknown time in the night: no grid_back toast, its end is "not after"
// the morning start, and the night is a gap, not time without light.
func TestMonitorOutageAcrossPCOffNight(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	evening := time.Date(2026, 11, 22, 21, 59, 0, 0, time.Local)
	clk := newClock(evening)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	stop := run(t, m, clk)
	clk.step(t, 90, func(sec int) {
		if sec == 59 { // the poll at 22:00:00 is the first to read "absent"
			fs.gridOff(true)
		}
	})
	clk.advance(t, 10*time.Second, evening.Add(time.Hour+time.Minute).Sub(clk.Now()))
	if ts := drainToasts(m); len(ts[notify.GridLost]) != 1 {
		t.Fatalf("run 1 grid_lost toasts = %+v, want one", ts[notify.GridLost])
	}
	stop() // 23:00: the PC is shut down; the outage stays open in the DB

	fs.gridOff(false)
	morning := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	clk2 := newClock(morning)
	m2, err := New(deps(st, newClient(t), fs.Addr, clk2))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m2, clk2)
	clk2.step(t, 29, func(sec int) {
		if g := m2.Latest().Grid; g != detect.GridUnknownState {
			t.Fatalf("grid at +%d s = %v, want unknown right after the start", sec, g)
		}
	})
	clk2.step(t, 61, nil)

	outs, err := st.Outages(testMac, evening, morning.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].End == nil {
		t.Fatalf("outages = %+v, want one closed", outs)
	}
	if end := *outs[0].End; end.Before(morning) || end.After(morning.Add(35*time.Second)) {
		t.Fatalf("outage end = %s, want 09:00:00..09:00:35", end)
	}
	s := m2.Latest()
	b := s.PreviousBounds
	if s.Previous == nil || s.Previous.ID != outs[0].ID || b == nil {
		t.Fatalf("previous = %+v, bounds = %+v", s.Previous, b)
	}
	// Observed: 22:00..23:00 and the seconds after 09:00 until the grid was confirmed.
	if !b.EndApprox || !b.AtLeast || b.Known < time.Hour || b.Known > time.Hour+35*time.Second {
		t.Fatalf("previous bounds = %+v, want EndApprox, AtLeast, Known ≈ 1 h", b)
	}
	if ts := drainToasts(m2); len(ts[notify.GridBack]) != 0 {
		t.Fatalf("grid_back toasts = %+v, want none", ts[notify.GridBack])
	}
	// Today 00:00..09:00 lies in the gap, so it is not counted as without light.
	if s.NoLightToday > 35*time.Second || !s.NoLightAtLeast || s.GapsToday < 9*time.Hour {
		t.Fatalf("today: no light %s (at least %v), gaps %s; want ≤ 35 s, at least, ≥ 9 h",
			s.NoLightToday, s.NoLightAtLeast, s.GapsToday)
	}
}

// The grid goes off while the PC sleeps: the outage is found after the
// pc_sleep gap, so its start is approximate, and it is still toasted.
func TestMonitorOutageStartingInSleepGapIsPartialAndToasted(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 60, nil) // grid present, confirmed
	m.Suspend()
	clk.advance(t, time.Minute, time.Hour)
	fs.gridOff(true)
	m.Resume()
	clk.step(t, 90, nil) // resync 30 s, then the 30 s debounce
	outs, err := st.Outages(testMac, t0, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || !outs[0].Partial || outs[0].End != nil {
		t.Fatalf("outages = %+v, want one open partial", outs)
	}
	ts := drainToasts(m)
	if l := ts[notify.GridLost]; len(l) != 1 || l[0].Args["approx"] != "1" || l[0].Args["approxReason"] != string(store.GapPCSleep) {
		t.Fatalf("grid_lost toasts = %+v, want one with approx=1, approxReason=pc_sleep", l)
	}
	// The window: from the last sample before the sleep (the gap's start).
	gaps, err := st.Gaps(testMac, t0, t0.Add(2*time.Hour))
	if err != nil || len(gaps) != 1 {
		t.Fatalf("gaps = %+v, %v; want the sleep gap", gaps, err)
	}
	if l := ts[notify.GridLost]; l[0].Args["from"] != gaps[0].Start.Local().Format(time.RFC3339) ||
		l[0].Args["to"] != outs[0].Start.Local().Format(time.RFC3339) {
		t.Fatalf("grid_lost args = %+v, want from %s to %s", l[0].Args, gaps[0].Start, outs[0].Start)
	}
}

// A restart of 2 min (no gap) in the middle of an outage misses nothing: the
// outage continues as known, and the grid's return is exact and toasted.
func TestMonitorShortRestartKeepsOutageExact(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	stop := run(t, m, clk)
	clk.step(t, 100, func(sec int) {
		if sec == 39 {
			fs.gridOff(true)
		}
	})
	stop()
	if _, ok, err := st.CurrentOutage(testMac); err != nil || !ok {
		t.Fatalf("run 1 left no open outage (%v)", err)
	}

	fs.gridOff(false)
	restart := clk.Now().Add(2 * time.Minute)
	clk2 := newClock(restart)
	m2, err := New(deps(st, newClient(t), fs.Addr, clk2))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m2, clk2)
	clk2.step(t, 40, nil)
	s := m2.Latest()
	if s.Current != nil || s.Previous == nil || s.Previous.End == nil || s.Previous.Partial {
		t.Fatalf("current = %+v, previous = %+v; want the outage closed, exact", s.Current, s.Previous)
	}
	if !s.Previous.End.Equal(restart) {
		t.Fatalf("outage end = %s, want the first sample of run 2 (%s)", s.Previous.End, restart)
	}
	if b := s.PreviousBounds; b == nil || b.EndApprox || b.StartApprox {
		t.Fatalf("previous bounds = %+v, want exact", b)
	}
	if l := drainToasts(m2)[notify.GridBack]; len(l) != 1 || l[0].Tag != strconv.FormatInt(s.Previous.ID, 10) {
		t.Fatalf("grid_back toasts = %+v, want one for outage %d", l, s.Previous.ID)
	}
}

// First run ever at 09:00: the night was not observed, so it is reported as
// a gap and never as "no outages today".
func TestMonitorMorningBootGapsToday(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	morning := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	clk := newClock(morning)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 10, nil)
	if s := m.Latest(); s.GapsToday != 9*time.Hour || s.OutagesToday != 0 {
		t.Fatalf("gaps today = %s, outages = %d; want 9 h, 0", s.GapsToday, s.OutagesToday)
	}
}

// The grid_back toast is rendered with the snapshot published with it
// (app.enrichToast reads PreviousBounds when Previous is the toast's outage):
// that snapshot must already carry the closed outage and its bounds.
func TestMonitorGridBackToastCarriesPreviousBounds(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	var back *notify.Toast
	var snap Snapshot
	clk.step(t, 125, func(sec int) {
		switch sec {
		case 39:
			fs.gridOff(true)
		case 79:
			fs.gridOff(false)
		}
		if back == nil {
			// Latest is set before the toasts of a publish are queued.
			s := m.Latest()
			if l := drainToasts(m)[notify.GridBack]; len(l) > 0 {
				back, snap = &l[0], s
			}
		}
	})
	if back == nil {
		t.Fatal("no grid_back toast")
	}
	p, b := snap.Previous, snap.PreviousBounds
	if p == nil || strconv.FormatInt(p.ID, 10) != back.Tag || p.End == nil || b == nil {
		t.Fatalf("snapshot with the toast: previous = %+v, bounds = %+v, toast tag %s", p, b, back.Tag)
	}
	if b.AtLeast || b.Known != p.End.Sub(p.Start) || b.Known != 40*time.Second {
		t.Fatalf("bounds = %+v, want exact, Known = 40 s", b)
	}
}

// A short restart with the grid present continues GridOn: a loss after the
// restart is exact (not partial, not approximate).
func TestMonitorShortRestartKeepsGridOnExact(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	stop := run(t, m, clk)
	clk.step(t, 60, nil)
	stop()

	clk2 := newClock(clk.Now().Add(2 * time.Minute))
	m2, err := New(deps(st, newClient(t), fs.Addr, clk2))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m2, clk2)
	clk2.step(t, 80, func(sec int) {
		if sec == 9 { // absent from the poll at +10 s
			fs.gridOff(true)
		}
	})
	outs, err := st.Outages(testMac, t0, clk2.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].Partial {
		t.Fatalf("outages = %+v, want one exact", outs)
	}
	if l := drainToasts(m2)[notify.GridLost]; len(l) != 1 || l[0].Args["approx"] != "" {
		t.Fatalf("grid_lost toasts = %+v, want one, not approximate", l)
	}
}

// setFlows sets the station's SOC and its ongrid, offgrid and PV power.
// BatteryW = -(on + off - pv); grid absent needs off > 20 W, so off = pv
// shows the grid absent with the battery idle.
func (s *station) setFlows(soc, on, off, pv float64) {
	s.mu.Lock()
	s.soc, s.on, s.off, s.pv = soc, on, off, pv
	s.mu.Unlock()
}

// pollSeconds steps the clock n seconds and returns the seconds (1..n) at
// which ES.GetStatus was called; each runs after the step's check, if any.
func pollSeconds(t *testing.T, clk *fakeClock, fs *station, n int, each func(sec int)) []int {
	t.Helper()
	var secs []int
	np := fs.Calls("ES.GetStatus")
	clk.step(t, n, func(sec int) {
		if c := fs.Calls("ES.GetStatus"); c != np {
			np = c
			secs = append(secs, sec)
		}
		if each != nil {
			each(sec)
		}
	})
	return secs
}

func intervals(secs []int) []int {
	var out []int
	for i := 1; i < len(secs); i++ {
		out = append(out, secs[i]-secs[i-1])
	}
	return out
}

func (m *Monitor) cadenceNow() Cadence {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cadence
}

// startCalm starts a monitor on a full idle station with the grid present
// and runs it through the 2 min warmup into Calm (the poll at t0+120 s goes
// calm; the next one is at t0+140 s, after which the clock is left).
func startCalm(t *testing.T) (*Monitor, *fakeClock, *station, *store.Store) {
	t.Helper()
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.setFlows(100, 0, 0, 0)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // poll at t0
	clk.step(t, 140, nil)
	if c := m.cadenceNow(); c != Calm {
		t.Fatalf("cadence after warmup = %v, want Calm", c)
	}
	return m, clk, fs, st
}

func TestMonitorGoesCalmWhenFullAndGridOn(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.setFlows(100, 0, 0, 0)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // poll at t0
	secs := pollSeconds(t, clk, fs, 300, nil)
	var want []int
	for s := 5; s <= 120; s += 5 {
		want = append(want, s)
	}
	for s := 140; s <= 300; s += 20 {
		want = append(want, s)
	}
	if !equalInts(secs, want) {
		t.Fatalf("poll seconds = %v, want %v", secs, want)
	}
	evs, err := st.Events(testMac, "monitor.cadence", t0, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Text != "calm" || !evs[0].At.Equal(t0.Add(120*time.Second)) {
		t.Fatalf("cadence events = %+v, want one \"calm\" at +120 s", evs)
	}
}

func TestMonitorNoStaleInCalm(t *testing.T) {
	m, clk, fs, _ := startCalm(t)
	secs := pollSeconds(t, clk, fs, 600, func(sec int) {
		if l := m.Latest().Link; l != link.Live {
			t.Fatalf("+%d s in Calm: link = %s, want LIVE", sec, l)
		}
	})
	for _, g := range intervals(secs) {
		if g != 20 {
			t.Fatalf("intervals in Calm = %v, want all 20 s", intervals(secs))
		}
	}
}

func TestMonitorCalmToActiveOnGridAbsent(t *testing.T) {
	m, clk, fs, st := startCalm(t) // last poll at +140 s, next at +160 s
	clk.step(t, 1, nil)
	fs.setFlows(100, 0, 300, 300) // grid gone at +141 s, battery still idle
	changed := 141
	confirmed := 0
	secs := pollSeconds(t, clk, fs, 80, func(sec int) {
		if _, ok, _ := st.CurrentOutage(testMac); ok && confirmed == 0 {
			confirmed = changed + sec
		}
	})
	for i := range secs {
		secs[i] += changed
	}
	if len(secs) < 2 || secs[0] != 160 || secs[1] != 165 {
		t.Fatalf("poll seconds after the change = %v, want 160, 165, …", secs)
	}
	if confirmed == 0 || confirmed > changed+20+30 {
		t.Fatalf("GridLost confirmed at +%d s, want by +%d s", confirmed, changed+50)
	}
	if c := m.cadenceNow(); c != Active {
		t.Fatalf("cadence = %v, want Active", c)
	}
}

func TestMonitorCalmToActiveOnPower(t *testing.T) {
	m, clk, fs, _ := startCalm(t) // next poll at +160 s
	clk.step(t, 1, nil)
	fs.setFlows(100, 400, 0, 0) // −400 W, grid present
	secs := pollSeconds(t, clk, fs, 40, nil)
	if len(secs) < 2 || secs[0] != 19 || secs[1] != 24 {
		t.Fatalf("poll seconds = %v, want 19 (+160 s) then 24 (5 s later)", secs)
	}
	if c := m.cadenceNow(); c != Active {
		t.Fatalf("cadence = %v, want Active", c)
	}
}

func TestMonitorRequestCountCalmVsActive(t *testing.T) {
	_, clk, fs, _ := startCalm(t)
	calm := len(pollSeconds(t, clk, fs, 600, nil))

	st := openStore(t)
	fa := startStation(t, testMac)
	fa.setFlows(80, 400, 0, 0) // discharging: never calm
	seedStation(t, st, fa.Addr.IP.String())
	clk2 := newClock(t0)
	m2, err := New(deps(st, newClient(t), fa.Addr, clk2))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m2, clk2)
	clk2.step(t, 140, nil)
	active := len(pollSeconds(t, clk2, fa, 600, nil))

	if calm < 28 || calm > 32 || active < 118 || active > 122 {
		t.Fatalf("ES.GetStatus in 10 min: calm %d (want ≈30), active %d (want ≈120)", calm, active)
	}
	if r := float64(active) / float64(calm); r < 3.5 || r > 4.5 {
		t.Fatalf("active/calm = %.2f, want 3.5..4.5", r)
	}
}

func TestMonitorResumeRestartsWarmup(t *testing.T) {
	m, clk, fs, _ := startCalm(t)
	m.Suspend()
	clk.step(t, 30, nil)
	m.Resume()
	if c := m.cadenceNow(); c != Active {
		t.Fatalf("cadence after Resume = %v, want Active", c)
	}
	secs := pollSeconds(t, clk, fs, 200, nil)
	// Resumed at +0: polls at +1, +6, … up to +121 (the first ≥ 2 min), then 20 s.
	var want []int
	for s := 1; s <= 121; s += 5 {
		want = append(want, s)
	}
	for s := 141; s <= 200; s += 20 {
		want = append(want, s)
	}
	if !equalInts(secs, want) {
		t.Fatalf("poll seconds after Resume = %v, want %v", secs, want)
	}
}

// cfg is a Config with the test defaults (deps) and the given active interval.
func cfg(poll time.Duration) Config {
	return Config{Notify: notify.Settings{LowThreshold: 20}, Poll: poll}
}

// startActive starts a monitor on a discharging station (never calm).
func startActive(t *testing.T) (*Monitor, *fakeClock, *station, *store.Store) {
	t.Helper()
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.setFlows(80, 400, 0, 0)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // poll at t0
	return m, clk, fs, st
}

func TestMonitorConfigurePollInterval(t *testing.T) {
	m, clk, fs, _ := startActive(t)
	if secs := pollSeconds(t, clk, fs, 20, nil); !equalInts(secs, []int{5, 10, 15, 20}) {
		t.Fatalf("poll seconds at 5 s = %v", secs)
	}
	m.Configure(cfg(10 * time.Second)) // right after the poll at +20 s
	if secs := pollSeconds(t, clk, fs, 40, nil); !equalInts(secs, []int{10, 20, 30, 40}) {
		t.Fatalf("poll seconds after Configure(10 s) = %v, want every 10 s", secs)
	}
	m.Configure(cfg(2 * time.Second)) // right after the poll at +40 s
	secs := pollSeconds(t, clk, fs, 10, nil)
	if len(secs) == 0 || secs[0] < 1 {
		t.Fatalf("poll seconds after Configure(2 s) = %v, want the first ≥ 1 s after the last call", secs)
	}
	if !equalInts(secs, []int{2, 4, 6, 8, 10}) {
		t.Fatalf("poll seconds after Configure(2 s) = %v, want every 2 s", secs)
	}
}

func TestMonitorConfigureFreshFollowsInterval(t *testing.T) {
	m, clk, fs, _ := startActive(t)
	clk.step(t, 5, nil)
	m.Configure(cfg(30 * time.Second))
	// Fresh = 3 × 30 s: 80 s without a poll answer is still LIVE.
	fs.Silence(true)
	clk.step(t, 80, func(sec int) {
		if l := m.Latest().Link; l != link.Live {
			t.Fatalf("+%d s after Configure(30 s), station silent: link = %s, want LIVE", sec, l)
		}
	})
}

func TestMonitorCalmUsesConfiguredActiveInterval(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.setFlows(100, 0, 0, 0)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	m.Configure(cfg(10 * time.Second))
	start(t, m, clk) // poll at t0
	secs := pollSeconds(t, clk, fs, 200, nil)
	var want []int
	for s := 10; s <= 120; s += 10 { // Active: 10 s
		want = append(want, s)
	}
	for s := 140; s <= 200; s += 20 { // Calm: max(20 s, 10 s)
		want = append(want, s)
	}
	if !equalInts(secs, want) {
		t.Fatalf("poll seconds = %v, want %v", secs, want)
	}
	if c := m.cadenceNow(); c != Calm {
		t.Fatalf("cadence = %v, want Calm", c)
	}
	m.Configure(cfg(30 * time.Second)) // right after the poll at +200 s
	if secs := pollSeconds(t, clk, fs, 90, nil); !equalInts(secs, []int{30, 60, 90}) {
		t.Fatalf("calm poll seconds after Configure(30 s) = %v, want every 30 s", secs)
	}
}

func TestMonitorConfigureLowThreshold(t *testing.T) {
	m, clk, fs, _ := startActive(t)
	clk.step(t, 10, nil)
	c := cfg(5 * time.Second)
	c.Notify.LowThreshold = 30
	m.Configure(c)
	if got := m.Latest().LowThreshold; got != 30 {
		t.Fatalf("Snapshot.LowThreshold = %v, want 30", got)
	}
	fs.setFlows(31, 400, 0, 0)
	clk.step(t, 10, nil)
	drainToasts(m)
	fs.setFlows(29, 400, 0, 0)
	clk.step(t, 10, nil)
	low := drainToasts(m)[notify.LowCharge]
	if len(low) != 1 || low[0].Args["threshold"] != "30" {
		t.Fatalf("low_charge toasts = %+v, want one at threshold 30", low)
	}
	if got := m.Latest().LowThreshold; got != 30 {
		t.Fatalf("Snapshot.LowThreshold after samples = %v, want 30", got)
	}
}

func TestMonitorSnapshotLowThresholdFromDeps(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Latest().LowThreshold; got != 20 {
		t.Fatalf("Snapshot.LowThreshold = %v, want 20 (Deps.Notify)", got)
	}
}

func TestMonitorConfigureUpdatesDNDSettings(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), nil, clk))
	if err != nil {
		t.Fatal(err)
	}
	c := cfg(5 * time.Second)
	c.Notify.QuietEnabled, c.Notify.QuietStart, c.Notify.QuietEnd = true, "23:00", "06:30"
	m.Configure(c)
	m.SetDND("morning")
	m.mu.Lock()
	got := m.policy.DNDUntil()
	m.mu.Unlock()
	l := t0.Local()
	want := time.Date(l.Year(), l.Month(), l.Day(), 6, 30, 0, 0, time.Local)
	if !want.After(l) {
		want = want.AddDate(0, 0, 1)
	}
	if got == nil || !got.Equal(want) {
		t.Fatalf("DND until = %v, want %v (the new QuietEnd)", got, want)
	}
}

func TestMonitorConfigureStationMinSOC(t *testing.T) {
	m, clk, fs, _ := startActive(t)
	if got := m.Latest().Profile.MinSOC; got != 10 {
		t.Fatalf("profile MinSOC = %v, want the model's 10", got)
	}
	c := cfg(5 * time.Second)
	c.StationMinSOC = 25
	m.Configure(c)
	if got := m.Latest().Profile.MinSOC; got != 25 {
		t.Fatalf("profile MinSOC after Configure = %v, want 25", got)
	}
	fs.setFlows(24, 0, 0, 0) // idle at 24 %: under the new cutoff
	clk.step(t, 10, nil)
	if got := m.Latest().Mode; got != detect.Cutoff {
		t.Fatalf("mode at 24 %% with MinSOC 25 = %v, want Cutoff", got)
	}
	m.mu.Lock()
	mp := m.machine.Profile().MinSOC
	m.mu.Unlock()
	if mp != 25 {
		t.Fatalf("link machine MinSOC = %v, want 25 (likely-dead)", mp)
	}
}

func TestMonitorStationMinSOCFromDeps(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	d := deps(st, newClient(t), nil, newClock(t0))
	d.StationMinSOC = 15
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Latest().Profile.MinSOC; got != 15 {
		t.Fatalf("profile MinSOC = %v, want 15", got)
	}
}

func TestMonitorRenameStation(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RenameStation("  Дача "); err != nil {
		t.Fatal(err)
	}
	if got := m.Latest().Station.Name; got != "Дача" {
		t.Fatalf("Snapshot.Station.Name = %q, want «Дача»", got)
	}
	all, err := st.Stations()
	if err != nil || len(all) != 1 || all[0].Name != "Дача" {
		t.Fatalf("stations = %+v, %v; want the row renamed", all, err)
	}
	if err := m.RenameStation("   "); !errors.Is(err, settings.ErrNameEmpty) {
		t.Fatalf("empty name: err = %v, want ErrNameEmpty", err)
	}
	if got := m.Latest().Station.Name; got != "Дача" {
		t.Fatalf("name after a rejected rename = %q", got)
	}
}

// Settings change on the UI goroutine while samples flow: run with -race.
func TestMonitorConfigureWhileSamplesFlow(t *testing.T) {
	m, clk, fs, _ := startActive(t)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		polls := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			c := cfg(polls[i%len(polls)])
			c.Notify.LowThreshold = float64(10 + 10*(i%5))
			c.Notify.Enabled = map[notify.Kind]bool{notify.Full: i%2 == 0}
			c.StationMinSOC = 5 + i%26
			m.Configure(c)
			m.SetDND("morning")
			_ = m.Latest()
			time.Sleep(100 * time.Microsecond)
		}
	}()
	clk.step(t, 60, func(sec int) { fs.setFlows(float64(80-sec%40), 400, 0, 0) })
	close(done)
	wg.Wait()
	if l := m.Latest().Link; l != link.Live {
		t.Fatalf("link = %s, want LIVE", l)
	}
}

func TestMonitorNoLinkToast(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac) // grid on: the link loss is no_link, not likely_dead
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	d.Notify.NoLinkAfter = 10 * time.Minute
	d.Notify.Enabled = map[notify.Kind]bool{notify.NoLink: true}
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil)
	fs.Silence(true)
	clk.advance(t, 15*time.Second, 16*time.Minute) // NO_LINK after ≈ 5 min, then 10 min more
	if s := m.Latest().Link; s != link.NoLink {
		t.Fatalf("link = %s, want NO_LINK", s)
	}
	ts := drainToasts(m)
	nl := ts[notify.NoLink]
	if len(nl) != 1 || !strings.HasPrefix(nl[0].Tag, "link-") || nl[0].StationID != testMac {
		t.Fatalf("no_link toasts = %+v", nl)
	}
	fs.Silence(false)
	clk.advance(t, time.Second, time.Minute)
	ts = drainToasts(m)
	r := ts[notify.NoLinkRestored]
	if len(r) != 1 || r[0].Tag != nl[0].Tag || r[0].Args["soc"] != "80" {
		t.Fatalf("no_link_restored toasts = %+v, want one with tag %q", r, nl[0].Tag)
	}
	if len(ts[notify.NoLink]) != 0 {
		t.Fatalf("no_link toasted again: %+v", ts[notify.NoLink])
	}
}

// journalCodes returns the journal entries since t0 − 1 h (newest first) and
// their codes, oldest first.
func journalCodes(t *testing.T, m *Monitor, clk *fakeClock) ([]journal.Entry, []string) {
	t.Helper()
	es, err := m.Journal(t0.Add(-time.Hour), clk.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for i := len(es) - 1; i >= 0; i-- {
		codes = append(codes, es[i].ID[:strings.LastIndex(es[i].ID, ":")])
	}
	return es, codes
}

func findEntry(es []journal.Entry, key string) *journal.Entry {
	for i := range es {
		if es[i].Key == key {
			return &es[i]
		}
	}
	return nil
}

func TestMonitorJournalNoLinkAndRestore(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac) // grid on: the loss is no_link, not likely_dead
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil)
	fs.Silence(true)
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if s := m.Latest().Link; s != link.NoLink {
		t.Fatalf("link = %s, want NO_LINK", s)
	}
	es, codes := journalCodes(t, m, clk)
	if len(codes) != 1 || codes[0] != journal.CodeAPISilent {
		t.Fatalf("journal while silent = %v, want [%s]", codes, journal.CodeAPISilent)
	}
	if es[0].Level != journal.Warning || es[0].Group != journal.Connection || es[0].Key != "conn.event.apiSilent" {
		t.Fatalf("apiSilent entry = %+v", es[0])
	}
	if n := m.Latest().NewEvents; n != 1 {
		t.Fatalf("NewEvents = %d, want 1", n)
	}
	fs.Silence(false)
	clk.advance(t, time.Second, 90*time.Second) // the backoff is at 60 s by now
	es, codes = journalCodes(t, m, clk)
	if len(codes) != 2 || codes[1] != journal.CodeLANRestored {
		t.Fatalf("journal after restore = %v, want apiSilent then lanRestored", codes)
	}
	r := findEntry(es, "conn.event.lanRestored")
	ms, err := strconv.ParseInt(r.Args["duration_ms"], 10, 64)
	if err != nil {
		t.Fatalf("lanRestored args = %v", r.Args)
	}
	// From the last sample before the silence to the first after it: the
	// 6 min of silence plus at most one 60 s backoff wait.
	if d := time.Duration(ms) * time.Millisecond; d < 6*time.Minute || d > 7*time.Minute+5*time.Second {
		t.Fatalf("lanRestored duration = %v, want ≈ 6m (6m..7m)", d)
	}
	if r.Level != journal.Info {
		t.Fatalf("lanRestored level = %s, want info", r.Level)
	}
}

func TestMonitorJournalLanLostWhenPCOffline(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	var mu sync.Mutex
	online := true
	d.PCOnline = func() bool { mu.Lock(); defer mu.Unlock(); return online }
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil)
	fs.Silence(true)
	mu.Lock()
	online = false
	mu.Unlock()
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if _, codes := journalCodes(t, m, clk); len(codes) != 1 || codes[0] != journal.CodeLANLost {
		t.Fatalf("journal = %v, want [%s]", codes, journal.CodeLANLost)
	}
}

// startLow starts a monitor on a station discharging 400 W at soc, with the
// given toast switches.
func startLow(t *testing.T, soc float64, enabled map[notify.Kind]bool) (*Monitor, *fakeClock, *station, *store.Store) {
	t.Helper()
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.setFlows(soc, 400, 0, 0)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	d.Notify.Enabled = enabled
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	return m, clk, fs, st
}

func TestMonitorJournalLowChargeEvenWhenToastOff(t *testing.T) {
	m, clk, fs, _ := startLow(t, 21, map[notify.Kind]bool{notify.LowCharge: false})
	clk.step(t, 10, nil)
	fs.setFlows(19, 400, 0, 0)
	clk.step(t, 10, nil)
	if low := drainToasts(m)[notify.LowCharge]; len(low) != 0 {
		t.Fatalf("low_charge toasts = %+v, want none (switched off)", low)
	}
	es, codes := journalCodes(t, m, clk)
	if len(codes) != 1 || codes[0] != journal.CodeLowCharge {
		t.Fatalf("journal = %v, want [%s]", codes, journal.CodeLowCharge)
	}
	if e := es[0]; e.Key != "journal.lowCharge" || e.TodoKey != "journal.lowCharge.todo" || e.Args["soc"] != "19" ||
		e.Level != journal.Warning || e.Group != journal.Station {
		t.Fatalf("lowCharge entry = %+v", e)
	}
	clk.step(t, 30, nil) // still below: once per crossing
	if _, codes := journalCodes(t, m, clk); len(codes) != 1 {
		t.Fatalf("journal = %v, want one lowCharge", codes)
	}
}

func TestMonitorJournalCutoffWhenGridAbsent(t *testing.T) {
	m, clk, fs, _ := startLow(t, 12, nil)
	clk.step(t, 10, nil)
	fs.setFlows(10, 0, 100, 100) // grid absent, battery 0 W at the 10 % floor
	clk.step(t, 20, nil)
	if _, codes := journalCodes(t, m, clk); slicesContains(codes, journal.CodeCutoff) {
		t.Fatalf("cutoff logged before it held %v: %v", modeHold, codes)
	}
	clk.step(t, 60, nil)
	es, codes := journalCodes(t, m, clk)
	n := 0
	for _, c := range codes {
		if c == journal.CodeCutoff {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("journal = %v, want one %s", codes, journal.CodeCutoff)
	}
	if e := findEntry(es, "journal.cutoff"); e.Args["soc"] != "10" || e.TodoKey != "journal.cutoff.todo" || e.Level != journal.Warning {
		t.Fatalf("cutoff entry = %+v", e)
	}
}

func TestMonitorJournalNoCutoffWithGrid(t *testing.T) {
	m, clk, fs, _ := startLow(t, 12, nil)
	clk.step(t, 10, nil)
	fs.setFlows(10, 0, 0, 0) // grid present, battery 0 W at the floor
	clk.step(t, 90, nil)
	if _, codes := journalCodes(t, m, clk); slicesContains(codes, journal.CodeCutoff) {
		t.Fatalf("journal = %v: cutoff with the grid present", codes)
	}
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestMonitorJournalIPChanged(t *testing.T) {
	m, clk, fs, _ := startActive(t)
	clk.step(t, 10, nil)
	// The stored IP goes stale: polling fails and rediscovery finds the
	// station at its real address.
	m.mu.Lock()
	m.station.IP, m.addr = "127.0.0.2", m.resolve("127.0.0.2")
	m.mu.Unlock()
	clk.advance(t, time.Second, 3*time.Minute)
	es, _ := journalCodes(t, m, clk)
	e := findEntry(es, "conn.event.ipChanged")
	if e == nil || e.Args["ip"] != fs.Addr.IP.String() || e.Level != journal.Info || e.Group != journal.Connection {
		t.Fatalf("journal = %+v, want ipChanged to %s", es, fs.Addr.IP)
	}
}

func TestMonitorJournalLikelyOut(t *testing.T) {
	m, clk, fs, _ := startLow(t, 14, nil)
	fs.setFlows(14, 0, 300, 0) // grid absent, discharging near the floor
	clk.step(t, 60, nil)
	fs.Silence(true)
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if s := m.Latest().Link; s != link.LikelyDead {
		t.Fatalf("link = %s, want LIKELY_DEAD", s)
	}
	es, codes := journalCodes(t, m, clk)
	e := findEntry(es, "journal.likelyOut")
	if e == nil || e.Args["soc"] != "14" || e.Args["at"] == "" || e.TodoKey != "journal.likelyOut.todo" || e.Group != journal.Station {
		t.Fatalf("journal = %v %+v, want likelyOut with soc 14", codes, es)
	}
	if slicesContains(codes, journal.CodeAPISilent) || slicesContains(codes, journal.CodeLANLost) {
		t.Fatalf("journal = %v: LIKELY_DEAD without NO_LINK logs no link loss", codes)
	}
}

func TestMonitorJournalLikelyOutOncePerGapAcrossRestart(t *testing.T) {
	m, clk, fs, st := startLow(t, 14, nil)
	fs.setFlows(14, 0, 300, 0)
	clk.step(t, 60, nil)
	fs.Silence(true)
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if _, codes := journalCodes(t, m, clk); !slicesContains(codes, journal.CodeLikelyOut) {
		t.Fatalf("journal = %v, want likelyOut", codes)
	}
	// Restart while the station is still silent: the adopted likely_dead gap
	// was already logged.
	m2, err := New(deps(st, newClient(t), nil, newClock(clk.Now().Add(time.Minute))))
	if err != nil {
		t.Fatal(err)
	}
	m2.mu.Lock()
	now := m2.clock.Now()
	m2.apply(m2.machine.Tick(now, true), now)
	m2.mu.Unlock()
	es, err := m2.Journal(t0.Add(-time.Hour), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if e.Key == "journal.likelyOut" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("likelyOut entries after restart = %d, want 1", n)
	}
}

func TestMonitorJournalAppGaps(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	id, err := st.OpenGap(store.Gap{StationID: testMac, Start: t0.Add(-3 * time.Hour), Reason: store.GapAppNotRunning})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CloseGap(id, t0.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	es, err := m.Journal(t0.Add(-24*time.Hour), t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 || es[0].Key != "journal.appOff" || es[0].Group != journal.App {
		t.Fatalf("journal = %+v, want one appOff", es)
	}
}

func TestMonitorNewEventsResetOnSeen(t *testing.T) {
	m, clk, fs, st := startLow(t, 21, nil)
	clk.step(t, 10, nil)
	fs.setFlows(19, 400, 0, 0)
	clk.step(t, 10, nil)
	fs.setFlows(26, 400, 0, 0) // re-arms
	clk.step(t, 10, nil)
	fs.setFlows(19, 400, 0, 0)
	clk.step(t, 10, nil)
	s := m.Latest()
	if s.NewEvents != 2 || s.JournalSeenAt != nil {
		t.Fatalf("NewEvents = %d, JournalSeenAt = %v; want 2, nil", s.NewEvents, s.JournalSeenAt)
	}
	if err := m.MarkJournalSeen(clk.Now()); err != nil {
		t.Fatal(err)
	}
	seen := clk.Now()
	s = m.Latest()
	if s.NewEvents != 0 || s.JournalSeenAt == nil || !s.JournalSeenAt.Equal(seen) {
		t.Fatalf("after MarkJournalSeen: NewEvents = %d, JournalSeenAt = %v; want 0, %v", s.NewEvents, s.JournalSeenAt, seen)
	}
	if v, ok, _ := st.GetSetting(SettingJournalSeen); !ok || v == "" {
		t.Fatalf("%s not stored", SettingJournalSeen)
	}
	// Restart on the same database.
	m2, err := New(deps(st, newClient(t), nil, newClock(seen.Add(time.Minute))))
	if err != nil {
		t.Fatal(err)
	}
	s = m2.Latest()
	if s.NewEvents != 0 || s.JournalSeenAt == nil || !s.JournalSeenAt.Equal(seen) {
		t.Fatalf("after restart: NewEvents = %d, JournalSeenAt = %v; want 0, %v", s.NewEvents, s.JournalSeenAt, seen)
	}
}

func (m *Monitor) journalReadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.journalReads
}

func TestMonitorNewEventsCached(t *testing.T) {
	m, clk, fs, _ := startLow(t, 30, nil)
	clk.step(t, 10, nil)
	before := m.journalReadCount()
	clk.step(t, 60, nil)
	if n := m.journalReadCount(); n != before {
		t.Fatalf("journal reads over 60 ticks = %d, want 0", n-before)
	}
	if n := m.Latest().NewEvents; n != 0 {
		t.Fatalf("NewEvents = %d, want 0", n)
	}
	fs.setFlows(19, 400, 0, 0)
	clk.step(t, 10, nil)
	if n := m.Latest().NewEvents; n != 1 {
		t.Fatalf("NewEvents after one warning = %d, want 1", n)
	}
}

func countCode(codes []string, code string) int {
	n := 0
	for _, c := range codes {
		if c == code {
			n++
		}
	}
	return n
}

// startCutoff: an outage opens (offgrid 300 W, no grid), then the station
// turns its output off at the 10 % floor (ongrid 0, offgrid 0) for 90 s.
func startCutoff(t *testing.T) (*Monitor, *fakeClock, *station, *store.Store) {
	t.Helper()
	m, clk, fs, st := startLow(t, 12, nil)
	fs.setFlows(12, 0, 300, 0)
	clk.step(t, 60, nil)
	if m.Latest().Current == nil {
		t.Fatal("no outage open")
	}
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 90, nil)
	return m, clk, fs, st
}

func TestMonitorCutoffOutputOffKeepsOutageOpen(t *testing.T) {
	m, clk, fs, _ := startCutoff(t)
	if _, codes := journalCodes(t, m, clk); countCode(codes, journal.CodeCutoff) != 1 {
		t.Fatalf("journal = %v, want one %s", codes, journal.CodeCutoff)
	}
	if m.Latest().Current == nil {
		t.Fatal("outage closed while the station's output is off (grid unknown at the floor)")
	}
	if gb := drainToasts(m)[notify.GridBack]; len(gb) != 0 {
		t.Fatalf("grid_back toasts = %+v, want none", gb)
	}
	fs.setFlows(10, -2000, 0, 0) // the grid is back: the station charges
	clk.step(t, 60, nil)
	if c := m.Latest().Current; c != nil {
		t.Fatalf("outage still open after charging resumed: %+v", c)
	}
	if gb := drainToasts(m)[notify.GridBack]; len(gb) != 1 {
		t.Fatalf("grid_back toasts = %+v, want one", gb)
	}
}

func TestFloorGridOnlyAtTheFloorWithoutFlow(t *testing.T) {
	p := model.Profile{MinSOC: 10}
	smp := func(soc, on, off float64) model.Sample {
		return model.Sample{SOC: &soc, OngridW: &on, OffgridW: &off, Grid: model.GridPresent}
	}
	cases := []struct {
		s    model.Sample
		want model.GridSignal
	}{
		{smp(10, 0, 0), model.GridUnknown},
		{smp(11, 15, 10), model.GridUnknown},
		{smp(12, 0, 0), model.GridPresent},  // above the floor: idle with the grid
		{smp(10, 0, 30), model.GridPresent}, // power flows: not cut off
		{smp(10, -25, 0), model.GridPresent},
	}
	for i, c := range cases {
		if got := floorGrid(c.s, p); got != c.want {
			t.Errorf("case %d: floorGrid = %v, want %v", i, got, c.want)
		}
	}
	a := smp(10, 0, 300)
	a.Grid = model.GridAbsent
	if got := floorGrid(a, p); got != model.GridAbsent {
		t.Errorf("absent at the floor = %v, want absent", got)
	}
}

func TestMonitorCutoffOnceAcrossRestart(t *testing.T) {
	m, clk, fs, st := startCutoff(t)
	if _, codes := journalCodes(t, m, clk); countCode(codes, journal.CodeCutoff) != 1 {
		t.Fatalf("journal = %v, want one cutoff before the restart", codes)
	}
	clk2 := newClock(clk.Now().Add(10 * time.Second)) // a short restart: no gap
	m2, err := New(deps(st, newClient(t), fs.Addr, clk2))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m2, clk2)
	clk2.step(t, 90, nil)
	if _, codes := journalCodes(t, m2, clk2); countCode(codes, journal.CodeCutoff) != 1 {
		t.Fatalf("journal after restart = %v, want still one cutoff", codes)
	}
}

func TestMonitorLinkLossOnceAcrossRestart(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil)
	fs.Silence(true)
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if _, codes := journalCodes(t, m, clk); countCode(codes, journal.CodeAPISilent) != 1 {
		t.Fatalf("journal = %v, want one apiSilent", codes)
	}
	// Restart during the absence: the adopted no_link gap was logged.
	clk2 := newClock(clk.Now().Add(time.Minute))
	m2, err := New(deps(st, newClient(t), fs.Addr, clk2))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m2, clk2)
	clk2.advance(t, 5*time.Second, 2*time.Minute)
	if s := m2.Latest().Link; s != link.NoLink {
		t.Fatalf("link after restart = %s, want NO_LINK", s)
	}
	if _, codes := journalCodes(t, m2, clk2); countCode(codes, journal.CodeAPISilent) != 1 {
		t.Fatalf("journal after restart = %v, want still one apiSilent", codes)
	}
}

// The grid is on and the station sits idle at the floor (MinSOC+1, then a
// 1-p.p. bounce to MinSOC): nothing changes, so the poll stays calm.
func TestMonitorCalmIdleAtTheFloor(t *testing.T) {
	m, clk, fs, st := startCalm(t)
	fs.setFlows(11, 0, 0, 0)
	clk.step(t, 120, nil) // the SOC drop leaves the 1 min window
	if c := m.cadenceNow(); c != Calm {
		t.Fatalf("cadence at MinSOC+1 idle = %v, want Calm", c)
	}
	from := clk.Now()
	for i := 0; i < 4; i++ {
		fs.setFlows(10+float64(i%2), 0, 0, 0)
		secs := pollSeconds(t, clk, fs, 60, nil)
		for _, g := range intervals(secs) {
			if g != 20 {
				t.Fatalf("round %d: intervals = %v, want all 20 s", i, intervals(secs))
			}
		}
	}
	evs, err := st.Events(testMac, "monitor.cadence", from, clk.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 {
		t.Fatalf("cadence changes during the floor bounce = %+v, want none", evs)
	}
	if g := m.Latest().Grid; g != detect.GridOn {
		t.Fatalf("grid = %v, want still on", g)
	}
}

// startCutoffOutage opens an outage at SOC 12 (offgrid 300 W) and returns
// with the station still discharging.
func startCutoffOutage(t *testing.T) (*Monitor, *fakeClock, *station) {
	t.Helper()
	m, clk, fs, _ := startLow(t, 12, nil)
	fs.setFlows(12, 0, 300, 0)
	clk.step(t, 60, nil)
	if m.Latest().Current == nil {
		t.Fatal("no outage open")
	}
	return m, clk, fs
}

// A 1-p.p. bounce (MinSOC → MinSOC+1 → MinSOC) with the output off does not
// restart the cutoff hold: 20 s + 10 s + 20 s at the floor log one cutoff.
func TestMonitorCutoffHoldSurvivesFloorBounce(t *testing.T) {
	m, clk, fs := startCutoffOutage(t)
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 20, nil)
	fs.setFlows(11, 0, 0, 0)
	clk.step(t, 10, nil)
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 20, nil)
	if _, codes := journalCodes(t, m, clk); countCode(codes, journal.CodeCutoff) != 1 {
		t.Fatalf("journal = %v, want one %s", codes, journal.CodeCutoff)
	}
	if m.Latest().Current == nil {
		t.Fatal("outage closed during the floor bounce")
	}
}

// Leaving the floor (idle at MinSOC+2) still breaks the hold.
func TestMonitorCutoffHoldBrokenAboveTheFloor(t *testing.T) {
	m, clk, fs := startCutoffOutage(t)
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 20, nil)
	fs.setFlows(12, 0, 0, 0)
	clk.step(t, 10, nil)
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 20, nil)
	if _, codes := journalCodes(t, m, clk); countCode(codes, journal.CodeCutoff) != 0 {
		t.Fatalf("journal = %v, want no %s", codes, journal.CodeCutoff)
	}
}

// The grid comes back while the station idles at MinSOC+1 (the load runs on
// pass-through, ongrid < −20 W): the outage closes, and a later Cutoff is not
// logged as a cutoff without the grid.
func TestMonitorCutoffHoldEndsWhenTheOutageCloses(t *testing.T) {
	m, clk, fs := startCutoffOutage(t)
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 20, nil)
	fs.setFlows(11, -300, 300, 0)
	clk.step(t, 45, nil)
	if c := m.Latest().Current; c != nil {
		t.Fatalf("outage still open with the grid back: %+v", c)
	}
	fs.setFlows(10, 0, 0, 0)
	clk.step(t, 40, nil)
	if _, codes := journalCodes(t, m, clk); countCode(codes, journal.CodeCutoff) != 0 {
		t.Fatalf("journal = %v, want no %s", codes, journal.CodeCutoff)
	}
}

// Task 11 carry-over: marking the journal seen clears only what the user saw
// (up to the newest entry on screen): a warning logged meanwhile stays new.
// The mark never moves back and never past now.
func TestMonitorMarkJournalSeenUpToKeepsLaterEvents(t *testing.T) {
	m, clk, fs, st := startLow(t, 21, nil)
	clk.step(t, 10, nil)
	fs.setFlows(19, 400, 0, 0)
	clk.step(t, 10, nil)
	fs.setFlows(26, 400, 0, 0) // re-arms
	clk.step(t, 10, nil)
	fs.setFlows(19, 400, 0, 0)
	clk.step(t, 10, nil)
	es, err := m.Journal(t0.Add(-time.Hour), clk.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var lows []journal.Entry // newest first
	for _, e := range es {
		if e.Key == "journal.lowCharge" {
			lows = append(lows, e)
		}
	}
	if len(lows) != 2 || m.Latest().NewEvents != 2 {
		t.Fatalf("low charge entries = %+v, NewEvents = %d; want 2, 2", lows, m.Latest().NewEvents)
	}
	older, newer := lows[1].At, lows[0].At

	// The user saw only the first warning: the second stays new.
	if err := m.MarkJournalSeen(older); err != nil {
		t.Fatal(err)
	}
	if s := m.Latest(); s.NewEvents != 1 || s.JournalSeenAt == nil || !s.JournalSeenAt.Equal(older) {
		t.Fatalf("seen up to the first: NewEvents = %d, JournalSeenAt = %v; want 1, %v", s.NewEvents, s.JournalSeenAt, older)
	}
	if err := m.MarkJournalSeen(newer); err != nil {
		t.Fatal(err)
	}
	if s := m.Latest(); s.NewEvents != 0 || !s.JournalSeenAt.Equal(newer) {
		t.Fatalf("seen up to the second: NewEvents = %d, JournalSeenAt = %v; want 0, %v", s.NewEvents, s.JournalSeenAt, newer)
	}
	// A late call from an older screen does not bring the first back.
	if err := m.MarkJournalSeen(older); err != nil {
		t.Fatal(err)
	}
	if s := m.Latest(); s.NewEvents != 0 || !s.JournalSeenAt.Equal(newer) {
		t.Fatalf("older mark: NewEvents = %d, JournalSeenAt = %v; want 0, %v", s.NewEvents, s.JournalSeenAt, newer)
	}
	// A time ahead of the clock is cut to now.
	if err := m.MarkJournalSeen(clk.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if s := m.Latest(); !s.JournalSeenAt.Equal(clk.Now()) {
		t.Fatalf("future mark: JournalSeenAt = %v, want now %v", s.JournalSeenAt, clk.Now())
	}
	// A stored mark ahead of the clock (set back since) is replaced by the
	// next one, else it would hide the new warnings until the clock caught up.
	if err := st.SetSetting(SettingJournalSeen, clk.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	m2, err := New(deps(st, newClient(t), nil, newClock(clk.Now())))
	if err != nil {
		t.Fatal(err)
	}
	if err := m2.MarkJournalSeen(older); err != nil {
		t.Fatal(err)
	}
	if s := m2.Latest(); s.NewEvents != 1 || s.JournalSeenAt == nil || !s.JournalSeenAt.Equal(older) {
		t.Fatalf("after a mark ahead of the clock: NewEvents = %d, JournalSeenAt = %v; want 1, %v", s.NewEvents, s.JournalSeenAt, older)
	}
}

// The 30-day chart reads hour buckets from the store (SOCBuckets), with the
// same result as averaging the minutes and raw samples in Go did before.
func TestSOCSeries30dReadsBuckets(t *testing.T) {
	st := openStore(t)
	seedStation(t, st, "127.0.0.1")
	// 34 days of samples every 7 min, SOC sawing 20..95, some unknown; a
	// Compact as of 5 days ahead aggregates the first 29 of them per minute,
	// so the 30-day range holds both aggregated minutes and raw samples.
	for i, at := 0, t0.AddDate(0, 0, -34); at.Before(t0); i, at = i+1, at.Add(7*time.Minute) {
		x := model.Sample{StationID: testMac, At: at, Source: model.SourceLAN, Grid: model.GridPresent}
		if i%11 != 3 {
			soc := 20 + float64(i%76)
			x.SOC = &soc
		}
		if err := st.InsertSample(x); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Compact(t0.AddDate(0, 0, 5)); err != nil {
		t.Fatal(err)
	}
	m, err := New(deps(st, newClient(t), nil, newClock(t0)))
	if err != nil {
		t.Fatal(err)
	}
	pts, _, err := m.SOCSeries("30d")
	if err != nil {
		t.Fatal(err)
	}

	// Before: minutes first, then raw samples, averaged per hour in Go.
	from, to := t0.Add(-30*24*time.Hour), t0.Add(time.Second)
	mins, err := st.MinuteSeries(testMac, from, to)
	if err != nil || len(mins) == 0 {
		t.Fatalf("minutes = %d, %v; want some", len(mins), err)
	}
	raw, err := st.Samples(testMac, from, to)
	if err != nil || len(raw) == 0 {
		t.Fatalf("raw = %d, %v; want some", len(raw), err)
	}
	type acc struct {
		sum float64
		n   int
	}
	var keys []time.Time
	sums := map[time.Time]*acc{}
	add := func(at time.Time, soc *float64) {
		k := at.Truncate(time.Hour)
		a := sums[k]
		if a == nil {
			a = &acc{}
			sums[k] = a
			keys = append(keys, k)
		}
		if soc != nil {
			a.sum += *soc
			a.n++
		}
	}
	for _, p := range mins {
		add(p.At, p.SOCAvg)
	}
	for _, s := range raw {
		add(s.At, s.SOC)
	}
	if len(pts) != len(keys) {
		t.Fatalf("points = %d, want %d", len(pts), len(keys))
	}
	for i, k := range keys {
		a := sums[k]
		if !pts[i].At.Equal(k) {
			t.Fatalf("point %d at %v, want %v", i, pts[i].At, k)
		}
		if a.n == 0 {
			if pts[i].SOC != nil {
				t.Fatalf("point %d SOC = %v, want unknown", i, *pts[i].SOC)
			}
			continue
		}
		if want := a.sum / float64(a.n); pts[i].SOC == nil || math.Abs(*pts[i].SOC-want) > 1e-9 {
			t.Fatalf("point %d SOC = %v, want %v", i, pts[i].SOC, want)
		}
	}
}

// One open gap per station (schema v5). One left by a previous run is
// adopted at startup whatever its start; ahead of the clock (set back since),
// it ends at its start, never before it.
func TestMonitorAdoptsAGapAheadOfTheClock(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	ahead := t0.Add(time.Hour)
	id, err := st.OpenGap(store.Gap{StationID: testMac, Start: ahead, Reason: store.GapPCSleep})
	if err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil)
	if g, ok, err := st.CurrentGap(testMac); err != nil || ok {
		t.Fatalf("open gap with data coming = %+v, %v, %v; want none", g, ok, err)
	}
	gs, err := st.Gaps(testMac, t0.Add(-time.Hour), ahead.Add(time.Hour))
	if err != nil || len(gs) != 1 || gs[0].ID != id || gs[0].End == nil || !gs[0].End.Equal(ahead) {
		t.Fatalf("gaps = %+v, %v; want gap %d ending at its start %v", gs, err, id, ahead)
	}
}

// An open gap written since startup (this run does not know it) takes the
// next absence instead of the refused OpenGap, keeps its own start, and is
// closed when the data is back instead of staying open for good.
func TestMonitorAdoptsAnUnknownOpenGap(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	clk.step(t, 30, nil)
	other := t0.Add(-time.Hour)
	id, err := st.OpenGap(store.Gap{StationID: testMac, Start: other, Reason: store.GapPCSleep})
	if err != nil {
		t.Fatal(err)
	}
	fs.Silence(true)
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if g, ok, err := st.CurrentGap(testMac); err != nil || !ok || g.ID != id || !g.Start.Equal(other) {
		t.Fatalf("open gap = %+v, %v, %v; want gap %d from %v", g, ok, err, id, other)
	}
	fs.Silence(false)
	clk.advance(t, time.Second, 90*time.Second)
	if g, ok, err := st.CurrentGap(testMac); err != nil || ok {
		t.Fatalf("open gap after the data is back = %+v, %v, %v; want none", g, ok, err)
	}
	gs, err := st.Gaps(testMac, other, clk.Now())
	if err != nil || len(gs) != 1 || gs[0].ID != id || !gs[0].Start.Equal(other) || gs[0].End == nil || !gs[0].End.After(t0) {
		t.Fatalf("gaps = %+v, %v; want gap %d from %v, closed after the silence", gs, err, id, other)
	}
}

// Task 20c: a poll that started before Suspend and answered after it (the
// PC went to sleep mid-call, then woke up) is dropped: no sample from a
// station state the monitor no longer tracks, and the poll Resume scheduled
// right away is not pushed back by the stale reply.
func TestPollResultAfterSuspendIsDropped(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	// A long call timeout: the held reply must still arrive as an answer.
	m, err := New(deps(st, lan.NewClient(conn, 5*time.Second), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk) // poll 1 at t0

	entered, release := make(chan struct{}, 1), make(chan struct{})
	fs.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
		entered <- struct{}{}
		<-release
		return map[string]any{"bat_soc": 70, "bat_cap": 5120, "ongrid_power": 0, "offgrid_power": 0}, nil
	})
	clk.Advance(5 * time.Second) // poll 2 starts at t0+5s and waits for the reply
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("poll 2 never reached the station")
	}
	m.Suspend()
	clk.Advance(time.Hour) // asleep (the loop is inside the call: no tick fires)
	m.Resume()
	woke := clk.Now()
	close(release)
	clk.waitIdle(t)

	samples, err := st.Samples(testMac, t0.Add(-time.Minute), clk.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1 (the reply that came after Suspend is dropped)", len(samples))
	}
	m.mu.Lock()
	next, fails := m.nextPoll, m.fails
	m.mu.Unlock()
	if !next.Equal(woke) || fails != 0 {
		t.Fatalf("nextPoll = %v, fails = %d; want %v (Resume's), 0", next, fails, woke)
	}
}

// Task 20c: Snapshot.Seq grows with every publication (Task 21 drops a
// snapshot older than the one shown).
func TestSnapshotHasSequence(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	first := m.Latest().Seq
	if first == 0 {
		t.Fatal("the snapshot New publishes has Seq 0")
	}
	start(t, m, clk)
	prev := m.Latest().Seq
	if prev <= first {
		t.Fatalf("Seq after the first step = %d, want > %d", prev, first)
	}
	for i := 0; i < 3; i++ {
		clk.step(t, 1, nil)
		cur := m.Latest().Seq
		if cur <= prev {
			t.Fatalf("tick %d: Seq = %d, want > %d", i+1, cur, prev)
		}
		prev = cur
	}
	m.SetDND("off") // publishes nothing
	m.Suspend()     // publishes
	if got := m.Latest().Seq; got != prev+1 {
		t.Fatalf("Seq after Suspend = %d, want %d", got, prev+1)
	}
	select {
	case s := <-m.Snapshots():
		if s.Seq != m.Latest().Seq {
			t.Fatalf("channel snapshot Seq = %d, want the latest %d", s.Seq, m.Latest().Seq)
		}
	default:
		t.Fatal("no snapshot on the channel")
	}
}

// Task 20c: the daily maintenance (capacity estimates, Prune, Compact) runs
// outside m.mu: while it is stuck in the store, the UI's calls (a settings
// change that publishes a snapshot, sleep and wake, DND) and Latest return.
func TestMaintenanceRunsOutsideLock(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	maintenanceHook = func() {
		once.Do(func() {
			close(entered)
			<-release
		})
	}
	t.Cleanup(func() { maintenanceHook = nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	var freed sync.Once
	t.Cleanup(func() { freed.Do(func() { close(release) }) })

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the start maintenance never ran")
	}
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		m.Configure(cfg(5 * time.Second))
		m.SetDND("off")
		m.Suspend()
		m.Resume()
		_ = m.Latest()
	}()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("Configure / SetDND / Suspend / Resume waited for the maintenance (it holds m.mu)")
	}
	freed.Do(func() { close(release) })
	clk.waitIdle(t)
}

// Task 20c: PCOnline (it may look at the network adapters) is asked outside
// m.mu, so a slow answer does not hold up the UI's calls either.
func TestPCOnlineAskedOutsideLock(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	var hold atomic.Bool
	entered, release := make(chan struct{}, 1), make(chan struct{})
	d.PCOnline = func() bool {
		if hold.Load() {
			hold.Store(false)
			entered <- struct{}{}
			<-release
		}
		return true
	}
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	var freed sync.Once
	t.Cleanup(func() { freed.Do(func() { close(release) }) })
	hold.Store(true)
	clk.Advance(time.Second)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("PCOnline was not asked on the tick")
	}
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		m.Configure(cfg(5 * time.Second))
		m.Suspend()
		m.Resume()
	}()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("Configure / Suspend / Resume waited for PCOnline (asked under m.mu)")
	}
	freed.Do(func() { close(release) })
	clk.waitIdle(t)
}

// Task 20c: Deps.Ctx cancels the first discovery: New returns ctx.Err() at
// once instead of listening for the whole discoverWait.
func TestFirstDiscoveryCancelledByCtx(t *testing.T) {
	old := discoverWait
	discoverWait = 5 * time.Second
	t.Cleanup(func() { discoverWait = old })
	st := openStore(t)
	fs := startStation(t, testMac)
	fs.Silence(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := deps(st, newClient(t), fs.Addr, newClock(t0))
	d.Ctx = ctx
	time.AfterFunc(100*time.Millisecond, cancel)
	began := time.Now()
	_, err := New(d)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if took := time.Since(began); took >= discoverWait {
		t.Fatalf("New took %v, want it back before discoverWait (%v)", took, discoverWait)
	}
	if all, _ := st.Stations(); len(all) != 0 {
		t.Fatalf("stations = %+v, want none", all)
	}
}

// An unconfirmed grid change is published (GridPending, GridPendingSince) from
// the first contradicting live sample until it is confirmed or contradicted;
// the confirmed Grid is untouched meanwhile.
func TestMonitorSnapshotGridPending(t *testing.T) {
	m, clk, fs, _ := startCalm(t)
	if s := m.Latest(); s.Grid != detect.GridOn || s.GridPending != detect.GridUnknownState || s.GridPendingSince != nil {
		t.Fatalf("settled: grid %v pending %v since %v", s.Grid, s.GridPending, s.GridPendingSince)
	}
	// A short blip: pending appears, then disappears on a contradicting sample.
	fs.setFlows(100, 0, 300, 300)
	sawPending := false
	pollSeconds(t, clk, fs, 25, func(int) {
		if s := m.Latest(); s.GridPending == detect.GridOff {
			sawPending = true
			if s.Grid != detect.GridOn || s.GridPendingSince == nil {
				t.Errorf("pending: grid %v since %v", s.Grid, s.GridPendingSince)
			}
		}
	})
	if !sawPending {
		t.Fatal("no pending Off after the grid went absent")
	}
	fs.setFlows(100, 0, 0, 0)
	pollSeconds(t, clk, fs, 30, nil)
	if s := m.Latest(); s.GridPending != detect.GridUnknownState || s.GridPendingSince != nil || s.Grid != detect.GridOn {
		t.Fatalf("after contradiction: grid %v pending %v since %v", s.Grid, s.GridPending, s.GridPendingSince)
	}
	// A real outage: confirmed after 30 s, pending clears.
	fs.setFlows(100, 0, 300, 300)
	pollSeconds(t, clk, fs, 90, nil)
	if s := m.Latest(); s.Grid != detect.GridOff || s.GridPending != detect.GridUnknownState || s.GridPendingSince != nil {
		t.Fatalf("after confirmation: grid %v pending %v since %v", s.Grid, s.GridPending, s.GridPendingSince)
	}
}

// Never published while the link is not LIVE.
func TestMonitorSnapshotGridPendingOnlyWhenLive(t *testing.T) {
	m, clk, fs, _ := startCalm(t)
	fs.setFlows(100, 0, 300, 300)
	pollSeconds(t, clk, fs, 25, nil)
	if m.Latest().GridPending != detect.GridOff {
		t.Fatal("setup: no pending Off")
	}
	fs.Silence(true)
	clk.advance(t, 5*time.Second, 6*time.Minute)
	if s := m.Latest(); s.Link == link.Live || s.GridPending != detect.GridUnknownState || s.GridPendingSince != nil {
		t.Fatalf("link %v pending %v since %v, want none off-LIVE", s.Link, s.GridPending, s.GridPendingSince)
	}
}

func TestMonitorLogsShortLinkDrop(t *testing.T) {
	linkDrops := func(es []journal.Entry) []journal.Entry {
		var out []journal.Entry
		for _, e := range es {
			if strings.HasPrefix(e.ID, journal.CodeLinkDropped+":") {
				out = append(out, e)
			}
		}
		return out
	}
	run := func(t *testing.T) (*Monitor, *fakeClock, *station) {
		st := openStore(t)
		fs := startStation(t, testMac)
		seedStation(t, st, fs.Addr.IP.String())
		clk := newClock(t0)
		m, err := New(deps(st, newClient(t), fs.Addr, clk))
		if err != nil {
			t.Fatal(err)
		}
		start(t, m, clk)
		clk.step(t, 30, nil)
		return m, clk, fs
	}

	t.Run("40 s of silence", func(t *testing.T) {
		m, clk, fs := run(t)
		last := m.Latest().Last.At // the last good sample
		fs.Silence(true)
		clk.step(t, 39, nil)
		if s := m.Latest().Link; s != link.Stale {
			t.Fatalf("link = %s, want STALE", s)
		}
		fs.Silence(false)
		clk.step(t, 10, func(int) {}) // the poll at +40 s answers; stop at the first LIVE
		if s := m.Latest().Link; s != link.Live {
			t.Fatalf("link = %s, want LIVE", s)
		}
		es, _ := journalCodes(t, m, clk)
		got := linkDrops(es)
		if len(got) != 1 {
			t.Fatalf("link drops = %+v, want exactly one", got)
		}
		e := got[0]
		if e.Level != journal.Info || e.Group != journal.Connection || e.Key != "journal.linkDropped" || e.TodoKey != "" {
			t.Fatalf("entry = %+v", e)
		}
		from, err1 := time.Parse(time.RFC3339, e.Args["from"])
		to, err2 := time.Parse(time.RFC3339, e.Args["to"])
		secs, err3 := strconv.Atoi(e.Args["seconds"])
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatalf("args = %v", e.Args)
		}
		if !from.Equal(last.Truncate(time.Second)) && from.Sub(last).Abs() > time.Second {
			t.Errorf("from = %v, want the last good sample %v", from, last)
		}
		if !to.After(from) || to.After(m.Latest().Last.At) {
			t.Errorf("to = %v, want the first sample after the silence (from %v)", to, from)
		}
		if secs < 39 || secs > 45 || secs != int(to.Sub(from).Seconds()) {
			t.Errorf("seconds = %d (from %v to %v), want ≈ 40", secs, from, to)
		}
	})

	t.Run("one missed poll", func(t *testing.T) {
		m, clk, fs := run(t)
		fs.Silence(true)
		clk.step(t, 6, nil)
		fs.Silence(false)
		clk.step(t, 40, nil)
		es, _ := journalCodes(t, m, clk)
		if got := linkDrops(es); len(got) != 0 {
			t.Fatalf("link drops = %+v, want none", got)
		}
	})

	t.Run("a gap covers it", func(t *testing.T) {
		m, clk, fs := run(t)
		fs.Silence(true)
		clk.advance(t, 5*time.Second, 6*time.Minute)
		fs.Silence(false)
		clk.advance(t, time.Second, 90*time.Second)
		es, codes := journalCodes(t, m, clk)
		if got := linkDrops(es); len(got) != 0 {
			t.Fatalf("link drops = %+v, want none (codes %v)", got, codes)
		}
		if findEntry(es, "conn.event.lanRestored") == nil {
			t.Fatalf("the gap's own entry is missing: %v", codes)
		}
	})
}

// Task 5 fix: the restart gap after the one-click update (Deps.AppUpdated)
// is an app update, not «Додаток не працював».
func TestMonitorStartupGapAfterUpdate(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String())
	soc := 70.0
	if err := st.InsertSample(model.Sample{StationID: testMac, At: t0.Add(-time.Hour), Source: model.SourceLAN,
		SOC: &soc, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	d.AppUpdated = true
	m, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	start(t, m, clk)
	card := m.Latest().GapCard
	if card == nil || card.Reason != store.GapAppUpdate || !card.Start.Equal(t0.Add(-time.Hour)) {
		t.Fatalf("gap card = %+v, want an app_update gap from t0-1h", card)
	}
	gaps, err := st.Gaps(testMac, t0.Add(-2*time.Hour), t0.Add(time.Hour))
	if err != nil || len(gaps) != 1 || gaps[0].Reason != store.GapAppUpdate {
		t.Fatalf("stored gaps = %+v, %v", gaps, err)
	}
	es, err := m.Journal(t0.Add(-24*time.Hour), t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if e.Key == "journal.appUpdate" && e.Group == journal.App {
			n++
		}
		if e.Key == "journal.appOff" {
			t.Fatalf("appOff entry after an update: %+v", e)
		}
	}
	if n != 1 {
		t.Fatalf("journal = %+v, want one appUpdate", es)
	}
}
