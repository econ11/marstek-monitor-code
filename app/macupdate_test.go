package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeMac is macTools over plain folders: the "dmg" is a folder that Attach
// returns as the mount, Copy copies a folder tree, and the Team IDs come
// from the fields (running: the app at runningApp; next: any other app).
type fakeMac struct {
	mount     string
	attachErr error
	copyErr   error
	verifyErr error
	running   string // runningApp
	runTeam   string
	newTeam   string

	attached, detached int
	verified           []string
}

func (f *fakeMac) Attach(string) (string, error) {
	if f.attachErr != nil {
		return "", f.attachErr
	}
	f.attached++
	return f.mount, nil
}

func (f *fakeMac) Detach(mount string) error {
	if mount != f.mount {
		return fmt.Errorf("detach %q, mounted %q", mount, f.mount)
	}
	f.detached++
	return nil
}

func (f *fakeMac) Copy(src, dst string) error {
	if f.copyErr != nil {
		return f.copyErr
	}
	return copyTree(src, dst)
}

func (f *fakeMac) Verify(app string) error {
	f.verified = append(f.verified, app)
	return f.verifyErr
}

func (f *fakeMac) TeamID(app string) (string, error) {
	if filepath.Clean(app) == filepath.Clean(f.running) {
		return f.runTeam, nil
	}
	return f.newTeam, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, 0o644)
	})
}

// makeApp builds a fake bundle whose Contents/marker holds tag.
func makeApp(t *testing.T, app, tag string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "marker"), []byte(tag), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appTag(t *testing.T, app string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app, "Contents", "marker"))
	if err != nil {
		t.Fatalf("read %s: %v", app, err)
	}
	return string(b)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// macSetup: Applications/Marstek Monitor.app (tag "old") is running; the
// dmg root holds Marstek Monitor.app (tag "new") and a non-app file.
func macSetup(t *testing.T) (*fakeMac, string) {
	t.Helper()
	root := t.TempDir()
	running := filepath.Join(root, "Applications", "Marstek Monitor.app")
	makeApp(t, running, "old")
	mount := filepath.Join(root, "mount")
	makeApp(t, filepath.Join(mount, "Marstek Monitor.app"), "new")
	if err := os.WriteFile(filepath.Join(mount, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &fakeMac{mount: mount, running: running, runTeam: "TEAM123", newTeam: "TEAM123"}, running
}

func tempUpdateDir(running string) string {
	return filepath.Join(filepath.Dir(running), fmt.Sprintf(".marstek-update-%d", os.Getpid()))
}

// untouched: the running app is the old bundle, there is no .old and no
// temp folder.
func untouched(t *testing.T, running string) {
	t.Helper()
	if got := appTag(t, running); got != "old" {
		t.Fatalf("running app holds %q, want old", got)
	}
	if exists(running + ".old") {
		t.Fatal(".old was created")
	}
	if exists(tempUpdateDir(running)) {
		t.Fatal("temp folder left behind")
	}
}

func TestMacInstallSwaps(t *testing.T) {
	f, running := macSetup(t)
	got, err := macInstall(f, "update.dmg", running)
	if err != nil {
		t.Fatal(err)
	}
	if got != running {
		t.Fatalf("new app %q, want %q", got, running)
	}
	if appTag(t, running) != "new" || appTag(t, running+".old") != "old" {
		t.Fatalf("after the swap: running %q, .old %q", appTag(t, running), appTag(t, running+".old"))
	}
	if exists(tempUpdateDir(running)) {
		t.Fatal("temp folder left behind")
	}
	if f.attached != 1 || f.detached != 1 {
		t.Fatalf("attached %d, detached %d; want 1, 1", f.attached, f.detached)
	}
	want := filepath.Join(tempUpdateDir(running), "Marstek Monitor.app")
	if len(f.verified) != 1 || f.verified[0] != want {
		t.Fatalf("verified %v, want [%s]", f.verified, want)
	}
}

func TestMacInstallTeamMismatch(t *testing.T) {
	for _, c := range []struct{ name, run, next string }{
		{"different", "TEAM123", "OTHER99"},
		{"new empty", "TEAM123", ""},
		{"running empty", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, running := macSetup(t)
			f.runTeam, f.newTeam = c.run, c.next
			if _, err := macInstall(f, "update.dmg", running); err == nil {
				t.Fatal("no error")
			}
			untouched(t, running)
		})
	}
}

func TestMacInstallVerifyFails(t *testing.T) {
	f, running := macSetup(t)
	f.verifyErr = errors.New("codesign: invalid signature")
	if _, err := macInstall(f, "update.dmg", running); err == nil {
		t.Fatal("no error")
	}
	untouched(t, running)
}

func TestMacInstallNoOrManyApps(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		f, running := macSetup(t)
		if err := os.RemoveAll(filepath.Join(f.mount, "Marstek Monitor.app")); err != nil {
			t.Fatal(err)
		}
		if _, err := macInstall(f, "update.dmg", running); err == nil {
			t.Fatal("no error")
		}
		untouched(t, running)
	})
	t.Run("two", func(t *testing.T) {
		f, running := macSetup(t)
		makeApp(t, filepath.Join(f.mount, "Other.app"), "other")
		if _, err := macInstall(f, "update.dmg", running); err == nil {
			t.Fatal("no error")
		}
		untouched(t, running)
	})
}

