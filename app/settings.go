package app

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/export"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Version is the app version, set at build time:
// -ldflags "-X github.com/econ11/marstek-monitor-code/app.Version=…".
var Version = "dev"

// SettingsEvent is the Go -> JS event carrying a SettingsView after every
// change.
const SettingsEvent = "settings"

func init() {
	application.RegisterEvent[SettingsView](SettingsEvent)
}

// SettingsView is what the Settings screen shows. Lang is the language in
// use ("uk" or "en"; S.Language "" resolves to the Windows language).
// HasHistory: the station has at least one stored sample.
// Partial: a Save or SetAutostart stored and applied the change, but the
// rest of the view (Autostart, HasHistory) could not be
// read; those fields are then unknown and the screen refreshes with Get.
type SettingsView struct {
	S                 settings.Settings
	Lang              string
	Autostart         bool
	Version, DataPath string
	Platform          string // runtime.GOOS
	HasHistory        bool
	Partial           bool
}

// SettingsService is bound to the frontend: it reads and saves the user's
// settings and applies them at once.
type SettingsService struct {
	st       *store.Store
	dataPath string
	monitor  func() *monitor.Monitor
	apply    func(settings.Settings) // monitor, tray, window, language
	emit     func(SettingsView)

	mu sync.Mutex // one change at a time: save, apply and emit stay in order

	// Deliver shows a toast (with the latest snapshot); main sets it before
	// the app runs. nil: SendTestNotification fails.
	Deliver func(notify.Toast) error
	openURL func(string) error // for tests; nil = the Windows URL handler
	// For tests: pickDir nil = the Wails folder dialog ("" = cancelled);
	// selectFile nil = Explorer with the file selected.
	pickDir    func() (string, error)
	selectFile func(string) error
	autostart  func() (bool, error) // for tests; nil = AutostartEnabled
	toastsOff  func() (bool, error) // for tests; nil = ToastsOff of the app's id

	// Simulate (-simulate*, Ruling 35): SetAutostart never writes the Run
	// entry, so a test run cannot point the owner's autostart at its exe.
	// main sets it before the app runs.
	Simulate     bool
	simulateOnce sync.Once
}

// NewSettingsService: mon returns the running monitor (nil until a station
// is found); apply and emit may be nil.
//
//wails:ignore
func NewSettingsService(st *store.Store, dataPath string, mon func() *monitor.Monitor,
	apply func(settings.Settings), emit func(SettingsView)) *SettingsService {
	if apply == nil {
		apply = func(settings.Settings) {}
	}
	if emit == nil {
		emit = func(SettingsView) {}
	}
	return &SettingsService{st: st, dataPath: dataPath, monitor: mon, apply: apply, emit: emit}
}

// Get returns the current settings view.
func (s *SettingsService) Get() (SettingsView, error) {
	v, err := settings.Load(s.st)
	if err != nil {
		return SettingsView{}, err
	}
	return s.view(v)
}

func (s *SettingsService) view(v settings.Settings) (SettingsView, error) {
	out := s.baseView(v)
	autostart := s.autostart
	if autostart == nil {
		autostart = AutostartEnabled
	}
	var err error
	if out.Autostart, err = autostart(); err != nil {
		return SettingsView{}, err
	}
	sts, err := s.st.Stations()
	if err != nil {
		return SettingsView{}, err
	}
	if len(sts) > 0 {
		if _, out.HasHistory, err = s.st.ObservedSince(sts[0].ID); err != nil {
			return SettingsView{}, err
		}
	}
	return out, nil
}

// baseView is the part of the view that cannot fail.
func (s *SettingsService) baseView(v settings.Settings) SettingsView {
	return SettingsView{S: v, Lang: ResolveLanguage(v.Language, SystemLanguage()), Version: Version, Platform: runtime.GOOS, DataPath: s.dataPath}
}

// appliedView is the view after a change was stored and applied: the full
// view, else the base one marked Partial (the error is logged, not returned).
func (s *SettingsService) appliedView(v settings.Settings) SettingsView {
	out, err := s.view(v)
	if err != nil {
		slog.Warn("settings: view after a saved change", "err", err)
		out = s.baseView(v)
		out.Partial = true
	}
	return out
}

// Save validates and stores v, applies it (monitor, tray, window, language)
// and emits SettingsEvent. An invalid v changes nothing. Once v is stored and
// applied, Save succeeds: a view that cannot be read in full comes back
// Partial (Task 4b review: the UI must not report a saved change as failed).
func (s *SettingsService) Save(v settings.Settings) (SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := settings.Save(s.st, v); err != nil {
		return SettingsView{}, err
	}
	// The language first: apply redraws the tray in it.
	i18n.SetLang(ResolveLanguage(v.Language, SystemLanguage()))
	s.apply(v)
	out := s.appliedView(v)
	s.emit(out)
	return out, nil
}

