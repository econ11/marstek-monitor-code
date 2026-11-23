package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/econ11/marstek-monitor-code/core/update"
)

// fakeAssets serves a release's installer and checksum file (httptest
// only: the doer points every request at it).
type fakeAssets struct {
	mu        sync.Mutex
	installer []byte
	checksum  string // the .sha256 file's body
	short     bool   // send half the installer, then close (Content-Length says all)
	base      *url.URL
	client    *http.Client
	paths     []string
	agents    []string
}

func newFakeAssets(t *testing.T, version string, installer []byte) *fakeAssets {
	t.Helper()
	sum := sha256.Sum256(installer)
	name := update.AssetName(version, runtime.GOOS)
	a := &fakeAssets{installer: installer, checksum: hex.EncodeToString(sum[:]) + "  " + name + "\n"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.paths = append(a.paths, r.URL.Path)
		a.agents = append(a.agents, r.UserAgent())
		body, sumBody, short := a.installer, a.checksum, a.short
		a.mu.Unlock()
		switch path.Base(r.URL.Path) {
		case name + ".sha256":
			fmt.Fprint(w, sumBody)
		case name: // the platform's asset (update.AssetName): the .exe or the .dmg
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			if short {
				w.Write(body[:len(body)/2])
				return // fewer bytes than announced: the connection is cut
			}
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	a.base, _ = url.Parse(srv.URL)
	a.client = srv.Client()
	return a
}

func (a *fakeAssets) Do(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.Scheme, u.Host = a.base.Scheme, a.base.Host
	r2 := r.Clone(r.Context())
	r2.URL, r2.Host = &u, a.base.Host
	return a.client.Do(r2)
}

// fakeStarter records the installer runs instead of starting one.
type fakeStarter struct {
	mu    sync.Mutex
	paths []string
	err   error
}

func (f *fakeStarter) start(p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, p)
	return f.err
}

func (f *fakeStarter) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

type installHarness struct {
	*updatesHarness
	st      *store.Store
	assets  *fakeAssets
	starter *fakeStarter
	tmp     string
	backups string
	quits   int
}

const (
	oldVersion = "1.0.0"
	newVersion = "1.1.0"
)

func newInstall(t *testing.T) *installHarness {
	t.Helper()
	st := openTestStore(t)
	for i := 0; i < 25; i++ {
		s := model.Sample{StationID: "st1", At: time.Date(2026, 11, 20, 0, 0, i, 0, time.UTC), Source: model.SourceLAN, Grid: model.GridPresent}
		if err := st.InsertSample(s); err != nil {
			t.Fatal(err)
		}
	}
	installer := make([]byte, 300<<10) // 300 KiB: several progress steps
	for i := range installer {
		installer[i] = byte(i * 7)
	}
	assets := newFakeAssets(t, newVersion, installer)
	h := &installHarness{st: st, assets: assets, starter: &fakeStarter{}}
	h.updatesHarness = newUpdates(t, st, nil, oldVersion, false, updT0)
	h.u.http = assets
	root := t.TempDir()
	h.tmp, h.backups = filepath.Join(root, "MarstekMonitor-update"), filepath.Join(root, "backups")
	h.u.TempDir, h.u.BackupDir = h.tmp, h.backups
	h.u.start = h.starter.start
	h.u.Quit = func() { h.quits++ }
	// The fake starter installs anywhere; the real CanInstall of a test
	// binary is false on macOS (no bundle, no Team ID).
	h.u.view.CanInstall = true
	h.u.installable = func() bool { return true }
	// What the last check found: the new release with both assets.
	h.u.rel = update.Release{Version: newVersion, PageURL: "https://github.com/econ11/marstek-monitor/releases/tag/v" + newVersion,
		InstallerURL: "https://github.com/econ11/marstek-monitor/releases/download/v" + newVersion + "/" + update.AssetName(newVersion, runtime.GOOS),
		ChecksumURL:  "https://github.com/econ11/marstek-monitor/releases/download/v" + newVersion + "/" + update.AssetName(newVersion, runtime.GOOS) + ".sha256"}
	h.u.view.Latest, h.u.view.Available, h.u.view.PageURL = newVersion, true, h.u.rel.PageURL
	return h
}

func (h *installHarness) noBackup(t *testing.T) {
	t.Helper()
	if ents, _ := os.ReadDir(h.backups); len(ents) != 0 {
		t.Errorf("backup made: %v", ents)
	}
}

// failedWith checks a failed Install: the code in Err, no phase, the app not
// quitting, «Завантажити» still possible (Available and PageURL kept).
func (h *installHarness) failedWith(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("Install succeeded, want a failure")
	}
	v := h.u.State()
	if v.Err != code || v.Phase != "" || v.Progress != 0 || !v.Available || v.PageURL == "" {
		t.Errorf("view after failure = %+v, want Err %q and the release kept", v, code)
	}
	if h.quits != 0 {
		t.Errorf("the app quit after a failure")
	}
	if last := h.views[len(h.views)-1]; last.Err != code {
		t.Errorf("last event %+v, want Err %q", last, code)
	}
}

