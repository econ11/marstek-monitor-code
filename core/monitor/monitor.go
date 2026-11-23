// Package monitor is the polling orchestrator: it polls one station over LAN,
// stores samples and feeds the link state machine, the grid tracker, the
// forecasts and the notification policy, and publishes a Snapshot after every
// sample and every 1 s tick.
//
// One goroutine (Run) owns the polling loop. All mutable state is guarded by
// one mutex that is never held during network I/O, the daily maintenance or
// Deps.PCOnline, so Suspend/Resume/SetDND/Configure and Latest never wait
// for them.
package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/energy"
	"github.com/econ11/marstek-monitor-code/core/forecast"
	"github.com/econ11/marstek-monitor-code/core/health"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// ErrNoStation is returned by New when the database holds no station and
// either Deps.AutoAdd is off or discovery found none with a BLE MAC.
var ErrNoStation = errors.New("monitor: no station found")

// discoverWait is how long a discovery broadcast collects replies (a var so
// tests can shorten it).
var discoverWait = 2 * time.Second

const (
	defaultPoll = 5 * time.Second
	// minCallGap: the station drops back-to-back UDP calls; any two calls
	// (polls and discovery) are at least this far apart.
	minCallGap      = time.Second
	maxBackoff      = 60 * time.Second
	rediscoverEvery = 60 * time.Second
	tickEvery       = time.Second
	compactEvery    = 24 * time.Hour
	gridDebounce    = 30 * time.Second
	smootherWindow  = 5 * time.Minute
	// modeHold: a battery mode must last this long before the forecast starts
	// over for it; power hovering around the ±20 W band must not restart it.
	modeHold        = 30 * time.Second
	avgRefreshEvery = time.Hour
	outageLookback  = 30 * 24 * time.Hour
	// energyChunk: the most sample time one tick rolls up, so the backfill of
	// an upgraded database (up to 30 days of raw samples, ~2 minutes of ticks)
	// runs lazily after start instead of blocking it. energyBackfill: older
	// raw samples are compacted away (store.Compact), so there is nothing to
	// roll up before it.
	energyChunk    = 6 * time.Hour
	energyBackfill = 30 * 24 * time.Hour // = store.rawKeep
	// energyBehind: a rollup further behind now than this is still
	// backfilling past days (the steady lag is under 2 min).
	energyBehind = 10 * time.Minute
	// energyWarnEvery: a failing rollup is logged at most this often.
	energyWarnEvery = 10 * time.Minute
	// batteryEvery: how often the snapshot's cycles and health are re-read
	// (they move slowly; the Battery tab reads them live).
	batteryEvery = 10 * time.Minute
	toastBuffer  = 64
	eventSource  = "monitor"
	// eventSourceStart marks the link.delayed event of a hole that spans the
	// time the app was not running; the chart breaks its line there.
	eventSourceStart = "monitor.start"
	// settingGapCard holds the id of the gap whose card the user closed.
	settingGapCard = "monitor.gap_card_dismissed"
)

// Clock abstracts time for tests.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Deps are the monitor's collaborators. Clock nil = system clock;
// PollInterval 0 = 5 s; PCOnline nil = always online. Notify, PollInterval,
// RetentionDays and StationMinSOC are the start settings (Configure changes
// them later; StationMinSOC 0 = the model's own). Targets returns the
// discovery addresses (asked before every discovery: interfaces come and go;
// nil = no discovery); the port of the first one is also the station's port.
type Deps struct {
	Store         *store.Store
	Client        *lan.Client
	Targets       func() []*net.UDPAddr
	Clock         Clock
	PollInterval  time.Duration
	Boot          bool
	Notify        notify.Settings
	RetentionDays int
	StationMinSOC int
	PCOnline      func() bool
	// AutoAdd: with no station in the database, true discovers the first one
	// that answers and adds it (plan 1, tests, -simulate); false returns
	// ErrNoStation without touching the network (the first-run screen adds
	// the station, core/onboard).
	AutoAdd bool
	// Ctx cancels the first discovery in New (AutoAdd): the app shutting down
	// does not wait for it. nil = context.Background().
	Ctx context.Context
	// AppUpdated: this start follows the one-click update (app.TakeUpdateRestart),
	// so the gap since the last sample is app_update, not app_not_running.
	AppUpdated bool
}

type action int

const (
	actNone action = iota
	actPoll
	actDiscover
)

type Monitor struct {
	d     Deps
	clock Clock
	poll  time.Duration
	port  int

	mu         sync.Mutex // guards everything below; never held during network I/O
	station    store.Station
	addr       *net.UDPAddr
	profile    model.Profile
	machine    *link.Machine
	tracker    *detect.Tracker
	policy     *notify.Policy
	smoother   *forecast.Smoother
	smoothMode detect.BatteryMode // mode the smoother's samples belong to
	// A new mode takes over the forecast only after it has held modeHold;
	// until then its samples are also collected here.
	pending      *forecast.Smoother
	pendingMode  detect.BatteryMode
	pendingSince time.Time
	pendingSeen  int            // samples in a row that read pendingMode
	outbox       []notify.Toast // toasts waiting for the next publish
	last         *model.Sample

	gapID int64 // DB id of the open gap, 0 = none
	// gapEnd: the gap gapID ended then, but the store failed to close it; the
	// close is retried with this time on every sample.
	gapEnd       *time.Time
	machineGap   bool // the link machine has a gap open
	firstBatch   bool // the next effects are the machine's first (startup) batch
	startupGap   *store.Gap
	gapDismissed bool

	gapReason store.GapReason // reason of the open gap gapID
	// approxReason: the gap the data last came out of (app_not_running when
	// none since start); a grid_lost found across it is toasted with it.
	approxReason store.GapReason
	// approxFrom: the start of that gap, i.e. the last observation before it (the
	// previous run's last sample at start; zero = nothing observed before). A
	// grid_lost found across the gap went off between it and its own start.
	approxFrom time.Time
	// approxTag / approxTagReason: the outage (toast Tag) found across a gap
	// and the reason of that gap, fixed when the outage opened.
	approxTag       string
	approxTagReason store.GapReason
	// beforeStart: the newest sample of the previous run, kept until the
	// first live sample (m.last is overwritten by then).
	beforeStart *model.Sample
	sawLive     bool // a live sample was seen since start
	// noLinkSince: when the link last entered NO_LINK / LIKELY_DEAD from
	// another state (the no-connection toast's episode start).
	noLinkSince time.Time

	cur, prev    *store.Outage
	avgOutageW   *float64
	avgAt        time.Time
	lastCall     time.Time
	nextPoll     time.Time
	pollStart    time.Time // start of the last poll (Configure reschedules from it)
	fails        int
	failingSince time.Time
	lastDiscover time.Time
	nextCompact  time.Time
	// energyNext: the first minute not rolled up yet (zero until the first
	// step reads it from the store). Only rollupEnergy (poll loop) uses it.
	energyNext   time.Time
	energyWarnAt time.Time // last rollup warning (energyWarn)
	// energyFailed: the last rollup attempt failed (energyWarn sets it, a
	// successful tick clears it). A failing rollup is not «backfilling»: it
	// makes no progress, and the report must not promise it will.
	energyFailed bool
	// battery: the snapshot's cycles and health, refreshed every batteryEvery
	// and after each capacity pass (refreshBattery); batteryAt: when.
	battery   BatteryState
	batteryAt time.Time
	suspended bool
	// sleeps counts Suspend calls: a call that started before one has its
	// reply dropped (the station state it read is from before the sleep).
	sleeps uint64
	// maintMu serializes the daily maintenance and PruneNow (both store work
	// outside m.mu): a prune between the capacity pass's read and its upsert
	// would see the pass store again estimates the prune just deleted. Taken
	// before m.mu, never while holding it.
	maintMu sync.Mutex

	// Adaptive polling (cadence.go): the current cadence, the start of the
	// active warmup (start or Resume) and the live SOC readings of the last
	// CalmSOCWindow.
	cadence  Cadence
	warmFrom time.Time
	socs     []socReading

	// The user's event journal (core/journal): gapStart is
	// the start of the open gap gapID (for the restore's duration);
	// lostLogged: the link loss of this absence is logged (reset when data
	// is back); deadLogged: likelyOut is logged for the open gap; cutoffSince
	// / cutoffLogged: the Cutoff hold and its entry (re-armed by charging).
	gapStart     time.Time
	lostLogged   bool
	deadLogged   bool
	cutoffSince  time.Time
	cutoffLogged bool
	// seenAt is journal.seen_at (nil = never viewed); newEvents is the
	// cached count of warnings since then, recomputed only when it can
	// change (refreshNewEvents); journalReads counts those recomputations.
	seenAt       *time.Time
	newEvents    int
	journalReads int

	latestMu sync.Mutex
	latest   Snapshot
	seq      uint64 // Snapshot.Seq of latest (latestMu)
	snaps    chan Snapshot
	toasts   chan notify.Toast
}

