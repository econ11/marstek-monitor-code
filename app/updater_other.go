//go:build !windows

package app

import "errors"

// TODO(macOS port, Task 6): updater_darwin.go brings the Mac startInstaller
// and canInstallHere (canSelfUpdate over the real macTools); this file's
// tag then becomes `!windows && !darwin`.

// startInstaller: the one-click update runs a Windows installer only (the
// macOS port brings its own).
func startInstaller(string) error { return errors.New("update: no installer on this platform") }

// canInstallHere: no installer here, so only «Завантажити» is offered.
func canInstallHere() bool { return false }
