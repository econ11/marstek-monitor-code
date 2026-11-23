# Marstek Monitor: hi-fi visual design

Date: 2026-11-21. Status: design proposal on top of the approved spec
(`../2026-11-21-marstek-monitor-design.md`, §5). This covers **visual quality** only. Content and
layout decisions marked [вирішено] in the spec are unchanged. Where I suggest a content change, it is
listed under "Suggestions" below and is **not** applied in the mockups unless it says so.

Open `index.html` in a browser (Chromium/Edge = WebView2). Every page is a self-contained document.

| File | What |
|---|---|
| `index.html` | Contents, colour rule, neutrals, type specimen, gauge anatomy, small components |
| `01-overview.html` | Overview: discharging with no grid; variants of the top row: charging, idle, low charge, stale data. The SOC chart is live (24 h / 7 d / 30 d, crosshair tooltip) |
| `02-outages.html` | Period switcher (works), 2 gauges + stats, day bars with tooltips, click-to-filter table, calendar popover |
| `03-errors.html` | Log with a working filter, level pills, code tokens |
| `04-battery.html` | 3 gauges, monthly capacity line chart |
| `05-energy.html` | Totals, grouped charge/discharge bars, PV gauge variant |
| `06-stations.html` | Station switcher open + "Додати станцію" dialog |
| `07-settings.html` | Settings → Сповіщення |
| `08-tray-toasts.html` | Tray icons: 7 states (incl. new STALE and CONNECTING) × 16/32 px × dark/light taskbar, live and stale tooltips, tray menu with the «Не турбувати» submenu (open + active), 8 toasts incl. «Станція, ймовірно, розрядилась» |
| `09-states.html` | Overview per connection state (flows §2.6): CONNECTING skeletons, STALE, NO_LINK + diagnosis banners, LIKELY_DEAD (live), return after a gap with the «Поки додаток не працював» card; Outages fragment: hatched no-data day vs zero day, ≥ / ≈ durations |
| `10-first-run.html` | First run (flows §1): NONE_FOUND, LAN_GUIDE, CLOUD_INCOMPATIBLE, DONE |
| `11-dialogs.html` | Remove station (archive by default, flows M3/Q1) + undo bar; SMARTSCREEN_BRIEF before installing an update |
| `tokens.css` | **Source of truth** for tokens (primitive + semantic layers) |
| `_src/` | `components.css`, `app.js` (tooltip engine, segmented control), `data.js` (sample-data simulator), `soc-chart.js` (one SOC chart renderer for every connection state), `pages/`, `build.mjs` |

Rebuild after editing `_src/` or `tokens.css`: `node docs/superpowers/specs/2026-11-21-marstek-monitor-design-hifi/_src/build.mjs`.
The build inlines tokens, components and scripts into each page and expands `<!--@gauge …-->`
and similar macros. **`build.mjs` → `gauge()` is the canonical markup for the ring gauge.**

---

## 1. Visual direction

**"A night instrument, not a dashboard."** Blue-graphite darkness (never pure black), one steady
rhythm of identical rings, and four signal colours that each mean exactly one thing. The screen is
built to be read in one look over someone's shoulder during a blackout. The three questions are
whether there is power, how much charge is left and when the station will shut down. Hierarchy
comes from size and position, with no decoration. Separation comes from lightness steps and 1 px
hairlines; shadows appear only on floating layers.

**The one signature:** the **outage rail**, a 2 px amber line under the tab bar that stays on for
as long as the grid is down. The window title also shows "Без світла 3 год 27 хв". It carries no
text, never animates and can be seen from across the room.

**Type:** **IBM Plex Sans** (400/500/600) + **IBM Plex Mono** (IP addresses, error codes). Plex has
a complete Ukrainian Cyrillic set (ґ є і ї), engineered tabular figures and an "instrument" voice
that is neither friendly-rounded nor corporate. Fallbacks are Segoe UI Variable Text → Segoe UI.
`font-feature-settings: "tnum"` is on globally because values update live and must not jitter.

> **Bundle the fonts.** The app runs during outages, often with no internet. Ship Plex as local
> woff2 (Cyrillic + Latin subsets, ~4 files, ~120 KB). The Google Fonts link exists only in the
> mockups.

## 2. Colour rule (tokens)

