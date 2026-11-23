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
| `menubar-on.svg`, `menubar-off.svg` | macOS menu-bar template masters (22 pt artboard, 16 pt bolt): filled bolt = on, outlined = off. Generated with `go run docs/design/logo/menubar_gen.go`, which also writes `app/assets/menubar-{on,off}-{22,44}.png` (black + alpha). |

## Where each goes
- **Exe resource, installer, Start menu shortcut**: `appicon-on.ico` (static).
- **Window and taskbar icon**: switch between `appicon-on.ico` and `appicon-off.ico` with the grid state; unknown = on.
- **Toasts**: `toast-on.png` or `toast-off.png` to match the event.
- **Tray icon**: same as the window icon on Windows; on macOS the menu-bar template icon `menubar-on` or `menubar-off` (same rule: unknown = on).

## Colours
Tile #2B323C to #0B0E12, white #FFFFFF, green #4FCB8D, amber #F2B63D, graphite #737D8B to #4A525E.