// Review Focus 2: a checksum mismatch makes no backup, never starts the
// installer, and deletes the file.
func TestUpdateAbortsOnBadChecksum(t *testing.T) {
	h := newInstall(t)
	h.assets.mu.Lock()
	h.assets.checksum = "0000000000000000000000000000000000000000000000000000000000000000  " + update.AssetName(newVersion, runtime.GOOS) + "\n"
	h.assets.mu.Unlock()
	err := h.u.Install()
	h.failedWith(t, err, UpdateErrChecksum)
	h.noBackup(t)
	if c := h.starter.calls(); len(c) != 0 {
		t.Fatalf("installer started: %v", c)
	}
	if _, err := os.Stat(filepath.Join(h.tmp, update.AssetName(newVersion, runtime.GOOS))); !os.IsNotExist(err) {
		t.Errorf("the mismatching file was not deleted: %v", err)
	}
	// A checksum file without our installer's line is a mismatch too.
	h.assets.mu.Lock()
	h.assets.checksum = "abc  other.exe\n"
	h.assets.mu.Unlock()
	h.failedWith(t, h.u.Install(), UpdateErrChecksum)
	h.noBackup(t)
	if c := h.starter.calls(); len(c) != 0 {
		t.Fatalf("installer started: %v", c)
	}
}

// Review Focus 2: a download cut short is a download error: no backup, no
// installer.
func TestUpdateAbortsOnShortDownload(t *testing.T) {
	h := newInstall(t)
	h.assets.mu.Lock()
	h.assets.short = true
	h.assets.mu.Unlock()
	h.failedWith(t, h.u.Install(), UpdateErrDownload)
	h.noBackup(t)
	if c := h.starter.calls(); len(c) != 0 {
		t.Fatalf("installer started: %v", c)
	}
	// No assets known (an old check, a release without them): download error.
	h2 := newInstall(t)
	h2.u.rel.ChecksumURL = ""
	h2.failedWith(t, h2.u.Install(), UpdateErrDownload)
	h2.noBackup(t)
	if c := h2.starter.calls(); len(c) != 0 {
		t.Fatalf("installer started: %v", c)
	}
}

func TestUpdateHappyPath(t *testing.T) {
	h := newInstall(t)
	// A leftover of an earlier run is cleaned first.
	if err := os.MkdirAll(h.tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.tmp, "stale.exe"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.u.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Phases in order, progress rising to 1 while downloading.
	var phases []string
	last := -1.0
	for _, v := range h.views {
		if len(phases) == 0 || phases[len(phases)-1] != v.Phase {
			phases = append(phases, v.Phase)
		}
		if v.Phase == PhaseDownloading {
			if v.Progress < last {
				t.Errorf("progress went back: %v after %v", v.Progress, last)
			}
			last = v.Progress
		}
	}
	want := []string{PhaseDownloading, PhaseVerifying, PhaseBackingUp, PhaseStarting}
	if fmt.Sprint(phases) != fmt.Sprint(want) {
		t.Fatalf("phases %v, want %v", phases, want)
	}
	if last != 1 {
		t.Errorf("download progress ended at %v, want 1", last)
	}
	// The installer started with the downloaded, verified file; the app quits.
	got := h.starter.calls()
	wantPath := filepath.Join(h.tmp, update.AssetName(newVersion, runtime.GOOS))
	if len(got) != 1 || got[0] != wantPath {
		t.Fatalf("starter calls %v, want [%s]", got, wantPath)
	}
	if b, err := os.ReadFile(wantPath); err != nil || len(b) != len(h.assets.installer) {
		t.Fatalf("downloaded file: %d bytes, %v", len(b), err)
	}
	if _, err := os.Stat(filepath.Join(h.tmp, "stale.exe")); !os.IsNotExist(err) {
		t.Errorf("download folder not cleaned: %v", err)
	}
	if h.quits != 1 {
		t.Errorf("quit %d times, want 1", h.quits)
	}
	// The backup is monitor-<old>.db, a valid DB with the same rows.
	bp := filepath.Join(h.backups, "monitor-"+oldVersion+".db")
	db, err := sql.Open("sqlite", bp)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n); err != nil || n != 25 {
		t.Fatalf("backup samples = %d, %v; want 25", n, err)
	}
	// Requests: the checksum, the installer, our User-Agent only.
	h.assets.mu.Lock()
	defer h.assets.mu.Unlock()
	if len(h.assets.paths) != 2 {
		t.Errorf("requests %v", h.assets.paths)
	}
	for _, ua := range h.assets.agents {
		if ua != "MarstekMonitor/"+oldVersion {
			t.Errorf("User-Agent %q", ua)
		}
	}
}

