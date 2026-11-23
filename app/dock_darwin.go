package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
void marstekSetDock(int shown, const void *png, int n);
*/
import "C"

import (
	"log/slog"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// dockPNG is the 256 px PNG frame of the app icon .ico of the state (the
// same artwork as the Windows taskbar button; the bundle's icons.icns is
// the "on" one).
func dockPNG(i appIcon) []byte {
	ico := windowICO(i)
	e, ok := pickICOImage(ico, 256)
	if !ok {
		return nil
	}
	return ico[e.offset : e.offset+e.length]
}

// setDock applies the Dock policy and, when shown, the icon, on the main
// thread. Any goroutine.
func setDock(p dockPolicy, i appIcon) {
	png := dockPNG(i)
	if png == nil {
		slog.Warn("dock icon: no PNG frame", "icon", i)
	}
	application.InvokeAsync(func() {
		var ptr unsafe.Pointer
		if len(png) > 0 {
			ptr = C.CBytes(png)
			defer C.free(ptr)
		}
		C.marstekSetDock(C.int(p), ptr, C.int(len(png)))
	})
}
