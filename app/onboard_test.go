package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lantest"
)

func stationInfo(mac string) lan.DeviceInfo {
	return lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: mac, WifiMac: "wifi-" + mac}
}

// timeLog records when a fake station received each Marstek.GetDevice.
type timeLog struct {
	mu    sync.Mutex
	times []time.Time
}

func (l *timeLog) add() {
	l.mu.Lock()
	l.times = append(l.times, time.Now())
	l.mu.Unlock()
}

func (l *timeLog) all() []time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]time.Time(nil), l.times...)
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// scanLog collects what the service emits.
type scanLog struct {
	mu      sync.Mutex
	devices []Device
	done    chan Diagnosis
}

func newScanLog() *scanLog { return &scanLog{done: make(chan Diagnosis, 4)} }

func (e *scanLog) emit(name string, data any) {
	switch name {
	case ScanDeviceEvent:
		e.mu.Lock()
		e.devices = append(e.devices, data.(Device))
		e.mu.Unlock()
	case ScanDoneEvent:
		e.done <- data.(Diagnosis)
	}
}

func (e *scanLog) waitDone(t *testing.T) Diagnosis {
	t.Helper()
	select {
	case dx := <-e.done:
		return dx
	case <-time.After(20 * time.Second):
		t.Fatal("no scan.done event")
	}
	return Diagnosis{}
}

// newTestOnboard is a service on a loopback client that talks to f: the
// scan's target and the probe's port are f's, loopback passes (Simulate),
// and 127.0.0.0/8 is the PC's only network.
func newTestOnboard(t *testing.T, f *lantest.FakeStation) (*OnboardService, *scanLog) {
	t.Helper()
	return newTestOnboardOpts(t, f, OnboardOptions{Simulate: true})
}

func newTestOnboardOpts(t *testing.T, f *lantest.FakeStation, opts OnboardOptions) (*OnboardService, *scanLog) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	targets := func() []*net.UDPAddr { return []*net.UDPAddr{f.Addr} }
	s := NewOnboardService(ctx, openTestStore(t), lan.NewClient(conn, time.Second), targets, f.Addr.Port, opts)
	ev := newScanLog()
	s.Emit = ev.emit
	s.nets = func() []*net.IPNet { return []*net.IPNet{mustNet(t, "127.0.0.1/8")} }
	s.diag = func() (Diagnosis, error) { return Diagnosis{FirewallMayBlock: true}, nil }
	return s, ev
}

func TestOnboardServiceSerializesCalls(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	var log timeLog
	f.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) {
		log.add()
		return stationInfo("aaaa"), nil
	})
	s, ev := newTestOnboard(t, f)
	ip := f.Addr.IP.String()

	if _, err := s.ProbeIP(ip); err != nil {
		t.Fatalf("1st probe: %v", err)
	}
	// A scan right after a probe, then a probe while it runs (it cancels
	// the scan), then another probe: none may come sooner than 1 s after
	// the call before it.
	if err := s.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(1500 * time.Millisecond) // the scan has sent its 1st broadcast
		if _, err := s.ProbeIP(ip); err != nil {
			t.Errorf("probe during the scan: %v", err)
		}
	}()
	ev.waitDone(t)
	wg.Wait()
	if _, err := s.ProbeIP(ip); err != nil {
		t.Fatalf("last probe: %v", err)
	}

	times := log.all()
	if len(times) < 4 {
		t.Fatalf("station got %d requests, want ≥ 4 (3 probes and a broadcast)", len(times))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < time.Second {
			t.Errorf("request %d came %v after the one before, want ≥ 1 s", i+1, gap)
		}
	}
}

func TestOnboardScanEmitsDevicesAndDiagnosis(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, ev := newTestOnboard(t, f)
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	if dx := ev.waitDone(t); dx != (Diagnosis{}) {
		t.Errorf("a station was found: diagnosis = %+v, want none", dx)
	}
	ev.mu.Lock()
	devs := append([]Device(nil), ev.devices...)
	ev.mu.Unlock()
	want := Device{Model: "VenusE 3.0", ModelCode: "VenusE 3.0", IP: f.Addr.IP.String(), BLEMac: "aaaa", Firmware: 151, Tested: true}
	if len(devs) != 1 || devs[0] != want {
		t.Errorf("devices = %+v, want [%+v]", devs, want)
	}

	// Nothing answers: the diagnosis comes with scan.done.
	f.Silence(true)
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	if dx := ev.waitDone(t); !dx.FirewallMayBlock {
		t.Errorf("none found: diagnosis = %+v, want the firewall hint", dx)
	}
}

