package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
	"github.com/econ11/marstek-monitor-code/core/update"
)

// fakeGitHub answers the latest-release request with the release of
// version (a 403 rate limit while fail is set), counts the requests and
// keeps every request off the network (the doer points at httptest).
type fakeGitHub struct {
	mu      sync.Mutex
	version string
	fail    int  // HTTP status to answer with; 0 = the release
	assets  bool // the release lists its installer and checksum
	hits    int
	base    *url.URL
	client  *http.Client
}

func newFakeGitHub(t *testing.T, version string) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{version: version}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.hits++
		if g.fail != 0 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(g.fail)
			return
		}
		assets := "[]"
		if g.assets {
			// The running platform's installer, as the app's check asks for it.
			assets = fmt.Sprintf(`[{"name":"%[2]s","browser_download_url":"https://github.com/econ11/marstek-monitor/releases/download/v%[1]s/%[2]s"},`+
				`{"name":"%[2]s.sha256","browser_download_url":"https://github.com/econ11/marstek-monitor/releases/download/v%[1]s/%[2]s.sha256"}]`,
				g.version, update.AssetName(g.version, runtime.GOOS))
		}
		fmt.Fprintf(w, `{"tag_name":"v%[1]s","html_url":"https://github.com/econ11/marstek-monitor/releases/tag/v%[1]s","assets":%[2]s}`, g.version, assets)
	}))
	t.Cleanup(srv.Close)
	g.base, _ = url.Parse(srv.URL)
	g.client = srv.Client()
	return g
}

func (g *fakeGitHub) Do(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.Scheme, u.Host = g.base.Scheme, g.base.Host
	r2 := r.Clone(r.Context())
	r2.URL, r2.Host = &u, g.base.Host
	return g.client.Do(r2)
}

func (g *fakeGitHub) set(version string, fail int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.version, g.fail = version, fail
}

func (g *fakeGitHub) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hits
}

// updatesHarness is an UpdatesService on a fake clock, recording toasts
// and emitted views.
type updatesHarness struct {
	u      *UpdatesService
	mu     sync.Mutex
	toasts []notify.Toast
	views  []UpdateView
}

func newUpdates(t *testing.T, st *store.Store, g *fakeGitHub, current string, auto bool, start time.Time) *updatesHarness {
	t.Helper()
	h := &updatesHarness{}
	h.u = NewUpdatesService(st, current, g, auto)
	h.u.started = start
	h.u.Deliver = func(n notify.Toast) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.toasts = append(h.toasts, n)
		return nil
	}
	h.u.Emit = func(v UpdateView) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.views = append(h.views, v)
	}
	return h
}

func (h *updatesHarness) toastCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.toasts)
}

var updT0 = time.Date(2026, 11, 23, 12, 0, 0, 0, time.Local)

// Review Focus 1: offline, rate-limited or a 5xx during an automatic check
// shows nothing (retried next day); the manual button says it failed.
func TestCheckFailsQuietly(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusBadGateway} {
		st := openTestStore(t)
		g := newFakeGitHub(t, "1.2.0")
		g.set("1.2.0", status)
		h := newUpdates(t, st, g, "1.0.0", true, updT0)

		h.u.autoTick(context.Background(), updT0.Add(2*time.Minute))
		if g.count() != 1 {
			t.Fatalf("%d: automatic check made %d requests, want 1", status, g.count())
		}
		v := h.u.State()
		if v.Err != "" || v.Available || v.Checking || h.toastCount() != 0 {
			t.Fatalf("%d: automatic failure shows %+v, %d toasts", status, v, h.toastCount())
		}
		for _, e := range h.views {
			if e.Err != "" {
				t.Fatalf("%d: automatic failure emitted %+v", status, e)
			}
		}

		v, err := h.u.Check()
		if err != nil || v.Err != UpdateErrCheck || v.Checking || v.Available {
			t.Fatalf("%d: manual failure = %+v, %v; want Err %q", status, v, err, UpdateErrCheck)
		}
		if h.u.State().Err != UpdateErrCheck || h.views[len(h.views)-1].Err != UpdateErrCheck {
			t.Fatalf("%d: the failure is not in the state / event", status)
		}
		// A later success clears it.
		g.set("1.2.0", 0)
		if v, _ := h.u.Check(); v.Err != "" || !v.Available || v.Latest != "1.2.0" {
			t.Fatalf("%d: after success %+v", status, v)
		}
	}
	// Offline: the doer itself fails.
	st := openTestStore(t)
	h := newUpdates(t, st, nil, "1.0.0", true, updT0)
	h.u.http = doerFail{}
	h.u.autoTick(context.Background(), updT0.Add(2*time.Minute))
	if v := h.u.State(); v.Err != "" || h.toastCount() != 0 {
		t.Fatalf("offline automatic: %+v", v)
	}
	if v, err := h.u.Check(); err != nil || v.Err != UpdateErrCheck {
		t.Fatalf("offline manual: %+v, %v", v, err)
	}
}