// SetAutostart turns "start with Windows" on or off. In a -simulate* run it
// changes nothing and reports the entry as it is (Ruling 35).
func (s *SettingsService) SetAutostart(on bool) (SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Simulate {
		s.simulateOnce.Do(func() { slog.Info("autostart: simulate run, the Run entry is left alone") })
		v, err := settings.Load(s.st)
		if err != nil {
			return SettingsView{}, err
		}
		return s.view(v)
	}
	exe, err := os.Executable()
	if err != nil {
		return SettingsView{}, err
	}
	if err := SetAutostart(on, exe); err != nil {
		return SettingsView{}, err
	}
	v, err := settings.Load(s.st)
	if err != nil {
		// The switch is set: report it, without settings to show or emit.
		slog.Warn("settings: view after autostart", "err", err)
		return SettingsView{Autostart: on, Version: Version, Platform: runtime.GOOS, DataPath: s.dataPath, Partial: true}, nil
	}
	out := s.appliedView(v)
	if out.Partial {
		out.Autostart = on
	}
	s.emit(out)
	return out, nil
}

// NotificationsOff reports whether Windows blocks the app's toasts (turned
// off for the app or for all apps, or by policy): Settings → Сповіщення then
// shows the banner settings.notify.osDisabled instead of the test button
// (copy §11.2). The screen asks again whenever the window gains focus. A
// setting that cannot be read counts as on: no banner.
func (s *SettingsService) NotificationsOff() bool {
	f := s.toastsOff
	if f == nil {
		f = func() (bool, error) { return ToastsOff(application.Get().Config().Name) }
	}
	off, err := f()
	if err != nil {
		slog.Debug("settings: notification setting", "err", err)
		return false
	}
	return off
}

// RenameStation sets the user's name for the station (settings.ValidName).
func (s *SettingsService) RenameStation(name string) error {
	m := s.monitor()
	if m == nil {
		return monitor.ErrNoStation
	}
	return m.RenameStation(name)
}

// SendTestNotification sends the test toast (copy.md notify.test). It is
// never silent: quiet hours and "do not disturb" do not apply, Windows
// Focus does (that is what the test checks).
func (s *SettingsService) SendTestNotification() error {
	if s.Deliver == nil {
		return errors.New("settings: notifications are not available")
	}
	t := notify.Toast{Kind: notify.Test, At: time.Now(), Silent: false}
	if m := s.monitor(); m != nil {
		t.StationID = m.Latest().Station.ID
	}
	return s.Deliver(t)
}

// ExportCSV asks for a folder and writes the station's outages, journal
// events and charge history of the local days fromDay..toDay ("2006-01-02",
// inclusive) there as three CSV files (core/export) in the app language's
// dialect (export.FormatFor: uk «;» and a decimal comma), each event's text
// the sentence the Errors tab shows (journalText); it returns their paths.
// A cancelled dialog returns nil, nil.
func (s *SettingsService) ExportCSV(fromDay, toDay string) (paths []string, err error) {
	sts, err := s.st.Stations()
	if err != nil {
		return nil, err
	}
	if len(sts) == 0 {
		return nil, errors.New("settings: no station to export")
	}
	now := time.Now()
	p, err := history.PeriodFor(fromDay, toDay, now)
	if err != nil {
		return nil, err
	}
	pick := s.pickDir
	if pick == nil {
		pick = func() (string, error) {
			return application.Get().Dialog.OpenFile().
				CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
				PromptForSingleSelection()
		}
	}
	dir, err := pick()
	if err != nil || dir == "" {
		return nil, err
	}
	f := export.FormatFor(i18n.Lang())
	f.EventText = journalText
	return export.Write(dir, s.st, sts[0].ID, p.From, p.To, now, f)
}

// ShowInFolder opens Explorer with the file at path selected: an exported
// CSV, so only an absolute path on a local drive letter (no UNC or \\?\
// path: the check itself would reach the network) to an existing .csv file.
func (s *SettingsService) ShowInFolder(path string) error {
	if vol := filepath.VolumeName(path); len(vol) != 2 || vol[1] != ':' || !filepath.IsAbs(path) {
		return errors.New("settings: not a local absolute path")
	}
	if !strings.EqualFold(filepath.Ext(path), ".csv") {
		return errors.New("settings: not a CSV file")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("settings: not a file")
	}
	sel := s.selectFile
	if sel == nil {
		sel = func(p string) error {
			// explorer wants /select,"<path>" as one argument with the quotes
			// inside it, which exec's own quoting does not produce.
			cmd := exec.Command("explorer.exe")
			cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
			if err := cmd.Start(); err != nil {
				return err
			}
			go cmd.Wait() // explorer's exit code means nothing
			return nil
		}
	}
	return sel(path)
}

// PruneNow applies the saved retention at once (after the user confirmed a
// shorter one), not at the next daily compaction: through the monitor when
// it runs, else directly on every stored station.
func (s *SettingsService) PruneNow() error {
	v, err := settings.Load(s.st)
	if err != nil {
		return err
	}
	if v.RetentionDays <= 0 {
		return nil
	}
	if m := s.monitor(); m != nil {
		return m.PruneNow()
	}
	sts, err := s.st.Stations()
	if err != nil {
		return err
	}
	before := monitor.RetentionBound(time.Now(), v.RetentionDays)
	for _, st := range sts {
		if err := s.st.Prune(st.ID, before); err != nil {
			return err
		}
	}
	return nil
}

// OpenWindowsNotificationSettings opens Settings → System → Notifications.
func (s *SettingsService) OpenWindowsNotificationSettings() error {
	open := s.openURL
	if open == nil {
		open = application.Get().Browser.OpenURL
	}
	return open("ms-settings:notifications")
}
