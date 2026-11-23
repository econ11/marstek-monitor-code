package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/econ11/marstek-monitor-code/core/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// UpdatesEvent is the Go -> JS event carrying an UpdateView after every
// change of the update state.
const UpdatesEvent = "updates:changed"

func init() {
	application.RegisterEvent[UpdateView](UpdatesEvent)
}

// UpdateErrCheck is UpdateView.Err after a manual check that failed
// (offline, rate-limited, a GitHub error). The one-click update has its
// own codes (updater.go).
const UpdateErrCheck = "check_failed"

// UpdateErrSimulated is UpdateView.Err after the manual button (or
// Install) in a -simulate* run, which never asks GitHub.
const UpdateErrSimulated = "simulated"

const (
	// settingNotifiedVersion: the newest version a toast was shown (or the
	// manual check reported) for; internal, in the settings table.
	settingNotifiedVersion = "update.notified_version"
	// settingLastCheck: when the last automatic check started (Unix
	// seconds), so a restart does not check again within the day.
	settingLastCheck = "update.last_check"
	// settingFoundRelease: the newer release the last successful check
	// found (JSON: version and its page, installer and checksum URLs), so
	// «Доступна версія …» and «Оновити» survive a restart; cleared by a check
	// that finds nothing newer and once the running version is not older.
	// Internal, in the settings table.
	settingFoundRelease = "update.found"

	firstCheckAfter = time.Minute    // after start: the network is up by then
	checkEvery      = 24 * time.Hour // automatic checks, at most
	checkTimeout    = 20 * time.Second
)

// UpdateView is what Settings → Про програму shows about updates. Current
// is the running version; Latest the newest published one ("" until a check
// succeeds); Available: Latest is newer than Current; PageURL its release
// page. Err is a code: UpdateErrCheck (a failed manual check), or why the
// one-click update failed (UpdateErrDownload…). Progress (0..1, the
// download) and Phase (Phase*) are set while that update runs. CanInstall:
// this copy can update itself («Оновити»; canInstallHere, per platform),
// else only «Завантажити» is offered.
type UpdateView struct {
	Current, Latest string
	Available       bool
	CanInstall      bool
	PageURL         string
	Checking        bool
	Err             string
	Progress        float64
	Phase           string
}

// UpdatesService checks for a newer version: automatically once a day
// (Run, when UpdateCheck is on) and on the «Перевірити зараз» button
// (Check). An automatic check that fails shows nothing and is retried the
// next day. A newer version found automatically gets one toast per version.
type UpdatesService struct {
	st      *store.Store
	current string
	http    update.HTTPDoer
	now     func() time.Time                     // for tests
	after   func(time.Duration) <-chan time.Time // for tests
	started time.Time

	// Emit sends UpdatesEvent; Deliver shows a toast; NotifyState gives the
	// quiet hours and the DND deadline that silence the toast. main sets
	// them before Run; nil = none.
	Emit        func(UpdateView)
	Deliver     func(notify.Toast) error
	NotifyState func() (notify.Settings, *time.Time)
	// Simulate (-simulate*): Check never asks GitHub (Run is not started
	// either) and Install never downloads or starts an installer. main sets
	// it before the app runs.
	Simulate bool

	// The one-click update (Install, updater.go). Download fetches the
	// assets (nil = the check's client); TempDir receives the installer
	// (%TEMP%\MarstekMonitor-update, cleaned before each run); BackupDir
	// takes the DB backup (main: <data dir>\backups; "" = the update fails
	// at the backup); Quit ends the app once the installer runs.
	Download  update.HTTPDoer
	TempDir   string
	BackupDir string
	Quit      func()
	start     func(path string) error // startInstaller; a fake in tests

	// dev: current is not a version (a developer build, "dev" or ""):
	// nothing is checked, restored or installed.
	dev bool

	mu   sync.Mutex
	auto bool
	view UpdateView
	rel  update.Release
}