// swapRename makes renameFile fail for the move whose source is under dir.
func swapRename(t *testing.T, failFrom func(src string) bool) {
	t.Helper()
	prev := renameFile
	renameFile = func(src, dst string) error {
		if failFrom(src) {
			return errors.New("rename: injected failure")
		}
		return prev(src, dst)
	}
	t.Cleanup(func() { renameFile = prev })
}

func TestSwapRollsBack(t *testing.T) {
	f, running := macSetup(t)
	tmp := tempUpdateDir(running)
	swapRename(t, func(src string) bool { return strings.HasPrefix(src, tmp) })
	if _, err := macInstall(f, "update.dmg", running); err == nil {
		t.Fatal("no error")
	}
	untouched(t, running)
	if f.detached != 1 {
		t.Fatalf("detached %d, want 1", f.detached)
	}
}

func TestMacInstallFirstRenameFails(t *testing.T) {
	f, running := macSetup(t)
	swapRename(t, func(src string) bool { return src == running })
	if _, err := macInstall(f, "update.dmg", running); err == nil {
		t.Fatal("no error")
	}
	untouched(t, running)
}

// Every failure after Attach detaches the dmg once; a failed Attach has
// nothing to detach.
func TestMacInstallDetachesOnError(t *testing.T) {
	cases := map[string]func(t *testing.T, f *fakeMac, running string){
		"no app":   func(t *testing.T, f *fakeMac, _ string) { os.RemoveAll(filepath.Join(f.mount, "Marstek Monitor.app")) },
		"two apps": func(t *testing.T, f *fakeMac, _ string) { makeApp(t, filepath.Join(f.mount, "B.app"), "b") },
		"copy":     func(_ *testing.T, f *fakeMac, _ string) { f.copyErr = errors.New("ditto failed") },
		"verify":   func(_ *testing.T, f *fakeMac, _ string) { f.verifyErr = errors.New("bad") },
		"team":     func(_ *testing.T, f *fakeMac, _ string) { f.newTeam = "OTHER" },
		"rename old": func(t *testing.T, _ *fakeMac, running string) {
			swapRename(t, func(src string) bool { return src == running })
		},
		"rename new": func(t *testing.T, _ *fakeMac, running string) {
			tmp := tempUpdateDir(running)
			swapRename(t, func(src string) bool { return strings.HasPrefix(src, tmp) })
		},
	}
	for name, prep := range cases {
		t.Run(name, func(t *testing.T) {
			f, running := macSetup(t)
			prep(t, f, running)
			if _, err := macInstall(f, "update.dmg", running); err == nil {
				t.Fatal("no error")
			}
			if f.attached != 1 || f.detached != 1 {
				t.Fatalf("attached %d, detached %d; want 1, 1", f.attached, f.detached)
			}
			untouched(t, running)
		})
	}
	t.Run("attach", func(t *testing.T) {
		f, running := macSetup(t)
		f.attachErr = errors.New("hdiutil failed")
		if _, err := macInstall(f, "update.dmg", running); err == nil {
			t.Fatal("no error")
		}
		if f.detached != 0 {
			t.Fatalf("detached %d, want 0", f.detached)
		}
		untouched(t, running)
	})
}

func TestCanSelfUpdate(t *testing.T) {
	const normal = "/Applications/Marstek Monitor.app"
	yes := func(string) bool { return true }
	for _, c := range []struct {
		name, app, team string
		writable        func(string) bool
		want            bool
	}{
		{"normal", normal, "TEAM123", yes, true},
		{"translocated", "/private/var/folders/xy/T/AppTranslocation/1A2B/d/Marstek Monitor.app", "TEAM123", yes, false},
		{"dmg", "/Volumes/Marstek Monitor/Marstek Monitor.app", "TEAM123", yes, false},
		{"unwritable", normal, "TEAM123", func(string) bool { return false }, false},
		{"no team", normal, "", yes, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeMac{running: c.app, runTeam: c.team}
			if got := canSelfUpdate(f, c.app, c.writable); got != c.want {
				t.Fatalf("canSelfUpdate(%q) = %v, want %v", c.app, got, c.want)
			}
		})
	}
	// The writable check is asked about the folder holding the app.
	var asked string
	canSelfUpdate(&fakeMac{running: normal, runTeam: "TEAM123"}, normal, func(d string) bool { asked = d; return true })
	if filepath.ToSlash(asked) != "/Applications" {
		t.Fatalf("writable asked about %q, want /Applications", asked)
	}
}

