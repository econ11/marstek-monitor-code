//go:build !release

package update

import (
	"net/url"
	"os"
	"strconv"
	"strings"
)

// releaseBuild: a build without the release tag (dev, tests), where the
// releases base may be overridden for an end-to-end update run against a
// local server.
const releaseBuild = false

// releasesBase is MARSTEK_UPDATE_BASE when it is a loopback http or an https
// URL ending in "/", else ReleasesBase. Dev builds only.
func releasesBase() string {
	if s := os.Getenv("MARSTEK_UPDATE_BASE"); strings.HasSuffix(s, "/") && overrideURL(s) {
		return s
	}
	return ReleasesBase
}

// latestAPI is MARSTEK_UPDATE_API when it is a loopback http or an https URL,
// else LatestURL. Dev builds only.
func latestAPI() string {
	if s := os.Getenv("MARSTEK_UPDATE_API"); overrideURL(s) {
		return s
	}
	return LatestURL
}

// overrideURL: s is https://<host>/... or http://127.0.0.1:<port>/..., with
// no user info, query or fragment.
func overrideURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		p, err := strconv.Atoi(u.Port())
		return u.Hostname() == "127.0.0.1" && strings.HasPrefix(s, "http://127.0.0.1:") && err == nil && p > 0 && p < 65536
	}
	return false
}
