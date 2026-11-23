package app

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
)

// autostartLabel is the launchd label and the plist name (the bundle id).
const autostartLabel = "com.econ11.marstekmonitor"

// autostartDir is the folder of per-user LaunchAgents (a var so tests can
// use a throw-away folder).
var autostartDir = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func autostartPlist() (string, error) {
	dir, err := autostartDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, autostartLabel+".plist"), nil
}

// SetAutostart adds or removes the per-user "open at login" LaunchAgent. The
// app then starts with --background: menu bar only, no window.
//
// A LaunchAgent and not SMAppService (what Wails' Autostart picks for a
// bundled app): SMAppService.mainApp cannot pass --background. macOS lists
// the agent in System Settings → General → Login Items under the
// executable's name, and notifies "Background item added" once.
func SetAutostart(enabled bool, exePath string) error {
	path, err := autostartPlist()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, launchAgentPlist(exePath), 0o644)
}

// AutostartEnabled reports whether the LaunchAgent exists.
func AutostartEnabled() (bool, error) {
	path, err := autostartPlist()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// launchAgentPlist: run exe --background once at login, no restarts.
func launchAgentPlist(exe string) []byte {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + autostartLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc(exe) + `</string>
		<string>--background</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
`)
}

// InstalledDir: macOS has no installer and no uninstall entry; the app is a
// bundle the user drags to a folder. "" means "never installed", for which
// RepointAutostart changes nothing (its contract on Windows).
func InstalledDir() (string, error) { return "", nil }

// RepointAutostart changes nothing on macOS: the LaunchAgent names the
// executable inside the bundle, and the one-click update (macOS port, Task 6)
// replaces the bundle at the same path.
func RepointAutostart(exe string, installDir string) (changed bool, err error) { return false, nil }
