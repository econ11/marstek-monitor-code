package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/econ11/marstek-monitor-code/app"
	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/econ11/marstek-monitor-code/core/update"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	appID = "MarstekMonitor" // mutex name, data folder
	// How often to look for a station while none is known: the station drops
	// about one discovery request in three, so the first retries come quickly
	// (5, 10, 20, 40 s), then every minute.
	firstMonitorRetry = 5 * time.Second
	newMonitorRetry   = 60 * time.Second
	callTimeout       = 3 * time.Second
	// onboardGap: from the first-run screen's last station call to the
	// monitor's first (the station drops calls < 1 s apart; with a margin).
	onboardGap   = 1100 * time.Millisecond
	shutdownWait = 3 * time.Second
	maxLogSize   = 1 << 20

	// simLabel marks everything a simulated run shows outside its window.
	simLabel = "[Імітація] "

	settingAutostartInit = "app.autostart_initialized"
	settingTrayHintShown = "app.tray_hint_shown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// parseArgs reads the flags by hand: Windows may start the app with foreign
// arguments (a toast click on a stopped app), which must not abort it.
// -simulate-quiet is -simulate with quiet hours on all day (toasts must not
// pop up); automated runs use it. It combines with -simulate-dir. -simulate-dir <folder> is -simulate that keeps its database and
// log in that folder between runs (to see what a restart after a pause shows).
// -simulate-firstrun is -simulate that does not add the simulated station by
// itself (Deps.AutoAdd off), so the first-run screen finds it; it combines
// with the other -simulate flags. -simulate-e2e is -simulate-firstrun whose
// first-run screen runs a fixed script by itself (frontendFlags).
func parseArgs(args []string) (background, simulate, quiet, firstRun bool, simDir string) {
	for i := 0; i < len(args); i++ {
		switch strings.TrimLeft(args[i], "-") {
		case "background":
			background = true
		case "simulate":
			simulate = true
		case "simulate-firstrun", "simulate-e2e":
			simulate, firstRun = true, true
		case "simulate-quiet":
			simulate, quiet = true, true
		case "simulate-dir":
			if i+1 < len(args) {
				i++
				simulate, simDir = true, args[i]
			}
		}
	}
	return
}

// frontendFlags are the Wails flags the frontend reads (Flags.GetFlag).
// e2e=firstrun (-simulate-e2e only): the first-run screen drives itself
// through search, a wrong IP, Add and the overview, with no input, so an
// agent can check the path end to end (frontend/src/views/firstRunE2E.ts).
func frontendFlags(args []string) map[string]any {
	for _, a := range args {
		if strings.TrimLeft(a, "-") == "simulate-e2e" {
			return map[string]any{"e2e": "firstrun"}
		}
	}
	return map[string]any{}
}

