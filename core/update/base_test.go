package update

import (
	"context"
	"net/http"
	"testing"
)

// The settled call: a release build keeps the releases repo pinned, whatever
// the environment says. Run with: go test -tags release ./core/update
func TestReleaseBasePinned(t *testing.T) {
	if !releaseBuild {
		t.Skip("needs -tags release")
	}
	// A loopback server (the dev build's end-to-end run) is ignored too.
	for _, env := range [][2]string{
		{"https://example.com/releases/", "https://example.com/api/latest"},
		{"http://127.0.0.1:8080/", "http://127.0.0.1:8080/x"},
	} {
		t.Setenv("MARSTEK_UPDATE_BASE", env[0])
		t.Setenv("MARSTEK_UPDATE_API", env[1])
		if got := releasesBase(); got != ReleasesBase {
			t.Fatalf("MARSTEK_UPDATE_BASE=%q: releasesBase() = %q, want %q", env[0], got, ReleasesBase)
		}
		if got := latestAPI(); got != LatestURL {
			t.Fatalf("MARSTEK_UPDATE_API=%q: latestAPI() = %q, want %q", env[1], got, LatestURL)
		}
	}
	t.Setenv("MARSTEK_UPDATE_BASE", "https://example.com/releases/")
	// Restore still pins to the real repo, not the override.
	dl := "https://example.com/releases/download/v1.2.0/"
	n := AssetName("1.2.0", "windows")
	if _, ok := Restore(Release{Version: "1.2.0", PageURL: "https://example.com/releases/tag/v1.2.0",
		InstallerURL: dl + n, ChecksumURL: dl + n + ".sha256"}, "windows"); ok {
		t.Fatal("restored a release under the overridden base")
	}
}

// Without the release tag the base and the API may point at a local server
// for a real end-to-end update run; anything but loopback http or https is
// ignored.
func TestDevBaseOverride(t *testing.T) {
	if releaseBuild {
		t.Skip("dev builds only")
	}
	for _, c := range []struct{ env, want string }{
		{"", ReleasesBase},
		{"http://127.0.0.1:8080/", "http://127.0.0.1:8080/"},
		{"http://127.0.0.1:8080/rel/", "http://127.0.0.1:8080/rel/"},
		{"https://example.com/rel/", "https://example.com/rel/"},
		{"http://example.com/", ReleasesBase},
		{"http://127.0.0.1/", ReleasesBase},       // no port
		{"http://127.0.0.1:x/", ReleasesBase},     // not a port
		{"http://127.0.0.1:8080", ReleasesBase},   // no trailing /
		{"https://example.com/rel", ReleasesBase}, // no trailing /
		{"http://localhost:8080/", ReleasesBase},
		{"http://127.0.0.1.evil.com:80/", ReleasesBase},
		{"https://u:p@example.com/", ReleasesBase},
		{"https://example.com/?q=1/", ReleasesBase},
		{"file:///c:/rel/", ReleasesBase},
	} {
		t.Setenv("MARSTEK_UPDATE_BASE", c.env)
		if got := releasesBase(); got != c.want {
			t.Errorf("MARSTEK_UPDATE_BASE=%q: releasesBase() = %q, want %q", c.env, got, c.want)
		}
	}
	for _, c := range []struct{ env, want string }{
		{"", LatestURL},
		{"http://127.0.0.1:8080/latest", "http://127.0.0.1:8080/latest"},
		{"https://example.com/latest", "https://example.com/latest"},
		{"http://example.com/latest", LatestURL},
		{"http://127.0.0.1/latest", LatestURL},
	} {
		t.Setenv("MARSTEK_UPDATE_API", c.env)
		if got := latestAPI(); got != c.want {
			t.Errorf("MARSTEK_UPDATE_API=%q: latestAPI() = %q, want %q", c.env, got, c.want)
		}
	}
	// Lookalikes of the loopback server: neither override takes them.
	for _, env := range []string{
		"http://127.0.0.1@evil/",        // user info, host evil
		"HTTP://127.0.0.1:80/",          // not the literal prefix
		"http://[::1]:80/",              // IPv6 loopback
		"http://localhost:80/",          // a name, not the address
		"http://127.0.0.1:80/?x",        // query
		"http://127.0.0.1:0/",           // port 0
		"http://127.0.0.1:65536/",       // port out of range
		"http://127.0.0.1.evil.com:80/", // another host
	} {
		t.Setenv("MARSTEK_UPDATE_BASE", env)
		if got := releasesBase(); got != ReleasesBase {
			t.Errorf("MARSTEK_UPDATE_BASE=%q: releasesBase() = %q, want %q", env, got, ReleasesBase)
		}
		t.Setenv("MARSTEK_UPDATE_API", env)
		if got := latestAPI(); got != LatestURL {
			t.Errorf("MARSTEK_UPDATE_API=%q: latestAPI() = %q, want %q", env, got, LatestURL)
		}
	}

	// A loopback base: Latest asks the overridden API and keeps the loopback
	// asset URLs under it; Restore pins to it.
	const base = "http://127.0.0.1:8080/rel/"
	t.Setenv("MARSTEK_UPDATE_BASE", base)
	t.Setenv("MARSTEK_UPDATE_API", "http://127.0.0.1:8080/api/latest")
	dl := base + "download/v1.2.0/"
	n := AssetName("1.2.0", "darwin")
	d := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"` + base + `tag/v1.2.0","assets":[` +
			`{"name":"` + n + `","browser_download_url":"` + dl + n + `"},` +
			`{"name":"` + n + `.sha256","browser_download_url":"` + dl + n + `.sha256"},` +
			`{"name":"x","browser_download_url":"http://127.0.0.1:9999/x"}]}`))
	})
	rel, _, err := Latest(context.Background(), d, "1.0.0", "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.got[0].URL.String(); got != "http://127.0.0.1:8080/api/latest" {
		t.Fatalf("asked %s", got)
	}
	want := Release{Version: "1.2.0", PageURL: base + "tag/v1.2.0", InstallerURL: dl + n, ChecksumURL: dl + n + ".sha256"}
	if rel != want {
		t.Fatalf("Latest = %+v, want %+v", rel, want)
	}
	if r, ok := Restore(want, "darwin"); !ok || r != want {
		t.Fatalf("Restore = %+v, %v", r, ok)
	}
	// Another loopback port is not under the base.
	other := want
	other.InstallerURL = "http://127.0.0.1:9999/rel/download/v1.2.0/" + n
	if _, ok := Restore(other, "darwin"); ok {
		t.Fatal("restored an asset outside the base")
	}
}

// LocalTestBase: only a dev build pointed at a loopback http base may let a
// -simulate* run check and install (the local update test); never a
// release build, never an https base, never without the override.
func TestLocalTestBase(t *testing.T) {
	for base, devWant := range map[string]bool{
		"":                                false,
		"http://127.0.0.1:8000/":          true,
		"http://127.0.0.1:8000/download/": true,
		"http://127.0.0.1:8000":           false, // no trailing slash: not a base
		"https://example.com/releases/":   false, // an override, but not local
		"http://localhost:8000/":          false,
		"http://127.0.0.1:8000/?x=1/":     false,
		"http://user@127.0.0.1:8000/":     false,
	} {
		t.Setenv("MARSTEK_UPDATE_BASE", base)
		want := devWant && !releaseBuild
		if got := LocalTestBase(); got != want {
			t.Errorf("release=%v base %q: LocalTestBase %v, want %v", releaseBuild, base, got, want)
		}
	}
}
