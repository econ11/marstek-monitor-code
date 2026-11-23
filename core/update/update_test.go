package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	chain := []string{"1.0.0", "1.0.1", "1.1.0", "2.0.0"}
	for i := range chain {
		for j := range chain {
			if got, want := Newer(chain[i], chain[j]), i > j; got != want {
				t.Errorf("Newer(%s, %s) = %v, want %v", chain[i], chain[j], got, want)
			}
		}
	}
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v1.0.1", "1.0.0", true}, // v-prefix tolerated on either side
		{"1.0.1", "v1.0.0", true},
		{"v1.0.0", "1.0.0", false},
		{"1.10.0", "1.9.0", true}, // numeric, not text
		{"1.0.0", "1.0.0-rc.1", true},
		{"1.0.0-rc.2", "1.0.0-rc.1", true},
		{"1.0.0-rc.1", "1.0.0", false},
		{"1.0.0+build.5", "1.0.0", false}, // build metadata is ignored
		{"dev", "1.0.0", false},           // dev is never newer
		{"1.0.0", "dev", false},           // nothing is newer than dev
		{"9.9.9", "", false},
		{"", "1.0.0", false},
		{"1.0", "0.9.0", false}, // not semver
		{"1.0.0.0", "0.9.0", false},
		{"01.0.0", "0.9.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// toServer sends every request to the test server instead of api.github.com
// and records what was asked.
type toServer struct {
	base *url.URL
	c    *http.Client
	got  []*http.Request
}

func (d *toServer) Do(r *http.Request) (*http.Response, error) {
	d.got = append(d.got, r.Clone(r.Context()))
	u := *r.URL
	u.Scheme, u.Host = d.base.Scheme, d.base.Host
	r2 := r.Clone(r.Context())
	r2.URL, r2.Host, r2.RequestURI = &u, d.base.Host, ""
	return d.c.Do(r2)
}

func server(t *testing.T, h http.HandlerFunc) *toServer {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	base, _ := url.Parse(srv.URL)
	return &toServer{base: base, c: srv.Client()}
}

const releaseJSON = `{
  "tag_name": "v1.2.0",
  "html_url": "https://github.com/econ11/marstek-monitor/releases/tag/v1.2.0",
  "body": "Fixes.",
  "draft": false, "prerelease": false,
  "assets": [
    {"name": "notes.txt", "browser_download_url": "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/notes.txt"},
    {"name": "marstek-monitor-1.2.0-setup.exe.sha256", "browser_download_url": "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/marstek-monitor-1.2.0-setup.exe.sha256"},
    {"name": "marstek-monitor-1.2.0-setup.exe", "browser_download_url": "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/marstek-monitor-1.2.0-setup.exe"}
  ]
}`

func TestLatestParses(t *testing.T) {
	var seen *http.Request
	d := server(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r
		w.Header().Set("Set-Cookie", "a=b") // must not come back
		w.Write([]byte(releaseJSON))
	})
	rel, newer, err := Latest(context.Background(), d, "1.1.0", "windows")
	if err != nil {
		t.Fatal(err)
	}
	want := Release{
		Version:      "1.2.0",
		PageURL:      "https://github.com/econ11/marstek-monitor/releases/tag/v1.2.0",
		InstallerURL: "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/marstek-monitor-1.2.0-setup.exe",
		ChecksumURL:  "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/marstek-monitor-1.2.0-setup.exe.sha256",
		Notes:        "Fixes.",
	}
	if rel != want || !newer {
		t.Fatalf("Latest = %+v, %v\nwant %+v, true", rel, newer, want)
	}
	// The real address, asked as the app and nothing more.
	asked := d.got[0]
	if asked.Method != http.MethodGet || asked.URL.String() != "https://api.github.com/repos/econ11/marstek-monitor/releases/latest" {
		t.Fatalf("asked %s %s", asked.Method, asked.URL)
	}
	if ua := seen.Header.Get("User-Agent"); ua != "MarstekMonitor/1.1.0" {
		t.Fatalf("User-Agent %q", ua)
	}
	if seen.URL.RawQuery != "" || seen.Header.Get("Cookie") != "" || seen.Header.Get("Authorization") != "" {
		t.Fatalf("sent query %q, cookie %q, auth %q", seen.URL.RawQuery, seen.Header.Get("Cookie"), seen.Header.Get("Authorization"))
	}
	for k := range seen.Header {
		switch k {
		case "User-Agent", "Accept", "Accept-Encoding", "X-Github-Api-Version":
		default:
			t.Errorf("unexpected header %s: %q", k, seen.Header.Get(k))
		}
	}
	// A second check sends no cookie either (no jar).
	if _, _, err := Latest(context.Background(), d, "1.2.0", "windows"); err != nil {
		t.Fatal(err)
	}
	if c := seen.Header.Get("Cookie"); c != "" {
		t.Fatalf("second request sent cookie %q", c)
	}
}

