package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var procGetDpiForSystem = windows.NewLazySystemDLL("user32.dll").NewProc("GetDpiForSystem")

// systemDPI is the system DPI, 0 when GetDpiForSystem is unavailable.
func systemDPI() int {
	if procGetDpiForSystem.Find() != nil {
		return 0
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	return int(dpi)
}

// trayImage is the tray icon of i at px: the app icon's .ico frame.
func trayImage(i appIcon, px int) ([]byte, error) { return trayPNG(i, px) }

// setTrayImage shows b in the notification area.
func setTrayImage(st *application.SystemTray, b []byte) { st.SetIcon(b) }