| Token | Hex | Means | Never used for |
|---|---|---|---|
| `--color-good` | `#4FCB8D` | charge level OK, charging, grid present, no errors | decoration, buttons |
| `--color-warn` | `#F2B63D` | discharging, **no grid** | warnings (see Errors) |
| `--color-info` | `#6DB3FF` | neutral magnitudes: time, counters, kWh, cycles. Also interactive accent (primary button, switch on, focus) | status |
| `--color-bad` | `#FF7A72` | **only** past a threshold: SOC < alert threshold, temperature outside 0…45 °C, station error, station died | "discharging" |

All four pass WCAG AA as text on every surface (≥ 6.9:1 on cards; see `tokens.css`). Surfaces step
+~0.03 L: chrome `#0B0E12` → page `#0E1115` → surface-1 `#151A20` (cards) → surface-2 `#1B2129`
(popovers, inputs) → surface-3 `#222932` (dialogs, tooltips). Text: primary `#E8EBEF` (14.6:1),
muted `#A6AEB9` (7.8:1), subtle `#8A94A1` (5.5:1; not for body text on surface-3). The focus ring
`#8CC4FF` is 2 px with a 2 px offset and is ≥ 8:1 on every surface.

**Decision on warnings:** amber already means "no grid", so error-log *warnings* use a **red outline**
pill and *errors* use a **red filled** pill. Connectivity and info entries use a neutral grey pill
with their own icon. Level is always carried by icon + word + shape, never by colour alone.

## 3. Scales

- **Type** (ratio ≈ 1.2, dense UI, 6 sizes): numeral 28/600 · heading 20/600 · title 16/500 ·
  body 14/400 · small 13 · caption 12 (UPPERCASE labels +0.07em). Numerals −0.02em. Line-height:
  1.5 body, 1.35 compact rows, 1.15 numerals.
- **Space:** 4 · 8 · 12 · 16 · 24 · 32 · 48. Content padding 24, panel padding 16×24, gaps 16.
- **Radii:** 4 (bars, code chips) · 6 (controls, tooltip) · 10 (panels) · 12 (dialogs, popovers) · pill.
- **Elevation:** panels have no shadow (border + lightness). `--shadow-popover` and `--shadow-dialog`
  are layered and carry a 1 px inset top highlight.
- **Motion:** 100 hover colour · 150 tooltip/switch · 200 popover/segmented thumb · 280 dialog ·
  **600 data** (gauge fill, slow on purpose: data is not UI). Easing: standard / decelerate (enter) /
  accelerate (exit). `prefers-reduced-motion` collapses all of these to ~0 and stops the live dot.
- **Z:** sticky 100 · popover 1000 · scrim 1100 · dialog 1200 · tooltip 1500 (portalled to `<body>`).

## 4. Component notes

### Ring gauge (one component, used everywhere)
- Box **120×120 px**, `viewBox 0 0 120 120`, `r=50`, **stroke 10** (outer Ø 110, inner Ø 90).
- Track `--color-track`. Fill in the tone colour, `stroke-linecap: round`, starts at 12 o'clock and
  runs clockwise (`rotate(-90 60 60)`), `pathLength="100"` so `stroke-dasharray = "<pct> 100"`.
- **0% → do not render the fill** (a round cap would draw a dot). 100% → full ring.
- **Title** above (12 px caps, muted). **Value** centred: 28/600, unit 15/500 muted. A value with 4
  digits plus a unit ("1750 Вт") steps down to 23 px (`.gauge__num--long`). Time values get an 11 px
  "год : хв" line under the number.
- **Under the ring, two lines:** primary 13/500 (may take the tone colour, for example amber
  "розряд") and secondary 12 subtle.
- **Fill rules (owner-approved):** SOC = %. Power = |P| / max in the current direction (2500 W discharge (off-grid), 2500 W
  charge). **Time rings: a full ring means the station is fully charged.** «До розряду» and «Вистачить на» fill = usable energy left above the
  cutoff / full usable energy ((SOC − 10) / 90). «До повного» fill = the current SOC. Health = %; cycles = n / 6000; temperature = t / 45 within the norm.
- **«Без світла сьогодні»** (replaces «Світло сьогодні»): the centre shows the hours *without* light in the standard numeral style, «5:27», with the «год : хв» unit line (as in «До розряду»; under an hour «0:27»).
  The sublabel is the outage count («2 відключення»). The fill is amber = share of the *elapsed* day without light. **No percentage anywhere on
  this ring**; percentages appear only in the Outages statistics.
