package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/econ11/marstek-monitor-code/core/update"
)

// InstallerArgs is the whole argv after the installer's path for the
// silent one-click update: /S = silent (per-user, no UAC), /relaunch =
// start the new version once installed. Task 6 (the NSIS template) defines
// these flags; adjust them here only. The running exe goes in PrevExeEnv
// (updater_windows.go), for the relaunch after a failed install.
var InstallerArgs = []string{"/S", "/relaunch"}

// The phases of the one-click update (UpdateView.Phase), in this order.
const (
	PhaseDownloading = "downloading"
	PhaseVerifying   = "verifying"
	PhaseBackingUp   = "backingUp"
	PhaseStarting    = "starting"
)

// Why the one-click update failed (UpdateView.Err). Each failure leaves the
// running version as it is; «Завантажити» (the release page) stays.
const (
	UpdateErrDownload = "download" // no assets, a network error, a cut-off or oversized download
	UpdateErrChecksum = "checksum" // no line for the installer, or the SHA-256 differs
	UpdateErrBackup   = "backup"   // the DB backup could not be written
	UpdateErrStart    = "start"    // the installer did not start
)

const (
	updateTempDir   = "MarstekMonitor-update" // under %TEMP%
	keepBackups     = 3
	maxInstaller    = 512 << 20 // an installer is ~10 MB; anything near this is wrong
	maxChecksumFile = 64 << 10
	downloadTimeout = 30 * time.Minute
)

// ErrUpdateBusy: Install while a check or an update is running.
var ErrUpdateBusy = errors.New("update: already running")

// updateFailure is a failed step: code is the UpdateView.Err.
type updateFailure struct {
	code string
	err  error
}

func (f *updateFailure) Error() string { return "update: " + f.code + ": " + f.err.Error() }
func (f *updateFailure) Unwrap() error { return f.err }

func failed(code string, err error) error { return &updateFailure{code, err} }

// Install is «Оновити»: it downloads the installer of the release the last
// check found, verifies its SHA-256 against the published checksum file,
// backs up the database to BackupDir\monitor-<running version>.db (keeping
// the last 3), starts the installer silently and quits the app. The order
// is fixed; a failure before the installer starts leaves everything as it
// is and comes back as Err (UpdateErrDownload…) with Available and PageURL
// kept. Progress comes as updates:changed. In -simulate* nothing runs.
func (u *UpdatesService) Install() error {
	u.mu.Lock()
	if u.Simulate {
		u.view.Err = UpdateErrSimulated
		u.emitLocked()
		u.mu.Unlock()
		return errors.New("update: off in a simulated run")
	}
	if u.dev {
		u.mu.Unlock()
		return errors.New("update: off in a developer build")
	}
	if u.view.Checking || u.view.Phase != "" {
		u.mu.Unlock()
		return ErrUpdateBusy
	}
	rel, avail := u.rel, u.view.Available
	u.view.Phase, u.view.Progress, u.view.Err = PhaseDownloading, 0, ""
	u.emitLocked()
	u.mu.Unlock()

	if err := u.install(rel, avail); err != nil {
		code := UpdateErrDownload
		var f *updateFailure
		if errors.As(err, &f) {
			code = f.code
		}
		slog.Warn("update: install failed", "version", rel.Version, "err", err)
		u.mu.Lock()
		u.view.Phase, u.view.Progress, u.view.Err = "", 0, code
		u.emitLocked()
		u.mu.Unlock()
		return err
	}
	slog.Info("update: installer started, quitting", "version", rel.Version)
	if u.Quit != nil {
		u.Quit()
	}
	return nil
}