// New takes the first station from the database, or discovers one and
// stores the first found with a BLE MAC.
func New(d Deps) (*Monitor, error) {
	if d.Store == nil || d.Client == nil {
		return nil, errors.New("monitor: Store and Client are required")
	}
	m := &Monitor{d: d, clock: d.Clock, poll: d.PollInterval, port: lan.DefaultPort,
		firstBatch: true, snaps: make(chan Snapshot, 1), toasts: make(chan notify.Toast, toastBuffer)}
	if m.clock == nil {
		m.clock = systemClock{}
	}
	if m.poll <= 0 {
		m.poll = defaultPoll
	}
	if ts := m.targets(); len(ts) > 0 && ts[0].Port != 0 {
		m.port = ts[0].Port
	}
	now := m.clock.Now()

	sts, err := d.Store.Stations()
	if err != nil {
		return nil, fmt.Errorf("monitor: load stations: %w", err)
	}
	if len(sts) > 0 {
		m.station = sts[0]
	} else if !d.AutoAdd {
		return nil, ErrNoStation
	} else if m.station, err = m.discoverFirst(now); err != nil {
		return nil, err
	}
	m.addr = m.resolve(m.station.IP)
	m.profile = profileFor(m.station.Model, d.StationMinSOC)
	id := m.station.ID

	last, ok, err := d.Store.LastSample(id)
	if err != nil {
		return nil, fmt.Errorf("monitor: last sample: %w", err)
	}
	if ok {
		m.last = &last
		before := last
		m.beforeStart = &before
		m.approxFrom = last.At
	}
	m.approxReason = store.GapAppNotRunning
	cfg := link.DefaultConfig()
	cfg.Fresh = freshFor(m.poll) // Active at start
	m.machine = link.NewMachine(cfg, m.last, m.profile, now, d.Boot)
	m.warmFrom = now

	cur, ok, err := d.Store.CurrentOutage(id)
	if err != nil {
		return nil, fmt.Errorf("monitor: current outage: %w", err)
	}
	if ok {
		m.cur = &cur
	}
	// The grid starts unknown; see assumeAfterShortRestart.
	m.tracker = detect.NewTracker(gridDebounce, m.cur)
	m.policy = notify.NewPolicy(d.Notify, now.Local())
	m.smoother = forecast.NewSmoother(smootherWindow)
	if err := m.loadPrevious(now); err != nil {
		return nil, err
	}
	if err := m.adoptOpenGap(now); err != nil {
		return nil, err
	}
	if err := m.loadJournalSeen(); err != nil {
		return nil, err
	}
	m.loadCutoffLogged(now)
	m.refreshNewEvents()

	if m.lastCall.IsZero() {
		m.nextPoll = now
	} else {
		m.nextPoll = m.lastCall.Add(minCallGap)
	}
	m.refreshAvg(now)
	// Stored totals only; the capacity pass over the raw samples waits for Run.
	m.refreshBattery(now)
	m.publish(m.snapshot(now))
	return m, nil
}

func (m *Monitor) discoverFirst(now time.Time) (store.Station, error) {
	ts := m.targets()
	if len(ts) == 0 {
		return store.Station{}, ErrNoStation
	}
	parent := m.d.Ctx
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return store.Station{}, err
	}
	ctx, cancel := context.WithTimeout(parent, discoverWait+time.Second)
	defer cancel()
	devs, err := m.d.Client.DiscoverMulti(ctx, ts, discoverWait)
	m.lastCall = m.clock.Now()
	if err := parent.Err(); err != nil { // cancelled: whatever was heard is not added
		return store.Station{}, err
	}
	if err != nil {
		return store.Station{}, fmt.Errorf("monitor: discover: %w", err)
	}
	for _, dv := range devs {
		if dv.BLEMac == "" { // phantom reply (null result): no identity to match later
			continue
		}
		ip, ok := stationIP(dv)
		if !ok {
			continue
		}
		st := store.Station{ID: dv.BLEMac, Model: dv.Device, BLEMac: dv.BLEMac, WifiMac: dv.WifiMac,
			IP: ip, Firmware: dv.Ver, CreatedAt: now.UTC()}
		if err := m.d.Store.UpsertStation(st); err != nil {
			return store.Station{}, fmt.Errorf("monitor: save station: %w", err)
		}
		return st, nil
	}
	return store.Station{}, ErrNoStation
}