type doerFail struct{}

func (doerFail) Do(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("offline") }

func TestDailySchedule(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.0.0")
	h := newUpdates(t, st, g, "1.0.0", true, updT0)
	ctx := context.Background()
	steps := []struct {
		at   time.Duration
		hits int
	}{
		{0, 0},
		{30 * time.Second, 0},
		{59 * time.Second, 0},
		{time.Minute, 1}, // the first check ~1 min after start
		{2 * time.Minute, 1},
		{2 * time.Hour, 1},
		{23 * time.Hour, 1},
		{24*time.Hour + 59*time.Second, 1},
		{24*time.Hour + time.Minute, 2}, // a day after the last one
		{30 * time.Hour, 2},
	}
	for _, s := range steps {
		h.u.autoTick(ctx, updT0.Add(s.at))
		if g.count() != s.hits {
			t.Fatalf("at +%v: %d checks, want %d", s.at, g.count(), s.hits)
		}
	}
	// A restart within the day does not check again: the last check is stored.
	start2 := updT0.Add(31 * time.Hour)
	h2 := newUpdates(t, st, g, "1.0.0", true, start2)
	h2.u.autoTick(ctx, start2.Add(2*time.Minute))
	if g.count() != 2 {
		t.Fatalf("restart within a day checked again: %d", g.count())
	}
	h2.u.autoTick(ctx, updT0.Add(48*time.Hour+time.Minute))
	if g.count() != 3 {
		t.Fatalf("restart: the next day's check did not come: %d", g.count())
	}
	// A failed check also waits a day.
	g.set("1.0.0", http.StatusBadGateway)
	h2.u.autoTick(ctx, updT0.Add(72*time.Hour+2*time.Minute))
	h2.u.autoTick(ctx, updT0.Add(73*time.Hour))
	if g.count() != 4 {
		t.Fatalf("a failure was retried within the day: %d", g.count())
	}

	// UpdateCheck off: none at all, until it is turned on.
	st3 := openTestStore(t)
	g3 := newFakeGitHub(t, "1.0.0")
	h3 := newUpdates(t, st3, g3, "1.0.0", false, updT0)
	for _, s := range steps {
		h3.u.autoTick(ctx, updT0.Add(s.at))
	}
	if g3.count() != 0 {
		t.Fatalf("UpdateCheck off: %d checks", g3.count())
	}
	// The manual button works with it off.
	if _, err := h3.u.Check(); err != nil || g3.count() != 1 {
		t.Fatalf("manual with UpdateCheck off: %v, %d requests", err, g3.count())
	}
	h3.u.SetAuto(true)
	h3.u.autoTick(ctx, updT0.Add(31*time.Hour))
	if g3.count() != 2 {
		t.Fatalf("turned on: %d", g3.count())
	}
}

