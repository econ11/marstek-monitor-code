//go:build !windows && !darwin

package app

import "errors"

// startInstaller: the one-click update exists on Windows (the installer) and
// macOS (the bundle swap, updater_darwin.go) only.
func startInstaller(string) error { return errors.New("update: no installer on this platform") }

// canInstallHere: no installer here, so only «Завантажити» is offered.
func canInstallHere() bool { return false }

// cleanAfterUpdate: nothing to clean on this platform.
func cleanAfterUpdate() {}