func run() error {
	background, simulate, quiet, firstRun, simDir := parseArgs(os.Args[1:])

	// The simulator has its own identity so it can run next to the real app.
	id := appID
	if simulate {
		id += "Simulate"
	}
	inst, first, err := app.FirstInstance(id)
	if err != nil {
		return fmt.Errorf("single instance: %w", err)
	}
	if !first {
		return nil // the first copy shows its window
	}

	dir, cleanup, err := dataDir(simulate, simDir)
	if err != nil {
		return err
	}
	defer cleanup()
	logFile := openLog(filepath.Join(dir, "monitor.log"), simulate)
	defer logFile.Close()

	st, err := store.Open(filepath.Join(dir, "monitor.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	user, err := settings.Load(st)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	// Tray and toasts speak the user's language; Save switches it live.
	i18n.SetLang(app.ResolveLanguage(user.Language, app.SystemLanguage()))
	// -simulate-quiet keeps quiet hours on all day whatever the settings say.
	notifyFor := func(n notify.Settings) notify.Settings {
		if quiet {
			n.QuietEnabled, n.QuietStart, n.QuietEnd = true, "00:00", "23:59"
		}
		return n
	}
	user.Notify = notifyFor(user.Notify)
	cfg := monitor.ConfigFor(user)
	deps := monitor.Deps{Store: st, Boot: background, Notify: cfg.Notify, PollInterval: cfg.Poll,
		RetentionDays: cfg.RetentionDays, StationMinSOC: cfg.StationMinSOC, PCOnline: pcOnline}
	// After the one-click update the restart gap is «оновлення додатка».
	deps.AppUpdated = !simulate && app.TakeUpdateRestart(st, app.Version, time.Now())
	listen := fmt.Sprintf(":%d", lan.DefaultPort)
	probePort := lan.DefaultPort // the port an IP typed by hand is asked on
	// Every interface is asked: with several adapters 255.255.255.255 alone
	// leaves through one of them only.
	deps.Targets = func() []*net.UDPAddr { return lan.BroadcastTargets(lan.DefaultPort) }
	if simulate {
		sim, err := startSimulator()
		if err != nil {
			return fmt.Errorf("simulator: %w", err)
		}
		defer sim.Close()
		listen, deps.PCOnline, probePort = "127.0.0.1:0", nil, sim.Addr.Port
		deps.Targets = func() []*net.UDPAddr { return []*net.UDPAddr{sim.Addr} }
		// The simulator is added at once, unless -simulate-firstrun leaves
		// it to the first-run screen (it is still the discovery target).
		deps.AutoAdd = !firstRun
		slog.Info("simulated station", "addr", sim.Addr.String(), "quiet", quiet, "firstrun", firstRun)
	}
	conn, err := net.ListenPacket("udp4", listen)
	if err != nil && !simulate {
		// Port 30000 is taken (e.g. by the probe): any port works for polling.
		conn, err = net.ListenPacket("udp4", ":0")
	}
	if err != nil {
		return fmt.Errorf("udp socket: %w", err)
	}
	defer conn.Close()
	deps.Client = lan.NewClient(conn, callTimeout)

	autostartAtStart(st, simulate)

	notifier := app.NewNotifier()
	// Toasts show the app icon of their event; the image must be a file.
	if err := notifier.SetLogoDir(filepath.Join(dir, "icons")); err != nil {
		slog.Warn("toast logos", "err", err)
	}
	if simulate {
		notifier.TitlePrefix = simLabel
	}
	svc := &app.MonitorService{}
	nav := &app.NavigateService{} // a toast click's tab, held until the page listens
	ctx, cancel := context.WithCancel(context.Background())
	deps.Ctx = ctx // quitting cancels a first discovery (-simulate) in monitor.New
	var wg sync.WaitGroup
	// The first-run screen shares the monitor's client (port 30000 has one
	// socket), and its calls keep the station's 1 s gap up to the monitor's.
	// -simulate*: a typed 127.0.0.1 reaches the simulator (Ruling 34). With
	// AutoAdd the monitor finds the station itself and the service stays out.
	onb := app.NewOnboardService(ctx, st, deps.Client, deps.Targets, probePort,
		app.OnboardOptions{Simulate: simulate, AutoAdd: deps.AutoAdd})
	r := &runner{st: st, deps: deps, cfg: cfg, svc: svc, notifier: notifier, onboard: onb,
		added: make(chan struct{}, 1)}
	onb.OnAdded = r.stationAdded
	// The update check (plan 3, Task 4): one GET to the GitHub API, no
	// cookie jar, https-only redirects. -simulate* never asks GitHub, not
	// even from the button (agents never reach GitHub).
	updates := app.NewUpdatesService(st, app.Version, update.NewClient(), user.UpdateCheck)
	updates.Simulate = simulate
	updates.Deliver = func(t notify.Toast) error { return notifier.Deliver(t, monitor.Snapshot{}, st) }
	// The one-click update (Task 5; never in -simulate*): the installer from
	// the release, its DB backup next to the database, then the app quits.
	updates.Download = update.NewDownloadClient()
	updates.BackupDir = filepath.Join(dir, "backups")
	var a *application.App
	settingsSvc := app.NewSettingsService(st, dir, r.monitor, func(s settings.Settings) {
		s.Notify = notifyFor(s.Notify)
		r.configure(monitor.ConfigFor(s))
		r.tray.SetSettings(s.Notify)
		r.win.SetCloseToTray(s.MinimizeToTray)
		updates.SetAuto(s.UpdateCheck)
	}, func(v app.SettingsView) { a.Event.Emit(app.SettingsEvent, v) })
	settingsSvc.Simulate = simulate // Ruling 35: never the owner's Run entry
	settingsSvc.Deliver = func(t notify.Toast) error {
		var s monitor.Snapshot
		if m := r.monitor(); m != nil {
			s = m.Latest()
		}
		return notifier.Deliver(t, s, st)
	}
	// Sleep / wake: polling pauses, the gap gets the reason "PC slept", and
	// waking reconnects at once.
	power := app.NewPowerEvents(
		func() {
			slog.Info("PC is going to sleep")
			if m := r.monitor(); m != nil {
				m.Suspend()
			}
		},
		func() {
			slog.Info("PC woke up")
			if m := r.monitor(); m != nil {
				m.Resume()
			}
		})

	a = application.New(application.Options{
		Name:        "Marstek Monitor",
		Description: "Marstek battery monitor",
		Services: []application.Service{
			application.NewService(&app.Service{}),
			application.NewService(svc),
			application.NewService(settingsSvc),
			application.NewService(onb),
			application.NewService(nav),
			application.NewService(updates),
			application.NewService(notifier.Service()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Flags: frontendFlags(os.Args[1:]),
		// The app lives in the tray: hiding the window must not end it.
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WndProcInterceptor:            power.WndProc,
		},
		// macOS: a menu-bar app; the Dock icon shows only while the window is
		// open (app/dock.go). --background starts with no window, so no Dock
		// icon from the first frame.
		Mac:        application.MacOptions{ActivationPolicy: macActivationPolicy(background)},
		OnShutdown: cancel,
	})

	onb.Emit = func(name string, data any) { a.Event.Emit(name, data) }
	nav.Emit = func(n app.Navigate) { a.Event.Emit(app.NavigateEvent, n) }
	updates.Emit = func(v app.UpdateView) { a.Event.Emit(app.UpdatesEvent, v) }
	updates.Quit = func() { go a.Quit() } // the installer runs; the shutdown path below closes the DB

	var hintOnce sync.Once
	win := app.NewWindow(a, background, func() {
		hintOnce.Do(func() { trayHint(st, notifier) })
	})
	win.SetCloseToTray(user.MinimizeToTray)
	onb.Owner = win.HWND // the UAC prompt comes up in front of the window
	tray := app.NewTray(a, win.Show, cfg.Notify)
	updates.NotifyState = tray.NotifyState
	if simulate {
		tray.SetLabel(simLabel)
	}
	inst.OnSecondLaunch(func() {
		slog.Info("second copy started: showing the window")
		win.Show()
	})
	// A toast click shows the window on the tab the toast is about (flows
	// 2.8): an outage still going on is Overview, an ended one is Outages.
	// Before the page listens (--background: no page loaded yet) the route
	// waits in nav until the frontend calls Ready.
	notifier.OnClick(func(_ string, kind notify.Kind, tag string) {
		var open int64
		if m := r.monitor(); m != nil {
			if c := m.Latest().Current; c != nil && c.End == nil {
				open = c.ID
			}
		}
		win.Show()
		nav.Push(app.ToastRoute(kind, tag, open))
	})

	r.a, r.tray, r.win = a, tray, win
	a.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		// macOS asks for notification permission once, at start (it waits
		// for the user, so off the UI thread); Windows does nothing.
		go app.AskNotificationPermission()
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.run(ctx)
		}()
		if simulate {
			slog.Info("update check: off in a simulated run")
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A finished update left its installer in %TEMP% (final review M6).
			updates.CleanTemp()
			updates.Run(ctx)
		}()
	})
	err = a.Run()
	cancel()
	// Let the monitor finish its tick before the database closes. Bounded: a
	// goroutine caught in a UI call after the UI thread ended never returns.
	stopped := make(chan struct{})
	go func() { wg.Wait(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(shutdownWait):
	}
	return err
}

// runner finds the station, runs the monitor and feeds the tray, the
// frontend and the toasts from it.
type runner struct {
	a        *application.App
	st       *store.Store
	deps     monitor.Deps
	cfg      monitor.Config // the latest settings; applied to a monitor when it starts
	svc      *app.MonitorService
	tray     *app.Tray
	win      *app.Window
	notifier *app.Notifier
	onboard  *app.OnboardService
	added    chan struct{} // a station was added on the first-run screen

	mu  sync.Mutex
	mon *monitor.Monitor
}

func (r *runner) monitor() *monitor.Monitor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mon
}

