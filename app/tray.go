package app

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/app/trayicon"
	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// dndOptions are the "Не турбувати" submenu entries: notify.DNDOption key
// and copy key, in menu order (flows 5.1).
var dndOptions = [][2]string{
	{"1h", "tray.menu.dnd.1h"},
	{"3h", "tray.menu.dnd.3h"},
	{"8h", "tray.menu.dnd.8h"},
	{"morning", "tray.menu.dnd.untilMorning"},
	{"manual", "tray.menu.dnd.untilOff"},
}

// Tray owns the notification-area icon: icon, tooltip and menu follow the
// monitor snapshots (flows 5.1, copy.md 12.1-12.2).
type Tray struct {
	app      *application.App
	tray     *application.SystemTray
	menu     *application.Menu
	show     func()
	settings notify.Settings

	// mu serializes refreshes, including the Wails calls. It is never taken
	// on the UI thread (menu callbacks run on their own goroutines).
	mu       sync.Mutex
	mon      *monitor.Monitor
	snap     monitor.Snapshot
	dndUntil *time.Time
	iconKey  string
	tip      string
	menuKey  string

	iconFailLogged bool   // the icon error was logged once
	label          string // in front of the tooltip (-simulate)

	// Test seams for pushMenu; NewTray points them at the real tray and menu.
	setMenuFn    func()
	updateMenuFn func()
	menuSet      bool
}

// NewTray creates the icon. show brings up the main window (left click,
// double click and "Відкрити"). Until SetMonitor it shows "no station".
func NewTray(a *application.App, show func(), settings notify.Settings) *Tray {
	t := &Tray{app: a, tray: a.SystemTray.New(), menu: a.NewMenu(), show: show, settings: settings}
	t.setMenuFn = func() { t.tray.SetMenu(t.menu) }
	t.updateMenuFn = t.menu.Update
	// Wails publishes the tray impl before its window exists; a SetIcon in that
	// gap is not kept, so the icon can stay stale. Push the latest again once
	// the tray has run (same start race as the menu, see pushMenu).
	a.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		time.AfterFunc(iconRepushDelay, t.repush)
	})
	t.tray.OnClick(show)
	t.tray.OnDoubleClick(show)
	t.refresh()
	return t
}

// SetLabel puts label in front of the tooltip. -simulate uses it, so the
// simulated station is never taken for the real one.
func (t *Tray) SetLabel(label string) {
	t.mu.Lock()
	t.label = label
	t.mu.Unlock()
	t.refresh()
}

// SetMonitor attaches the running monitor (the target of "Не турбувати").
func (t *Tray) SetMonitor(m *monitor.Monitor) {
	t.mu.Lock()
	t.mon = m
	t.mu.Unlock()
}

// SetSettings applies changed notification settings: the "until morning" end
// in the menu follows them at once.
func (t *Tray) SetSettings(s notify.Settings) {
	t.mu.Lock()
	t.settings = s
	t.menuKey = "" // redraw the menu
	t.mu.Unlock()
	t.refresh()
}

// Update shows a new snapshot.
func (t *Tray) Update(s monitor.Snapshot) {
	t.mu.Lock()
	t.snap = s
	t.mu.Unlock()
	t.refresh()
}

// NotifyState is the notification settings and the DND deadline (nil: off)
// the tray last applied: the update toast, which is not a monitor toast,
// is silenced by them too.
func (t *Tray) NotifyState() (notify.Settings, *time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var dnd *time.Time
	if t.dndUntil != nil {
		u := *t.dndUntil
		dnd = &u
	}
	return t.settings, dnd
}

func (t *Tray) setDND(opt string) {
	t.mu.Lock()
	m := t.mon
	if m != nil {
		t.dndUntil = notify.DNDOption(opt, time.Now(), t.settings)
	}
	t.mu.Unlock()
	if m == nil {
		return
	}
	m.SetDND(opt)
	t.refresh()
}

// refresh pushes icon, tooltip and menu to Windows, each only when changed.
func (t *Tray) refresh() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if t.dndUntil != nil && !now.Before(*t.dndUntil) {
		t.dndUntil = nil // expired; the policy drops it by itself
	}
	s := t.snap

	px := trayIconPx(systemDPI())
	key := trayIconKey(s.Grid, px)
	if key != t.iconKey {
		icon := iconForGrid(s.Grid)
		b, err := trayImage(icon, px)
		switch {
		case err != nil:
			if !t.iconFailLogged {
				t.iconFailLogged = true
				slog.Warn("tray icon", "icon", string(icon), "px", px, "err", err)
			}
		default:
			t.iconKey = key
			t.iconFailLogged = false
			slog.Debug("tray icon", "icon", string(icon), "px", px)
			setTrayImage(t.tray, b)
		}
	}

	if tip := t.label + trayicon.Tooltip(s); tip != t.tip {
		t.tip = tip
		slog.Debug("tray tooltip", "text", tip)
		t.tray.SetTooltip(tip)
	}

	header, dnd := menuHeader(s), dndLabel(t.dndUntil)
	// The language is part of the key: every menu entry is translated.
	key = i18n.Lang() + "\n" + header + "\n" + dnd + "\n" + strconv.FormatBool(t.mon != nil)
	if key != t.menuKey {
		t.menuKey = key
		slog.Debug("tray menu", "header", header, "dnd", dnd)
		t.buildMenu(header, dnd, now)
	}
}

