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
	rel, newer, err := Latest(context.Background(), d, "1.1.0")
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
	if _, _, err := Latest(context.Background(), d, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if c := seen.Header.Get("Cookie"); c != "" {
		t.Fatalf("second request sent cookie %q", c)
	}
}

func TestLatestSameOrOlderIsNotNewer(t *testing.T) {
	d := server(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(releaseJSON)) })
	for _, cur := range []string{"1.2.0", "1.3.0", "dev"} {
		rel, newer, err := Latest(context.Background(), d, cur)
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
		if _, newer, err := Latest(context.Background(), d, "1.0.0"); err == nil || newer {
			t.Errorf("%s: err %v, newer %v; want an error", name, err, newer)
		}
	}
	// Offline: the doer itself fails.
	off := doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("no network") })
	if _, _, err := Latest(context.Background(), off, "1.0.0"); err == nil {
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
	rel, newer, err := Latest(context.Background(), d, "1.0.0")
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
		InstallerURL: dl + InstallerName("1.2.0"), ChecksumURL: dl + InstallerName("1.2.0") + ".sha256"}
	if r, ok := Restore(good); !ok || r != good {
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
		"foreign installer": with(func(r *Release) { r.InstallerURL = "https://evil.example/" + InstallerName("1.2.0") }),
		"other repo": with(func(r *Release) {
			r.InstallerURL = "https://github.com/econ11/marstek-monitor-evil/releases/download/v1.2.0/" + InstallerName("1.2.0")
		}),
		"wrong installer":   with(func(r *Release) { r.InstallerURL = dl + "evil.exe" }),
		"other version exe": with(func(r *Release) { r.InstallerURL = dl + InstallerName("1.1.0") }),
		"wrong checksum":    with(func(r *Release) { r.ChecksumURL = dl + "sums.txt" }),
		"foreign checksum":  with(func(r *Release) { r.ChecksumURL = "https://evil.example/" + InstallerName("1.2.0") + ".sha256" }),
		"no installer":      with(func(r *Release) { r.InstallerURL = "" }),
		"no checksum":       with(func(r *Release) { r.ChecksumURL = "" }),
		"userinfo": with(func(r *Release) {
			r.InstallerURL = "https://u:p@github.com/econ11/marstek-monitor/releases/download/v1.2.0/" + InstallerName("1.2.0")
		}),
	}
	for name, r := range bad {
		if got, ok := Restore(r); ok || got != (Release{}) {
			t.Errorf("%s: Restore = %+v, %v; want dropped", name, got, ok)
		}
	}
}