// stationIP is the address a discovered device is polled at: the IPv4 its
// reply came from. The IP inside the payload is not trusted (a device on the
// LAN that knows the BLE MAC could name any address, with any port, there),
// and a station that reports a stale IP is still found where it answers.
func stationIP(dv lan.DeviceInfo) (string, bool) {
	ip := dv.Sender.To4()
	if ip == nil {
		return "", false
	}
	if dv.IP != ip.String() {
		slog.Warn("monitor: discovery reply names another address; using the sender", "payload", dv.IP, "sender", ip.String())
	}
	return ip.String(), true
}

// resolve turns the stored IP into the poll address; nil if unusable. Only a
// bare IPv4 is accepted: no port and no host name, so nothing stored can
// point polling at another port or make it resolve a name. The port is the
// one of the discovery targets (lan.DefaultPort unless a fake is used).
func (m *Monitor) resolve(ip string) *net.UDPAddr {
	p := net.ParseIP(ip).To4()
	if p == nil {
		return nil
	}
	return &net.UDPAddr{IP: p, Port: m.port}
}

func (m *Monitor) loadPrevious(now time.Time) error {
	outs, err := m.d.Store.Outages(m.station.ID, time.UnixMilli(0), now.Add(time.Second))
	if err != nil {
		return fmt.Errorf("monitor: outages: %w", err)
	}
	for i := len(outs) - 1; i >= 0; i-- {
		if outs[i].End != nil {
			o := outs[i]
			m.prev = &o
			break
		}
	}
	return nil
}

// adoptOpenGap picks up the gap left open by a previous run (e.g. pc_sleep
// and then shutdown), so one absence keeps one gap. The store holds at most
// one open gap per station (schema v5), whatever its start.
func (m *Monitor) adoptOpenGap(now time.Time) error {
	g, ok, err := m.d.Store.CurrentGap(m.station.ID)
	if err != nil {
		return fmt.Errorf("monitor: gaps: %w", err)
	}
	if ok {
		m.adoptGap(g)
		m.startupGap = &g
	}
	if m.startupGap != nil {
		v, ok, err := m.d.Store.GetSetting(settingGapCard)
		if err != nil {
			return fmt.Errorf("monitor: settings: %w", err)
		}
		m.gapDismissed = ok && v == strconv.FormatInt(m.startupGap.ID, 10)
	}
	return nil
}

// Run polls until ctx is cancelled. The daily maintenance (maintain) runs on
// start and every 24 h.
func (m *Monitor) Run(ctx context.Context) error {
	now := m.clock.Now()
	m.maintain(now)
	next := now
	for {
		m.step(ctx)
		if ctx.Err() != nil {
			return nil
		}
		now = m.clock.Now()
		next = next.Add(tickEvery)
		if !next.After(now) { // late (slow call, sleep, clock jump): no catch-up burst
			next = now.Add(tickEvery)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-m.clock.After(next.Sub(now)):
		}
	}
}

// step is one 1 s tick: time-driven transitions, then at most one call.
func (m *Monitor) step(ctx context.Context) {
	now := m.clock.Now()
	m.rollupEnergy(now) // store work outside m.mu
	m.mu.Lock()
	due, battDue := !now.Before(m.nextCompact), now.Sub(m.batteryAt) >= batteryEvery
	m.mu.Unlock()
	if due {
		m.maintain(now) // outside m.mu too; refreshes the battery
	} else if battDue {
		m.refreshBattery(now)
	}
	online := m.online() // may look at the adapters: outside m.mu
	m.mu.Lock()
	if now.Sub(m.avgAt) >= avgRefreshEvery {
		m.refreshAvg(now)
	}
	m.apply(m.machine.Tick(now, online), now)
	if m.machine.State() == link.Live {
		m.emit(m.policy.Tick(now.Local()))
	}
	m.emit(m.linkToasts(now))
	act, addr := m.nextAction(now), m.addr
	sleeps := m.sleeps
	m.mu.Unlock()
	// stale: the PC slept (and maybe woke) while the call was out. The reply
	// describes the station before the sleep, and Resume has rescheduled, so
	// it is dropped; the call still counts for minCallGap. Caller holds m.mu.
	stale := func() bool { return m.suspended || m.sleeps != sleeps }

	switch act {
	case actPoll:
		var raw []byte
		err := errors.New("monitor: station address unknown")
		if addr != nil {
			raw, err = m.d.Client.Call(ctx, addr, "ES.GetStatus", map[string]any{"id": 0})
		}
		at := m.clock.Now() // receive time; keeps the monotonic reading for link.Machine
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		if addr != nil {
			m.lastCall = at
		}
		if !stale() {
			m.onPoll(now, at, raw, err)
		}
		m.mu.Unlock()
	case actDiscover:
		dctx, cancel := context.WithTimeout(ctx, discoverWait+time.Second)
		devs, err := m.d.Client.DiscoverMulti(dctx, m.targets(), discoverWait)
		cancel()
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		m.lastCall = m.clock.Now()
		if !stale() {
			m.onDiscover(now, devs, err)
		}
		m.mu.Unlock()
	}

	m.mu.Lock()
	m.publish(m.snapshot(m.clock.Now()))
	m.mu.Unlock()
}

func (m *Monitor) nextAction(now time.Time) action {
	if m.suspended {
		return actNone
	}
	if !m.lastCall.IsZero() && now.Sub(m.lastCall) < minCallGap {
		return actNone
	}
	if m.fails > 0 && m.d.Targets != nil {
		ref := m.failingSince
		if m.lastDiscover.After(ref) {
			ref = m.lastDiscover
		}
		if now.Sub(ref) >= rediscoverEvery {
			return actDiscover
		}
	}
	if !now.Before(m.nextPoll) {
		return actPoll
	}
	return actNone
}