func TestLatestSameOrOlderIsNotNewer(t *testing.T) {
	d := server(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(releaseJSON)) })
	for _, cur := range []string{"1.2.0", "1.3.0", "dev"} {
		rel, newer, err := Latest(context.Background(), d, cur, "windows")
		if err != nil || newer || rel.Version != "1.2.0" {
			t.Fatalf("current %s: %+v, %v, %v", cur, rel, newer, err)
		}
	}
}

func TestLatestFailures(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"rate limit": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		},
		"429":      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
		"5xx":      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) },
		"404":      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) },
		"not json": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) },
		"bad tag": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"tag_name":"latest","html_url":"https://github.com/x"}`))
		},
		"draft": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"tag_name":"v2.0.0","draft":true}`)) },
		"too large": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"body":"` + strings.Repeat("x", 3<<20) + `"}`))
		},
	} {
		d := server(t, h)
		if _, newer, err := Latest(context.Background(), d, "1.0.0", "windows"); err == nil || newer {
			t.Errorf("%s: err %v, newer %v; want an error", name, err, newer)
		}
	}
	// Offline: the doer itself fails.
	off := doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("no network") })
	if _, _, err := Latest(context.Background(), off, "1.0.0", "windows"); err == nil {
		t.Fatal("offline: no error")
	}
}

// A page or asset that is not an https URL is never handed on (the UI opens
// PageURL in the browser, Task 5 downloads the assets).
func TestLatestDropsUnsafeURLs(t *testing.T) {
	d := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"1.2.0","html_url":"javascript:alert(1)","assets":[
		  {"name":"marstek-monitor-1.2.0-setup.exe","browser_download_url":"http://example.com/x.exe"},
		  {"name":"marstek-monitor-1.2.0-setup.exe.sha256","browser_download_url":"file:///c:/x"}]}`))
	})
	rel, newer, err := Latest(context.Background(), d, "1.0.0", "windows")
	if err != nil || !newer {
		t.Fatal(err)
	}
	if rel.PageURL != ReleasesPage || rel.InstallerURL != "" || rel.ChecksumURL != "" {
		t.Fatalf("%+v", rel)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

// Fix round 1: the client follows redirects to https only.
func TestNewClientHTTPSOnlyRedirects(t *testing.T) {
	c := NewClient()
	if c.Jar != nil || c.Timeout == 0 || c.CheckRedirect == nil {
		t.Fatalf("client %+v", c)
	}
	req := func(s string) *http.Request { r, _ := http.NewRequest(http.MethodGet, s, nil); return r }
	if err := c.CheckRedirect(req("https://objects.githubusercontent.com/x"), []*http.Request{req(LatestURL)}); err != nil {
		t.Fatalf("https redirect refused: %v", err)
	}
	if err := c.CheckRedirect(req("http://example.com/x"), []*http.Request{req(LatestURL)}); err == nil {
		t.Fatal("http redirect followed")
	}
	via := make([]*http.Request, 10)
	if err := c.CheckRedirect(req("https://example.com/x"), via); err == nil {
		t.Fatal("11th redirect followed")
	}
	// A real redirect from the (http) test server to http is not followed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/a" {
			http.Redirect(w, r, "/b", http.StatusFound)
			return
		}
		t.Error("followed the redirect to http")
	}))
	defer srv.Close()
	if _, err := c.Get(srv.URL + "/a"); err == nil {
		t.Fatal("no error for an http redirect")
	}
}

// Task 5: the installer download has room for a slow line (no 30 s cap on
// the whole request) but the same redirect rule and no cookie jar.
func TestNewDownloadClient(t *testing.T) {
	c := NewDownloadClient()
	if c.Jar != nil || c.Timeout < 10*time.Minute || c.CheckRedirect == nil {
		t.Fatalf("client %+v", c)
	}
	req := func(s string) *http.Request { r, _ := http.NewRequest(http.MethodGet, s, nil); return r }
	if err := c.CheckRedirect(req("http://example.com/x"), []*http.Request{req(LatestURL)}); err == nil {
		t.Fatal("http redirect followed")
	}
}