func TestOnboardProbeIPErrorCodes(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	ip := f.Addr.IP.String()
	code := probeCode

	for _, bad := range []string{"abc", "192.168.1.255", "192.168.1"} {
		if _, err := s.ProbeIP(bad); code(err) != "format" {
			t.Errorf("%q: %v, want format", bad, err)
		}
	}
	if f.Calls("Marstek.GetDevice") != 0 {
		t.Errorf("a bad address reached the network")
	}

	f.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { return map[string]int{"foo": 1}, nil })
	if _, err := s.ProbeIP(ip); code(err) != "notMarstek" {
		t.Errorf("not a Marstek: %v, want notMarstek", err)
	}

	f.Silence(true)
	if _, err := s.ProbeIP(ip); code(err) != "noAnswer" {
		t.Errorf("silent, on the PC's subnet: %v, want noAnswer", err)
	}
	s.nets = func() []*net.IPNet { return []*net.IPNet{mustNet(t, "192.168.50.20/24")} }
	if _, err := s.ProbeIP(ip); code(err) != "otherSubnet" {
		t.Errorf("silent, off the PC's subnets: %v, want otherSubnet", err)
	}

	f.Silence(false)
	f.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { return stationInfo("aaaa"), nil })
	d, err := s.ProbeIP(" " + ip + " ")
	if err != nil || d.BLEMac != "aaaa" || d.IP != ip || d.Firmware != 151 || d.Added {
		t.Errorf("station: %+v, %v", d, err)
	}
}

// probeCode is err's ProbeError code.
func probeCode(err error) string {
	var pe *ProbeError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return "not a ProbeError: " + errString(err)
}