// Run ticks once a minute on its clock and stops with the context.
func TestUpdatesRunTicks(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.0.0")
	h := newUpdates(t, st, g, "1.0.0", true, updT0)
	ticks := make(chan time.Time)
	var mu sync.Mutex
	now := updT0
	h.u.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	h.u.after = func(d time.Duration) <-chan time.Time {
		if d != time.Minute {
			t.Errorf("Run waits %v, want 1 min", d)
		}
		return ticks
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.u.Run(ctx); close(done) }()
	mu.Lock()
	now = updT0.Add(time.Minute)
	mu.Unlock()
	ticks <- now
	ticks <- now // the second tick waits for the first check to end
	if g.count() != 1 {
		t.Fatalf("%d checks after the first minute", g.count())
	}
	cancel()
	<-done
}

func TestToastOncePerVersion(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.2.0")
	h := newUpdates(t, st, g, "1.0.0", true, updT0)
	day := 24 * time.Hour
	at := func(n int) time.Time { return updT0.Add(time.Minute + time.Duration(n)*day) }

	h.u.autoTick(context.Background(), at(0))
	h.u.autoTick(context.Background(), at(1))
	if h.toastCount() != 1 {
		t.Fatalf("1.2.0 twice: %d toasts, want 1", h.toastCount())
	}
	tt := h.toasts[0]
	if tt.Kind != notify.Update || tt.Args["version"] != "1.2.0" {
		t.Fatalf("toast %+v", tt)
	}
	if v, _, _ := st.GetSetting(settingNotifiedVersion); v != "1.2.0" {
		t.Fatalf("stored notified version %q", v)
	}
	// A restart does not toast the same version again.
	h2 := newUpdates(t, st, g, "1.0.0", true, at(2))
	h2.u.autoTick(context.Background(), at(2).Add(time.Minute))
	if h2.toastCount() != 0 {
		t.Fatal("restart toasted 1.2.0 again")
	}
	// A later version: another toast.
	g.set("1.3.0", 0)
	h2.u.autoTick(context.Background(), at(3).Add(time.Minute))
	if h2.toastCount() != 1 || h2.toasts[0].Args["version"] != "1.3.0" {
		t.Fatalf("1.3.0: %+v", h2.toasts)
	}
	// The manual button shows the row, no toast, and the version counts as told.
	g.set("1.4.0", 0)
	if v, _ := h2.u.Check(); !v.Available || v.Latest != "1.4.0" {
		t.Fatalf("manual %+v", v)
	}
	h2.u.autoTick(context.Background(), at(4).Add(time.Minute))
	if h2.toastCount() != 1 {
		t.Fatalf("manual / automatic of 1.4.0 toasted: %+v", h2.toasts)
	}
	// Up to date or older: no toast.
	h3 := newUpdates(t, openTestStore(t), g, "1.4.0", true, updT0)
	h3.u.autoTick(context.Background(), at(0))
	if h3.toastCount() != 0 || h3.u.State().Available {
		t.Fatalf("up to date: %+v", h3.u.State())
	}
}

// Quiet hours and DND (from NotifyState) make the update toast silent.
func TestUpdateToastRespectsQuiet(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.2.0")
	h := newUpdates(t, st, g, "1.0.0", true, updT0)
	until := updT0.Add(time.Hour)
	h.u.NotifyState = func() (notify.Settings, *time.Time) { return notify.Settings{}, &until }
	h.u.autoTick(context.Background(), updT0.Add(time.Minute))
	if h.toastCount() != 1 || !h.toasts[0].Silent {
		t.Fatalf("toasts %+v, want one silent", h.toasts)
	}
}

func TestUpdatesState(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.2.0")
	h := newUpdates(t, st, g, "1.1.0", false, updT0)
	if v := h.u.State(); v != (UpdateView{Current: "1.1.0"}) {
		t.Fatalf("before a check: %+v", v)
	}
	v, err := h.u.Check()
	want := UpdateView{Current: "1.1.0", Latest: "1.2.0", Available: true, PageURL: "https://github.com/econ11/marstek-monitor/releases/tag/v1.2.0"}
	if err != nil || v != want || h.u.State() != want {
		t.Fatalf("after: %+v, %v; want %+v", v, err, want)
	}
	if len(h.views) < 2 || !h.views[0].Checking || h.views[len(h.views)-1] != want {
		t.Fatalf("events %+v: want Checking first, then the result", h.views)
	}
	if r := h.u.LatestRelease(); r.Version != "1.2.0" {
		t.Fatalf("LatestRelease %+v", r)
	}
}

