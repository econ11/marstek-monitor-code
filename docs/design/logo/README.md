# Marstek Monitor app icon (H2)

Unofficial third-party app; the icon is an original split-bolt mark and contains no Marstek logo or wordmark.

## States
- **on** = grid present: white blades, green seam. Also the default: use it when the grid state is unknown.
- **off** = no grid: graphite upper blade, amber seam, amber lower blade.

Both states share one silhouette.

## Files
| File | Use |
|---|---|
| `appicon-on.svg`, `appicon-off.svg` | Vector masters, 1024 artboard (viewBox 256). Source for 32 px and up. |
| `appicon-on-small.svg`, `appicon-off-small.svg` | Flat hinted masters. Source for 16, 20 and 24 px. |
| `appicon-{on,off}-{16,20,24,32,40,48,64,128,256,512,1024}.png` | PNG renders with alpha. 16 to 24 come from the small masters. |
| `appicon-on.ico`, `appicon-off.ico` | Multi-size: 16, 20, 24, 32, 40, 48, 64 as 32-bit BMP, 256 as PNG. |
| `toast-on.png`, `toast-off.png` | 96x96, for toast `appLogoOverride`. |
| `app/assets/dock-on-1024.png`, `app/assets/dock-off-1024.png` | macOS Dock images while the window is open. `on` is a copy of `build/appicon.png`; `off` is `appicon-off.svg` rendered at 1024 px with alpha by headless Chrome, the renderer that matches `build/appicon.png` (mean difference 0.09/255 when `appicon-on.svg` is rendered the same way): an HTML page with `<img src="appicon-off.svg" width="1024" height="1024">` on a transparent body, `--headless=new --default-background-color=00000000 --force-device-scale-factor=1 --window-size=1024,1024 --screenshot=…`. |
| `menubar-on.svg`, `menubar-off.svg` | macOS menu-bar template masters (22 pt artboard, 16 pt bolt): filled bolt = on, outlined = off. Generated with `go run docs/design/logo/menubar_gen.go`, which also writes `app/assets/menubar-{on,off}-{22,44}.png` (black + alpha). |

## Where each goes
- **Exe resource, installer, Start menu shortcut**: `appicon-on.ico` (static).
- **Window and taskbar icon**: switch between `appicon-on.ico` and `appicon-off.ico` with the grid state; unknown = on.
- **Toasts**: `toast-on.png` or `toast-off.png` to match the event.
- **Tray icon**: same as the window icon on Windows; on macOS the menu-bar template icon `menubar-on` or `menubar-off` (same rule: unknown = on).
- **macOS Dock** (only while the window is open): `dock-on-1024.png` or `dock-off-1024.png`, same rule; drawn on the macOS icon grid (tile 824 of 1024).

## Colours
Tile #2B323C to #0B0E12, white #FFFFFF, green #4FCB8D, amber #F2B63D, graphite #737D8B to #4A525E.
