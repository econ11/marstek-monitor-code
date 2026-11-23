package app

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNewIconPair(t *testing.T) {
	for _, i := range []appIcon{iconOn, iconOff} {
		for _, dpi := range []int{96, 120, 144, 192} {
			p, err := newIconPair(windowICO(i), dpi)
			if err != nil {
				t.Fatalf("%s @%d: %v", i, dpi, err)
			}
			// 16/32 px at 100 %, 20/40 at 125 %, 24/48 at 150 %, 32/64 at 200 %.
			small, big := dpi/6, dpi/3
			if got := iconWidth(t, p.small); got != small {
				t.Errorf("%s @%d: small icon %d px, want %d", i, dpi, got, small)
			}
			if got := iconWidth(t, p.big); got != big {
				t.Errorf("%s @%d: big icon %d px, want %d", i, dpi, got, big)
			}
			p.destroy()
		}
	}
	if _, err := newIconPair([]byte("not an icon"), 96); err == nil {
		t.Error("newIconPair accepted a broken file")
	}
}

var (
	procGetIconInfo  = windows.NewLazySystemDLL("user32.dll").NewProc("GetIconInfo")
	procGetObject    = windows.NewLazySystemDLL("gdi32.dll").NewProc("GetObjectW")
	procDeleteObject = windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject")
)

// iconWidth reads the width of the icon's bitmaps.
func iconWidth(t *testing.T, h windows.Handle) int {
	t.Helper()
	if h == 0 {
		t.Fatal("no icon")
	}
	var ii struct {
		fIcon             int32
		xHot, yHot        uint32
		hbmMask, hbmColor uintptr
	}
	if r, _, err := procGetIconInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&ii))); r == 0 {
		t.Fatalf("GetIconInfo: %v", err)
	}
	defer procDeleteObject.Call(ii.hbmMask)
	defer procDeleteObject.Call(ii.hbmColor)
	var bm struct {
		typ, width, height, widthBytes int32
		planes, bitsPixel              uint16
		bits                           uintptr
	}
	if r, _, err := procGetObject.Call(ii.hbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		t.Fatalf("GetObject: %v", err)
	}
	return int(bm.width)
}

// Task 20c (icon review): an icon that cannot be made is retried with every
// snapshot, but logged once, not at the poll rate.
func TestWindowIconFailureLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	w := &windowIcon{want: iconOff, newPair: func([]byte, int) (iconPair, error) {
		return iconPair{}, errors.New("CreateIconFromResourceEx failed")
	}}
	for i := 0; i < 3; i++ {
		w.show(1, 96)
	}
	if n := strings.Count(buf.String(), "window icon"); n != 1 {
		t.Fatalf("log has %d window icon entries, want 1:\n%s", n, buf.String())
	}
	if w.shown == iconOff {
		t.Fatal("a failed icon counts as shown")
	}
}