// Final review M2: a dev build ("dev" or unstamped) never asks GitHub —
// neither the button nor the daily check — and never installs.
func TestDevBuildNeverChecks(t *testing.T) {
	for _, cur := range []string{"dev", ""} {
		g := newFakeGitHub(t, "1.2.0")
		st := openTestStore(t)
		h := newUpdates(t, st, g, cur, true, updT0)
		v, err := h.u.Check()
		if err != nil || v.Available || v.Latest != "" || v.Checking || v.Err != "" {
			t.Fatalf("%q: Check = %+v, %v", cur, v, err)
		}
		h.u.autoTick(context.Background(), updT0.Add(2*time.Minute))
		if g.count() != 0 {
			t.Fatalf("%q: %d requests, want none", cur, g.count())
		}
		if _, ok, _ := st.GetSetting(settingLastCheck); ok {
			t.Fatalf("%q: the check time was stored", cur)
		}
		started := false
		h.u.start = func(string) error { started = true; return nil }
		if err := h.u.Install(); err == nil || started {
			t.Fatalf("%q: Install = %v, started %v; want refused", cur, err, started)
		}
	}
}

// Final review M3: the release a check found survives a restart (the
// «Оновити» row stays) until a check says otherwise or the app runs it.
func TestFoundReleaseSurvivesRestart(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.2.0")
	g.mu.Lock()
	g.assets = true
	g.mu.Unlock()
	h := newUpdates(t, st, g, "1.0.0", false, updT0)
	if v, _ := h.u.Check(); !v.Available {
		t.Fatalf("check: %+v", v)
	}
	found := h.u.LatestRelease()
	if found.InstallerURL == "" || found.ChecksumURL == "" {
		t.Fatalf("fake release has no assets: %+v", found)
	}

	// Restart: the row is back without a request.
	h2 := newUpdates(t, st, g, "1.0.0", false, updT0)
	want := UpdateView{Current: "1.0.0", Latest: "1.2.0", Available: true, PageURL: found.PageURL}
	if v := h2.u.State(); v != want {
		t.Fatalf("after restart: %+v, want %+v", v, want)
	}
	if r := h2.u.LatestRelease(); r.Version != found.Version || r.PageURL != found.PageURL ||
		r.InstallerURL != found.InstallerURL || r.ChecksumURL != found.ChecksumURL {
		t.Fatalf("restored %+v, want %+v", r, found)
	}
	if g.count() != 1 {
		t.Fatalf("%d requests, want 1", g.count())
	}

	// A failed check keeps it.
	g.set("1.2.0", http.StatusBadGateway)
	h2.u.Check()
	if v := newUpdates(t, st, g, "1.0.0", false, updT0).u.State(); !v.Available {
		t.Fatalf("lost after a failed check: %+v", v)
	}

	// Installed: the running version is the one found; it is forgotten.
	h3 := newUpdates(t, st, g, "1.2.0", false, updT0)
	if v := h3.u.State(); v != (UpdateView{Current: "1.2.0"}) {
		t.Fatalf("running 1.2.0: %+v", v)
	}
	if raw, ok, _ := st.GetSetting(settingFoundRelease); ok && raw != "" {
		t.Fatalf("still stored: %q", raw)
	}

	// A check that finds nothing newer clears it too.
	st2 := openTestStore(t)
	g.set("1.2.0", 0)
	h4 := newUpdates(t, st2, g, "1.0.0", false, updT0)
	h4.u.Check()
	g.set("1.0.0", 0)
	h4.u.Check()
	if v := newUpdates(t, st2, g, "1.0.0", false, updT0).u.State(); v.Available || v.Latest != "" {
		t.Fatalf("after «up to date»: %+v", v)
	}

	// A dev build neither shows nor clears it.
	st3 := openTestStore(t)
	g.set("1.2.0", 0)
	newUpdates(t, st3, g, "1.0.0", false, updT0).u.Check()
	if v := newUpdates(t, st3, g, "dev", false, updT0).u.State(); v.Available || v.Latest != "" {
		t.Fatalf("dev shows %+v", v)
	}
	if raw, ok, _ := st3.GetSetting(settingFoundRelease); !ok || raw == "" {
		t.Fatal("a dev build cleared the stored release")
	}

	// Garbage in the setting is ignored.
	st4 := openTestStore(t)
	if err := st4.SetSetting(settingFoundRelease, "{not json"); err != nil {
		t.Fatal(err)
	}
	if v := newUpdates(t, st4, g, "1.0.0", false, updT0).u.State(); v.Available {
		t.Fatalf("garbage restored: %+v", v)
	}
}