// backoff is the wait after the k-th consecutive failure: 5, 10, 20, 40, 60, 60… s.
func (m *Monitor) backoff(k int) time.Duration {
	d := m.poll
	for i := 1; i < k && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

func (m *Monitor) onPoll(start, at time.Time, raw []byte, err error) {
	m.pollStart = start
	var s model.Sample
	if err == nil {
		// A parse error (model.ParseESStatus) is a failed poll.
		s, err = model.NewSample(m.station.ID, at, model.SourceLAN, raw)
		s.Grid = floorGrid(s, m.profile)
	}
	if err == nil && s.SOC == nil && s.BatteryW == nil && s.OngridW == nil && s.OffgridW == nil && s.PVW == nil {
		// An empty or null result (a known firmware failure) tells nothing:
		// counting it as data would show LIVE with every value unknown.
		err = errors.New("monitor: ES.GetStatus result has no usable fields")
	}
	if err != nil {
		m.fails++
		if m.fails == 1 {
			m.failingSince = start
		}
		m.nextPoll = start.Add(m.backoff(m.fails))
		return
	}
	m.fails = 0
	m.onSample(s, at)
	// After onSample: a sample that ends the calm brings the next poll in at
	// the active interval from this one.
	m.nextPoll = start.Add(m.pollInterval())
}

// onDiscover matches by BLE MAC only (results without one are skipped); a
// match answering from a new address (stationIP) updates the one station row
// and polling moves there.
func (m *Monitor) onDiscover(start time.Time, devs []lan.DeviceInfo, err error) {
	m.lastDiscover = start
	if err != nil {
		slog.Warn("monitor: rediscovery failed", "err", err)
		return
	}
	for _, dv := range devs {
		if dv.BLEMac == "" || dv.BLEMac != m.station.BLEMac {
			continue
		}
		ip, ok := stationIP(dv)
		if !ok {
			continue
		}
		if ip == m.station.IP {
			// Same address: only a changed firmware (an update or a
			// rollback) is stored, so the row stays current; no journal entry.
			if dv.Ver > 0 && dv.Ver != m.station.Firmware {
				st := m.station
				st.Firmware = dv.Ver
				if err := m.d.Store.UpsertStation(st); err != nil {
					slog.Warn("monitor: store firmware", "err", err)
				} else {
					m.station = st
				}
			}
			return
		}
		st := m.station
		st.IP = ip
		if dv.Ver > 0 {
			st.Firmware = dv.Ver
		}
		if dv.WifiMac != "" {
			st.WifiMac = dv.WifiMac
		}
		if err := m.d.Store.UpsertStation(st); err != nil {
			slog.Warn("monitor: update station", "err", err)
		}
		m.addEvent(start, "link.moved", fmt.Sprintf("station found at %s (was %s)", ip, m.station.IP))
		m.journalEvent(start, journal.CodeIPChanged, "info", map[string]string{"ip": ip})
		m.station, m.addr = st, m.resolve(st.IP)
		m.nextPoll = start // next tick, still minCallGap after the discovery
		return
	}
}

func (m *Monitor) onSample(s model.Sample, now time.Time) {
	if err := m.d.Store.InsertSample(s); err != nil {
		slog.Warn("monitor: store sample", "err", err)
	}
	m.apply(m.machine.Observe(s, now), now)
	st := m.machine.State()
	live := st == link.Live
	cp := s
	m.last = &cp

	// A gap adopted from the previous run that the machine does not know about
	// ends with the first fresh sample.
	if m.gapID != 0 && !m.machineGap && (live || st == link.Resyncing) {
		m.closeGap(s.At)
	}

	if live && !m.sawLive {
		m.sawLive = true
		m.assumeAfterShortRestart()
	}
	for _, ev := range m.tracker.Observe(s, live) {
		m.onPower(ev, now)
	}
	mode := detect.Mode(s.BatteryW, s.SOC, m.profile.MinSOC)
	m.emit(m.policy.OnSample(s, mode, live, now.Local()))
	for _, k := range m.policy.Crossings() {
		if k == notify.LowCharge {
			m.journalEvent(s.At, journal.CodeLowCharge, "warn", socArgs(s.SOC))
		}
	}
	m.noteCutoff(s, mode, live)
	m.smooth(s, mode, live)
	m.updateCadence(s, mode, live)
}

// assumeAfterShortRestart runs at the first live sample since start. After a
// restart shorter than a gap (no startup gap, a sample of the previous run)
// nothing was missed: the grid state of the previous run continues, so an
// open outage stays known as GridOff (its end is exact and toasted) and a
// grid seen present stays GridOn (a later loss is exact). Otherwise the grid
// stays unknown until confirmed (detect.Tracker).
func (m *Monitor) assumeAfterShortRestart() {
	before := m.beforeStart
	m.beforeStart = nil
	if m.startupGap != nil || before == nil {
		return
	}
	switch {
	case m.cur != nil:
		m.tracker.Assume(detect.GridOff)
	case before.Grid == model.GridPresent:
		m.tracker.Assume(detect.GridOn)
	}
}

// smooth feeds the forecast smoother. Power from another mode says nothing
// about the new one, so once a new battery mode has held for modeHold the
// forecast starts over from that mode's samples alone ("calculating…" until
// it has 60 s of them). A mode that flickers (power around ±20 W) never
// holds, and the forecast carries on undisturbed. While the new mode is
// taking over, the old average is not shown either (see takingOver).
func (m *Monitor) smooth(s model.Sample, mode detect.BatteryMode, live bool) {
	switch {
	case mode == detect.ModeUnknown: // no reading: neither confirms nor breaks a mode
	case m.smoothMode == detect.ModeUnknown:
		m.smoothMode = mode
	case mode == m.smoothMode:
		m.pending = nil
	case m.pending == nil || mode != m.pendingMode:
		m.pending, m.pendingMode, m.pendingSince, m.pendingSeen = forecast.NewSmoother(smootherWindow), mode, s.At, 1
	default:
		m.pendingSeen++
	}
	if live && s.BatteryW != nil {
		m.smoother.Add(s.At, *s.BatteryW)
		if m.pending != nil {
			m.pending.Add(s.At, *s.BatteryW)
		}
	}
	if m.pending != nil && s.At.Sub(m.pendingSince) >= modeHold {
		m.smoother, m.smoothMode, m.pending = m.pending, m.pendingMode, nil
	}
}

// takingOver reports that mode is new and has been read twice in a row: the
// smoother still averages the previous mode, so its forecast would be stale
// ("40 h left" right after a 400 W load appears). One reading is not enough:
// power flickering around ±20 W changes mode on every sample.
func (m *Monitor) takingOver(mode detect.BatteryMode) bool {
	return m.pending != nil && m.pendingMode == mode && m.pendingSeen >= 2
}

func (m *Monitor) onPower(ev detect.PowerEvent, now time.Time) {
	switch ev.Kind {
	case detect.GridLost:
		// Found across a data gap (startup, PC sleep, no link): the real start
		// is earlier. It is still toasted (Ruling 11), marked approximate.
		if ev.AcrossGap {
			ev.GapFrom = m.windowStart(m.approxFrom, now) // the toast names the window it went off in
		}
		o := store.Outage{StationID: m.station.ID, Start: ev.At, SOCStart: ev.SOC, Partial: ev.AcrossGap}
		id, err := m.d.Store.OpenOutage(o)
		if err != nil {
			slog.Warn("monitor: open outage", "err", err)
			if cur, ok, _ := m.d.Store.CurrentOutage(m.station.ID); ok {
				m.cur = &cur
				id = cur.ID
			}
		} else {
			o.ID = id
			m.cur = &o
		}
		// The policy marks the toast approximate (ev.AcrossGap); emit adds
		// which gap it was found across.
		if ev.AcrossGap {
			m.approxTag, m.approxTagReason = strconv.FormatInt(id, 10), m.approxReason
		}
		m.emit(m.policy.OnPower(ev, id, true, now.Local()))
	case detect.GridBack:
		var id int64
		if m.cur != nil {
			id = m.cur.ID
			if err := m.d.Store.CloseOutage(id, ev.At, ev.SOC); err != nil {
				slog.Warn("monitor: close outage", "err", err)
			}
			closed := *m.cur
			end := ev.At
			closed.End, closed.SOCEnd = &end, ev.SOC
			m.prev, m.cur = &closed, nil
			// The grid is back: a cutoff hold started without it no longer
			// holds (a later Cutoff is not "without the grid").
			m.cutoffSince = time.Time{}
		}
		m.emit(m.policy.OnPower(ev, id, true, now.Local()))
		m.refreshAvg(now)
	}
}

// noLinkState: the link is lost (the no-connection toast counts from here).
func noLinkState(s link.State) bool { return s == link.NoLink || s == link.LikelyDead }

// linkToasts feeds the link state to the no-connection toast (once a tick).
func (m *Monitor) linkToasts(now time.Time) []notify.Toast {
	var lastAt time.Time
	var soc *float64
	if m.last != nil {
		lastAt, soc = m.last.At, m.last.SOC
	}
	return m.policy.OnLink(noLinkState(m.machine.State()), m.noLinkSince, lastAt, soc, now.Local())
}

// apply executes link machine effects against the store and the policy.
func (m *Monitor) apply(effs []link.Effect, now time.Time) {
	first := m.firstBatch
	m.firstBatch = false
	// EnteredLikelyDead together with a transition to LIKELY_DEAD was
	// detected while running; alone it is the startup retro path.
	detectedLive := false
	for _, e := range effs {
		if e.Kind == link.StateChanged && e.To == link.LikelyDead {
			detectedLive = true
		}
	}
	for _, e := range effs {
		switch e.Kind {
		case link.OpenGap:
			m.machineGap = true
			if m.gapEnd != nil {
				// The previous gap is still waiting to be closed in the store.
				// If that fails again the two absences become one gap, which
				// is better than leaving an open one behind.
				m.closeGap(*m.gapEnd)
				m.gapEnd = nil
			}
			if m.gapID != 0 {
				if e.Reason == store.GapLikelyDead {
					m.setGapReason(store.GapLikelyDead)
				}
			} else {
				reason := e.Reason
				if first && reason == store.GapAppNotRunning && m.d.AppUpdated {
					reason = store.GapAppUpdate // the restart of the one-click update
				}
				id, err := m.d.Store.OpenGap(store.Gap{StationID: m.station.ID, Start: e.At, Reason: reason})
				m.gapID, m.gapReason = id, reason
				m.gapStart, m.deadLogged = e.At, false
				if err != nil {
					slog.Warn("monitor: open gap", "err", err)
					// The store holds one open gap per station: one this run
					// does not know of (written since startup) takes this
					// absence too, so it gets closed with it.
					cur, ok, cerr := m.d.Store.CurrentGap(m.station.ID)
					if cerr != nil {
						slog.Warn("monitor: current gap", "err", cerr)
					}
					if ok {
						m.adoptGap(cur)
						if e.Reason == store.GapLikelyDead {
							m.setGapReason(store.GapLikelyDead)
						}
					}
				}
				m.refreshNewEvents()
			}
			if first && m.gapID != 0 && (m.startupGap == nil || m.startupGap.ID != m.gapID) {
				// The gap's own start and reason: an adopted one began earlier.
				m.startupGap = &store.Gap{ID: m.gapID, StationID: m.station.ID, Start: m.gapStart, Reason: m.gapReason}
				m.gapDismissed = false
			}
			m.addEvent(now, "link.gap_opened", "no data since "+e.At.UTC().Format(time.RFC3339)+" ("+string(m.gapReason)+")")
		case link.CloseGap:
			m.closeGap(e.At)
			m.machineGap = false
			m.addEvent(now, "link.gap_closed", "data resumed at "+e.At.UTC().Format(time.RFC3339))
		case link.LogDelay:
			m.addEvent(now, "link.delayed", "data delayed since "+e.At.UTC().Format(time.RFC3339))
			// The journal tells it too: from the last good sample to the first after.
			// seconds is taken from the same whole-second instants as from / to, so the
			// text never shows a duration that differs from its two times.
			from, to := e.At.UTC().Truncate(time.Second), now.UTC().Truncate(time.Second)
			m.journalEvent(now, journal.CodeLinkDropped, "info", map[string]string{
				"from": journal.Stamp(from), "to": journal.Stamp(to),
				"seconds": strconv.FormatInt(int64(to.Sub(from)/time.Second), 10)})
		case link.LogStartDelay:
			// A restart or reboot under 5 min: no gap and no card (flows 2.2),
			// but it is logged, and SOCSeries breaks the line across the hole.
			m.addEventFrom(eventSourceStart, "info", now, "link.delayed", "no data since "+e.At.UTC().Format(time.RFC3339)+
				": the app was not running (under 5 min, not a gap)")
		case link.EnteredLikelyDead:
			m.setGapReason(store.GapLikelyDead)
			if m.cur != nil && m.cur.DiedAt == nil {
				if err := m.d.Store.MarkDied(m.cur.ID, e.At); err != nil {
					slog.Warn("monitor: mark died", "err", err)
				}
				at := e.At
				m.cur.DiedAt = &at
			}
			var soc *float64
			if m.last != nil {
				soc = m.last.SOC
			}
			m.emit(m.policy.OnLikelyDead(e.At, soc, detectedLive, now.Local()))
			m.addEvent(now, "link.likely_dead", "station probably ran out at "+e.At.UTC().Format(time.RFC3339))
			if !m.deadLogged {
				m.deadLogged = true
				args := socArgs(soc)
				args["at"] = journal.Stamp(e.At)
				m.journalEvent(now, journal.CodeLikelyOut, "warn", args)
			}
		case link.StateChanged:
			if noLinkState(e.To) && !noLinkState(e.From) {
				m.noLinkSince = e.At
			}
			switch {
			case e.To == link.Live || e.To == link.Resyncing:
				m.lostLogged = false // data is back
			case e.To == link.NoLink && !m.lostLogged:
				// Once per absence: a sleep inside it re-enters NO_LINK.
				m.lostLogged = true
				code := journal.CodeAPISilent
				if m.machine.Diagnosis() == link.DiagPCOffline {
					code = journal.CodeLANLost
				}
				m.journalEvent(e.At, code, "warn", nil)
			}
			m.addEvent(now, "link."+strings.ToLower(string(e.To)), string(e.From)+" -> "+string(e.To))
		}
	}
}

// closeGap ends the open gap at at. If the store fails, the gap id is kept
// and the close is retried (with the same end) on the following samples;
// forgetting it would leave an open gap in the database for good.
//
// When the gap really ends (not a retried write), the grid state becomes
// unknown: whatever changed during the gap was not seen, so the next
// confirmed change is reported across the gap (detect.PowerEvent.AcrossGap).
func (m *Monitor) closeGap(at time.Time) {
	// An adopted gap may start ahead of the clock (set back since): it ends
	// where it starts rather than before.
	if !m.gapStart.IsZero() && at.Before(m.gapStart) {
		at = m.gapStart
	}
	if m.gapEnd == nil {
		m.tracker.Unconfirm()
		if m.gapReason != "" {
			m.approxReason = m.gapReason
		}
		m.approxFrom = m.gapStart
		m.beforeStart = nil
		if (m.gapReason == store.GapNoLink || m.gapReason == store.GapLikelyDead) && !m.gapStart.IsZero() {
			m.journalEvent(at, journal.CodeLANRestored, "info",
				map[string]string{"duration_ms": strconv.FormatInt(at.Sub(m.gapStart).Milliseconds(), 10)})
		}
		m.gapStart, m.deadLogged = time.Time{}, false
		m.refreshNewEvents()
	}
	if m.gapID == 0 { // the store never opened it
		m.gapReason = ""
		return
	}
	if m.gapEnd != nil {
		at = *m.gapEnd
	}
	if m.startupGap != nil && m.startupGap.ID == m.gapID {
		end := at
		m.startupGap.End = &end
	}
	if err := m.d.Store.CloseGap(m.gapID, at); err != nil {
		slog.Warn("monitor: close gap", "err", err)
		// A missing row cannot be closed later either, nor one already closed.
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, store.ErrAlreadyClosed) {
			m.gapEnd = &at
			return
		}
	}
	m.gapID, m.gapEnd, m.gapReason = 0, nil, ""
}

