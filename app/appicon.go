package app

import (
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows"
)

// The app icon has two states (docs/design/logo/README.md). The exe carries
// "on" (build/windows/icon.ico); the window, the taskbar button and the
// toasts switch between the two.

// appIcon is the state the app icon shows.
type appIcon string

const (
	iconOn  appIcon = "on"  // grid present; also the default when unknown
	iconOff appIcon = "off" // no grid
)

//go:embed assets/appicon-on.ico assets/appicon-off.ico assets/toast-on.png assets/toast-off.png
var iconAssets embed.FS

func mustAsset(name string) []byte {
	b, err := iconAssets.ReadFile("assets/" + name)
	if err != nil {
		panic(err) // embedded: cannot happen
	}
	return b
}

// windowICO is the multi-size .ico of the state.
func windowICO(i appIcon) []byte { return mustAsset("appicon-" + string(i) + ".ico") }

// toastPNG is the 96 px toast logo of the state.
func toastPNG(i appIcon) []byte { return mustAsset("toast-" + string(i) + ".png") }

// iconForGrid: only a confirmed "no grid" shows the off icon.
func iconForGrid(g detect.GridState) appIcon {
	if g == detect.GridOff {
		return iconOff
	}
	return iconOn
}

// iconForToast: the toasts about trouble carry the off logo.
func iconForToast(k notify.Kind) appIcon {
	switch k {
	case notify.GridLost, notify.LikelyDead, notify.LowCharge:
		return iconOff
	}
	return iconOn
}

// writeToastLogos writes both toast logos into dir and returns their file
// URIs. Toast XML can only point at an image, not carry it.
func writeToastLogos(dir string) (map[appIcon]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	out := make(map[appIcon]string, 2)
	for _, i := range []appIcon{iconOn, iconOff} {
		path := filepath.Join(dir, "toast-"+string(i)+".png")
		if err := os.WriteFile(path, toastPNG(i), 0o644); err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		out[i] = (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(abs)}).String()
	}
	return out, nil
}

// icoEntry is one image of an .ico file.
type icoEntry struct {
	size           int // width in px (square icons)
	offset, length int
}

// icoEntries lists the images of an .ico file; nil if it is not one or an
// entry points outside the file.
func icoEntries(ico []byte) []icoEntry {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[0:]) != 0 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	if n == 0 || len(ico) < 6+16*n {
		return nil
	}
	out := make([]icoEntry, 0, n)
	for k := 0; k < n; k++ {
		e := ico[6+16*k:]
		size := int(e[0])
		if size == 0 {
			size = 256
		}
		l, off := int(binary.LittleEndian.Uint32(e[8:])), int(binary.LittleEndian.Uint32(e[12:]))
		if l <= 0 || off < 0 || off > len(ico) || l > len(ico)-off {
			return nil
		}
		out = append(out, icoEntry{size: size, offset: off, length: l})
	}
	return out
}

// pickICOImage picks the image drawn for px: the exact size, else the
// smallest larger one (scaled down), else the largest.
func pickICOImage(ico []byte, px int) (icoEntry, bool) {
	var best icoEntry
	found := false
	for _, e := range icoEntries(ico) {
		switch {
		case !found:
			best, found = e, true
		case best.size < px && e.size > best.size: // still too small: grow
			best = e
		case e.size >= px && e.size < best.size: // big enough and closer
			best = e
		}
	}
	return best, found
}

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
