package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/onboard"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ScanDeviceEvent carries a Device as soon as the scan finds it;
// ScanDoneEvent carries the Diagnosis when the scan ends (empty unless
// nothing was found; Cancelled when ProbeIP or Handover stopped it).
const ScanDeviceEvent, ScanDoneEvent = "scan.device", "scan.done"

func init() {
	application.RegisterEvent[Device](ScanDeviceEvent)
	application.RegisterEvent[Diagnosis](ScanDoneEvent)
}

// Device is a station the first-run screen found. Model is its device
// string as the station reports it («VenusE 3.0»), also the default name.
// ModelCode is the model code from the reply's "src" («VNSE3-0» from
// «VNSE3-0-0a1b2c3d4e5f», Ruling 33), or the device string when the src
// does not end in -<BLE MAC>. Tested: the model has a tested profile
// (flows O4); Added: the station is already in the database (flows O1).
type Device struct {
	Model, ModelCode, IP, BLEMac string
	Firmware                     int
	Tested, Added                bool
}

// Diagnosis is why a scan may have found nothing (flows 1.4 NONE_FOUND):
// a connected network (with a gateway) is Public; the firewall's Private
// profile is on without an enabled rule for this exe; no interface with a
// gateway has a private IPv4. Cancelled (scan.done only): the scan was
// stopped by ProbeIP or Handover, so it says nothing about the network and
// the screen ignores it.
type Diagnosis struct{ PublicNetwork, FirewallMayBlock, NoLAN, Cancelled bool }

var (
	// ErrCancelled: the user said no to the UAC prompt.
	ErrCancelled = errors.New("app: cancelled by user")
	// ErrHandedOver: the monitor runs and owns the station's calls now.
	ErrHandedOver = errors.New("app: the monitor talks to the station now")
	// ErrFirewallPending: a UAC prompt for the rule is already open.
	ErrFirewallPending = errors.New("app: firewall rule already being added")
)

// ProbeIP error codes (the frontend maps them to copy, Task 19).
const (
	ProbeFormat      = "format"      // not a unicast IPv4 (addStation.ip.error.format)
	ProbeNoAnswer    = "noAnswer"    // silent; worth trying again (addStation.ip.error.noAnswer)
	ProbeNotMarstek  = "notMarstek"  // something answered, not a station (addStation.ip.error.notMarstek)
	ProbeOtherSubnet = "otherSubnet" // silent, and the IP is on none of the PC's networks (+ onboarding.dx.otherSubnet.body)
	ProbeCancelled   = "cancelled"   // the monitor took over or the app is closing: nothing to show
)

// ProbeError is a ProbeIP failure; its message is the code, which is what
// the frontend receives.
type ProbeError struct{ Code string }

func (e *ProbeError) Error() string { return e.Code }

// OnboardOptions: Simulate (-simulate*) lets a typed loopback address
// reach the simulator (Ruling 34); AutoAdd: the monitor adds the station by
// itself (plain -simulate), so there is no first run and the service makes
// no station calls.
type OnboardOptions struct{ Simulate, AutoAdd bool }

// OnboardService is bound to the frontend: the first-run screen finds a
// station (scan, IP by hand), adds it, and explains a scan that found
// nothing. Its station calls go one at a time, ≥ 1 s apart, over the
// monitor's client; once the monitor starts (Handover) it makes none.
type OnboardService struct {
	ctx     context.Context
	st      *store.Store
	client  *lan.Client
	targets func() []*net.UDPAddr
	port    int
	opts    OnboardOptions

	// Emit sends an event to the frontend; OnAdded tells the runner a
	// station was stored; Owner is the main window's HWND (the UAC prompt's
	// owner). main sets them before the app runs; nil = none.
	Emit    func(name string, data any)
	OnAdded func()
	Owner   func() uintptr

	// System hooks; tests replace them.
	nets  func() []*net.IPNet
	diag  func() (Diagnosis, error)
	allow func(hwnd uintptr) (running <-chan struct{}, err error)

	// mu: one station call at a time; lastCall: when the last one ended.
	mu       sync.Mutex
	lastCall time.Time
	handed   atomic.Bool
	allowing atomic.Bool // a UAC prompt is open

	smu        sync.Mutex // scanCancel, found
	scanCancel context.CancelFunc
	found      map[string]lan.DeviceInfo // by BLE MAC: what Add may store

	addMu   sync.Mutex // one Add at a time
	addedID string
}

// NewOnboardService: c is the monitor's client (port 30000 has one socket);
// targets are the scan's broadcast addresses and port the station port
// ProbeIP asks; ctx ends with the app.
//
//wails:ignore
func NewOnboardService(ctx context.Context, st *store.Store, c *lan.Client, targets func() []*net.UDPAddr, port int, opts OnboardOptions) *OnboardService {
	s := &OnboardService{ctx: ctx, st: st, client: c, targets: targets, port: port, opts: opts,
		nets: localNets, diag: systemDiagnosis, allow: systemAllowFirewall,
		found: map[string]lan.DeviceInfo{}}
	// With AutoAdd the monitor's own discovery uses the client unserialised:
	// the service must never call.
	s.handed.Store(opts.AutoAdd)
	return s
}