- Tones: `good | warn | info | bad`. `.gauge--stale` greys the fill and the number. A row of gauges
  sits inside **one** panel with 1 px dividers, not one card per gauge.
- a11y: `role="img"` on the dial with a full sentence as `aria-label`; the caption is the
  `<figcaption>`.

### Charts
- Gridlines `rgba(255,255,255,.06)` 1 px crisp. The baseline is `--color-border-strong`. Axis text is
  11 px subtle, and the current day/period is muted 500. Y axes always start at 0 for bars. Line
  charts may use a tight scale only when it is labelled (Battery).
- **No grid** = grey band `--color-chart-outage-band`, label above it, centred. Labels never collide:
  full text → duration only → no label. **No data** = 45° hatch + dashed 1 px frame. The line is
  **broken** across gaps and never interpolated.
- Alert threshold = red dashed 1 px at 60% opacity, labelled at the right.
- Bars: top corners 4 px, width ≤ 44 px (single) or 2 × 18 px (grouped, 2 px apart). Value label
  above each bar when ≤ 10 bars. Partial-cloud data = diagonal hatch over the amber fill. Station died
  = red 3.5 px dot by the bar top.
- Hover/focus target = the **whole column** (invisible hit rect), not the bar itself. Bars are
  focusable (`tabindex=0`, Enter/Space = click). The selected bar keeps full opacity and gets a
  1.5 px outline; the others fall to 35%.
- Line chart hover: a vertical crosshair (`rgba(232,235,239,.35)`) + 4 px dot + tooltip at the point.

### Tooltip (spec §5.9)
One `.tip` element is portalled to `<body>`, `position: fixed`, max-width 280 px. It sits **above by
default** with a 10 px gap. If there is no room above it **flips below**. If there is no room
either way it is pinned inside. Horizontally it is **clamped** to the bounds with an 8 px margin,
and the arrow re-points at the anchor (kept ≥ 10 px from the tooltip corners). Bounds = the app
window's content area ∩ viewport. Open delay 120 ms, then 0 ms during a 300 ms "warm" window. It
closes on leave, blur, Esc, scroll and resize. Contents: title row (bold + small subtle meta), then a
`dl` of label/value pairs, then an optional footer separated by a hairline. Reference
implementation: `_src/app.js → place()`. **Whatever chart library is chosen must route its tooltips
through this function**, or reproduce it exactly.

### Panels and status list
- `.panel`: surface-1, 1 px border, radius 10, padding 16×24. The head is title 16/500 + subtle
  sub-label + a control right-aligned (it wraps for English).
- Overview right column: rows of 32 px tinted icon square + caps label + value 14/500 + meta 13
  subtle. **Only the «Світло» row is tinted** (amber gradient, value 16 px amber) while the grid is
  down. Rows are links to their tab.

### Pills
22 px tall, radius pill, 12/500, 12 px icon with stroke 2. Tones use the soft background + tone
text. `pill--outline` = unknown, stale or secondary information.

### Segmented control
Track = page colour + 1 px border, radius 8, padding 3. Items are 26 px (24 px for `--sm`), 13/500
muted, and the active item is primary. A single **sliding thumb** (`ink-650`, 200 ms) sits under
the active item. `aria-pressed` is used for filters/periods. Use it for 2–6 options. More than that,
or options that change rarely, get a `select`.

### Table
Sticky header 36 px, 12 px caps subtle, `inset` bottom hairline (a border does not paint when
sticky). Rows 40 px min, cells 13 px, padding 12 (24 on the outer columns). Numbers are right
aligned with tabular figures. Dates are two lines: time, then a subtle "сьогодні / вчора / пт
20.11". Rows grow vertically and never truncate. Advice column ~38ch.

### Switch, select, button
Switch 36×20, on = info blue with a dark thumb, 150 ms. It applies immediately; there is no "Save".
Dependent controls dim to 45% and become inert when their switch is off. Select and input are
32 px (the control height) on surface-2. Buttons: primary (blue, dark text), secondary (surface-2 +
strong border), ghost. All are 32 px with 1 px of press travel.

