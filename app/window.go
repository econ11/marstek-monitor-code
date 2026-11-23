package app

import (
	"sync/atomic"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Window is the main window. Closing it hides it and monitoring goes on in
// the tray (flows 5.2), unless the user turned that off (SetCloseToTray):
// then closing it quits the app.
type Window struct {
	win         *application.WebviewWindow
	icon        *windowIcon
	closeToTray atomic.Bool
}

// NewWindow creates the main window; hidden starts it in the tray only
// (--background). onHide runs, off the UI thread, each time the user closes
// the window.
func NewWindow(a *application.App, hidden bool, onHide func()) *Window {
	win := a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Marstek Monitor",
		// The hi-fi design is drawn for 1100 x 720 (title bar included); below
		// 720 px the Overview no longer fits (flows 8).
		Width:            1100,
		Height:           720,
		MinWidth:         720,
		MinHeight:        480,
		URL:              "/",
		BackgroundColour: application.NewRGB(14, 17, 21), // --color-bg-page
		Hidden:           hidden,
		Windows:          application.WindowsWindow{Theme: application.Dark},
	})
	w := &Window{win: win, icon: newWindowIcon(win)}
	w.icon.setOpen(!hidden) // macOS: the Dock icon only while the window is open
	w.closeToTray.Store(true)
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		if !w.closeToTray.Load() {
			go a.Quit() // off the UI thread: Quit closes the windows
			return
		}
		win.Hide()
		w.icon.setOpen(false)
		if onHide != nil {
			go onHide()
		}
	})
	return w
}

// SetCloseToTray sets what closing the window does: hide it to the tray (on,
// the default) or quit the app (Settings MinimizeToTray off). Any goroutine.
func (w *Window) SetCloseToTray(on bool) { w.closeToTray.Store(on) }

// HWND is the native window handle (0 before the window exists), e.g. the
// owner of a UAC prompt.
func (w *Window) HWND() uintptr {
	return uintptr(w.win.NativeWindow())
}

// Show shows, restores and focuses the window. It never hides it (flows 5.1:
// the tray click is not a toggle).
func (w *Window) Show() {
	w.icon.setOpen(true) // macOS: the Dock icon comes first, then the window
	w.win.Show()
	w.win.UnMinimise()
	w.win.Focus()
}

// SetGrid shows the app icon of the grid state in the title bar and on the
// taskbar: off without grid, on otherwise (unknown included). Any goroutine.
func (w *Window) SetGrid(g detect.GridState) {
	w.icon.set(iconForGrid(g))
}
