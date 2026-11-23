package app

import (
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
)

func TestMenuHeader(t *testing.T) {
	at := time.Date(2026, 11, 21, 15, 17, 0, 0, time.Local)
	home := store.Station{ID: "mac", Name: "Дім", Model: "VenusE 3.0"}
	last := &model.Sample{At: at, SOC: f64(72.4)}

	for name, tc := range map[string]struct {
		s    monitor.Snapshot
		want string
	}{
		"no station": {monitor.Snapshot{}, "Станцію ще не додано"},
		"grid on": {monitor.Snapshot{Station: home, Link: link.Live, Grid: detect.GridOn, Last: last},
			"Дім · 72% · світло є"},
		"grid off": {monitor.Snapshot{Station: home, Link: link.Live, Grid: detect.GridOff, Last: last,
			Current: &store.Outage{Start: at.Add(-207 * time.Minute)}},
			"Дім · 72% · без світла 3:27"},
		"stale": {monitor.Snapshot{Station: home, Link: link.Stale, Grid: detect.GridOn, Last: last},
			"Дім · 72% · дані о 15:17"},
		"no data yet": {monitor.Snapshot{Station: store.Station{ID: "mac", Model: "VenusE 3.0"}, Link: link.Connecting},
			"VenusE 3.0"},
		"soc unknown": {monitor.Snapshot{Station: home, Link: link.Live, Grid: detect.GridOn, Last: &model.Sample{At: at}},
			"Дім · світло є"},
	} {
		if got := menuHeader(tc.s); got != tc.want {
			t.Errorf("%s: menuHeader = %q, want %q", name, got, tc.want)
		}
	}
}

func TestDNDLabel(t *testing.T) {
	if got := dndLabel(nil); got != "" {
		t.Errorf("off: %q", got)
	}
	until := time.Date(2026, 11, 21, 18, 30, 0, 0, time.Local)
	if got, want := dndLabel(&until), "Не турбувати до 18:30 · Вимкнути"; got != want {
		t.Errorf("timed: %q, want %q", got, want)
	}
	manual := notify.DNDManualUntil
	if got, want := dndLabel(&manual), "Не турбувати увімкнено · Вимкнути"; got != want {
		t.Errorf("manual: %q, want %q", got, want)
	}
}

func TestPushMenuSetsOnce(t *testing.T) {
	var sets, updates int
	tr := &Tray{
		setMenuFn:    func() { sets++ },
		updateMenuFn: func() { updates++ },
	}
	tr.pushMenu()
	if sets != 1 || updates != 0 {
		t.Fatalf("first push: sets=%d updates=%d, want 1/0", sets, updates)
	}
	tr.pushMenu()
	tr.pushMenu()
	if sets != 1 || updates != 2 {
		t.Fatalf("later pushes: sets=%d updates=%d, want 1/2", sets, updates)
	}
}

func TestRepushClearsIconAndTooltipKeys(t *testing.T) {
	tr := &Tray{iconKey: "x", tip: "y", menuKey: "m"}
	tr.clearPushedKeys()
	if tr.iconKey != "" || tr.tip != "" {
		t.Fatalf("iconKey=%q tip=%q, want both empty", tr.iconKey, tr.tip)
	}
	if tr.menuKey != "m" {
		t.Fatalf("menuKey must stay, got %q", tr.menuKey)
	}
}

func TestTrayIconPx(t *testing.T) {
	for dpi, want := range map[int]int{0: 16, 96: 16, 120: 20, 144: 24, 168: 28, 192: 32} {
		if got := trayIconPx(dpi); got != want {
			t.Errorf("trayIconPx(%d) = %d, want %d", dpi, got, want)
		}
	}
}

func TestTrayIconKey(t *testing.T) {
	if got := trayIconKey(detect.GridOff, 16); !strings.HasPrefix(got, "off/") {
		t.Errorf("GridOff key %q, want off/...", got)
	}
	for _, g := range []detect.GridState{detect.GridOn, detect.GridUnknownState} {
		if got := trayIconKey(g, 16); !strings.HasPrefix(got, "on/") {
			t.Errorf("state %v key %q, want on/...", g, got)
		}
	}
	if trayIconKey(detect.GridOn, 16) != trayIconKey(detect.GridOn, 16) {
		t.Error("same state and px must give the same key")
	}
	if trayIconKey(detect.GridOn, 16) == trayIconKey(detect.GridOn, 24) {
		t.Error("different px must give a different key")
	}
}

func TestMenuHeaderGridPending(t *testing.T) {
	at := time.Date(2026, 11, 21, 15, 17, 0, 0, time.Local)
	home := store.Station{ID: "mac", Name: "Дім"}
	last := &model.Sample{At: at, SOC: func() *float64 { v := 72.0; return &v }()}
	for name, tc := range map[string]struct {
		s    monitor.Snapshot
		want string
	}{
		"coming back": {monitor.Snapshot{Station: home, Link: link.Live, Grid: detect.GridOff, GridPending: detect.GridOn, GridPendingSince: &at, Last: last,
			Current: &store.Outage{Start: at.Add(-time.Hour)}}, "Дім · 72% · світло з'являється… перевіряємо"},
		"unknown": {monitor.Snapshot{Station: home, Link: link.Live, Grid: detect.GridUnknownState, GridPending: detect.GridOn, GridPendingSince: &at, Last: last},
			"Дім · 72% · перевіряємо…"},
		"going off": {monitor.Snapshot{Station: home, Link: link.Live, Grid: detect.GridOn, GridPending: detect.GridOff, GridPendingSince: &at, Last: last},
			"Дім · 72% · світло зникає… перевіряємо"},
	} {
		if got := menuHeader(tc.s); got != tc.want {
			t.Errorf("%s: menuHeader = %q, want %q", name, got, tc.want)
		}
	}
}