// Final review M2: a dev build ("dev", "") is not a version, so it never
// checks for updates.
func TestIsVersion(t *testing.T) {
	for _, v := range []string{"1.0.0", "v1.2.3", "1.0.0-rc.1", "2.10.0+build"} {
		if !IsVersion(v) {
			t.Errorf("IsVersion(%q) = false", v)
		}
	}
	for _, v := range []string{"", "dev", "1.0", "01.0.0", "1.0.0-"} {
		if IsVersion(v) {
			t.Errorf("IsVersion(%q) = true", v)
		}
	}
}

// Final review M3 (Ruling 7): a release read back from the settings is
// pinned to the releases repo; anything else drops it whole (the next check
// refills it).
func TestRestore(t *testing.T) {
	dl := ReleasesBase + "download/v1.2.0/"
	good := Release{Version: "1.2.0", PageURL: ReleasesBase + "tag/v1.2.0",
		InstallerURL: dl + AssetName("1.2.0", "windows"), ChecksumURL: dl + AssetName("1.2.0", "windows") + ".sha256"}
	if r, ok := Restore(good, "windows"); !ok || r != good {
		t.Fatalf("Restore(good) = %+v, %v", r, ok)
	}
	with := func(f func(*Release)) Release { r := good; f(&r); return r }
	bad := map[string]Release{
		"non-version":       with(func(r *Release) { r.Version = "dev" }),
		"v prefix":          with(func(r *Release) { r.Version = "v1.2.0" }),
		"+build":            with(func(r *Release) { r.Version = "1.2.0+abc" }),
		"foreign page":      with(func(r *Release) { r.PageURL = "https://evil.example/econ11/marstek-monitor/releases/tag/v1.2.0" }),
		"no page":           with(func(r *Release) { r.PageURL = "" }),
		"http page":         with(func(r *Release) { r.PageURL = "http://github.com/econ11/marstek-monitor/releases/tag/v1.2.0" }),
		"foreign installer": with(func(r *Release) { r.InstallerURL = "https://evil.example/" + AssetName("1.2.0", "windows") }),
		"other repo": with(func(r *Release) {
			r.InstallerURL = "https://github.com/econ11/marstek-monitor-evil/releases/download/v1.2.0/" + AssetName("1.2.0", "windows")
		}),
		"wrong installer":   with(func(r *Release) { r.InstallerURL = dl + "evil.exe" }),
		"other version exe": with(func(r *Release) { r.InstallerURL = dl + AssetName("1.1.0", "windows") }),
		"wrong checksum":    with(func(r *Release) { r.ChecksumURL = dl + "sums.txt" }),
		"foreign checksum":  with(func(r *Release) { r.ChecksumURL = "https://evil.example/" + AssetName("1.2.0", "windows") + ".sha256" }),
		"no installer":      with(func(r *Release) { r.InstallerURL = "" }),
		"no checksum":       with(func(r *Release) { r.ChecksumURL = "" }),
		"userinfo": with(func(r *Release) {
			r.InstallerURL = "https://u:p@github.com/econ11/marstek-monitor/releases/download/v1.2.0/" + AssetName("1.2.0", "windows")
		}),
	}
	for name, r := range bad {
		if got, ok := Restore(r, "windows"); ok || got != (Release{}) {
			t.Errorf("%s: Restore = %+v, %v; want dropped", name, got, ok)
		}
	}
}

func TestAssetName(t *testing.T) {
	for _, c := range []struct{ goos, want string }{
		{"windows", "marstek-monitor-1.1.0-setup.exe"},
		{"darwin", "marstek-monitor-1.1.0-macos.dmg"},
		{"linux", ""},
		{"", ""},
	} {
		if got := AssetName("1.1.0", c.goos); got != c.want {
			t.Errorf("AssetName(1.1.0, %q) = %q, want %q", c.goos, got, c.want)
		}
	}
}

