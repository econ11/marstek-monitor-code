package app

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// The Mac one-click update (Task 6): pure logic over injected commands, so
// it builds and is tested on every platform. updater_darwin.go supplies the
// real macTools (hdiutil, ditto, codesign) and calls macInstall from
// startInstaller.

// macTools are the commands the Mac update runs.
type macTools interface {
	Attach(dmg string) (mount string, err error) // hdiutil attach -nobrowse -readonly
	Detach(mount string) error
	Copy(src, dst string) error        // ditto
	Verify(app string) error           // codesign --verify --deep --strict
	TeamID(app string) (string, error) // "" for ad-hoc/unsigned
}

// renameFile is os.Rename; tests swap it to fail a step of the swap.
var renameFile = os.Rename

// macInstall installs the app inside dmg over runningApp (a .app bundle):
//  1. it mounts the dmg and takes the one *.app at its root;
//  2. it copies that app into <parent of runningApp>/.marstek-update-<pid>/
//     (the same volume, so the swap is a rename);
//  3. the copy must pass Verify, and its TeamID must equal the running
//     app's, which must not be empty;
//  4. the running app becomes <runningApp>.old and the copy takes its path;
//     if that second rename fails, .old is renamed back; if that fails too,
//     the new one is tried once more, and if that fails both bundles stay
//     where they are (the temp folder is kept) and the error names them;
//  5. the dmg is detached (on every return once mounted) and the temp
//     folder removed.
//
// runningApp must be absolute. It returns the new app's path (runningApp).
// The .old bundle is removed on the next start (cleanOldBundle). On any
// error the running app is in place, except that last case, where neither
// bundle is lost.
func macInstall(t macTools, dmg, runningApp string) (newApp string, err error) {
	if !isAbsPath(runningApp) {
		return "", fmt.Errorf("update: the running app path %q is not absolute", runningApp)
	}
	runningApp = filepath.Clean(runningApp)
	mount, err := t.Attach(dmg)
	if err != nil {
		return "", fmt.Errorf("mount the update: %w", err)
	}
	tmp := updateTempPath(runningApp)
	keepTmp := false // set when tmp holds the only copy of the new bundle
	defer func() {
		if err := t.Detach(mount); err != nil {
			slog.Warn("update: detach the dmg", "err", err)
		}
		if keepTmp {
			return
		}
		if err := os.RemoveAll(tmp); err != nil {
			slog.Warn("update: remove the temp folder", "err", err)
		}
	}()

	src, err := dmgApp(mount)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(tmp); err != nil {
		return "", fmt.Errorf("clear the temp folder: %w", err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", fmt.Errorf("create the temp folder: %w", err)
	}
	next := filepath.Join(tmp, filepath.Base(src))
	if err := t.Copy(src, next); err != nil {
		return "", fmt.Errorf("copy the new app: %w", err)
	}
	if err := t.Verify(next); err != nil {
		return "", fmt.Errorf("verify the new app: %w", err)
	}
	want, err := t.TeamID(runningApp)
	if err != nil {
		return "", fmt.Errorf("team id of the running app: %w", err)
	}
	got, err := t.TeamID(next)
	if err != nil {
		return "", fmt.Errorf("team id of the new app: %w", err)
	}
	if want == "" || got != want {
		return "", fmt.Errorf("team id: the new app has %q, the running app %q", got, want)
	}

	old := runningApp + ".old"
	if err := os.RemoveAll(old); err != nil { // left by an earlier update
		return "", fmt.Errorf("remove the old bundle: %w", err)
	}
	if err := renameFile(runningApp, old); err != nil {
		return "", fmt.Errorf("move the running app aside: %w", err)
	}
	if err := renameFile(next, runningApp); err != nil {
		rerr := renameFile(old, runningApp)
		if rerr == nil {
			return "", fmt.Errorf("move the new app into place: %w", err)
		}
		// Neither bundle is at runningApp. Try the new one once more; if
		// that fails too, keep both where they are (tmp is not removed) and
		// say where they are.
		err2 := renameFile(next, runningApp)
		if err2 == nil {
			slog.Warn("update: the new app moved into place on the retry", "first", err, "restore", rerr)
			return runningApp, nil
		}
		keepTmp = true
		slog.Error("update: the swap failed, no app at the running path",
			"running", runningApp, "old", old, "new", next, "err", err, "restore", rerr, "retry", err2)
		return "", fmt.Errorf("update: no app at %s: the old one is at %s, the new one at %s (move: %v; restore: %v; retry: %v)",
			runningApp, old, next, err, rerr, err2)
	}
	return runningApp, nil
}

// updateTempPath is this process's temp folder beside runningApp.
func updateTempPath(runningApp string) string {
	return filepath.Join(filepath.Dir(runningApp), fmt.Sprintf("%s%d", updateTempPrefix, os.Getpid()))
}

// updateTempPrefix names the temp folders macInstall copies into.
const updateTempPrefix = ".marstek-update-"

// isAbsPath: an absolute path. filepath.IsAbs, or rooted with "/" (the Mac
// form, so the tests also run on Windows).
func isAbsPath(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(filepath.ToSlash(p), "/")
}

// dmgApp is the one *.app at the root of mount.
func dmgApp(mount string) (string, error) {
	entries, err := os.ReadDir(mount)
	if err != nil {
		return "", fmt.Errorf("read the dmg: %w", err)
	}
	var apps []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			apps = append(apps, filepath.Join(mount, e.Name()))
		}
	}
	if len(apps) != 1 {
		return "", errors.New("update: the dmg must hold exactly one app, found " + fmt.Sprint(len(apps)))
	}
	return apps[0], nil
}

// canSelfUpdate: the running app can replace itself. Not when it runs
// translocated (Gatekeeper's read-only copy of an app run from Downloads),
// from a mounted dmg (/Volumes/), from a folder it cannot write, or without
// a Team ID (ad-hoc or unsigned: no update could match it). Then only
// «Завантажити» is offered.
func canSelfUpdate(t macTools, runningApp string, writable func(dir string) bool) bool {
	if !isAbsPath(runningApp) {
		return false
	}
	// /Volumes/ also covers apps on an external disk: they get «Завантажити»
	// by design (a mounted dmg and an external disk look the same here).
	if isTranslocated(runningApp) || strings.HasPrefix(filepath.ToSlash(runningApp), "/Volumes/") {
		return false
	}
	if !writable(filepath.Dir(filepath.Clean(runningApp))) {
		return false
	}
	team, err := t.TeamID(runningApp)
	return err == nil && team != ""
}

// isTranslocated: path is under App Translocation.
func isTranslocated(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/AppTranslocation/")
}

// cleanOldBundle removes <runningApp>.old, the bundle an update swapped out,
// and the temp folders (.marstek-update-<pid>) other runs left beside it;
// never this process's own. Nothing if there are none. The darwin start
// calls it.
func cleanOldBundle(runningApp string) error {
	runningApp = filepath.Clean(runningApp)
	errs := []error{os.RemoveAll(runningApp + ".old")}
	entries, err := os.ReadDir(filepath.Dir(runningApp))
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	own := filepath.Base(updateTempPath(runningApp))
	for _, e := range entries {
		if n := e.Name(); e.IsDir() && strings.HasPrefix(n, updateTempPrefix) && n != own {
			errs = append(errs, os.RemoveAll(filepath.Join(filepath.Dir(runningApp), n)))
		}
	}
	return errors.Join(errs...)
}