// callGap is the least time between two calls to the station.
const callGap = time.Second

// call runs fn, a station call, after the one before it ended ≥ 1 s ago.
func (s *OnboardService) call(ctx context.Context, fn func(context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handed.Load() {
		return ErrHandedOver
	}
	if err := sleepCtx(ctx, time.Until(s.lastCall.Add(callGap))); err != nil {
		return err
	}
	err := fn(ctx)
	s.lastCall = time.Now()
	return err
}

// Handover stops the service's station calls for good, so the monitor can
// take over: a running scan is cancelled and waited for. It returns when
// the last call ended (zero if none); the monitor's first call must come
// ≥ 1 s after it.
//
//wails:ignore
func (s *OnboardService) Handover() time.Time {
	s.handed.Store(true)
	s.cancelScan()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCall
}

func (s *OnboardService) cancelScan() {
	s.smu.Lock()
	defer s.smu.Unlock()
	if s.scanCancel != nil {
		s.scanCancel()
	}
}

func (s *OnboardService) emit(name string, data any) {
	if s.Emit != nil {
		s.Emit(name, data)
	}
}

// Scan looks for stations on the LAN (8 s) in the background: each new one
// comes as ScanDeviceEvent, the end as ScanDoneEvent with the Diagnosis
// when nothing was found. A scan already running goes on (no second one);
// ProbeIP and Handover cancel it (scan.done still comes, Cancelled).
func (s *OnboardService) Scan() error {
	if s.handed.Load() {
		return ErrHandedOver
	}
	s.smu.Lock()
	if s.scanCancel != nil {
		s.smu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.scanCancel = cancel
	s.smu.Unlock()

	go func() {
		defer cancel()
		n := 0
		err := s.call(ctx, func(ctx context.Context) error {
			return onboard.Scan(ctx, s.client, s.targets, func(d lan.DeviceInfo) {
				n++
				s.emit(ScanDeviceEvent, s.remember(d))
			})
		})
		var dx Diagnosis
		switch {
		case ctx.Err() != nil || errors.Is(err, ErrHandedOver):
			dx.Cancelled = true
		case n == 0:
			if err != nil {
				slog.Info("scan", "err", err)
			}
			var derr error
			if dx, derr = s.diag(); derr != nil {
				slog.Warn("network diagnosis", "err", derr)
			}
		}
		// Cleared before scan.done, so a rescan the event triggers starts.
		s.smu.Lock()
		s.scanCancel = nil
		s.smu.Unlock()
		s.emit(ScanDoneEvent, dx)
	}()
	return nil
}

// ProbeIP checks an address typed by hand (≤ 5 s once the station is free;
// a running scan is cancelled first). Every failure is a *ProbeError with
// code ProbeFormat, ProbeNoAnswer, ProbeNotMarstek, ProbeOtherSubnet or
// ProbeCancelled.
func (s *OnboardService) ProbeIP(ip string) (Device, error) {
	if s.handed.Load() {
		return Device{}, &ProbeError{ProbeCancelled}
	}
	p, err := s.validIP(strings.TrimSpace(ip))
	if err != nil {
		return Device{}, &ProbeError{ProbeFormat}
	}
	s.cancelScan()
	var dv lan.DeviceInfo
	err = s.call(s.ctx, func(ctx context.Context) error {
		var perr error
		dv, perr = onboard.ProbeIP(ctx, s.client, p.String(), s.port)
		return perr
	})
	switch {
	case err == nil:
		return s.remember(dv), nil
	case errors.Is(err, onboard.ErrNotMarstek):
		return Device{}, &ProbeError{ProbeNotMarstek}
	case errors.Is(err, onboard.ErrNoAnswer):
		if !inSubnets(p, s.nets()) {
			return Device{}, &ProbeError{ProbeOtherSubnet}
		}
		return Device{}, &ProbeError{ProbeNoAnswer}
	case errors.Is(err, onboard.ErrBadIP):
		return Device{}, &ProbeError{ProbeFormat}
	}
	// ErrHandedOver or the app's ctx ended.
	return Device{}, &ProbeError{ProbeCancelled}
}

// validIP is onboard.ValidIPv4; with Simulate a loopback address passes too
// (the simulator listens on 127.0.0.1, Ruling 34).
func (s *OnboardService) validIP(ip string) (net.IP, error) {
	if s.opts.Simulate {
		if p := net.ParseIP(ip).To4(); p != nil && p.IsLoopback() && !strings.Contains(ip, ":") {
			return p, nil
		}
	}
	return onboard.ValidIPv4(ip)
}

// remember keeps a found device for Add and returns it for the screen.
func (s *OnboardService) remember(dv lan.DeviceInfo) Device {
	s.smu.Lock()
	s.found[dv.BLEMac] = dv
	s.smu.Unlock()
	_, added, err := s.st.StationByBLEMac(dv.BLEMac)
	if err != nil {
		slog.Warn("onboard: station lookup", "err", err)
	}
	return Device{Model: dv.Device, ModelCode: modelCode(dv), IP: dv.IP, BLEMac: dv.BLEMac, Firmware: dv.Ver,
		Tested: model.ProfileFor(dv.Device).Tested, Added: added}
}

// modelCode is the reply's src without its "-<BLE MAC>" suffix (Ruling 33:
// «VNSE3-0-0a1b2c3d4e5f» → «VNSE3-0»); a src in any other shape is not
// trusted, and the device string stands in.
func modelCode(dv lan.DeviceInfo) string {
	suffix := "-" + strings.ToLower(dv.BLEMac)
	if dv.BLEMac != "" && len(dv.Src) > len(suffix) && strings.HasSuffix(strings.ToLower(dv.Src), suffix) {
		return dv.Src[:len(dv.Src)-len(suffix)]
	}
	return dv.Device
}

// Add stores a station this run found (by its BLE MAC; the rest is taken
// from what the station reported, not from d) and lets the runner start the
// monitor. The name is the model; model and firmware are stored as reported. v1
// monitors one station: adding a second is refused (plan 3).
func (s *OnboardService) Add(d Device) error {
	s.addMu.Lock()
	defer s.addMu.Unlock()
	s.smu.Lock()
	dv, ok := s.found[d.BLEMac]
	s.smu.Unlock()
	switch {
	case !ok:
		return errors.New("app: add: this station was not found")
	case s.addedID == d.BLEMac:
		return nil
	case s.addedID != "":
		return errors.New("app: add: a station is already added")
	}
	if _, exists, err := s.st.StationByBLEMac(dv.BLEMac); err != nil {
		return err
	} else if !exists {
		st := store.Station{ID: dv.BLEMac, Name: stationName(dv.Device), Model: dv.Device, BLEMac: dv.BLEMac,
			WifiMac: dv.WifiMac, IP: dv.IP, Firmware: dv.Ver, CreatedAt: time.Now().UTC()}
		if err := s.st.UpsertStation(st); err != nil {
			return err
		}
		slog.Info("station added", "model", st.Model, "ip", st.IP, "firmware", st.Firmware)
	}
	s.addedID = d.BLEMac
	if s.OnAdded != nil {
		s.OnAdded()
	}
	return nil
}

// stationName is the model as a valid station name (settings.ValidName):
// cut to settings.NameMax characters, «Marstek» when empty.
func stationName(device string) string {
	name := strings.TrimSpace(device)
	if utf8.RuneCountInString(name) > settings.NameMax {
		name = strings.TrimSpace(string([]rune(name)[:settings.NameMax]))
	}
	if settings.ValidName(name) != nil {
		return "Marstek"
	}
	return name
}

// HasStation reports whether the first-run screen is not needed: a station
// is stored (also after a restart in the middle of the first run), or
// AutoAdd adds one by itself.
func (s *OnboardService) HasStation() (bool, error) {
	if s.opts.AutoAdd {
		return true, nil
	}
	sts, err := s.st.Stations()
	return len(sts) > 0, err
}

// Diagnose looks at the PC's network: Public profile, firewall, LAN. Parts
// it could not read are false, with the error.
func (s *OnboardService) Diagnose() (Diagnosis, error) {
	return s.diag()
}

// AllowFirewall adds the inbound UDP rule for the app on the Private
// profile through one UAC prompt (replacing any old rule of that name), and
// waits for it. A «No» is ErrCancelled; a call while the prompt is open is
// ErrFirewallPending at once (no second prompt). That holds until the
// elevated PowerShell has exited, also when the wait for it ran out. Only
// ever on the user's click.
func (s *OnboardService) AllowFirewall() error {
	if !s.allowing.CompareAndSwap(false, true) {
		return ErrFirewallPending
	}
	var hwnd uintptr
	if s.Owner != nil {
		hwnd = s.Owner()
	}
	running, err := s.allow(hwnd)
	if running == nil {
		s.allowing.Store(false)
		return err
	}
	go func() {
		<-running
		s.allowing.Store(false)
	}()
	return err
}

// systemDiagnosis is Diagnose on this PC (the PowerShell parts in parallel,
// about 1–2 s). Only adapters with a default gateway count for NoLAN and
// PublicNetwork (virtual switches and most VPNs have none).
func systemDiagnosis() (Diagnosis, error) {
	var dx Diagnosis
	as, adErr := systemAdapters()
	var aliases map[string]bool
	if adErr == nil {
		var nets []*net.IPNet
		nets, aliases = lanAdapters(as)
		dx.NoLAN = !hasPrivateLAN(nets)
	} else {
		dx.NoLAN = !hasPrivateLAN(localNets())
	}
	var pubErr, fwErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); dx.PublicNetwork, pubErr = systemPublicNetwork(aliases) }()
	go func() { defer wg.Done(); dx.FirewallMayBlock, fwErr = systemFirewallMayBlock() }()
	wg.Wait()
	return dx, errors.Join(adErr, pubErr, fwErr)
}

// sleepCtx waits d (nothing if d ≤ 0) or until ctx is done; either way a
// done ctx is its error.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return ctx.Err()
	}
}