// buildMenu rebuilds the menu (flows 5.1; "Станція в треї", "Доступне
// оновлення" and "Налаштування" belong to later plans).
func (t *Tray) buildMenu(header, dnd string, now time.Time) {
	m := t.menu
	m.Clear()
	m.Add(header).SetEnabled(false)
	m.AddSeparator()
	m.Add(i18n.T("tray.menu.open")).OnClick(func(*application.Context) { t.show() })
	switch {
	case t.mon == nil: // nothing to mute yet
	case dnd != "":
		m.Add(dnd).OnClick(func(*application.Context) { t.setDND("off") })
	default:
		sub := m.AddSubmenu(i18n.T("tray.menu.dnd"))
		for _, o := range dndOptions {
			opt, label := o[0], i18n.T(o[1])
			if opt == "morning" {
				if until := notify.DNDOption(opt, now, t.settings); until != nil {
					label = i18n.T(o[1], "time", hhmm(*until))
				}
			}
			sub.Add(label).OnClick(func(*application.Context) { t.setDND(opt) })
		}
	}
	m.AddSeparator()
	m.Add(i18n.T("tray.menu.quit")).OnClick(func(*application.Context) { t.app.Quit() })
	t.pushMenu()
}

// dndLabel is the single "active" menu row, or "" while DND is off.
func dndLabel(until *time.Time) string {
	switch {
	case until == nil:
		return ""
	case until.Equal(notify.DNDManualUntil):
		return i18n.T("tray.menu.dnd.active.indefinite")
	}
	return i18n.T("tray.menu.dnd.active", "time", hhmm(*until))
}

// menuHeader is the inactive first row: station, charge and grid state
// (copy.md 12.2). Unknown parts are left out.
func menuHeader(s monitor.Snapshot) string {
	if s.Station.ID == "" {
		return i18n.T("tray.menu.header.noStations")
	}
	var soc *float64
	if s.Last != nil {
		soc = s.Last.SOC
	}
	grid := ""
	switch s.Link {
	case link.Live, link.Resyncing:
		switch {
		case s.PendingKind() != "": // an unconfirmed change; the window says the same
			grid = i18n.T("tray.menu.header.gridPending" + s.PendingKind())
		case s.Grid == detect.GridOn:
			grid = i18n.T("tray.menu.header.gridOn")
		case s.Grid == detect.GridOff && s.Current != nil && s.Last != nil:
			m := int(max(s.Last.At.Sub(s.Current.Start), 0) / time.Minute)
			grid = i18n.T("tray.menu.header.gridOff", "h", strconv.Itoa(m/60), "mm", fmt.Sprintf("%02d", m%60))
		}
	default: // old data: its time instead of a grid state
		if s.Last != nil {
			grid = i18n.T("tray.menu.header.stale", "time", hhmm(s.Last.At))
		}
	}
	return segments("tray.menu.header", "station", stationLabel(s), "soc", socText(soc), "gridShort", grid)
}

// stationLabel is the user's name for the station, or its model.
func stationLabel(s monitor.Snapshot) string {
	switch {
	case s.Station.Name != "":
		return s.Station.Name
	case s.Station.Model != "":
		return s.Station.Model
	}
	return "Marstek"
}

// socText is the whole-percent charge, "" when unknown.
func socText(soc *float64) string {
	if soc == nil {
		return ""
	}
	return strconv.Itoa(int(*soc + 0.5))
}

// trayIconPx is the tray icon size for dpi: 16 px at 100%, scaled with the DPI.
func trayIconPx(dpi int) int {
	return max(16, (16*dpi+48)/96)
}

// trayIconKey identifies what the tray shows: the icon (as iconForGrid picks it)
// and its size. Equal keys need no new SetIcon.
func trayIconKey(g detect.GridState, px int) string {
	return fmt.Sprintf("%s/%d", iconForGrid(g), px)
}

// pushMenu hands the rebuilt t.menu to Windows. SetMenu is called exactly
// once, by the first build in NewTray, before app.Run, when it only stores
// the menu. After start Wails' SystemTray.Run publishes impl before its
// window exists, and a SetMenu in that gap builds the popup with owner
// window 0, so TrackPopupMenuEx fails for good ("Invalid window handle").
// Later rebuilds change the same menu and use Update instead.
func (t *Tray) pushMenu() {
	if !t.menuSet {
		t.menuSet = true
		t.setMenuFn()
		return
	}
	t.updateMenuFn()
}

// iconRepushDelay is how long after ApplicationStarted the icon and tooltip
// are pushed again, so the tray window surely exists.
const iconRepushDelay = 3 * time.Second

// clearPushedKeys forgets what was pushed, so the next refresh pushes the
// icon and the tooltip again.
func (t *Tray) clearPushedKeys() {
	t.mu.Lock()
	t.iconKey, t.tip = "", ""
	t.mu.Unlock()
}

func (t *Tray) repush() {
	t.clearPushedKeys()
	t.refresh()
}
