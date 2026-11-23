package app

import (
	"embed"
	"encoding/binary"
	"net/url"
	"os"
	"path/filepath"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/notify"
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
