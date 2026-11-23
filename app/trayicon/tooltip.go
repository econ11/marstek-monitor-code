// Package trayicon builds the tray tooltip text and the short number/duration
// formats shared by the tray, toasts and journal.
package trayicon

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/monitor"
)

const (
	// maxTipUnits is the Windows NOTIFYICONDATA.szTip limit (128 incl. NUL),
	// counted in UTF-16 code units.
	maxTipUnits = 127
	// maxNameRunes is the station name limit in the tooltip (copy.md 12.1).
	maxNameRunes = 24
)

// Tooltip builds the tray tooltip text (copy.md 12.1) in the current
// language. The result never exceeds 127 UTF-16 code units: the station name
// is shortened first.
func Tooltip(s monitor.Snapshot) string {
	if s.Station.ID == "" && s.Station.Name == "" {
		return i18n.T("tray.tooltip.noStations")
	}
	name := truncRunes(s.Station.Name, maxNameRunes)
	for {
		out := build(s, name)
		if units(out) <= maxTipUnits || name == "" {
			return out
		}
		r := []rune(name)
		if len(r) <= 1 {
			name = ""
		} else {
			name = strings.TrimRight(string(r[:len(r)-2]), " ") + "…"
		}
	}
}

// build renders the tooltip; a station without a name shows its model alone.
func build(s monitor.Snapshot, name string) string {
	out := buildNamed(s, name)
	if name == "" {
		out = strings.TrimPrefix(out, " · ")
	}
	return out
}

func buildNamed(s monitor.Snapshot, name string) string {
	model := s.Station.Model
	if model == "" {
		model = s.Profile.Model
	}
	switch s.Link {
	case link.Live, link.Resyncing:
		return live(s, name, model)
	case link.Connecting:
		if s.Last == nil {
			return i18n.T("tray.tooltip.connecting", "station", name, "model", model)
		}
		fallthrough // old data: same as STALE (flows 2.6)
	case link.Stale:
		return i18n.T("tray.tooltip.stale", "station", name, "model", model,
			"time", clock(s), "lastSoc", socText(s))
	default: // NO_LINK, LIKELY_DEAD, SUSPENDED
		out := i18n.T("tray.tooltip.noConnection", "station", name, "model", model, "time", clock(s))
		if s.Last == nil { // never had data: drop the "last data at" line
			out = out[:strings.LastIndex(out, "\n")]
		}
		return out
	}
}

func live(s monitor.Snapshot, name, model string) string {
	lines := []string{i18n.T("tray.tooltip.line1", "station", name, "model", model)}

	l2 := socText(s) + "%"
	if sp := stateWithPower(s); sp != "" {
		l2 = i18n.T("tray.tooltip.line2", "soc", socText(s), "stateWithPower", sp)
	}
	lines = append(lines, l2)

	if k := s.PendingKind(); k != "" {
		// An unconfirmed grid change: same wording as the window's «Світло» row.
		return strings.Join(append(lines, i18n.T("tray.tooltip.line3.pending"+k)), "\n")
	}
	switch s.Grid {
	case detect.GridOff:
		if s.Current == nil || s.Last == nil {
			break
		}
		dur := ShortDuration(s.Last.At.Sub(s.Current.Start))
		eta := s.F.ToEmpty
		if s.Mode != detect.Discharging {
			eta = s.F.LastsFor
		}
		if eta != nil {
			lines = append(lines, i18n.T("tray.tooltip.line3.off.toEmpty", "duration", dur, "eta", ShortDuration(*eta)))
		} else {
			lines = append(lines, i18n.T("tray.tooltip.line3.off.noEta", "duration", dur))
		}
	case detect.GridOn:
		if s.Mode == detect.Charging && s.F.FullAt != nil {
			lines = append(lines, i18n.T("tray.tooltip.line3.on.charging", "time", s.F.FullAt.Local().Format("15:04")))
		} else {
			lines = append(lines, i18n.T("tray.tooltip.line3.on.idle"))
		}
	}
	return strings.Join(lines, "\n")
}

// stateWithPower is "" when the battery state is unknown.
func stateWithPower(s monitor.Snapshot) string {
	switch s.Mode {
	case detect.Idle, detect.Cutoff:
		return i18n.T("state.idle")
	case detect.Charging, detect.Discharging:
		if s.Last == nil || s.Last.BatteryW == nil {
			return ""
		}
		w := *s.Last.BatteryW
		if w < 0 {
			w = -w
		}
		key := "state.withPower.charging"
		if s.Mode == detect.Discharging {
			key = "state.withPower.discharging"
		}
		return i18n.T(key, "watts", GroupInt(int64(w+0.5)))
	}
	return ""
}

func socText(s monitor.Snapshot) string {
	if s.Last == nil || s.Last.SOC == nil {
		return "—"
	}
	return strconv.Itoa(int(*s.Last.SOC + 0.5))
}

func clock(s monitor.Snapshot) string {
	if s.Last == nil {
		return "—"
	}
	return s.Last.At.Local().Format("15:04")
}

// Decimal formats x with as many decimals as it needs (5.12) and the
// separator of the current language (copy.md 9.1): "5,12" in uk, "5.12" in en.
func Decimal(x float64) string {
	s := strconv.FormatFloat(x, 'f', -1, 64)
	if i18n.Lang() == "uk" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// GroupInt groups thousands (copy.md 9.1): U+00A0 in uk, a comma in en.
func GroupInt(n int64) string {
	sep := ","
	if i18n.Lang() == "uk" {
		sep = " "
	}
	d := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range d {
		if i > 0 && (len(d)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// ShortDuration is the "short" form (copy.md 9.2), whole minutes floored.
func ShortDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	m := int(d / time.Minute)
	switch {
	case m < 1:
		return i18n.T("duration.short.lt1m")
	case m < 60:
		return i18n.T("duration.short.m", "m", strconv.Itoa(m))
	case m < 24*60:
		if m%60 == 0 {
			return i18n.T("duration.short.h", "h", strconv.Itoa(m/60))
		}
		return i18n.T("duration.short.hm", "h", strconv.Itoa(m/60), "mm", pad2(m%60))
	}
	days, hours := m/(24*60), m%(24*60)/60
	return i18n.T("duration.short.dh", "d", strconv.Itoa(days), "h", strconv.Itoa(hours))
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func units(s string) int { return len(utf16.Encode([]rune(s))) }
