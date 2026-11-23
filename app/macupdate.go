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
//     if that second rename fails, .old is renamed back;
//  5. the dmg is detached (on every return once mounted) and the temp
//     folder removed.
//
// It returns the new app's path (runningApp). The .old bundle is removed on
// the next start (cleanOldBundle). On any error the running app is in place.
func macInstall(t macTools, dmg, runningApp string) (newApp string, err error) {
	runningApp = filepath.Clean(runningApp)
	mount, err := t.Attach(dmg)
	if err != nil {
		return "", fmt.Errorf("mount the update: %w", err)
	}
	tmp := filepath.Join(filepath.Dir(runningApp), fmt.Sprintf(".marstek-update-%d", os.Getpid()))
	defer func() {
		if err := t.Detach(mount); err != nil {
			slog.Warn("update: detach the dmg", "err", err)
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
		if rerr := renameFile(old, runningApp); rerr != nil {
			return "", fmt.Errorf("move the new app into place: %w (and restoring the old one: %v)", err, rerr)
		}
		return "", fmt.Errorf("move the new app into place: %w", err)
	}
	return runningApp, nil
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

// cleanOldBundle removes <runningApp>.old, the bundle an update swapped out
// (nothing if there is none). The darwin start calls it.
func cleanOldBundle(runningApp string) error {
	return os.RemoveAll(filepath.Clean(runningApp) + ".old")
}