// adoptGap makes the open gap g this absence's gap. A likely_dead gap had its
// likelyOut entry already, and a no_link / likely_dead gap its link-loss
// entry, so neither is logged again.
func (m *Monitor) adoptGap(g store.Gap) {
	m.gapID, m.gapReason, m.gapStart = g.ID, g.Reason, g.Start
	m.deadLogged = m.deadLogged || g.Reason == store.GapLikelyDead
	m.lostLogged = m.lostLogged || g.Reason == store.GapNoLink || g.Reason == store.GapLikelyDead
}

func (m *Monitor) setGapReason(r store.GapReason) {
	if m.gapID == 0 {
		return
	}
	m.gapReason = r
	if err := m.d.Store.SetGapReason(m.gapID, r); err != nil {
		slog.Warn("monitor: set gap reason", "err", err)
	}
	if m.startupGap != nil && m.startupGap.ID == m.gapID {
		m.startupGap.Reason = r
	}
}

// addEvent logs an internal (debug) event; it never reaches the journal.
func (m *Monitor) addEvent(at time.Time, code, text string) {
	m.addEventFrom(eventSource, "info", at, code, text)
}

// addEventFrom stores an event; level is info | warn | error. It reports
// whether the event was stored.
func (m *Monitor) addEventFrom(source, level string, at time.Time, code, text string) bool {
	if err := m.d.Store.AddEvent(store.Event{StationID: m.station.ID, At: at, Level: level,
		Code: code, Text: text, Source: source}); err != nil {
		slog.Warn("monitor: add event", "err", err)
		return false
	}
	return true
}

