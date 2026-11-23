package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// windowIcon on macOS is the Dock icon (dock.go): a window has no icon of
// its own. The Dock icon shows only while the window is open, with the
// grid state; the menu-bar icon is always there, so the app is never
// invisible.
type windowIcon struct{ d *dock }

func newWindowIcon(*application.WebviewWindow) *windowIcon {
	w := &windowIcon{d: newDock(setDock)}
	// NSApp exists from here on; until then the state is only recorded.
	application.Get().Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		w.d.start()
	})
	return w
}

// set shows the icon of the grid state (in the Dock while the window is
// open). Any goroutine.
func (w *windowIcon) set(i appIcon) { w.d.setIcon(i) }

// setOpen shows the Dock icon while the window is open and hides it when
// the window is closed to the menu bar. Any goroutine.
func (w *windowIcon) setOpen(open bool) { w.d.setOpen(open) }
