//go:build release

package update

// releaseBuild: this binary was built with the release tag, so the releases
// repo is pinned and no environment variable can move it.
const releaseBuild = true

// releasesBase is ReleasesBase, always, in a release build.
func releasesBase() string { return ReleasesBase }

// latestAPI is LatestURL, always, in a release build.
func latestAPI() string { return LatestURL }