// NewUpdatesService: current is the running version (Version), c the HTTP
// client (no cookie jar), auto the UpdateCheck setting. The release an
// earlier run found (settingFoundRelease) is shown again if it is still
// newer than current.
//
//wails:ignore
func NewUpdatesService(st *store.Store, current string, c update.HTTPDoer, auto bool) *UpdatesService {
	u := &UpdatesService{st: st, current: current, http: c, now: time.Now, after: time.After,
		started: time.Now(), auto: auto, view: UpdateView{Current: current, CanInstall: canInstallHere()},
		TempDir: filepath.Join(os.TempDir(), updateTempDir), start: startInstaller,
		dev: !update.IsVersion(current)}
	u.restoreFound()
	return u
}

// foundRelease is settingFoundRelease's JSON.
type foundRelease struct {
	Version, PageURL, InstallerURL, ChecksumURL string
}

// restoreFound shows the stored release again when it is still newer than
// the running version, and forgets it when it is not (it was installed, or
// something newer runs). A dev build leaves it alone.
func (u *UpdatesService) restoreFound() {
	if u.dev || u.st == nil {
		return
	}
	raw, ok, err := u.st.GetSetting(settingFoundRelease)
	if err != nil || !ok || raw == "" {
		return
	}
	var f foundRelease
	rel, valid := update.Release{}, false
	if json.Unmarshal([]byte(raw), &f) == nil {
		rel, valid = update.Restore(update.Release{Version: f.Version, PageURL: f.PageURL,
			InstallerURL: f.InstallerURL, ChecksumURL: f.ChecksumURL}, runtime.GOOS)
	}
	if !valid || !update.Newer(rel.Version, u.current) {
		u.storeFound(update.Release{}, false)
		return
	}
	u.rel = rel
	u.view.Latest, u.view.Available, u.view.PageURL = rel.Version, true, rel.PageURL
}

// storeFound keeps rel when it is newer, else clears the setting.
func (u *UpdatesService) storeFound(rel update.Release, newer bool) {
	val := ""
	if newer {
		b, err := json.Marshal(foundRelease{rel.Version, rel.PageURL, rel.InstallerURL, rel.ChecksumURL})
		if err != nil {
			slog.Warn("update: store the found release", "err", err)
			return
		}
		val = string(b)
	}
	if err := u.st.SetSetting(settingFoundRelease, val); err != nil {
		slog.Warn("update: store the found release", "err", err)
	}
}

// CleanTemp removes the download folder a finished update left behind
// (%TEMP%\MarstekMonitor-update with the installer). Only that fixed
// folder: TempDir with any other name is never touched. main calls it at
// start, not in -simulate* (a real copy may be downloading). If the
// installer still holds its file, the next start removes it.
//
//wails:ignore
func (u *UpdatesService) CleanTemp() {
	if u.TempDir == "" || filepath.Base(u.TempDir) != updateTempDir {
		return
	}
	if err := os.RemoveAll(u.TempDir); err != nil {
		slog.Info("update: remove the old download folder", "err", err)
	}
}

// SetAuto applies the UpdateCheck setting.
//
//wails:ignore
func (u *UpdatesService) SetAuto(on bool) {
	u.mu.Lock()
	u.auto = on
	u.mu.Unlock()
}

// State returns the current update view.
func (u *UpdatesService) State() UpdateView {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.view
}

// LatestRelease is the release the last successful check found (Task 5
// downloads its assets).
//
//wails:ignore
func (u *UpdatesService) LatestRelease() update.Release {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.rel
}

// Check asks GitHub now (the «Перевірити зараз» button). A failure is not
// an error of the call: it comes back as Err = UpdateErrCheck in the view.
// While a check is running, it returns the view as it is.
func (u *UpdatesService) Check() (UpdateView, error) {
	if u.Simulate {
		// -simulate*: never GitHub, the button says so.
		u.mu.Lock()
		defer u.mu.Unlock()
		u.view.Err = UpdateErrSimulated
		u.emitLocked()
		return u.view, nil
	}
	if u.dev {
		// A developer build has no version to compare: the row says so
		// (settings.about.update.dev) and nothing is asked.
		return u.State(), nil
	}
	return u.check(context.Background(), true, u.now(), nil), nil
}

