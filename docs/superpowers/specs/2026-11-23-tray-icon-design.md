# Tray / menu-bar icon in the app's identity — design (short)

Date: 2026-11-23 · Owner-approved in conversation · Size: small change

## Why
The current tray icon is a battery glyph with a charge level and 8 states (`app/trayicon/render.go`).
It is drawn differently from the app icon (H2 split bolt, `docs/design/logo/`). Laptop users also
confuse it with the laptop's own battery icon (the owner did on the Mac).

## Goals
- Windows tray icon = an exact copy of the taskbar/window icon states: **on** (grid present; also
  the default for unknown, connecting, no link and stale data) and **off** (no grid). This is the same
  rule as `iconForGrid` / `windowIcon` today.
- macOS menu bar: a monochrome **template** icon with 2 states: **filled bolt** = on (incl. unknown),
  **outline bolt** = off.
- The detailed state (charge %, charging, low charge, stale, no link) lives in the tooltip and the
  first (header) menu row, which already exist, plus the existing toasts.

## Non-goals / anti-goals
- No charge level, badges or numbers in the icon. Nothing that reads as a battery.
- No new states beyond on/off.

## Approach (the only sensible one)
Reuse the existing hand-hinted icon assets (16/20/24 px from the small masters, 32+ from the
vector masters, as in `appicon-on/off.ico`). Do not redraw them in code: re-drawing would lose the
hinting. A coloured icon on the Mac was rejected by the owner in favour of a template icon.

## Decided
- Windows: choose the size as the code does today (16 px design below 150 % scaling, 32 px from
  150 %). Pick the matching frame from the on/off ICO or PNG set. Switch it with the same grid-state
  rule as the window icon, and redraw only when the state or size changes. The taskbar light/dark
  theme no longer matters, because the app-icon tile works on both.
- Remove the battery renderer (`trayicon.Render`, `StateFor`, palette, dashed/badge drawing) and its
  tests. Keep `trayicon.Tooltip` and the tooltip/menu text.
- Mac: two new template masters from the same silhouette (filled, outline). Ship PNGs at 18 px and
  36 px (@2x), black plus alpha, and set them as template images. This work is part of
  the macOS port, which is in progress.

## Left to the builder
- How the tray gets the PNG for a given size (decode the ICO frame or ship PNG renders in
  `app/assets/`).
- How the Mac outline variant is drawn (stroke width at 18 px so it stays legible). Show the owner
  a preview before wiring.
