//go:build !darwin

package app

// setDock: only macOS has a Dock (dock.go).
func setDock(dockPolicy, appIcon) {}