func TestIsTranslocated(t *testing.T) {
	if !isTranslocated("/private/var/folders/x/AppTranslocation/ABC/d/A.app") {
		t.Fatal("translocated path not recognised")
	}
	if isTranslocated("/Applications/A.app") {
		t.Fatal("normal path taken as translocated")
	}
}

func TestCleanOldBundle(t *testing.T) {
	root := t.TempDir()
	running := filepath.Join(root, "Marstek Monitor.app")
	makeApp(t, running, "new")
	if err := cleanOldBundle(running); err != nil { // nothing to remove
		t.Fatal(err)
	}
	makeApp(t, running+".old", "old")
	if err := cleanOldBundle(running); err != nil {
		t.Fatal(err)
	}
	if exists(running+".old") || appTag(t, running) != "new" {
		t.Fatal(".old not removed, or the running app touched")
	}
}

// Fix: the second rename and the rename back both fail. The verified new
// bundle stays in the temp folder, .old stays, and the error names all
// three paths, so nothing is lost.
func TestSwapNeverLosesBothBundles(t *testing.T) {
	f, running := macSetup(t)
	first := true
	swapRename(t, func(string) bool {
		if first {
			first = false
			return false
		}
		return true
	})
	_, err := macInstall(f, "update.dmg", running)
	if err == nil {
		t.Fatal("no error")
	}
	next := filepath.Join(tempUpdateDir(running), "Marstek Monitor.app")
	if exists(running) {
		t.Fatal("running path not empty")
	}
	if appTag(t, next) != "new" || appTag(t, running+".old") != "old" {
		t.Fatal("a bundle was lost")
	}
	for _, p := range []string{running + ".old", next, running} {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("error %q does not name %s", err, p)
		}
	}
	if f.detached != 1 {
		t.Fatalf("detached %d, want 1", f.detached)
	}
}

// The second rename fails once, the rename back fails, the retry works:
// the new app is in place.
func TestSwapRetriesNewIntoPlace(t *testing.T) {
	f, running := macSetup(t)
	n := 0
	swapRename(t, func(string) bool { n++; return n == 2 || n == 3 })
	if _, err := macInstall(f, "update.dmg", running); err != nil {
		t.Fatal(err)
	}
	if appTag(t, running) != "new" || exists(tempUpdateDir(running)) {
		t.Fatal("new app not in place, or temp folder left")
	}
}

func TestMacInstallReplacesLeftoverOld(t *testing.T) {
	f, running := macSetup(t)
	makeApp(t, running+".old", "older")
	if _, err := macInstall(f, "update.dmg", running); err != nil {
		t.Fatal(err)
	}
	if appTag(t, running) != "new" || appTag(t, running+".old") != "old" {
		t.Fatalf("running %q, .old %q", appTag(t, running), appTag(t, running+".old"))
	}
}

// A symlinked .app at the dmg root is not an app: the one real app is taken.
func TestMacInstallIgnoresSymlinkedApp(t *testing.T) {
	f, running := macSetup(t)
	target := filepath.Join(t.TempDir(), "Evil.app")
	makeApp(t, target, "evil")
	if err := os.Symlink(target, filepath.Join(f.mount, "Evil.app")); err != nil {
		t.Skipf("symlinks need privileges here: %v", err)
	}
	if _, err := macInstall(f, "update.dmg", running); err != nil {
		t.Fatal(err)
	}
	if appTag(t, running) != "new" {
		t.Fatalf("installed %q, want new", appTag(t, running))
	}
}

func TestMacRelativePathRefused(t *testing.T) {
	f, _ := macSetup(t)
	if _, err := macInstall(f, "update.dmg", "Marstek Monitor.app"); err == nil {
		t.Fatal("macInstall took a relative path")
	}
	if f.attached != 0 {
		t.Fatal("mounted for a relative path")
	}
	yes := func(string) bool { return true }
	if canSelfUpdate(&fakeMac{running: "Applications/A.app", runTeam: "TEAM123"}, "Applications/A.app", yes) {
		t.Fatal("canSelfUpdate took a relative path")
	}
}

// cleanOldBundle also removes temp folders other runs left, never this
// process's own.
func TestCleanOldBundleRemovesStaleTemp(t *testing.T) {
	root := t.TempDir()
	running := filepath.Join(root, "Marstek Monitor.app")
	makeApp(t, running, "new")
	stale := filepath.Join(root, ".marstek-update-99999999")
	makeApp(t, filepath.Join(stale, "A.app"), "x")
	own := tempUpdateDir(running)
	makeApp(t, filepath.Join(own, "A.app"), "x")
	other := filepath.Join(root, "Other.app")
	makeApp(t, other, "other")
	if err := cleanOldBundle(running); err != nil {
		t.Fatal(err)
	}
	if exists(stale) || !exists(own) || !exists(other) || appTag(t, running) != "new" {
		t.Fatalf("stale %v, own %v, other %v", exists(stale), exists(own), exists(other))
	}
}
