package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
void marstekSetDock(int shown, const void *png, int n);
*/
import "C"

import (
	"embed"
	"log/slog"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The Dock images are 1024 px renders of the app icon: "on" is
// build/appicon.png (the bundle's icons.icns comes from it), "off" is
// appicon-off.svg rendered the same way (docs/design/logo/README.md). A
// 256 px source looked soft in a Retina Dock.
//
//go:embed assets/dock-on-1024.png assets/dock-off-1024.png
var dockFS embed.FS

// dockPNG is the Dock image of the state, nil if missing.
func dockPNG(i appIcon) []byte {
	b, err := dockFS.ReadFile("assets/dock-" + string(i) + "-1024.png")
	if err != nil {
		return nil
	}
	return b
}

// setDock applies the Dock policy and, when shown, the icon, on the main
// thread. It reports false when the icon cannot be shown (no image), so
// dock keeps trying. Any goroutine.
func setDock(p dockPolicy, i appIcon) bool {
	var png []byte
	if p == dockShown {
		if png = dockPNG(i); png == nil {
			slog.Warn("dock icon: no image", "icon", i)
			return false
		}
	}
	application.InvokeAsync(func() {
		var ptr unsafe.Pointer
		if len(png) > 0 {
			ptr = C.CBytes(png)
			defer C.free(ptr)
		}
		C.marstekSetDock(C.int(p), ptr, C.int(len(png)))
	})
	return true
}
