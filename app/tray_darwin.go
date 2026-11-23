package app

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// systemDPI on macOS: the menu bar is drawn at 2x on a Retina display, so the
// tray asks for the @2x image. Wails scales the image to the menu bar height.
func systemDPI() int { return 192 }

// The menu bar shows a monochrome template image instead of the coloured app
// icon (docs/superpowers/specs/2026-11-23-tray-icon-design.md): a filled
// split bolt when the grid is on (unknown included, as iconForGrid picks it),
// an outlined one when it is off. macOS tints a template image for the menu
// bar (black on a light bar, white on a dark one, dimmed when inactive), so
// there is no theme handling. Masters and generator: docs/design/logo/menubar-*.
//
//go:embed assets/menubar-on-22.png assets/menubar-on-44.png assets/menubar-off-22.png assets/menubar-off-44.png
var menubarFS embed.FS

// menubarPx is the menu-bar image for a tray size: 44 px (@2x of 22 pt) from
// 32 px up, as systemDPI asks for, 22 px below. 22 pt is the menu bar
// thickness Wails scales the image to, so it is shown 1:1; the bolt is
// 16 pt tall inside it (docs/design/logo/menubar_gen.go).
func menubarPx(px int) int {
	if px >= 32 {
		return 44
	}
	return 22
}

// trayImage is the menu-bar template PNG of i for px.
func trayImage(i appIcon, px int) ([]byte, error) {
	return menubarFS.ReadFile(fmt.Sprintf("assets/menubar-%s-%d.png", i, menubarPx(px)))
}

// setTrayImage shows b in the menu bar as a template image. Before the tray
// runs Wails keeps it (and the template flag) for the start.
func setTrayImage(st *application.SystemTray, b []byte) { st.SetTemplateIcon(b) }
