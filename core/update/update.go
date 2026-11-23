// Package update finds out whether a newer version of the app is published
// (plan 3, Task 4): one unauthenticated GET of the latest release of the
// public releases repo, with the app's name and version as the User-Agent
// and nothing else about the user, the PC or the station.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LatestURL is the GitHub API address of the newest published release.
const LatestURL = "https://api.github.com/repos/econ11/marstek-monitor/releases/latest"

// ReleasesBase is the releases repo's release pages and assets: a release
// restored from storage (Restore) must point under it.
const ReleasesBase = "https://github.com/econ11/marstek-monitor/releases/"

// ReleasesPage is the releases page: PageURL when the release names none
// that can be opened safely.
const ReleasesPage = ReleasesBase + "latest"

// maxBody bounds the API answer (a release with notes is a few kB).
const maxBody = 2 << 20

// HTTPDoer sends a request: *http.Client, or a fake in tests.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// NewClient is the client for Latest: no cookie jar, a timeout, and
// redirects followed only to https (GitHub sends asset downloads to its
// CDN), at most 10.
func NewClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: httpsOnly}
}

// NewDownloadClient is NewClient for the one-click update's downloads: the
// same rules, but the whole request may take up to 30 minutes (an installer
// on a slow line).
func NewDownloadClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Minute, CheckRedirect: httpsOnly}
}

func httpsOnly(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("update: too many redirects")
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("update: redirect to %s refused (not https)", req.URL.Scheme)
	}
	return nil
}

// Release is a published version. InstallerURL and ChecksumURL are the
// assets InstallerName and InstallerName+".sha256", "" when missing.
type Release struct {
	Version                            string // without the "v"
	PageURL, InstallerURL, ChecksumURL string
	Notes                              string
}

// InstallerName is the installer asset of version v (the release script,
// Task 7, publishes it with its checksum as InstallerName(v)+".sha256").
func InstallerName(v string) string { return "marstek-monitor-" + v + "-setup.exe" }

type apiRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest asks GitHub for the newest release; newer reports whether it is
// newer than current (Newer). Any answer but 200 with a release whose tag is
// semver is an error: offline, a rate limit (403 / 429), a 5xx, no release.
func Latest(ctx context.Context, c HTTPDoer, current string) (rel Release, newer bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestURL, nil)
	if err != nil {
		return Release{}, false, err
	}
	req.Header.Set("User-Agent", "MarstekMonitor/"+current)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.Do(req)
	if err != nil {
		return Release{}, false, fmt.Errorf("update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 403 with no requests left, or 429: GitHub limits unauthenticated checks.
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return Release{}, false, fmt.Errorf("update: rate limited (%s)", resp.Status)
		}
		return Release{}, false, fmt.Errorf("update: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return Release{}, false, fmt.Errorf("update: %w", err)
	}
	if len(body) > maxBody {
		return Release{}, false, errors.New("update: answer too large")
	}
	var a apiRelease
	if err := json.Unmarshal(body, &a); err != nil {
		return Release{}, false, fmt.Errorf("update: %w", err)
	}
	if a.Draft || a.Prerelease {
		return Release{}, false, errors.New("update: not a published release")
	}
	v := strings.TrimPrefix(a.TagName, "v")
	if _, ok := parse(v); !ok {
		return Release{}, false, fmt.Errorf("update: tag %q is not a version", a.TagName)
	}
	rel = Release{Version: v, PageURL: httpsURL(a.HTMLURL), Notes: a.Body}
	if rel.PageURL == "" {
		rel.PageURL = ReleasesPage
	}
	for _, as := range a.Assets {
		switch as.Name {
		case InstallerName(v):
			rel.InstallerURL = httpsURL(as.URL)
		case InstallerName(v) + ".sha256":
			rel.ChecksumURL = httpsURL(as.URL)
		}
	}
	return rel, Newer(v, current), nil
}

// IsVersion reports whether v is a version (MAJOR.MINOR.PATCH, see Newer):
// false for a developer build ("dev", ""), which never checks for updates.
func IsVersion(v string) bool {
	_, ok := parse(v)
	return ok
}

// Restore checks a release read back from storage (the app keeps the last
// one found across restarts, settingFoundRelease). It is pinned to what
// Latest finds in the releases repo: Version is plain MAJOR.MINOR.PATCH
// (an optional -pre, no "v", no "+build"); PageURL, InstallerURL and
// ChecksumURL are https under ReleasesBase, the installer ends in
// "/"+InstallerName(Version) and the checksum in that +".sha256". Anything
// else drops the whole release (ok false): the next check finds it again.
// Whether it is newer than the running version is the caller's check.
func Restore(r Release) (Release, bool) {
	v := r.Version
	if !IsVersion(v) || strings.HasPrefix(v, "v") || strings.Contains(v, "+") {
		return Release{}, false
	}
	if !pinned(r.PageURL, "") || !pinned(r.InstallerURL, "/"+InstallerName(v)) ||
		!pinned(r.ChecksumURL, "/"+InstallerName(v)+".sha256") {
		return Release{}, false
	}
	return Release{Version: v, PageURL: r.PageURL, InstallerURL: r.InstallerURL, ChecksumURL: r.ChecksumURL}, true
}

// pinned: s is an https URL under ReleasesBase ending in suffix, with no
// query, fragment, user info or "..".
func pinned(s, suffix string) bool {
	if httpsURL(s) == "" || !strings.HasPrefix(s, ReleasesBase) || !strings.HasSuffix(s, suffix) || strings.Contains(s, "..") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// httpsURL returns s if it is an absolute https URL with a host, else "".
func httpsURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return s
}

type version struct {
	n   [3]int
	pre []string // nil: a release
}

// parse reads MAJOR.MINOR.PATCH[-pre][+build] with an optional "v"; numbers
// have no leading zeros.
func parse(s string) (version, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v version
	if i := strings.IndexByte(s, '-'); i >= 0 {
		if s[i+1:] == "" {
			return version{}, false
		}
		v.pre = strings.Split(s[i+1:], ".")
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	for i, p := range parts {
		n, ok := number(p)
		if !ok {
			return version{}, false
		}
		v.n[i] = n
	}
	for _, id := range v.pre {
		if id == "" {
			return version{}, false
		}
	}
	return v, true
}

func number(p string) (int, bool) {
	if p == "" || len(p) > 1 && p[0] == '0' {
		return 0, false
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(p)
	return n, err == nil
}

// Newer reports whether version a is newer than b (semver precedence; a "v"
// prefix is allowed). Anything that is not semver, "dev" included, is never
// newer, and nothing is newer than it.
func Newer(a, b string) bool {
	va, ok1 := parse(a)
	vb, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	return compare(va, vb) > 0
}

func compare(a, b version) int {
	for i := range a.n {
		if a.n[i] != b.n[i] {
			return cmpInt(a.n[i], b.n[i])
		}
	}
	switch {
	case a.pre == nil && b.pre == nil:
		return 0
	case a.pre == nil:
		return 1 // a release is newer than its pre-releases
	case b.pre == nil:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, y := a.pre[i], b.pre[i]
		nx, okx := number(x)
		ny, oky := number(y)
		switch {
		case okx && oky:
			if nx != ny {
				return cmpInt(nx, ny)
			}
		case okx:
			return -1 // numeric identifiers sort first
		case oky:
			return 1
		case x != y:
			return strings.Compare(x, y)
		}
	}
	return cmpInt(len(a.pre), len(b.pre))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
