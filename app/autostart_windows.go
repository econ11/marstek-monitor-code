package app

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const autostartValue = "MarstekMonitor"

// autostartKeyPath is the HKCU key holding the autostart value (a var so
// tests can use a throw-away key).
var autostartKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// uninstallKeyPath is the installer's HKCU «Програми й компоненти» entry
// (UNINST_KEY in build/windows/nsis/project.nsi); a var for tests.
var uninstallKeyPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\MarstekMonitor`

// SetAutostart adds or removes the per-user "start with Windows" entry. The
// app then starts with --background: tray only, no window.
func SetAutostart(enabled bool, exePath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autostartKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if enabled {
		return k.SetStringValue(autostartValue, `"`+exePath+`" --background`)
	}
	if err := k.DeleteValue(autostartValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// AutostartEnabled reports whether the autostart entry exists.
func AutostartEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autostartValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// InstalledDir is the folder the installer put the app in (InstallLocation of
// its uninstall entry), or "" when the app was never installed.
func InstalledDir() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	dir, _, err := k.GetStringValue("InstallLocation")
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return dir, err
}

// RepointAutostart makes an existing autostart entry start exe when exe runs
// from installDir (the installed app) and the entry starts something else,
// e.g. an older hand-copied exe that would otherwise win the single-instance
// guard at every login. With no entry (autostart off), outside installDir (a
// dev build or a hand-copied exe) or with no installDir it changes nothing.
// Never call it in -simulate* (Ruling 35).
func RepointAutostart(exe string, installDir string) (changed bool, err error) {
	if installDir == "" || !strings.EqualFold(filepath.Dir(filepath.Clean(exe)), filepath.Clean(installDir)) {
		return false, nil
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	cur, _, err := k.GetStringValue(autostartValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	want := `"` + exe + `" --background`
	if strings.EqualFold(cur, want) {
		return false, nil
	}
	return true, k.SetStringValue(autostartValue, want)
}
