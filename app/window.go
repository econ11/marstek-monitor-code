package app

import (
	"sync/atomic"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows"
)

// Instance is the single-instance guard: a named mutex plus an event the
// second copy signals to make the first one show its window (flows 5.2).
type Instance struct {
	event windows.Handle
}

// FirstInstance takes the mutex `Local\<name>`. When another copy already
// holds it, that copy is asked to show its window and ok is false: the caller
// must exit. The handles live until the process ends.
func FirstInstance(name string) (inst *Instance, ok bool, err error) {
	mutexName, err := windows.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, false, err
	}
	eventName, err := windows.UTF16PtrFromString(`Local\` + name + `.show`)
	if err != nil {
		return nil, false, err
	}
	_, mErr := windows.CreateMutex(nil, false, mutexName)
	if mErr != nil && mErr != windows.ERROR_ALREADY_EXISTS {
		return nil, false, mErr
	}
	// Auto-reset event; CreateEvent opens it if the other copy made it first.
	ev, eErr := windows.CreateEvent(nil, 0, 0, eventName)
	if ev == 0 {
		return nil, false, eErr
	}
	if mErr == windows.ERROR_ALREADY_EXISTS {
		// This process was started by the user, so it may hand the right to
		// take the foreground to the first copy.
		_, _, _ = procAllowSetForegroundWindow.Call(asfwAny)
		err = windows.SetEvent(ev)
		_ = windows.CloseHandle(ev)
		return nil, false, err
	}
	return &Instance{event: ev}, true, nil
}

// OnSecondLaunch calls fn each time another copy of the app is started.
func (i *Instance) OnSecondLaunch(fn func()) {
	go func() {
		for {
			if r, err := windows.WaitForSingleObject(i.event, windows.INFINITE); err != nil || r != windows.WAIT_OBJECT_0 {
				return
			}
			fn()
		}
	}()
}

const asfwAny = ^uintptr(0) // ASFW_ANY

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

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
	w.closeToTray.Store(true)
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		if !w.closeToTray.Load() {
			go a.Quit() // off the UI thread: Quit closes the windows
			return
		}
		win.Hide()
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
	w.win.Show()
	w.win.UnMinimise()
	w.win.Focus()
}

// SetGrid shows the app icon of the grid state in the title bar and on the
// taskbar: off without grid, on otherwise (unknown included). Any goroutine.
func (w *Window) SetGrid(g detect.GridState) {
	w.icon.set(iconForGrid(g))
}
