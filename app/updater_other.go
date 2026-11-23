//go:build !windows

package app

import "errors"

// startInstaller: the one-click update runs a Windows installer only (the
// macOS port brings its own).
func startInstaller(string) error { return errors.New("update: no installer on this platform") }
