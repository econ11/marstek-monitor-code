package app

import "github.com/wailsapp/wails/v3/pkg/application"

// windowIcon on macOS: a window has no icon of its own; the Dock icon is the
// bundle's icons.icns ("on").
//
// TODO(macos): switch the Dock icon with the grid state
// (NSApp.applicationIconImage from windowICO's PNG, on the main thread).
type windowIcon struct{}

func newWindowIcon(*application.WebviewWindow) *windowIcon { return &windowIcon{} }

// set does nothing yet (see the TODO above). Any goroutine.
func (*windowIcon) set(appIcon) {}