// Review Focus 3: a release with both platforms' assets hands each platform
// only its own; a release without the current platform's asset offers none.
func TestLatestPicksPlatformAsset(t *testing.T) {
	const dl = "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/"
	asset := func(n string) string { return `{"name":"` + n + `","browser_download_url":"` + dl + n + `"}` }
	exe, dmg := "marstek-monitor-1.2.0-setup.exe", "marstek-monitor-1.2.0-macos.dmg"
	both := `{"tag_name":"v1.2.0","assets":[` + asset(exe) + `,` + asset(exe+".sha256") + `,` + asset(dmg) + `,` + asset(dmg+".sha256") + `]}`
	winOnly := `{"tag_name":"v1.2.0","assets":[` + asset(exe) + `,` + asset(exe+".sha256") + `]}`
	get := func(body, goos string) Release {
		t.Helper()
		d := server(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		rel, _, err := Latest(context.Background(), d, "1.0.0", goos)
		if err != nil {
			t.Fatal(err)
		}
		return rel
	}
	if r := get(both, "windows"); r.InstallerURL != dl+exe || r.ChecksumURL != dl+exe+".sha256" {
		t.Errorf("windows: %+v", r)
	}
	if r := get(both, "darwin"); r.InstallerURL != dl+dmg || r.ChecksumURL != dl+dmg+".sha256" {
		t.Errorf("darwin: %+v", r)
	}
	if r := get(winOnly, "darwin"); r.InstallerURL != "" || r.ChecksumURL != "" {
		t.Errorf("darwin, Windows-only release: %+v", r)
	}
	if r := get(both, "linux"); r.InstallerURL != "" || r.ChecksumURL != "" {
		t.Errorf("linux: %+v", r)
	}
}

// Review Focus 3: a stored release keeps only the current platform's asset;
// a Windows .exe stored in settings is dropped on Mac, and the reverse.
func TestRestorePlatform(t *testing.T) {
	dl := ReleasesBase + "download/v1.2.0/"
	rel := func(name string) Release {
		return Release{Version: "1.2.0", PageURL: ReleasesBase + "tag/v1.2.0",
			InstallerURL: dl + name, ChecksumURL: dl + name + ".sha256"}
	}
	win, mac := rel("marstek-monitor-1.2.0-setup.exe"), rel("marstek-monitor-1.2.0-macos.dmg")
	if r, ok := Restore(win, "windows"); !ok || r != win {
		t.Errorf("exe on windows: %+v, %v", r, ok)
	}
	if r, ok := Restore(win, "darwin"); ok || r != (Release{}) {
		t.Errorf("exe on darwin: %+v, %v; want dropped", r, ok)
	}
	if r, ok := Restore(mac, "darwin"); !ok || r != mac {
		t.Errorf("dmg on darwin: %+v, %v", r, ok)
	}
	if r, ok := Restore(mac, "windows"); ok || r != (Release{}) {
		t.Errorf("dmg on windows: %+v, %v; want dropped", r, ok)
	}
	if _, ok := Restore(win, "linux"); ok {
		t.Error("restored on linux")
	}
}

// Task 5 review: Latest keeps an asset URL only under the releases repo and
// ending in the platform's asset name, the rule Restore uses (pinned): a
// foreign host, a lookalike repo, another file or a query gives "".
func TestLatestPinsAssetURLs(t *testing.T) {
	const dl = "https://github.com/econ11/marstek-monitor/releases/download/v1.2.0/"
	for _, goos := range []string{"windows", "darwin"} {
		n := AssetName("1.2.0", goos)
		for name, urls := range map[string][2]string{
			"foreign host":   {"https://evil.example/" + n, "https://evil.example/" + n + ".sha256"},
			"lookalike repo": {"https://github.com/econ11/marstek-monitor-evil/releases/download/v1.2.0/" + n, "https://github.com/econ11/marstek-monitor-evil/releases/download/v1.2.0/" + n + ".sha256"},
			"other file":     {dl + "evil.exe", dl + "evil.sha256"},
			"query":          {dl + n + "?x=1", dl + n + ".sha256?x=1"},
			"dot dot":        {dl + "../../../../evil/" + n, dl + "../../../../evil/" + n + ".sha256"},
		} {
			body := `{"tag_name":"v1.2.0","assets":[` +
				`{"name":"` + n + `","browser_download_url":"` + urls[0] + `"},` +
				`{"name":"` + n + `.sha256","browser_download_url":"` + urls[1] + `"}]}`
			d := server(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
			rel, _, err := Latest(context.Background(), d, "1.0.0", goos)
			if err != nil {
				t.Fatal(err)
			}
			if rel.InstallerURL != "" || rel.ChecksumURL != "" {
				t.Errorf("%s, %s: %+v", goos, name, rel)
			}
		}
	}
}