func TestUpdateStartFailureKeepsRunning(t *testing.T) {
	h := newInstall(t)
	h.starter.err = errors.New("blocked")
	h.failedWith(t, h.u.Install(), UpdateErrStart)
	if c := h.starter.calls(); len(c) != 1 {
		t.Fatalf("starter calls %v", c)
	}
}

func TestUpdateBackupFailureStartsNothing(t *testing.T) {
	h := newInstall(t)
	// The backups "folder" is a file: the backup cannot be written.
	if err := os.WriteFile(h.backups, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.failedWith(t, h.u.Install(), UpdateErrBackup)
	if c := h.starter.calls(); len(c) != 0 {
		t.Fatalf("installer started: %v", c)
	}
}

func TestUpdateBackupsKeep3(t *testing.T) {
	h := newInstall(t)
	if err := os.MkdirAll(h.backups, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, v := range []string{"0.7.0", "0.8.0", "0.9.0"} {
		p := filepath.Join(h.backups, "monitor-"+v+".db")
		os.WriteFile(p, []byte("x"), 0o644)
		mt := updT0.Add(-time.Duration(10-i) * time.Hour)
		os.Chtimes(p, mt, mt)
	}
	if err := h.u.Install(); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(h.backups)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if fmt.Sprint(names) != fmt.Sprint([]string{"monitor-0.8.0.db", "monitor-0.9.0.db", "monitor-1.0.0.db"}) {
		t.Fatalf("backups %v", names)
	}
}

// In -simulate* Install never downloads or starts anything.
func TestInstallOffInSimulation(t *testing.T) {
	h := newInstall(t)
	h.u.Simulate = true
	err := h.u.Install()
	if err == nil || h.u.State().Err != UpdateErrSimulated {
		t.Fatalf("simulated Install = %v, view %+v", err, h.u.State())
	}
	h.assets.mu.Lock()
	n := len(h.assets.paths)
	h.assets.mu.Unlock()
	if n != 0 || len(h.starter.calls()) != 0 || h.quits != 0 {
		t.Fatalf("simulated Install: %d requests, %v starts, %d quits", n, h.starter.calls(), h.quits)
	}
}

// While an update runs, a second Install and a Check do nothing.
func TestInstallWhileBusy(t *testing.T) {
	h := newInstall(t)
	h.u.mu.Lock()
	h.u.view.Phase = PhaseDownloading
	h.u.mu.Unlock()
	if err := h.u.Install(); err == nil {
		t.Fatal("second Install ran")
	}
	if v, _ := h.u.Check(); v.Checking || v.Phase != PhaseDownloading {
		t.Fatalf("Check during an update: %+v", v)
	}
	if len(h.starter.calls()) != 0 {
		t.Fatal("installer started")
	}
}

func TestParseChecksum(t *testing.T) {
	name := update.AssetName("1.1.0", "windows")
	h := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{h + "  " + name + "\n", true},
		{h + " *" + name + "\r\n", true}, // binary-mode marker of sha256sum
		{"abc  x.exe\n" + h + "  " + name, true},
		{h + "  other.exe\n", false},
		{"nothex  " + name + "\n", false},
		{h[:60] + "  " + name + "\n", false},
		{"", false},
	} {
		got, err := parseChecksum([]byte(tc.body), name)
		if tc.ok != (err == nil) || tc.ok && hex.EncodeToString(got) != h {
			t.Errorf("parseChecksum(%q) = %x, %v", tc.body, got, err)
		}
	}
}

// Task 5 fix: the installer start leaves a marker; the next start of a
// newer version takes it (once) and calls its restart gap an app update.
func TestUpdateRestartMarker(t *testing.T) {
	h := newInstall(t)
	h.u.now = func() time.Time { return updT0 }
	if err := h.u.Install(); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := h.st.GetSetting(settingUpdateRestart); !ok || v == "" {
		t.Fatalf("no marker after the installer started: %q", v)
	}
	// The same (old) version starting again: the update did not happen.
	if TakeUpdateRestart(h.st, oldVersion, updT0.Add(5*time.Minute)) {
		t.Fatal("old version took the marker as an update")
	}
	if v, _, _ := h.st.GetSetting(settingUpdateRestart); v != "" {
		t.Fatalf("marker not cleared: %q", v)
	}
	// The new version, minutes later: an update, once.
	h.st.SetSetting(settingUpdateRestart, oldVersion+"|"+strconv.FormatInt(updT0.Unix(), 10))
	if !TakeUpdateRestart(h.st, newVersion, updT0.Add(5*time.Minute)) {
		t.Fatal("new version did not take the marker")
	}
	if TakeUpdateRestart(h.st, newVersion, updT0.Add(6*time.Minute)) {
		t.Fatal("marker taken twice")
	}
	// Too old (a day later): not this restart.
	h.st.SetSetting(settingUpdateRestart, oldVersion+"|"+strconv.FormatInt(updT0.Unix(), 10))
	if TakeUpdateRestart(h.st, newVersion, updT0.Add(25*time.Hour)) {
		t.Fatal("a day-old marker counted")
	}
	// A failed start leaves no marker.
	h2 := newInstall(t)
	h2.starter.err = errors.New("blocked")
	h2.u.Install()
	if v, _, _ := h2.st.GetSetting(settingUpdateRestart); v != "" {
		t.Fatalf("marker after a failed start: %q", v)
	}
	// Garbage is ignored.
	h.st.SetSetting(settingUpdateRestart, "nonsense")
	if TakeUpdateRestart(h.st, newVersion, updT0) {
		t.Fatal("garbage counted")
	}
}

// Install refuses on a copy that cannot update itself (CanInstall false):
// nothing is downloaded, backed up or started.
func TestUpdateRefusedWhenCannotInstall(t *testing.T) {
	h := newInstall(t)
	h.u.view.CanInstall = false
	if err := h.u.Install(); !errors.Is(err, ErrUpdateCannotInstall) {
		t.Fatalf("Install = %v, want ErrUpdateCannotInstall", err)
	}
	if len(h.starter.paths) != 0 || h.quits != 0 {
		t.Fatalf("started %v, quit %d; want nothing", h.starter.paths, h.quits)
	}
	if _, err := os.Stat(h.tmp); !os.IsNotExist(err) {
		t.Fatalf("download folder made: %v", err)
	}
}

// The local update test (docs/macos-signing.md): under -simulate*, Check and
// Install run only in a build without the release tag pointed at a loopback
// MARSTEK_UPDATE_BASE. A release build, or no override, stays simulated.
func TestUpdateLocalTestUnderSimulate(t *testing.T) {
	for name, tc := range map[string]struct {
		base    string
		allowed bool // in a build without the release tag
	}{
		"loopback override": {"http://127.0.0.1:8000/", true},
		"no override":       {"", false},
		"https override":    {"https://example.com/releases/", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("MARSTEK_UPDATE_BASE", tc.base)
			want := tc.allowed && !releaseTagTest
			h := newInstall(t)
			h.u.Simulate = true
			if got := !h.u.simulated(); got != want {
				t.Fatalf("release=%v: update allowed %v, want %v", releaseTagTest, got, want)
			}
			v, err := h.u.Check()
			if err != nil {
				t.Fatal(err)
			}
			if gotSim := v.Err == UpdateErrSimulated; gotSim == want {
				t.Fatalf("Check Err %q (allowed=%v)", v.Err, want)
			}
			err = h.u.Install()
			if want {
				// The whole install runs (download, checksum, backup, start).
				if err != nil || len(h.starter.paths) != 1 || h.quits != 1 {
					t.Fatalf("Install = %v, started %v, quits %d; want one start and a quit", err, h.starter.paths, h.quits)
				}
			} else {
				if err == nil || h.u.State().Err != UpdateErrSimulated || len(h.starter.paths) != 0 {
					t.Fatalf("Install = %v, Err %q, started %v; want refused as simulated", err, h.u.State().Err, h.starter.paths)
				}
			}
		})
	}
	// A release build ignores the environment even when the function would
	// say yes (the var stands for update.LocalTestBase).
	old := localUpdateTest
	localUpdateTest = func() bool { return false }
	t.Cleanup(func() { localUpdateTest = old })
	t.Setenv("MARSTEK_UPDATE_BASE", "http://127.0.0.1:8000/")
	h := newInstall(t)
	h.u.Simulate = true
	if err := h.u.Install(); err == nil || len(h.starter.paths) != 0 {
		t.Fatalf("release build: Install = %v, started %v", err, h.starter.paths)
	}
}