// journalEvent logs a journal event (a journal.Code*) with its copy
// arguments as the JSON text. A warning or error changes NewEvents.
func (m *Monitor) journalEvent(at time.Time, code, level string, args map[string]string) {
	if args == nil {
		args = map[string]string{}
	}
	text, err := json.Marshal(args)
	if err != nil { // a map[string]string always marshals
		slog.Warn("monitor: journal args", "err", err)
		text = []byte("{}")
	}
	if m.addEventFrom(eventSource, level, at, code, string(text)) && level != "info" {
		m.refreshNewEvents()
	}
}

// socArgs is {"soc": rounded percent}, empty when the charge is unknown.
func socArgs(soc *float64) map[string]string {
	args := map[string]string{}
	if soc != nil {
		args["soc"] = strconv.FormatFloat(*soc, 'f', 0, 64)
	}
	return args
}

// floorGrid is model.GridFromStatus with the station's floor known (Ruling
// 12): a station at its minimum charge with no power flowing (|ongrid| and
// offgrid ≤ 20 W) has turned its output off and cannot tell whether the grid
// is back, so "present by idle" is unknown there. An open outage then stays
// open (no false grid_back); the grid's return shows as charging.
func floorGrid(s model.Sample, p model.Profile) model.GridSignal {
	if s.Grid == model.GridPresent && atFloorIdle(s, p) {
		return model.GridUnknown
	}
	return s.Grid
}

// atFloor: SOC within 1 p.p. of the floor (MinSOC or MinSOC+1, the reading
// bounces between the two there).
func atFloor(soc *float64, p model.Profile) bool {
	return soc != nil && *soc <= p.MinSOC+1
}

// atFloorIdle: at the floor with no power flowing (|ongrid| and offgrid ≤
// 20 W), where floorGrid makes "present by idle" unknown.
func atFloorIdle(s model.Sample, p model.Profile) bool {
	return atFloor(s.SOC, p) &&
		s.OngridW != nil && math.Abs(*s.OngridW) <= model.GridDeadbandW &&
		s.OffgridW != nil && *s.OffgridW <= model.GridDeadbandW
}

// loadCutoffLogged: a station.cutoff already logged during the open outage
// (by a previous run) is not logged again after a restart.
func (m *Monitor) loadCutoffLogged(now time.Time) {
	if m.cur == nil {
		return
	}
	es, err := m.d.Store.Events(m.station.ID, journal.CodeCutoff, m.cur.Start, now.Add(time.Second))
	if err != nil {
		slog.Warn("monitor: cutoff events", "err", err)
		return
	}
	m.cutoffLogged = len(es) > 0
}

// noteCutoff logs station.cutoff once the Cutoff mode (charge at the floor,
// no discharge) has held modeHold without the grid. "Without the grid" is
// decided when the hold starts (the sample reads the grid absent, or an
// outage is open); the hold then lasts while the mode stays Cutoff, or reads
// Idle within 1 p.p. of the floor (a bounce to MinSOC+1 does not restart it).
// Charging re-arms it.
func (m *Monitor) noteCutoff(s model.Sample, mode detect.BatteryMode, live bool) {
	if !live {
		return
	}
	switch {
	case mode == detect.Cutoff:
		if m.cutoffSince.IsZero() {
			if s.Grid != model.GridAbsent && m.cur == nil {
				return
			}
			m.cutoffSince = s.At
		}
		if !m.cutoffLogged && s.At.Sub(m.cutoffSince) >= modeHold {
			m.cutoffLogged = true
			m.journalEvent(s.At, journal.CodeCutoff, "warn", socArgs(s.SOC))
		}
	case mode == detect.Charging:
		m.cutoffSince, m.cutoffLogged = time.Time{}, false
	case mode == detect.Idle && atFloor(s.SOC, m.profile):
		// a 1-p.p. bounce at the floor: the hold goes on
	case mode != detect.ModeUnknown:
		m.cutoffSince = time.Time{} // the hold is broken
	}
}