// configure applies changed settings to the running monitor, and to the one
// still to start.
func (r *runner) configure(c monitor.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = c
	r.deps.Notify, r.deps.PollInterval, r.deps.RetentionDays, r.deps.StationMinSOC = c.Notify, c.Poll, c.RetentionDays, c.StationMinSOC
	if r.mon != nil {
		r.mon.Configure(c)
	}
}

// stationAdded wakes run up at once (OnboardService.OnAdded).
func (r *runner) stationAdded() {
	select {
	case r.added <- struct{}{}:
	default:
	}
}

func (r *runner) depsNow() monitor.Deps {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deps
}

func (r *runner) run(ctx context.Context) {
	var m *monitor.Monitor
	retry := firstMonitorRetry
	for {
		var err error
		d := r.depsNow()
		if m, err = monitor.New(d); err == nil {
			break
		}
		// No station in the database: the tray says "no station yet". The
		// first-run screen adds one and wakes us; with AutoAdd (-simulate)
		// we also look again later.
		if ctx.Err() != nil { // quitting cancelled a first discovery: not an error
			return
		}
		wait := time.After(retry)
		switch {
		case errors.Is(err, monitor.ErrNoStation) && !d.AutoAdd:
			slog.Info("no station yet: waiting for the first-run screen")
			wait = nil
		case errors.Is(err, monitor.ErrNoStation):
			slog.Info("no station yet")
		default:
			slog.Error("monitor cannot start", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-r.added:
		case <-wait:
			retry = min(2*retry, newMonitorRetry)
		}
	}
	// The first-run screen makes no more station calls, and the monitor's
	// first one comes ≥ 1 s after its last.
	if last := r.onboard.Handover(); !last.IsZero() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(last.Add(onboardGap))):
		}
		slog.Info("first run handed over", "sinceLastCall", time.Since(last).Round(time.Millisecond))
	}
	r.mu.Lock()
	r.mon = m
	// Settings saved while New ran reached r.cfg only.
	m.Configure(r.cfg)
	r.mu.Unlock()
	r.svc.SetMonitor(m)
	r.tray.SetMonitor(m)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case s := <-m.Snapshots():
				r.tray.Update(s)
				r.win.SetGrid(s.Grid)
				r.a.Event.Emit(app.SnapshotEvent, s)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-m.Toasts():
				if err := r.notifier.Deliver(t, m.Latest(), r.st); err != nil {
					slog.Warn("toast", "kind", t.Kind, "err", err)
				}
			}
		}
	}()
	if err := m.Run(ctx); err != nil {
		slog.Error("monitor stopped", "err", err)
	}
	wg.Wait()
}