### Station switcher and dialog
The switcher is a popover 464 px wide under the station button, with a light scrim over the
content only. Station rows: 40 px mini ring + name/model + status pills + a check mark on the
selected row. An offline station shows an empty ring with "—" and outline pills ("останнє: 64% о
14:52"). The dialog is 640 px wide on surface-3 with a full scrim. Method tiles are a radiogroup;
the selected tile gets a blue border and an inverted icon. "Already added" devices stay in the list
with a pill.

### Tray icon
Drawn on a 16×16 pixel grid, with a separate 32×32 drawing (not a scaled copy). The battery body is
12×8 and the level is quantised to 9 px. The status badge is a 7 px disc in the bottom-right with a
1 px knockout in the taskbar colour. There are two ink variants, for dark and for light taskbars.
The "grid present" state has no badge. Generate the icons in Go into a multi-size .ico (16/20/24/32).

## 5. English (+30%)
Nothing has a fixed width that holds text: tabs, pills and buttons are `white-space: nowrap` and
content-sized. Gauge captions wrap to two lines, and panel heads and toolbars `flex-wrap`. The
Overview right column is 296 px. With the longest English strings ("Previous outage", "Battery
health") the caps labels still fit on one line at 12 px. Format numbers and dates with `Intl` for
the UI language (uk: `3,7`, en: `3.7`).

## 6. Connection states (flows §2.6, §3)

- **Grey means "not current".** In STALE, NO_LINK and LIKELY_DEAD the gauges lose their state colour (`.gauge--stale`: grey fill, muted number). Forecasts show «—». In NO_LINK the SOC ring keeps the last value with «о 15:15» under the number, and power shows «—».
- **Outage rail:** amber only while «без світла» is a *fresh* fact. When the grid state is unknown, a grey dashed rail is shown (`data-grid="unknown"`). When the grid is on, there is no rail.
- **Banners:** one `.banner` component, one at a time. STALE uses a clock icon and is neutral. NO_LINK uses wifi-off, and the router hint is shown only when the last known state is «без світла». LIKELY_DEAD uses a reddish border. cloud_auth_expired is the only one with an action button. CONNECTING-with-old-data and cloud fallback use the quiet dashed variant.
- **SOC chart tail:** STALE draws a grey line that stops at the last measurement. NO_LINK draws an open dashed frame to «зараз». LIKELY_DEAD draws the frame labelled «ймовірно, розрядилась». Outage bands stop at the last measurement and the label becomes «≥ …».
- **Gap card** «Поки додаток не працював»: a dashed frame (the same visual language as «немає даних»), dismissible, shown above the gauges.
- **Outages:** a whole-day gap is a hatched full-height column labelled «немає даних», never 0. A day with 0 outages is a 2 px tick on the baseline. Days before `observed_since` are not drawn and are marked «спостереження з …». ≥ / ≈ durations are shown in the table and stats with a footnote.
- **Skeletons** use the exact gauge geometry (120 px ring, 10 px stroke) and row heights, with a static shimmer under reduced motion.

## 7. Sample data
`_src/data.js` simulates one consistent week (and 30 days) at 5-minute resolution: Sat 21.11.2026,
15:17, outage since 11:50, 72%, 420 W. Every number on every screen comes from it or agrees with
it. Note: 21.11.2026 is a **Saturday**. The earlier wireframes labelled it "чт".

---

## Copy

All strings come from `../2026-11-21-marstek-monitor-copy.md`, including the keys the writer added for the gap card, LIKELY_DEAD, «≥» durations, onboarding (§13.1), remove/archive (§11.3), the tray menu with «Не турбувати» (§12.2) and SmartScreen (§13.6). Still without a key: the test-notification toast and the button label.

## Owner decisions (2026-11-21)

1. «Світло сьогодні» → «Без світла сьогодні», hours in the centre, no percentages. *Applied everywhere (01, 06, 09).*
2. Time rings: a full ring means fully charged (see Fill rules). *Applied.*
3. Red dashed low-charge threshold on the SOC chart. *Kept.*
4. «Надіслати тестове сповіщення» in Settings → Сповіщення. *Added* (secondary button next to the intro; the demo toast appears bottom-right). Toast strings are a draft with no catalogue key.
5. Error-log warnings use a red outline. *Kept.*
6. Line icons (not emoji) in the notification settings. *Approved; the comparison was removed.*
8. «Без світла сьогодні» value: one numeral «5:27» + «год : хв», the same as the other rings. *Applied (01, 06, 09).*
7. Toast station line: the attribution row «Дім · Venus E 3.0» (notify.attribution), with notify.titleWithStation as the fallback. *Applied.*
