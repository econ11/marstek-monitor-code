package app

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	procSendMessage              = user32.NewProc("SendMessageW")
)

const (
	wmSetIcon = 0x0080 // WM_SETICON
	iconSmall = 0      // ICON_SMALL: title bar
	iconBig   = 1      // ICON_BIG: taskbar, Alt+Tab
)

// iconPair is the title-bar and taskbar icon of one state at one DPI.
type iconPair struct{ small, big windows.Handle }

// newIconPair makes both icons from the .ico for the given DPI: 16/32 px at
// 96 dpi, scaled with it, each from the drawing made for that size.
func newIconPair(ico []byte, dpi int) (iconPair, error) {
	small, err := newIcon(ico, dpi/6)
	if err != nil {
		return iconPair{}, err
	}
	big, err := newIcon(ico, dpi/3)
	if err != nil {
		destroyIcon(small)
		return iconPair{}, err
	}
	return iconPair{small: small, big: big}, nil
}

func newIcon(ico []byte, px int) (windows.Handle, error) {
	e, ok := pickICOImage(ico, px)
	if !ok {
		return 0, errors.New("app icon: not an .ico file")
	}
	img := ico[e.offset : e.offset+e.length]
	h, _, err := procCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&img[0])), uintptr(len(img)),
		1, 0x00030000, uintptr(px), uintptr(px), 0) // icon, version 3, LR_DEFAULTCOLOR
	runtime.KeepAlive(img)
	if h == 0 {
		return 0, fmt.Errorf("CreateIconFromResourceEx(%d px): %w", px, err)
	}
	return windows.Handle(h), nil
}

func destroyIcon(h windows.Handle) {
	if h != 0 {
		_, _, _ = procDestroyIcon.Call(uintptr(h))
	}
}

func (p iconPair) destroy() {
	destroyIcon(p.small)
	destroyIcon(p.big)
}

// windowIcon switches the window's title-bar and taskbar icon with the grid
// state. The icons are swapped on the UI thread, and only when the state or
// the window's DPI differs from what is shown.
type windowIcon struct {
	win *application.WebviewWindow

	mu          sync.Mutex
	want, shown appIcon
	shownDPI    int
	icons       iconPair // ours, destroyed once replaced
	// failLogged: a failure to make the icons is logged once until they are
	// made again (set retries with every snapshot).
	failLogged bool
	newPair    func(ico []byte, dpi int) (iconPair, error) // for tests; nil = newIconPair
}

func newWindowIcon(win *application.WebviewWindow) *windowIcon {
	w := &windowIcon{win: win, want: iconOn}
	// Another monitor, another icon size.
	win.OnWindowEvent(events.Windows.WindowDPIChanged, func(*application.WindowEvent) {
		application.InvokeAsync(w.apply)
	})
	return w
}

// set shows the icon of the state. Any goroutine.
func (w *windowIcon) set(i appIcon) {
	w.mu.Lock()
	stale := w.shown != i
	w.want = i
	w.mu.Unlock()
	if stale {
		application.InvokeAsync(w.apply)
	}
}

// apply runs on the UI thread.
func (w *windowIcon) apply() {
	hwnd := uintptr(w.win.NativeWindow())
	if hwnd == 0 {
		return // not created yet: the next snapshot tries again
	}
	dpi := 96
	if procGetDpiForWindow.Find() == nil {
		if d, _, _ := procGetDpiForWindow.Call(hwnd); d != 0 {
			dpi = int(d)
		}
	}
	w.show(hwnd, dpi)
}

// show sets the icons of the wanted state on the window hwnd at dpi, unless
// they are shown already. UI thread.
func (w *windowIcon) show(hwnd uintptr, dpi int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.want == w.shown && dpi == w.shownDPI {
		return
	}
	newPair := w.newPair
	if newPair == nil {
		newPair = newIconPair
	}
	p, err := newPair(windowICO(w.want), dpi)
	if err != nil {
		if !w.failLogged {
			w.failLogged = true
			slog.Warn("window icon", "err", err)
		}
		return
	}
	w.failLogged = false
	_, _, _ = procSendMessage.Call(hwnd, wmSetIcon, iconSmall, uintptr(p.small))
	_, _, _ = procSendMessage.Call(hwnd, wmSetIcon, iconBig, uintptr(p.big))
	w.icons.destroy() // the window no longer uses them
	w.icons, w.shown, w.shownDPI = p, w.want, dpi
}

// setOpen: on Windows the taskbar button comes and goes with the window by
// itself (the Dock rule is macOS only, dock.go).
func (w *windowIcon) setOpen(bool) {}