func (u *UpdatesService) install(rel update.Release, avail bool) (err error) {
	if !avail || rel.InstallerURL == "" || rel.ChecksumURL == "" {
		return failed(UpdateErrDownload, errors.New("the release has no installer or checksum"))
	}
	name := update.AssetName(rel.Version, runtime.GOOS)
	if filepath.Base(name) != name || u.TempDir == "" {
		return failed(UpdateErrDownload, fmt.Errorf("bad installer name %q or folder", name))
	}
	dir := u.TempDir
	if err := os.RemoveAll(dir); err != nil {
		return failed(UpdateErrDownload, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return failed(UpdateErrDownload, err)
	}
	// Until the installer runs, nothing downloaded is kept.
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	sums, err := u.fetch(ctx, rel.ChecksumURL, maxChecksumFile)
	if err != nil {
		return failed(UpdateErrDownload, fmt.Errorf("checksum file: %w", err))
	}
	want, err := parseChecksum(sums, name)
	if err != nil {
		return failed(UpdateErrChecksum, err)
	}
	path := filepath.Join(dir, name)
	if err := u.download(ctx, rel.InstallerURL, path); err != nil {
		return failed(UpdateErrDownload, err)
	}

	u.setPhase(PhaseVerifying)
	got, err := fileSHA256(path)
	if err != nil {
		return failed(UpdateErrChecksum, err)
	}
	if !bytes.Equal(got, want) {
		os.Remove(path)
		return failed(UpdateErrChecksum, fmt.Errorf("sha256 %x, published %x", got, want))
	}

	u.setPhase(PhaseBackingUp)
	if u.BackupDir == "" {
		return failed(UpdateErrBackup, errors.New("no backup folder"))
	}
	if err := u.st.BackupTo(filepath.Join(u.BackupDir, "monitor-"+fileSafe(u.current)+".db")); err != nil {
		return failed(UpdateErrBackup, err)
	}
	if err := store.PruneBackups(u.BackupDir, keepBackups); err != nil {
		slog.Warn("update: prune backups", "err", err) // the new backup is there; not a reason to stop
	}

	u.setPhase(PhaseStarting)
	// The next start's gap is this update (TakeUpdateRestart). Written
	// before the installer runs: it closes the app at once.
	marker := u.current + "|" + strconv.FormatInt(u.now().Unix(), 10)
	if err := u.st.SetSetting(settingUpdateRestart, marker); err != nil {
		slog.Warn("update: restart marker", "err", err) // only the gap's label is lost
	}
	if err := u.start(path); err != nil {
		if err := u.st.SetSetting(settingUpdateRestart, ""); err != nil {
			slog.Warn("update: restart marker", "err", err)
		}
		return failed(UpdateErrStart, err)
	}
	return nil
}

// settingUpdateRestart: "<old version>|<unix seconds>", written right before
// the installer starts; internal, in the settings table.
const settingUpdateRestart = "update.restart"

// updateRestartWithin: a marker older than this is not the current restart.
const updateRestartWithin = 24 * time.Hour

// TakeUpdateRestart reports whether this start follows the one-click update:
// the marker is there, from another version than current (the installer did
// replace the app), and recent. The marker is cleared either way, so it
// counts once (monitor.Deps.AppUpdated).
//
//wails:ignore
func TakeUpdateRestart(st *store.Store, current string, now time.Time) bool {
	raw, ok, err := st.GetSetting(settingUpdateRestart)
	if err != nil || !ok || raw == "" {
		return false
	}
	if err := st.SetSetting(settingUpdateRestart, ""); err != nil {
		slog.Warn("update: clear the restart marker", "err", err)
	}
	old, at, ok := strings.Cut(raw, "|")
	sec, err := strconv.ParseInt(at, 10, 64)
	if !ok || err != nil || old == current {
		return false
	}
	age := now.Sub(time.Unix(sec, 0))
	return age >= 0 && age <= updateRestartWithin
}

// setPhase moves the running update to phase p (the download is done).
func (u *UpdatesService) setPhase(p string) {
	u.mu.Lock()
	u.view.Phase, u.view.Progress = p, 1
	u.emitLocked()
	u.mu.Unlock()
}

func (u *UpdatesService) setProgress(p float64) {
	u.mu.Lock()
	u.view.Progress = p
	u.emitLocked()
	u.mu.Unlock()
}

func (u *UpdatesService) doer() update.HTTPDoer {
	if u.Download != nil {
		return u.Download
	}
	return u.http
}

// get sends a GET with our User-Agent only; any status but 200 is an error.
func (u *UpdatesService) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MarstekMonitor/"+u.current)
	resp, err := u.doer().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s", resp.Status)
	}
	return resp, nil
}

// fetch reads a small asset (at most max bytes).
func (u *UpdatesService) fetch(ctx context.Context, url string, max int64) ([]byte, error) {
	resp, err := u.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("too large")
	}
	return b, nil
}

// download writes the installer to path, reporting the progress by whole
// percents. A body shorter than its Content-Length (a cut connection) or
// larger than maxInstaller is an error.
func (u *UpdatesService) download(ctx context.Context, url, path string) error {
	resp, err := u.get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	total := resp.ContentLength
	if total > maxInstaller {
		return fmt.Errorf("installer too large (%d bytes)", total)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	pw := &progressWriter{total: total, report: u.setProgress}
	n, err := io.Copy(io.MultiWriter(f, pw), io.LimitReader(resp.Body, maxInstaller+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		return err
	case n > maxInstaller:
		return errors.New("installer too large")
	case total >= 0 && n != total:
		return fmt.Errorf("short download: %d of %d bytes", n, total)
	case n == 0:
		return errors.New("empty download")
	}
	u.setProgress(1)
	return nil
}

// progressWriter counts the bytes and reports each new whole percent.
type progressWriter struct {
	total, n int64
	last     int64
	report   func(float64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if p.total > 0 {
		if pct := p.n * 100 / p.total; pct > p.last && pct < 100 {
			p.last = pct
			p.report(float64(pct) / 100)
		}
	}
	return len(b), nil
}

// parseChecksum finds name's SHA-256 in a checksum file of lines
// "<hex>  <name>" (sha256sum; "*<name>" in binary mode also counts).
func parseChecksum(body []byte, name string) ([]byte, error) {
	for _, line := range strings.Split(string(body), "\n") {
		hash, file, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		file = strings.TrimPrefix(strings.TrimSpace(file), "*")
		if file != name {
			continue
		}
		sum, err := hex.DecodeString(hash)
		if err != nil || len(sum) != sha256.Size {
			return nil, fmt.Errorf("bad sha256 %q for %s", hash, name)
		}
		return sum, nil
	}
	return nil, fmt.Errorf("no sha256 for %s in the checksum file", name)
}

func fileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// fileSafe keeps a version usable in a file name (monitor-<version>.db).
func fileSafe(v string) string {
	if v == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-', r == '+', r == '_':
			return r
		}
		return '_'
	}, v)
}