// Final review M6: the start removes a leftover download folder — only the
// fixed MarstekMonitor-update folder.
func TestCleanTempRemovesLeftover(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, updateTempDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marstek-monitor-1.0.0-setup.exe"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newUpdates(t, openTestStore(t), nil, "1.0.0", false, updT0)
	h.u.TempDir = dir
	h.u.CleanTemp()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("leftover still there: %v", err)
	}
	h.u.CleanTemp() // nothing there: no harm
	// Any other folder is never removed.
	h.u.TempDir = other
	h.u.CleanTemp()
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("a foreign folder was removed: %v", err)
	}
}

func TestUpdateToastTextAndRoute(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	toast := notify.Toast{Kind: notify.Update, Args: map[string]string{"version": "1.2.0"}}
	wantText(t, toast, "Доступна версія 1.2.0", "Натисніть, щоб оновити")
	i18n.SetLang("en")
	wantText(t, toast, "Version 1.2.0 is available", "Click to update")
	if got := ToastRoute(notify.Update, "", 0); got != (Navigate{View: "settings", Section: "about"}) {
		t.Fatalf("route %+v", got)
	}
	if p := toastPlace(notify.Toast{Kind: notify.Update, Silent: true}); p.Tag != "update" || !p.SuppressPopup {
		t.Fatalf("place %+v", p)
	}
}

// Fix round 1: a tick while a manual check is in flight neither checks nor
// stores the check time (the day's automatic check is not used up).
func TestAutoTickDuringManualCheckStoresNothing(t *testing.T) {
	st := openTestStore(t)
	g := newFakeGitHub(t, "1.0.0")
	h := newUpdates(t, st, g, "1.0.0", true, updT0)
	h.u.mu.Lock()
	h.u.view.Checking = true // the manual check is running
	h.u.mu.Unlock()
	h.u.autoTick(context.Background(), updT0.Add(2*time.Minute))
	if _, ok, _ := st.GetSetting(settingLastCheck); ok || g.count() != 0 {
		t.Fatalf("stored %v, %d requests; want neither", ok, g.count())
	}
	h.u.mu.Lock()
	h.u.view.Checking = false
	h.u.mu.Unlock()
	h.u.autoTick(context.Background(), updT0.Add(3*time.Minute))
	if _, ok, _ := st.GetSetting(settingLastCheck); !ok || g.count() != 1 {
		t.Fatalf("after: stored %v, %d requests; want both", ok, g.count())
	}
}

// Fix round 1: in -simulate* the button never reaches GitHub and says so.
func TestCheckOffInSimulation(t *testing.T) {
	g := newFakeGitHub(t, "1.2.0")
	h := newUpdates(t, openTestStore(t), g, "1.0.0", true, updT0)
	h.u.Simulate = true
	v, err := h.u.Check()
	if err != nil || v.Err != UpdateErrSimulated || v.Checking || v.Available || g.count() != 0 {
		t.Fatalf("simulated Check = %+v, %v, %d requests", v, err, g.count())
	}
	if len(h.views) == 0 || h.views[len(h.views)-1].Err != UpdateErrSimulated {
		t.Fatalf("not emitted: %+v", h.views)
	}
}