func errString(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

func TestOnboardAddStoresStation(t *testing.T) {
	f := lantest.Start(t, lan.DeviceInfo{Device: "VenusC", Ver: 120, BLEMac: "bbbb", WifiMac: "wifi-bbbb"})
	s, _ := newTestOnboard(t, f)
	added := 0
	s.OnAdded = func() { added++ }

	if has, err := s.HasStation(); err != nil || has {
		t.Fatalf("empty database: HasStation = %v, %v", has, err)
	}
	// Only a device this run found can be added.
	if err := s.Add(Device{BLEMac: "cccc", Model: "VenusE 3.0", IP: "192.168.1.47"}); err == nil {
		t.Error("unknown device added")
	}
	d, err := s.ProbeIP(f.Addr.IP.String())
	if err != nil {
		t.Fatal(err)
	}
	if d.Tested {
		t.Error("VenusC has no tested profile")
	}
	if err := s.Add(d); err != nil {
		t.Fatal(err)
	}
	sts, err := s.st.Stations()
	if err != nil || len(sts) != 1 {
		t.Fatalf("stations = %+v, %v", sts, err)
	}
	got := sts[0]
	if got.ID != "bbbb" || got.BLEMac != "bbbb" || got.Name != "VenusC" || got.Model != "VenusC" ||
		got.Firmware != 120 || got.IP != f.Addr.IP.String() || got.WifiMac != "wifi-bbbb" || got.CreatedAt.IsZero() {
		t.Errorf("stored %+v", got)
	}
	if added != 1 {
		t.Errorf("OnAdded ran %d times, want 1", added)
	}
	if has, err := s.HasStation(); err != nil || !has {
		t.Errorf("after Add: HasStation = %v, %v", has, err)
	}
	// Found again, it is marked as added.
	if d, err := s.ProbeIP(f.Addr.IP.String()); err != nil || !d.Added {
		t.Errorf("probe after Add: %+v, %v; want Added", d, err)
	}
}

func TestOnboardHandoverStopsCalls(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, ev := newTestOnboard(t, f)
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // the 1st broadcast is out
	quiet := s.Handover()              // cancels the scan and waits for it
	if dx := ev.waitDone(t); !dx.Cancelled {
		t.Errorf("scan.done after handover: %+v, want Cancelled", dx)
	}
	n := f.Calls("Marstek.GetDevice")
	if n == 0 || quiet.IsZero() || time.Since(quiet) < 0 {
		t.Fatalf("handover: %d calls, last at %v", n, quiet)
	}
	if err := s.Scan(); !errors.Is(err, ErrHandedOver) {
		t.Errorf("scan after handover: %v", err)
	}
	if _, err := s.ProbeIP(f.Addr.IP.String()); probeCode(err) != ProbeCancelled {
		t.Errorf("probe after handover: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if f.Calls("Marstek.GetDevice") != n {
		t.Error("a call went out after the handover")
	}
	// Nothing ran: the monitor may call at once.
	s2, _ := newTestOnboard(t, f)
	if q := s2.Handover(); !q.IsZero() {
		t.Errorf("unused service: last call %v, want zero", q)
	}
}

func TestOnboardAllowFirewallCancelled(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	s.allow = func(uintptr) (<-chan struct{}, error) { return nil, ErrCancelled }
	if err := s.AllowFirewall(); !errors.Is(err, ErrCancelled) {
		t.Errorf("UAC cancelled: %v", err)
	}
}

// A second click while the UAC prompt is open gets ErrFirewallPending at
// once and opens no second prompt; once it is closed, a click runs again.
// The prompt's owner is the window's HWND.
func TestOnboardAllowFirewallOnce(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	s.Owner = func() uintptr { return 0x1234 }
	var runs atomic.Int32
	release, entered := make(chan struct{}), make(chan uintptr, 4)
	s.allow = func(hwnd uintptr) (<-chan struct{}, error) {
		runs.Add(1)
		entered <- hwnd
		<-release
		return nil, nil
	}
	first := make(chan error, 1)
	go func() { first <- s.AllowFirewall() }()
	if hwnd := <-entered; hwnd != 0x1234 {
		t.Errorf("owner = %#x, want the window's 0x1234", hwnd)
	}
	second := make(chan error, 1)
	go func() { second <- s.AllowFirewall() }()
	returned := false
	select {
	case err := <-second:
		returned = true
		if !errors.Is(err, ErrFirewallPending) {
			t.Errorf("2nd click while pending: %v, want ErrFirewallPending", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("2nd click while pending did not return at once")
	}
	close(release)
	if !returned {
		<-second
	}
	if err := <-first; err != nil {
		t.Errorf("1st click: %v", err)
	}
	if err := s.AllowFirewall(); err != nil {
		t.Errorf("click after the prompt closed: %v", err)
	}
	if n := runs.Load(); n != 2 {
		t.Errorf("elevated runs = %d, want 2", n)
	}
}

// Ruling 34: 127.0.0.1 typed by hand reaches the simulator in -simulate*
// only; the real app refuses it before any network call.
func TestOnboardLoopbackOnlyWhenSimulating(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	prod, _ := newTestOnboardOpts(t, f, OnboardOptions{})
	for _, ip := range []string{"127.0.0.1", f.Addr.IP.String()} {
		if _, err := prod.ProbeIP(ip); probeCode(err) != ProbeFormat {
			t.Errorf("real app, %s: %v, want format", ip, err)
		}
	}
	if n := f.Calls("Marstek.GetDevice"); n != 0 {
		t.Fatalf("real app sent %d calls for a loopback address", n)
	}
	sim, _ := newTestOnboardOpts(t, f, OnboardOptions{Simulate: true})
	if d, err := sim.ProbeIP("127.0.0.1"); err != nil || d.BLEMac != "aaaa" {
		t.Errorf("simulate: %+v, %v", d, err)
	}
	for _, bad := range []string{"::1", "127.0.0.255.1", "0.0.0.0"} {
		if _, err := sim.ProbeIP(bad); probeCode(err) != ProbeFormat {
			t.Errorf("simulate, %q: %v, want format", bad, err)
		}
	}
}

// Ruling 33: ModelCode is the reply's src without "-<BLE MAC>".
func TestOnboardModelCodeFromSrc(t *testing.T) {
	f := lantest.Start(t, lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: "0a1b2c3d4e5f"})
	f.SetSrc("VNSE3-0-0a1b2c3d4e5f")
	s, ev := newTestOnboard(t, f)
	d, err := s.ProbeIP(f.Addr.IP.String())
	if err != nil || d.ModelCode != "VNSE3-0" || d.Model != "VenusE 3.0" {
		t.Errorf("probe: %+v, %v; want ModelCode VNSE3-0", d, err)
	}
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	ev.waitDone(t)
	ev.mu.Lock()
	if len(ev.devices) != 1 || ev.devices[0].ModelCode != "VNSE3-0" {
		t.Errorf("scan: %+v", ev.devices)
	}
	ev.mu.Unlock()

	for _, c := range []struct{ src, mac, want string }{
		{"VNSE3-0-0a1b2c3d4e5f", "0a1b2c3d4e5f", "VNSE3-0"},
		{"VNSE3-0-0A1B2C3D4E5F", "0a1b2c3d4e5f", "VNSE3-0"}, // case of the MAC
		{"fake-station", "0a1b2c3d4e5f", "VenusE 3.0"},      // no MAC suffix
		{"-0a1b2c3d4e5f", "0a1b2c3d4e5f", "VenusE 3.0"},     // nothing before it
		{"", "0a1b2c3d4e5f", "VenusE 3.0"},
		{"VNSE3-0-", "", "VenusE 3.0"},
	} {
		if got := modelCode(lan.DeviceInfo{Device: "VenusE 3.0", BLEMac: c.mac, Src: c.src}); got != c.want {
			t.Errorf("modelCode(src %q, mac %q) = %q, want %q", c.src, c.mac, got, c.want)
		}
	}
}

// A scan stopped by a probe ends with scan.done Cancelled (not "found
// nothing" with an empty diagnosis).
func TestOnboardScanCancelledByProbe(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	f.Silence(true)
	s, ev := newTestOnboard(t, f)
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	f.Silence(false)
	if _, err := s.ProbeIP(f.Addr.IP.String()); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if dx := ev.waitDone(t); !dx.Cancelled || dx.FirewallMayBlock {
		t.Errorf("scan.done = %+v, want Cancelled without a diagnosis", dx)
	}
}

// Two Adds at once for two found stations: only one is stored (v1 has one).
func TestOnboardAddConcurrent(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	s.found["aaaa"] = stationInfo("aaaa")
	s.found["bbbb"] = stationInfo("bbbb")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, mac := range []string{"aaaa", "bbbb"} {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Add(Device{BLEMac: mac}) }()
	}
	wg.Wait()
	close(errs)
	ok := 0
	for err := range errs {
		if err == nil {
			ok++
		}
	}
	sts, err := s.st.Stations()
	if err != nil || len(sts) != 1 || ok != 1 {
		t.Errorf("%d Adds succeeded, stations %+v, %v; want exactly 1", ok, sts, err)
	}
}

// Queued behind another call when the monitor takes over, or when the app
// closes, a probe is ProbeCancelled (a code the screen knows), not a raw error.
func TestOnboardProbeCancelledCodes(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	s.mu.Lock() // another call is running
	res := make(chan error, 1)
	go func() { _, err := s.ProbeIP(f.Addr.IP.String()); res <- err }()
	time.Sleep(200 * time.Millisecond) // the probe waits on mu
	s.handed.Store(true)               // what Handover does first
	s.mu.Unlock()
	if err := <-res; probeCode(err) != ProbeCancelled {
		t.Errorf("probe queued over the handover: %v, want cancelled", err)
	}

	s2, _ := newTestOnboard(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	s2.ctx = ctx
	cancel() // the app is closing
	if _, err := s2.ProbeIP(f.Addr.IP.String()); probeCode(err) != ProbeCancelled {
		t.Errorf("probe while closing: %v, want cancelled", err)
	}
	if n := f.Calls("Marstek.GetDevice"); n != 0 {
		t.Errorf("%d calls went out", n)
	}
}

// A call whose ctx is already done never runs, even as the service's first.
func TestOnboardCallCancelledBeforeFirst(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	if err := s.call(ctx, func(context.Context) error { ran = true; return nil }); !errors.Is(err, context.Canceled) || ran {
		t.Errorf("call = %v, ran %v; want context.Canceled, not run", err, ran)
	}
}

// AutoAdd (plain -simulate): the monitor's own discovery uses the client,
// so the service makes no call and the first-run screen is not needed.
func TestOnboardAutoAddStaysOut(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboardOpts(t, f, OnboardOptions{Simulate: true, AutoAdd: true})
	if has, err := s.HasStation(); err != nil || !has {
		t.Errorf("HasStation = %v, %v; want true", has, err)
	}
	if err := s.Scan(); !errors.Is(err, ErrHandedOver) {
		t.Errorf("scan: %v", err)
	}
	if _, err := s.ProbeIP(f.Addr.IP.String()); probeCode(err) != ProbeCancelled {
		t.Errorf("probe: %v", err)
	}
	if q := s.Handover(); !q.IsZero() {
		t.Errorf("handover: last call %v, want zero", q)
	}
	if n := f.Calls("Marstek.GetDevice"); n != 0 {
		t.Errorf("%d calls went out", n)
	}
}

// The wait for the elevated PowerShell ran out but the process lives on:
// clicks stay ErrFirewallPending (no second elevated run) until it exits.
func TestOnboardAllowFirewallGuardOutlivesTimeout(t *testing.T) {
	f := lantest.Start(t, stationInfo("aaaa"))
	s, _ := newTestOnboard(t, f)
	exited := make(chan struct{})
	var runs atomic.Int32
	s.allow = func(uintptr) (<-chan struct{}, error) {
		runs.Add(1)
		return exited, errors.New("powershell did not finish")
	}
	if err := s.AllowFirewall(); err == nil || errors.Is(err, ErrFirewallPending) {
		t.Fatalf("timed-out run: %v, want its error", err)
	}
	if err := s.AllowFirewall(); !errors.Is(err, ErrFirewallPending) {
		t.Errorf("click while the elevated process still runs: %v, want ErrFirewallPending", err)
	}
	close(exited)
	deadline := time.Now().Add(2 * time.Second)
	for s.allowing.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.allow = func(uintptr) (<-chan struct{}, error) { runs.Add(1); return nil, nil }
	if err := s.AllowFirewall(); err != nil {
		t.Errorf("click after the process exited: %v", err)
	}
	if n := runs.Load(); n != 2 {
		t.Errorf("elevated runs = %d, want 2", n)
	}
}