// Run makes the automatic checks: it looks once a minute whether one is due
// (autoTick) until ctx ends.
//
//wails:ignore
func (u *UpdatesService) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-u.after(time.Minute):
		}
		u.autoTick(ctx, u.now())
	}
}

// autoTick checks if UpdateCheck is on, the first minute after start is
// over and the last automatic check (stored, so restarts count) is a day
// old. The time is stored before the request: a failure also waits a day.
func (u *UpdatesService) autoTick(ctx context.Context, now time.Time) {
	u.mu.Lock()
	on := u.auto
	u.mu.Unlock()
	if !on || u.dev || now.Before(u.started.Add(firstCheckAfter)) {
		return
	}
	if last, ok := u.lastCheck(); ok && !last.After(now) && now.Sub(last) < checkEvery {
		return
	}
	// Stored only once this check is the one running (not while a manual
	// one is in flight): without it every minute would check.
	u.check(ctx, false, now, func() bool {
		if err := u.st.SetSetting(settingLastCheck, strconv.FormatInt(now.Unix(), 10)); err != nil {
			slog.Warn("update: store the check time", "err", err)
			return false
		}
		return true
	})
}

func (u *UpdatesService) lastCheck() (time.Time, bool) {
	raw, ok, err := u.st.GetSetting(settingLastCheck)
	if err != nil || !ok {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// check asks GitHub; now is the time of the toast, if one is due. While a
// check (or an update, Install) runs, another returns the view as it is. start (may be nil) runs
// once this check is the one running, under u.mu; false calls it off.
func (u *UpdatesService) check(ctx context.Context, manual bool, now time.Time, start func() bool) UpdateView {
	u.mu.Lock()
	if u.view.Checking || u.view.Phase != "" || start != nil && !start() {
		v := u.view
		u.mu.Unlock()
		return v
	}
	u.view.Checking = true
	if manual {
		u.view.Err = ""
	}
	u.emitLocked()
	u.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	rel, newer, err := update.Latest(ctx, u.http, u.current, runtime.GOOS)

	u.mu.Lock()
	u.view.Checking = false
	if err != nil {
		slog.Info("update: check failed", "manual", manual, "err", err)
		if manual {
			u.view.Err = UpdateErrCheck
		}
		v := u.view
		u.emitLocked()
		u.mu.Unlock()
		return v
	}
	u.rel = rel
	u.view.Latest, u.view.Available, u.view.PageURL, u.view.Err = rel.Version, newer, rel.PageURL, ""
	v := u.view
	u.emitLocked()
	u.mu.Unlock()
	u.storeFound(rel, newer)
	if newer {
		u.tell(rel.Version, manual, now)
	}
	return v
}

// emitLocked sends the view. u.mu is held so the events keep their order;
// invariant: Emit (main: a.Event.Emit, which only queues the event) must
// never call back into the service, or it deadlocks.
func (u *UpdatesService) emitLocked() {
	if u.Emit != nil {
		u.Emit(u.view)
	}
}

// tell shows the toast for version once: not if it (or a newer one) was
// already told. A manual check needs no toast (the user sees the row) but
// counts as told.
func (u *UpdatesService) tell(version string, manual bool, now time.Time) {
	told, ok, err := u.st.GetSetting(settingNotifiedVersion)
	if err != nil {
		slog.Warn("update: notified version", "err", err)
		return
	}
	if ok && (told == version || update.Newer(told, version)) {
		return
	}
	if !manual && u.Deliver != nil {
		var s notify.Settings
		var dnd *time.Time
		if u.NotifyState != nil {
			s, dnd = u.NotifyState()
		}
		if err := u.Deliver(notify.UpdateToast(version, s, dnd, now.Local())); err != nil {
			slog.Warn("update: toast", "err", err)
			return // told next time
		}
	}
	if err := u.st.SetSetting(settingNotifiedVersion, version); err != nil {
		slog.Warn("update: notified version", "err", err)
	}
}