// emit sets StationID where the policy left it empty and holds the toasts
// until the next publish, so the snapshot a toast is shown with is never
// older than the toast itself.
func (m *Monitor) emit(ts []notify.Toast) {
	for _, t := range ts {
		if t.StationID == "" {
			t.StationID = m.station.ID
		}
		if t.Kind == notify.GridLost && t.Args["approx"] == "1" && t.Tag == m.approxTag && t.Args["approxReason"] == "" {
			// A copy: the policy keeps its Args for a later report.
			a := make(map[string]string, len(t.Args)+1)
			for k, v := range t.Args {
				a[k] = v
			}
			a["approxReason"] = string(m.approxTagReason)
			t.Args = a
		}
		m.outbox = append(m.outbox, t)
	}
}

// maintain is the daily maintenance, at start and every compactEvery, in
// this order: the capacity pass (estimateCapacity), which must see the raw
// samples Compact is about to delete; with a retention set, Prune of what is
// older than it; then Compact, which aggregates the old raw samples. Poll
// loop only, NOT under m.mu: a pass over 30 days of samples takes a while,
// and the snapshot, the UI calls and Suspend must not wait for it.
func (m *Monitor) maintain(now time.Time) {
	m.maintMu.Lock() // PruneNow waits: it must not delete what the pass re-stores
	defer m.maintMu.Unlock()
	m.estimateCapacity(now)
	m.mu.Lock()
	m.nextCompact = now.Add(compactEvery)
	p := m.pruneArgs()
	m.mu.Unlock()
	if maintenanceHook != nil {
		maintenanceHook()
	}
	// A failed prune is one rolled-back transaction (store.Prune):
	// nothing was deleted, so Compact still runs and nothing is refreshed.
	pruned, err := p.run(m.d.Store, now)
	if err != nil {
		slog.Warn("monitor: prune", "err", err)
		pruned = false
	}
	if err := m.d.Store.Compact(now); err != nil {
		slog.Warn("monitor: compact", "err", err)
	}
	if pruned {
		m.refreshBattery(now) // estimates may be gone
		m.mu.Lock()
		m.refreshNewEvents() // old warnings may be gone
		m.mu.Unlock()
	}
}

// online is Deps.PCOnline (always online without one). Not under m.mu.
func (m *Monitor) online() bool {
	if m.d.PCOnline == nil {
		return true
	}
	return m.d.PCOnline()
}

// rollupEnergy stores the energy of the minutes that ended at least
// energy.MaxStep ago (by then every step starting in them has its next
// sample), at most energyChunk per call. It resumes after the newest stored
// minute, or at ObservedSince (not before energyBackfill) on a database
// without one. Rows are replaced, so redoing a minute never duplicates it. A
// store failure is retried on the next tick and logged at most once per
// energyWarnEvery.
//
// Only the poll loop calls it, and it must NOT hold m.mu: a backfill chunk
// reads hours of samples, and the snapshot, the UI calls and the poll
// handling must not wait for it. m.mu is taken only to read the station and
// to advance the progress (energyNext and energyWarnAt are loop-only state,
// guarded by m.mu all the same).
func (m *Monitor) rollupEnergy(now time.Time) {
	m.mu.Lock()
	id, next := m.station.ID, m.energyNext
	m.mu.Unlock()

	if next.IsZero() {
		next = now.Truncate(time.Minute)
		if last, ok, err := m.d.Store.LastEnergyMinute(id); err != nil {
			m.energyWarn(now, "last energy minute", err)
			return
		} else if ok {
			next = last.Add(time.Minute)
		} else if since, ok, err := m.d.Store.ObservedSince(id); err != nil {
			m.energyWarn(now, "observed since", err)
			return
		} else if ok {
			next = since.Truncate(time.Minute)
		}
		if floor := now.Add(-energyBackfill).Truncate(time.Minute); next.Before(floor) {
			next = floor
		}
	}
	to := now.Add(-energy.MaxStep).Truncate(time.Minute)
	if lim := next.Add(energyChunk); to.After(lim) {
		to = lim
	}
	if to.After(next) {
		if rollupHook != nil {
			rollupHook()
		}
		// The step of a sample before `to` ends before to + MaxStep.
		samples, err := m.d.Store.EnergySamples(id, next, to.Add(energy.MaxStep))
		if err != nil {
			m.energyWarn(now, "samples for energy", err)
			return
		}
		outs, err := m.d.Store.Outages(id, next, to)
		if err != nil {
			m.energyWarn(now, "outages for energy", err)
			return
		}
		if err := m.d.Store.UpsertEnergyMinutes(id, energy.Rollup(samples, outs, next, to)); err != nil {
			m.energyWarn(now, "store energy", err)
			return
		}
		next = to
	}
	m.mu.Lock()
	m.energyNext = next
	m.energyFailed = false
	m.mu.Unlock()
}

// rollupHook, when set by a test, runs before a chunk's store work.
var rollupHook func()

// maintenanceHook, when set by a test, runs before the daily Prune and
// Compact.
var maintenanceHook func()

// energyWarn logs a rollup failure, at most once per energyWarnEvery: a
// broken store would otherwise log every second.
func (m *Monitor) energyWarn(now time.Time, what string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.energyFailed = true
	if !m.energyWarnAt.IsZero() && now.Sub(m.energyWarnAt) < energyWarnEvery {
		return
	}
	m.energyWarnAt = now
	slog.Warn("monitor: "+what, "err", err)
}

// estimateChunk: the sample time the capacity pass reads at once (a few MB
// of samples), so a pass over 30 days never holds them all in memory.
const estimateChunk = 6 * time.Hour

