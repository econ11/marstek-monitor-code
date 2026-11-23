package trayicon

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/store"
)

func u16(s string) int { return len(utf16.Encode([]rune(s))) }

func dp(d time.Duration) *time.Duration { return &d }

var t1515 = time.Date(2026, 11, 22, 15, 15, 0, 0, time.Local)

func base(name string) monitor.Snapshot {
	return monitor.Snapshot{
		Station: store.Station{ID: "s1", Name: name, Model: "Venus E 3.0"},
		Link:    link.Live, Grid: detect.GridOn, Mode: detect.Idle,
		Last: &model.Sample{At: t1515, SOC: f(72)},
	}
}

func TestTooltipMaxLen(t *testing.T) {
	name := strings.Repeat("Я", 24)
	// Worst case from copy.md 12.1: 24-char name, discharging 800 W,
	// 23 h 59 min outage, 23 h 55 min left.
	s := base(name)
	s.Grid, s.Mode = detect.GridOff, detect.Discharging
	s.Last.BatteryW = f(-800)
	start := t1515.Add(-(23*time.Hour + 59*time.Minute))
	s.Current = &store.Outage{Start: start}
	s.F.ToEmpty = dp(23*time.Hour + 55*time.Minute)
	got := Tooltip(s)
	if n := u16(got); n > 127 {
		t.Errorf("UTF-16 length %d > 127: %q", n, got)
	}
	if !strings.Contains(got, name) {
		t.Errorf("24-char name must not be cut: %q", got)
	}

	// An over-long name is truncated; the whole thing still fits.
	s = base(strings.Repeat("Ж", 80))
	s.Grid, s.Mode = detect.GridOff, detect.Discharging
	s.Last.BatteryW = f(-2500)
	s.Current = &store.Outage{Start: start}
	s.F.ToEmpty = dp(2*time.Hour + 55*time.Minute)
	if n := u16(Tooltip(s)); n > 127 {
		t.Errorf("long name: UTF-16 length %d > 127", n)
	}
}

func TestTooltipCountsUTF16NotRunes(t *testing.T) {
	// Astral characters take 2 UTF-16 units each; the limit must hold anyway.
	s := base(strings.Repeat("😀", 24))
	s.Grid, s.Mode = detect.GridOff, detect.Discharging
	s.Last.BatteryW = f(-800)
	s.Current = &store.Outage{Start: t1515.Add(-(23*time.Hour + 59*time.Minute))}
	s.F.ToEmpty = dp(2*time.Hour + 55*time.Minute)
	if n := u16(Tooltip(s)); n > 127 {
		t.Errorf("UTF-16 length %d > 127", n)
	}
}

func TestTooltipLive(t *testing.T) {
	s := base("Дім")
	s.Grid, s.Mode = detect.GridOff, detect.Discharging
	s.Last.BatteryW = f(-800)
	s.Current = &store.Outage{Start: t1515.Add(-(3*time.Hour + 27*time.Minute))}
	s.F.ToEmpty = dp(7*time.Hour + 30*time.Minute + 20*time.Second)
	want := "Дім · Venus E 3.0\n72% · розряд 800 Вт\nБез світла 3 год 27 хв · вистачить на 7 год 30 хв"
	if got := Tooltip(s); got != want {
		t.Errorf("got %q want %q", got, want)
	}

	s = base("Дім")
	s.Mode = detect.Charging
	s.Last.BatteryW = f(1500)
	full := time.Date(2026, 11, 22, 17, 40, 0, 0, time.Local)
	s.F.FullAt = &full
	want = "Дім · Venus E 3.0\n72% · заряд 1 500 Вт\nСвітло є · до 100% о 17:40"
	if got := Tooltip(s); got != want {
		t.Errorf("got %q want %q", got, want)
	}

	s = base("Дім")
	want = "Дім · Venus E 3.0\n72% · очікування\nСвітло є"
	if got := Tooltip(s); got != want {
		t.Errorf("got %q want %q", got, want)
	}

	s = base("Дім")
	s.Grid, s.Mode = detect.GridOff, detect.Idle
	s.Current = &store.Outage{Start: t1515.Add(-5 * time.Minute)}
	if got := Tooltip(s); !strings.HasSuffix(got, "Без світла 5 хв") {
		t.Errorf("no ETA: %q", got)
	}
}

