package app

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/export"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	// Save with Language "" switches to the system language (en on an
	// English Windows or Mac): the tests after this one expect it unchanged.
	lang := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(lang) })
	return st
}

func noMonitor() *monitor.Monitor { return nil }

func TestSettingsViewHasHistory(t *testing.T) {
	useTempAutostartKey(t)
	st := openTestStore(t)
	s := NewSettingsService(st, `C:\data`, noMonitor, nil, nil)

	v, err := s.Get()
	if err != nil || v.HasHistory {
		t.Fatalf("no station: HasHistory = %v, err = %v; want false", v.HasHistory, err)
	}
	at := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if err := st.UpsertStation(store.Station{ID: "mac", Model: "VenusE 3.0", BLEMac: "mac", IP: "10.0.0.2", CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	if v, err = s.Get(); err != nil || v.HasHistory {
		t.Fatalf("no samples: HasHistory = %v, err = %v; want false", v.HasHistory, err)
	}
	soc := 80.0
	if err := st.InsertSample(model.Sample{StationID: "mac", At: at, Source: model.SourceLAN, SOC: &soc}); err != nil {
		t.Fatal(err)
	}
	if v, err = s.Get(); err != nil || !v.HasHistory {
		t.Fatalf("after the first sample: HasHistory = %v, err = %v; want true", v.HasHistory, err)
	}
	if v.Version != Version || v.DataPath != `C:\data` {
		t.Fatalf("view = %+v", v)
	}
}

func TestSettingsSaveAppliesAndEmits(t *testing.T) {
	useTempAutostartKey(t)
	st := openTestStore(t)
	var applied []settings.Settings
	var emitted []SettingsView
	s := NewSettingsService(st, "", noMonitor,
		func(v settings.Settings) { applied = append(applied, v) },
		func(v SettingsView) { emitted = append(emitted, v) })

	v := settings.Defaults()
	v.Language, v.PollInterval, v.Notify.LowThreshold = "uk", 10*time.Second, 30
	got, err := s.Save(v)
	if err != nil {
		t.Fatal(err)
	}
	if got.S.PollInterval != 10*time.Second || got.S.Notify.LowThreshold != 30 || got.Lang != "uk" {
		t.Fatalf("view = %+v", got)
	}
	if len(applied) != 1 || applied[0].Notify.LowThreshold != 30 {
		t.Fatalf("applied = %+v, want once with threshold 30", applied)
	}
	if len(emitted) != 1 || emitted[0].S.PollInterval != 10*time.Second {
		t.Fatalf("emitted = %+v, want once", emitted)
	}
	if l, err := settings.Load(st); err != nil || l.PollInterval != 10*time.Second {
		t.Fatalf("stored = %+v, %v", l, err)
	}

	bad := v
	bad.Notify.LowThreshold = 15
	if _, err := s.Save(bad); err == nil {
		t.Fatal("Save(threshold 15) = nil error, want invalid")
	}
	if len(applied) != 1 || len(emitted) != 1 {
		t.Fatalf("an invalid Save was applied (%d) or emitted (%d)", len(applied), len(emitted))
	}
}

func TestSettingsSetAutostart(t *testing.T) {
	useTempAutostartKey(t)
	var emitted []SettingsView
	s := NewSettingsService(openTestStore(t), "", noMonitor, nil, func(v SettingsView) { emitted = append(emitted, v) })
	v, err := s.SetAutostart(true)
	if err != nil || !v.Autostart {
		t.Fatalf("SetAutostart(true): Autostart = %v, err = %v", v.Autostart, err)
	}
	if v, err = s.SetAutostart(false); err != nil || v.Autostart {
		t.Fatalf("SetAutostart(false): Autostart = %v, err = %v", v.Autostart, err)
	}
	if len(emitted) != 2 {
		t.Fatalf("emitted %d views, want 2", len(emitted))
	}
}

// Ruling 35: a -simulate* run never writes the Run entry (it would point the
// owner's autostart at a test exe); the switch reports what is there.
func TestSettingsSetAutostartSimulateIsNoOp(t *testing.T) {
	useTempAutostartKey(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := NewSettingsService(openTestStore(t), "", noMonitor, nil, nil)
	s.Simulate = true
	v, err := s.SetAutostart(true)
	if err != nil || v.Autostart {
		t.Fatalf("SetAutostart(true) in simulate: Autostart = %v, err = %v; want false, nil", v.Autostart, err)
	}
	if on, err := AutostartEnabled(); err != nil || on {
		t.Fatalf("the Run entry was written: %v, %v", on, err)
	}
	// An entry already there stays, and is what the switch shows.
	if err := SetAutostart(true, exe); err != nil {
		t.Fatal(err)
	}
	if v, err = s.SetAutostart(false); err != nil || !v.Autostart {
		t.Fatalf("SetAutostart(false) in simulate: Autostart = %v, err = %v; want true, nil", v.Autostart, err)
	}
	if on, _ := AutostartEnabled(); !on {
		t.Fatal("the Run entry was removed")
	}
}

func TestSettingsRenameWithoutStation(t *testing.T) {
	s := NewSettingsService(openTestStore(t), "", noMonitor, nil, nil)
	if err := s.RenameStation("Дача"); !errors.Is(err, monitor.ErrNoStation) {
		t.Fatalf("err = %v, want ErrNoStation", err)
	}
}

// Saving another language switches tray and toast texts before apply
// redraws the tray.
func TestSettingsSaveSwitchesLanguage(t *testing.T) {
	useTempAutostartKey(t)
	t.Cleanup(func() { i18n.SetLang("uk") })
	i18n.SetLang("uk")
	var quit string
	s := NewSettingsService(openTestStore(t), "", noMonitor,
		func(settings.Settings) { quit = i18n.T("tray.menu.quit") }, nil)
	v := settings.Defaults()
	v.Language = "en"
	got, err := s.Save(v)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lang != "en" || i18n.Lang() != "en" || quit != "Quit" {
		t.Fatalf("view lang %q, i18n %q, menu during apply %q; want en, en, Quit", got.Lang, i18n.Lang(), quit)
	}
}

// The test toast pops whatever quiet hours or "do not disturb" say: it is
// never silent.
func TestSettingsSendTestNotification(t *testing.T) {
	s := NewSettingsService(openTestStore(t), "", noMonitor, nil, nil)
	if err := s.SendTestNotification(); err == nil {
		t.Fatal("no Deliver: want an error")
	}
	var got []notify.Toast
	s.Deliver = func(t notify.Toast) error { got = append(got, t); return nil }
	if err := s.SendTestNotification(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != notify.Test || got[0].Silent || got[0].Tag != "" || got[0].At.IsZero() {
		t.Fatalf("delivered %+v, want one non-silent test toast", got)
	}
}

func TestSettingsOpenWindowsNotificationSettings(t *testing.T) {
	s := NewSettingsService(openTestStore(t), "", noMonitor, nil, nil)
	var opened string
	s.openURL = func(u string) error { opened = u; return nil }
	if err := s.OpenWindowsNotificationSettings(); err != nil || opened != notificationSettingsURL {
		t.Fatalf("opened %q, err %v", opened, err)
	}
}

// Task 4b review minor 1: a change that was stored and applied is never
// reported as failed because the view after it could not be built (here the
// autostart state): Save returns the view with Partial set and emits it.
func TestSettingsSaveSucceedsWhenViewFails(t *testing.T) {
	st := openTestStore(t)
	var applied, emitted int
	s := NewSettingsService(st, `C:\data`, noMonitor,
		func(settings.Settings) { applied++ },
		func(SettingsView) { emitted++ })
	s.autostart = func() (bool, error) { return false, errors.New("registry unreadable") }
	v := settings.Defaults()
	v.RetentionDays = 365
	got, err := s.Save(v)
	if err != nil {
		t.Fatalf("Save = %v, want success: the change was stored and applied", err)
	}
	if !got.Partial || got.S.RetentionDays != 365 || got.DataPath != `C:\data` || got.Version == "" {
		t.Fatalf("view = %+v, want partial with the saved settings", got)
	}
	if applied != 1 || emitted != 1 {
		t.Fatalf("applied %d, emitted %d; want 1, 1", applied, emitted)
	}
	if l, _ := settings.Load(st); l.RetentionDays != 365 {
		t.Fatalf("stored retention = %d", l.RetentionDays)
	}
	if _, err := s.Get(); err == nil {
		t.Fatal("Get = nil error, want the view's error (a refresh reports it)")
	}
}

// ExportCSV asks for a folder and writes the three files there for the
// station's local days; a cancelled dialog is no error and writes nothing.
func TestSettingsExportCSV(t *testing.T) {
	st := openTestStore(t)
	s := NewSettingsService(st, "", noMonitor, nil, nil)
	dir := t.TempDir()
	picked := ""
	s.pickDir = func() (string, error) { return picked, nil }
	if _, err := s.ExportCSV("2026-11-01", "2026-11-02"); err == nil {
		t.Fatal("no station: want an error")
	}
	if err := st.UpsertStation(store.Station{ID: "mac", Model: "VenusE 3.0", BLEMac: "mac", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if paths, err := s.ExportCSV("2026-11-01", "2026-11-02"); err != nil || paths != nil {
		t.Fatalf("cancelled: %v, %v; want nil, nil", paths, err)
	}
	picked = dir
	paths, err := s.ExportCSV("2026-11-01", "2026-11-02")
	if err != nil || len(paths) != 3 || paths[0] != filepath.Join(dir, "marstek-outages-20261101-20261102.csv") {
		t.Fatalf("paths = %v, %v", paths, err)
	}
	// The dialect follows the app language: uk «;», en «,».
	old := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(old) })
	for lang, header := range map[string]string{"uk": "start;end;", "en": "start,end,"} {
		i18n.SetLang(lang)
		ps, err := s.ExportCSV("2026-11-01", "2026-11-02")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(ps[0])
		if !strings.HasPrefix(string(b[3:]), header) {
			t.Fatalf("%s: file starts %q, want %q", lang, b[3:20], header)
		}
	}
	if _, err := s.ExportCSV("2026-11-03", "2026-11-02"); err == nil {
		t.Fatal("from after to: want an error")
	}
}

// ShowInFolder selects an existing file in Explorer; anything else is refused.
func TestSettingsShowInFolder(t *testing.T) {
	s := NewSettingsService(openTestStore(t), "", noMonitor, nil, nil)
	var shown []string
	s.selectFile = func(p string) error { shown = append(shown, p); return nil }
	p := filepath.Join(t.TempDir(), "marstek-outages-20261101-20261102.csv")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(filepath.Dir(p), "notes.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "relative.csv", filepath.Dir(p), p + ".missing", txt,
		`\\server\share\x.csv`, `\\?\` + p, `\\.\` + p} {
		if err := s.ShowInFolder(bad); err == nil {
			t.Fatalf("ShowInFolder(%q): want an error", bad)
		}
	}
	if err := s.ShowInFolder(p); err != nil || len(shown) != 1 || shown[0] != p {
		t.Fatalf("shown %v, %v", shown, err)
	}
}

// PruneNow without a running monitor prunes the stored stations by the saved
// retention; with retention «always» it keeps everything.
func TestSettingsPruneNowWithoutMonitor(t *testing.T) {
	useTempAutostartKey(t)
	st := openTestStore(t)
	s := NewSettingsService(st, "", noMonitor, nil, nil)
	if err := st.UpsertStation(store.Station{ID: "mac", Model: "VenusE 3.0", BLEMac: "mac", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-400 * 24 * time.Hour)
	if err := st.AddEvent(store.Event{StationID: "mac", At: old, Level: "warn", Code: "conn.lanLost", Text: "{}"}); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		es, err := st.Events("mac", "conn.lanLost", time.UnixMilli(0), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return len(es)
	}
	if err := s.PruneNow(); err != nil || count() != 1 {
		t.Fatalf("retention always: %v, %d events; want kept", err, count())
	}
	v := settings.Defaults()
	v.RetentionDays = 365
	if _, err := s.Save(v); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneNow(); err != nil || count() != 0 {
		t.Fatalf("retention 1 year: %v, %d events; want pruned", err, count())
	}
}

// Task 20c (Task 17 carry-over): the Settings banner settings.notify.osDisabled
// shows when Windows blocks the app's toasts; a setting that cannot be read
// shows no banner.
func TestSettingsNotificationsOff(t *testing.T) {
	st := openTestStore(t)
	s := NewSettingsService(st, `C:\data`, noMonitor, nil, nil)
	for _, c := range []struct {
		name string
		off  bool
		err  error
		want bool
	}{
		{"enabled", false, nil, false},
		{"blocked by Windows", true, nil, true},
		{"unknown", true, errors.New("element not found"), false},
	} {
		s.toastsOff = func() (bool, error) { return c.off, c.err }
		if got := s.NotificationsOff(); got != c.want {
			t.Errorf("%s: NotificationsOff = %v, want %v", c.name, got, c.want)
		}
	}
}

// Final review M1: the events CSV «text» column is the sentence the Errors
// tab shows for the entry (the journal's copy key and args, in the app's
// language), not the stored JSON args.
func TestSettingsExportEventsText(t *testing.T) {
	st := openTestStore(t)
	s := NewSettingsService(st, "", noMonitor, nil, nil)
	dir := t.TempDir()
	s.pickDir = func() (string, error) { return dir, nil }
	if err := st.UpsertStation(store.Station{ID: "mac", Model: "VenusE 3.0", BLEMac: "mac", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.Local) }
	for _, e := range []store.Event{
		{At: at(1, 10, 0), Level: "warn", Code: "station.likelyOut", Text: `{"soc":"23","at":"` + at(1, 9, 55).UTC().Format(time.RFC3339) + `"}`},
		{At: at(1, 11, 0), Level: "info", Code: "conn.lanRestored", Text: `{"duration_ms":"5400000"}`},
		{At: at(2, 0, 10), Level: "warn", Code: "station.likelyOut", Text: `{"soc":"9","at":"` + at(1, 23, 50).UTC().Format(time.RFC3339) + `"}`},
		{At: at(2, 8, 0), Level: "info", Code: "battery.fwUnverified", Text: `{"firmware":"152"}`},
		{At: at(2, 9, 0), Level: "warn", Code: "station.lowCharge", Text: `{"soc":"20"}`},
	} {
		e.StationID = "mac"
		e.Source = "app"
		if err := st.AddEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	old := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(old) })
	for lang, want := range map[string][]string{
		"uk": {
			"Станція, ймовірно, розрядилась ≈ о 09:55 (заряд перед цим 23%)",
			"Зв'язок по локальній мережі відновлено (не було 1 год 30 хв)",
			"Станція, ймовірно, розрядилась ≈ о 01.11 23:50 (заряд перед цим 9%)",
			"Нова прошивка v152 — дані батареї вимкнено, поки не перевірено",
			"Низький заряд — 20%",
		},
		"en": {
			"The station probably ran out at ≈ 09:55 (charge before that 23%)",
			"Local connection restored (lost for 1 h 30 min)",
			"The station probably ran out at ≈ 1 Nov 23:50 (charge before that 9%)",
			"New firmware v152 — battery data off until tested",
			"Low charge — 20%",
		},
	} {
		i18n.SetLang(lang)
		paths, err := s.ExportCSV("2026-11-01", "2026-11-02")
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(paths[1])
		if err != nil {
			t.Fatal(err)
		}
		r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\uFEFF")))
		r.Comma = export.FormatFor(lang).Sep
		rows, err := r.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(rows[0], " ") != "at level code text" {
			t.Fatalf("%s header %v (a raw column must not be added)", lang, rows[0])
		}
		var got []string
		for _, row := range rows[1:] {
			got = append(got, row[3])
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s text column:\n%s\nwant:\n%s", lang, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// Every journal code has its sentence in both Go catalogues, so the events
// CSV never shows a raw copy key.
func TestJournalTextEveryCode(t *testing.T) {
	old := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(old) })
	for _, lang := range []string{"uk", "en"} {
		i18n.SetLang(lang)
		for _, code := range journal.Codes {
			en := journal.Build([]store.Event{{ID: 1, At: time.Now(), Code: code}}, nil, time.Now())
			if len(en) != 1 {
				t.Fatalf("%s: %d entries", code, len(en))
			}
			cat := i18n.UK
			if lang == "en" {
				cat = i18n.EN
			}
			if _, ok := cat[en[0].Key]; !ok {
				t.Errorf("%s: %s (%s) not in the Go catalogue", lang, en[0].Key, code)
			}
		}
	}
}

func TestSettingsViewPlatform(t *testing.T) {
	useTempAutostartKey(t)
	s := NewSettingsService(openTestStore(t), `C:\data`, noMonitor, nil, nil)
	v, err := s.Get()
	if err != nil || v.Platform != runtime.GOOS {
		t.Fatalf("Platform = %q, err = %v; want %q", v.Platform, err, runtime.GOOS)
	}
}