// dataDir is %LOCALAPPDATA%\MarstekMonitor (macOS: ~/Library/Application
// Support/MarstekMonitor); the simulator gets a temporary folder that
// cleanup removes.
func dataDir(simulate bool, simDir string) (dir string, cleanup func(), err error) {
	if simDir != "" { // kept between simulated runs
		return simDir, func() {}, os.MkdirAll(simDir, 0o755)
	}
	if simulate {
		dir, err = os.MkdirTemp("", appID+"-simulate-")
		return dir, func() { os.RemoveAll(dir) }, err
	}
	base, err := userDataBase()
	if err != nil {
		return "", nil, err
	}
	dir = filepath.Join(base, appID)
	return dir, func() {}, os.MkdirAll(dir, 0o755)
}

// openLog sends slog to a file (a GUI app has no console); the file starts
// over once it passes 1 MB. debug also logs every tray change (-simulate).
func openLog(path string, debug bool) io.Closer {
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogSize {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return io.NopCloser(nil)
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})))
	return f
}

// autostartAtStart turns autostart on at the first start and repoints it to
// the installed exe (plan 3, Task 6). -simulate* never touches the owner's
// Run entry (Ruling 35).
func autostartAtStart(st *store.Store, simulate bool) {
	if simulate {
		return
	}
	enableAutostartOnce(st)
	repointAutostart()
}

// repointAutostart points an existing autostart entry at this exe when it
// runs from the installer's folder (a var so main_test can check it is not
// called in -simulate*).
var repointAutostart = func() {
	exe, err := os.Executable()
	if err != nil {
		slog.Warn("autostart repoint", "err", err)
		return
	}
	dir, err := app.InstalledDir()
	if err != nil {
		slog.Warn("autostart repoint", "err", err)
		return
	}
	changed, err := app.RepointAutostart(exe, dir)
	switch {
	case err != nil:
		slog.Warn("autostart repoint", "err", err)
	case changed:
		slog.Info("autostart repointed to the installed exe", "exe", exe)
	}
}

// enableAutostartOnce turns "start with Windows" on at the first start only,
// so switching it off later sticks.
func enableAutostartOnce(st *store.Store) {
	if _, done, err := st.GetSetting(settingAutostartInit); err != nil || done {
		return
	}
	exe, err := os.Executable()
	if err == nil {
		err = app.SetAutostart(true, exe)
	}
	if err != nil {
		slog.Warn("autostart", "err", err)
		return
	}
	if err := st.SetSetting(settingAutostartInit, "1"); err != nil {
		slog.Warn("autostart", "err", err)
	}
}

// trayHint tells the user, once ever, that closing the window left the app
// running in the tray (flows 5.2).
func trayHint(st *store.Store, n *app.Notifier) {
	if _, shown, err := st.GetSetting(settingTrayHintShown); err != nil || shown {
		return
	}
	if err := n.Toast("tray-hint", i18n.T("tray.firstMinimize.toast.title"),
		i18n.T("tray.firstMinimize.toast.body")); err != nil {
		slog.Warn("toast", "err", err)
		return
	}
	if err := st.SetSetting(settingTrayHintShown, "1"); err != nil {
		slog.Warn("tray hint", "err", err)
	}
}

// pcOnline reports whether the PC has a network at all: an up, non-loopback
// interface with an IPv4 address.
func pcOnline() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return true
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				return true
			}
		}
	}
	return false
}

// macActivationPolicy is the macOS activation policy at start: accessory (no
// Dock icon) for --background, regular when the window opens at once. The
// Dock follows the window from then on (app/dock.go).
func macActivationPolicy(background bool) application.ActivationPolicy {
	if background {
		return application.ActivationPolicyAccessory
	}
	return application.ActivationPolicyRegular
}