// estimateCapacity runs health.FindDischarges over all raw samples still
// stored (≤ 30 days plus whatever Compact has not reached yet), a chunk at a
// time, then stores the deep runs in capacity_estimates and raises
// SettingMaxDelta. It runs at start and before each daily Compact, so a run
// is estimated before its samples are deleted; the estimates outlive the
// samples, and only the retention's Prune deletes them (keeping the newest
// month that has any). Rows are replaced by start: a run still going on at one pass is
// stored again, longer, at the next. A run that overlaps a stored estimate
// with another start is the tail of a run whose head Compact deleted, and is
// dropped. Failures are logged (once a day at most) and leave the stored
// estimates as they are. Poll loop only, NOT under m.mu.
func (m *Monitor) estimateCapacity(now time.Time) {
	defer m.refreshBattery(now)
	m.mu.Lock()
	id := m.station.ID
	m.mu.Unlock()

	first, ok, err := m.d.Store.FirstSampleAt(id)
	if err != nil || !ok {
		if err != nil {
			slog.Warn("monitor: first sample for capacity", "err", err)
		}
		return
	}
	var f health.Finder
	end := now.Add(time.Second)
	for from := first; from.Before(end); from = from.Add(estimateChunk) {
		samples, err := m.d.Store.EnergySamples(id, from, minTime(from.Add(estimateChunk), end))
		if err != nil {
			slog.Warn("monitor: samples for capacity", "err", err)
			return
		}
		for _, s := range samples {
			f.Add(s)
		}
	}
	found, maxDelta := f.Result()

	old, err := m.d.Store.CapacityEstimates(id)
	if err != nil {
		slog.Warn("monitor: capacity estimates", "err", err)
		return
	}
	var keep []store.CapacityEstimate
	for _, e := range found {
		if !overlapsOther(old, e) {
			keep = append(keep, e)
		}
	}
	if err := m.d.Store.UpsertCapacityEstimates(id, keep); err != nil {
		slog.Warn("monitor: store capacity estimates", "err", err)
		return
	}
	if maxDelta > m.storedMaxDelta() {
		if err := m.d.Store.SetSetting(SettingMaxDelta, strconv.FormatFloat(maxDelta, 'f', -1, 64)); err != nil {
			slog.Warn("monitor: store largest discharge", "err", err)
		}
	}
}

// overlapsOther: e shares time with a stored estimate that starts elsewhere.
func overlapsOther(old []store.CapacityEstimate, e store.CapacityEstimate) bool {
	for _, o := range old {
		if !o.Start.Equal(e.Start) && !e.Start.After(o.End) && !o.Start.After(e.End) {
			return true
		}
	}
	return false
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// refreshBattery re-reads the snapshot's cycles and health. Not under m.mu.
func (m *Monitor) refreshBattery(now time.Time) {
	s, err := m.Battery()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batteryAt = now
	if err != nil {
		slog.Warn("monitor: battery health", "err", err)
		return
	}
	m.battery.Cycles, m.battery.HealthPct, m.battery.HealthApprox = s.Cycles, s.HealthPct, s.Approx
}

// refreshAvg recomputes the average outage discharge from the last 30 days:
// only outages started in that window and only samples inside them (all
// stored samples come from live polls).
func (m *Monitor) refreshAvg(now time.Time) {
	m.avgAt = now
	from := now.Add(-outageLookback)
	outs, err := m.d.Store.Outages(m.station.ID, from, now.Add(time.Second))
	if err != nil {
		slog.Warn("monitor: outages for forecast", "err", err)
		return
	}
	var keep []store.Outage
	var samples []model.Sample
	for _, o := range outs {
		if o.Start.Before(from) {
			continue
		}
		end := now
		if o.End != nil {
			end = *o.End
		}
		s, err := m.d.Store.Samples(m.station.ID, o.Start, end.Add(time.Millisecond))
		if err != nil {
			slog.Warn("monitor: samples for forecast", "err", err)
			return
		}
		keep = append(keep, o)
		samples = append(samples, s...)
	}
	m.avgOutageW = forecast.AvgOutageDischargeW(samples, keep)
}

func (m *Monitor) publish(s Snapshot) {
	m.latestMu.Lock()
	m.seq++
	s.Seq = m.seq
	m.latest = s
	m.latestMu.Unlock()
	// Buffer 1, replace semantics: drop a stale unread snapshot, never block.
	select {
	case <-m.snaps:
	default:
	}
	select {
	case m.snaps <- s:
	default:
	}
	for _, t := range m.outbox {
		select {
		case m.toasts <- t:
		default:
			slog.Warn("monitor: toast queue full, dropped", "kind", t.Kind)
		}
	}
	m.outbox = nil
}

// Snapshots delivers a snapshot after every change; unread ones are replaced
// by newer ones. Latest always returns the newest.
func (m *Monitor) Snapshots() <-chan Snapshot { return m.snaps }

func (m *Monitor) Latest() Snapshot {
	m.latestMu.Lock()
	defer m.latestMu.Unlock()
	return m.latest
}

func (m *Monitor) Toasts() <-chan notify.Toast { return m.toasts }

// Suspend: the PC is going to sleep; polling pauses.
func (m *Monitor) Suspend() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.apply(m.machine.Suspend(now), now)
	m.suspended = true
	m.sleeps++ // a call out now has its reply dropped (step)
	m.publish(m.snapshot(now))
}

// Resume: the PC woke up; reconnect at once with a fresh backoff.
func (m *Monitor) Resume() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.apply(m.machine.Resume(now), now)
	m.policy.OnResume(now.Local())
	m.suspended, m.fails = false, 0
	m.nextPoll = now
	m.warmFrom = now // the warmup starts over
	m.setCadence(Active, now)
	m.publish(m.snapshot(now))
}

// SetDND applies a tray "do not disturb" option (notify.DNDOption); an
// unknown option such as "off" turns DND off.
func (m *Monitor) SetDND(opt string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policy.SetDND(notify.DNDOption(opt, m.clock.Now().Local(), m.d.Notify))
}

// DismissGapCard closes the "while the app was not running" card for good.
func (m *Monitor) DismissGapCard(gapID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startupGap == nil || m.startupGap.ID != gapID {
		return fmt.Errorf("monitor: no gap card %d", gapID)
	}
	if err := m.d.Store.SetSetting(settingGapCard, strconv.FormatInt(gapID, 10)); err != nil {
		return err
	}
	m.gapDismissed = true
	m.publish(m.snapshot(m.clock.Now()))
	return nil
}

// targets are the current discovery addresses; none = discovery is off.
func (m *Monitor) targets() []*net.UDPAddr {
	if m.d.Targets == nil {
		return nil
	}
	return m.d.Targets()
}

// windowStart is history.WindowStart for the gap that began at from, over the
// stored gaps (zero stays zero): the toast's {from} is the UI's StartFrom.
// Each round loads the gaps that overlap the current start.
func (m *Monitor) windowStart(from, now time.Time) time.Time {
	for !from.IsZero() {
		gaps, err := m.d.Store.Gaps(m.station.ID, from, from.Add(time.Millisecond))
		if err != nil {
			slog.Warn("monitor: gaps for the toast window", "err", err)
			return from
		}
		x := history.WindowStart(gaps, from, now)
		if !x.Before(from) {
			return from
		}
		from = x
	}
	return from
}