// copy.md 12.1 tray.tooltip.stale (flows 2.6 says "дані на ЧЧ:ХХ"; the copy
// key is the authority for the string).
func TestTooltipStale(t *testing.T) {
	s := base("Дім")
	s.Link = link.Stale
	want := "Дім · Venus E 3.0\nДані застарілі, останні о 15:15\n72% на той момент"
	if got := Tooltip(s); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestTooltipOtherStates(t *testing.T) {
	s := base("Дім")
	s.Link = link.NoLink
	want := "Дім · Venus E 3.0\nНемає зв'язку зі станцією\nОстанні дані о 15:15"
	if got := Tooltip(s); got != want {
		t.Errorf("no link: got %q", got)
	}
	s = monitor.Snapshot{Station: store.Station{ID: "s1", Name: "Дім", Model: "Venus E 3.0"}, Link: link.Connecting}
	if got, w := Tooltip(s), "Дім · Venus E 3.0\nПідключаємось…"; got != w {
		t.Errorf("connecting: got %q", got)
	}
	if got, w := Tooltip(monitor.Snapshot{}), "Marstek Monitor\nСтанцію ще не додано"; got != w {
		t.Errorf("no stations: got %q", got)
	}
}

// Ruling 15c: a station without a name shows its model alone, with no
// leading separator.
func TestTooltipWithoutStationName(t *testing.T) {
	s := base("")
	if got, want := Tooltip(s), "Venus E 3.0\n72% · очікування\nСвітло є"; got != want {
		t.Errorf("live: %q, want %q", got, want)
	}
	for _, st := range []link.State{link.Stale, link.NoLink, link.Connecting} {
		s.Link = st
		if got := Tooltip(s); !strings.HasPrefix(got, "Venus E 3.0\n") {
			t.Errorf("%s: %q, want the model as the first line", st, got)
		}
	}
	s.Last = nil
	if got := Tooltip(s); !strings.HasPrefix(got, "Venus E 3.0\n") {
		t.Errorf("connecting, no data: %q", got)
	}
}

func TestTooltipEnglishMaxLen(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	i18n.SetLang("en")
	name := strings.Repeat("W", 24)
	s := base(name)
	s.Grid, s.Mode = detect.GridOff, detect.Discharging
	s.Last.BatteryW = f(-2500)
	s.Current = &store.Outage{Start: t1515.Add(-(23*time.Hour + 59*time.Minute))}
	s.F.ToEmpty = dp(23*time.Hour + 55*time.Minute)
	got := Tooltip(s)
	if n := u16(got); n > 127 {
		t.Errorf("UTF-16 length %d > 127: %q", n, got)
	}
	want := name + " · Venus E 3.0\n72% · discharging 2,500 W\nGrid off 23 h 59 min · ~23 h 55 min left"
	if got != want {
		t.Errorf("tooltip = %q, want %q", got, want)
	}
	// Every other state stays in the limit too.
	for _, l := range []link.State{link.Stale, link.NoLink, link.Connecting} {
		s.Link = l
		if n := u16(Tooltip(s)); n > 127 {
			t.Errorf("%v: UTF-16 length %d > 127", l, n)
		}
	}
}

func f(v float64) *float64 { return &v }

func TestTooltipGridPending(t *testing.T) {
	for name, tc := range map[string]struct {
		grid, pending detect.GridState
		lang, want    string
	}{
		"uk coming back": {detect.GridOff, detect.GridOn, "uk", "Світло з'являється… перевіряємо"},
		"uk going off":   {detect.GridOn, detect.GridOff, "uk", "Світло зникає… перевіряємо"},
		"en unknown on":  {detect.GridUnknownState, detect.GridOn, "en", "Grid: checking…"},
		"uk unknown off": {detect.GridUnknownState, detect.GridOff, "uk", "Світло: перевіряємо…"},
		"en coming back": {detect.GridOff, detect.GridOn, "en", "Grid coming back… checking"},
		"en going off":   {detect.GridOn, detect.GridOff, "en", "Grid going off… checking"},
	} {
		i18n.SetLang(tc.lang)
		s := base("Дім")
		s.Grid, s.GridPending, s.GridPendingSince = tc.grid, tc.pending, &t1515
		if tc.grid == detect.GridOff {
			s.Current = &store.Outage{Start: t1515.Add(-time.Hour)}
		}
		got := Tooltip(s)
		lines := strings.Split(got, "\n")
		if len(lines) != 3 || lines[2] != tc.want {
			t.Errorf("%s: tooltip = %q, want third line %q", name, got, tc.want)
		}
		if u16(got) > 127 {
			t.Errorf("%s: %d units", name, u16(got))
		}
	}
	i18n.SetLang("uk")
}
