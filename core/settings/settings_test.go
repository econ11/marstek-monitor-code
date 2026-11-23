package settings

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestDefaults(t *testing.T) {
	want := Settings{
		Language:       "",
		MinimizeToTray: true,
		Notify: notify.Settings{
			LowThreshold: 20,
			QuietEnabled: false,
			QuietStart:   "23:00",
			QuietEnd:     "07:00",
			NoLinkAfter:  10 * time.Minute,
			Enabled:      map[notify.Kind]bool{notify.NoLink: false},
		},
		PollInterval:  5 * time.Second,
		RetentionDays: 0,
		StationMinSOC: 10,
		UpdateCheck:   true,
	}
	got := Defaults()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Defaults:\n got %+v\nwant %+v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	// Each call returns its own Enabled map.
	got.Notify.Enabled[notify.GridLost] = false
	if _, ok := Defaults().Notify.Enabled[notify.GridLost]; ok {
		t.Fatal("Defaults shares its Enabled map between calls")
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	got, err := Load(openStore(t))
	if err != nil || !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("Load = %+v, %v; want defaults", got, err)
	}
}

func TestLoadCorruptJSONReturnsDefaults(t *testing.T) {
	st := openStore(t)
	if err := st.SetSetting(Key, "{"); err != nil {
		t.Fatal(err)
	}
	got, err := Load(st)
	if err != nil || !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("Load = %+v, %v; want defaults", got, err)
	}
}

func TestLoadInvalidValuesFallBackToDefaults(t *testing.T) {
	st := openStore(t)
	raw := `{"Language":"de","PollInterval":3000000000,"MinimizeToTray":false,"BatteryDetails":"always",` +
		`"Notify":{"LowThreshold":15,"QuietStart":"25:00","QuietEnd":"06:30","QuietEnabled":true}}`
	if err := st.SetSetting(Key, raw); err != nil {
		t.Fatal(err)
	}
	got, err := Load(st)
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.MinimizeToTray = false
	want.Notify.QuietEnd = "06:30"
	want.Notify.QuietEnabled = true
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load:\n got %+v\nwant %+v", got, want)
	}

	// A wrongly typed field falls back alone; the rest is kept.
	raw = `{"Language":"en","PollInterval":"fast","RetentionDays":365,"StationMinSOC":50,` +
		`"Notify":{"LowThreshold":"x","NoLinkAfter":900000000000,"QuietStart":"7:00"}}`
	if err := st.SetSetting(Key, raw); err != nil {
		t.Fatal(err)
	}
	got, err = Load(st)
	if err != nil {
		t.Fatal(err)
	}
	want = Defaults()
	want.Language = "en"
	want.RetentionDays = 365
	want.Notify.NoLinkAfter = 15 * time.Minute
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load (types):\n got %+v\nwant %+v", got, want)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	st := openStore(t)
	good := Defaults()
	good.Language = "uk"
	if err := Save(st, good); err != nil {
		t.Fatal(err)
	}
	before, _, _ := st.GetSetting(Key)
	bad := map[string]func(*Settings){
		"poll 3s":       func(s *Settings) { s.PollInterval = 3 * time.Second },
		"low 15":        func(s *Settings) { s.Notify.LowThreshold = 15 },
		"quiet 7:00":    func(s *Settings) { s.Notify.QuietStart = "7:00" },
		"quiet 24:00":   func(s *Settings) { s.Notify.QuietEnd = "24:00" },
		"language de":   func(s *Settings) { s.Language = "de" },
		"no link 7m":    func(s *Settings) { s.Notify.NoLinkAfter = 7 * time.Minute },
		"retention 100": func(s *Settings) { s.RetentionDays = 100 },
		"min soc 4":     func(s *Settings) { s.StationMinSOC = 4 },
		"min soc 31":    func(s *Settings) { s.StationMinSOC = 31 },
	}
	for name, mutate := range bad {
		s := good
		s.Notify.Enabled = map[notify.Kind]bool{notify.NoLink: false}
		mutate(&s)
		if err := Save(st, s); err == nil {
			t.Errorf("%s: Save accepted it", name)
		}
		if after, _, _ := st.GetSetting(Key); after != before {
			t.Errorf("%s: stored value changed to %s", name, after)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	st := openStore(t)
	in := Settings{
		Language:       "en",
		MinimizeToTray: false,
		Notify: notify.Settings{
			LowThreshold: 40,
			QuietEnabled: true,
			QuietStart:   "22:30",
			QuietEnd:     "06:00",
			NoLinkAfter:  60 * time.Minute,
			Enabled:      map[notify.Kind]bool{notify.NoLink: true, notify.Full: false},
		},
		PollInterval:  30 * time.Second,
		RetentionDays: 1825,
		StationMinSOC: 25,
	}
	if err := Save(st, in); err != nil {
		t.Fatal(err)
	}
	got, err := Load(st)
	if err != nil || !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip:\n got %+v (%v)\nwant %+v", got, err, in)
	}
}

func TestValidName(t *testing.T) {
	if err := ValidName("  "); !errors.Is(err, ErrNameEmpty) {
		t.Errorf("blank: %v", err)
	}
	if err := ValidName(strings.Repeat("ж", NameMax+1)); !errors.Is(err, ErrNameTooLong) {
		t.Errorf("25 runes: %v", err)
	}
	if err := ValidName(" " + strings.Repeat("ж", NameMax) + " "); err != nil {
		t.Errorf("24 runes with spaces around: %v", err)
	}
	if err := ValidName("Дача"); err != nil {
		t.Errorf("Дача: %v", err)
	}
}

func TestApplyProfileOverridesMinSOC(t *testing.T) {
	s := Defaults()
	s.StationMinSOC = 20
	p := s.ApplyProfile(model.ProfileFor("VenusE 3.0"))
	if p.MinSOC != 20 || p.NominalWh != 5120 {
		t.Fatalf("profile: %+v", p)
	}
}

func TestLoadEnabledMergesWithDefaults(t *testing.T) {
	st := openStore(t)
	load := func(raw string) map[notify.Kind]bool {
		t.Helper()
		if err := st.SetSetting(Key, raw); err != nil {
			t.Fatal(err)
		}
		s, err := Load(st)
		if err != nil {
			t.Fatal(err)
		}
		return s.Notify.Enabled
	}
	got := load(`{"Notify":{"Enabled":{"full":false}}}`)
	if want := (map[notify.Kind]bool{notify.NoLink: false, notify.Full: false}); !reflect.DeepEqual(got, want) {
		t.Errorf("full:false: got %v want %v", got, want)
	}
	got = load(`{"Notify":{"Enabled":{"no_link":true,"bogus":false,"full":null,"grid_lost":1}}}`)
	if want := (map[notify.Kind]bool{notify.NoLink: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("unknown/null/non-bool: got %v want %v", got, want)
	}
	for _, raw := range []string{`{"Notify":{"Enabled":null}}`, `{"Notify":{"Enabled":[1]}}`} {
		if got := load(raw); !reflect.DeepEqual(got, Defaults().Notify.Enabled) {
			t.Errorf("%s: got %v", raw, got)
		}
	}
	// Save with a nil map round-trips to the defaults, no_link still off.
	s := Defaults()
	s.Notify.Enabled = nil
	if err := Save(st, s); err != nil {
		t.Fatal(err)
	}
	if l, _ := Load(st); !reflect.DeepEqual(l, Defaults()) {
		t.Errorf("nil Enabled round trip: %+v", l)
	}
}

func TestApplyProfileZeroSettingsKeepsProfile(t *testing.T) {
	if p := (Settings{}).ApplyProfile(model.ProfileFor("VenusE 3.0")); p.MinSOC != 10 {
		t.Fatalf("zero Settings changed MinSOC to %v", p.MinSOC)
	}
}

// Lives here, not in notify: notify importing settings would be an import cycle.
func TestNoLinkOffByDefault(t *testing.T) {
	t0 := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	p := notify.NewPolicy(Defaults().Notify, t0.Add(-time.Hour))
	since, last := t0, t0.Add(-5*time.Minute)
	for d := time.Duration(0); d <= 3*time.Hour; d += time.Minute {
		if got := p.OnLink(true, since, last, nil, since.Add(d)); got != nil {
			t.Fatalf("+%s: default settings toasted %+v", d, got)
		}
	}
	if got := p.OnLink(false, time.Time{}, t0.Add(4*time.Hour), nil, t0.Add(4*time.Hour)); got != nil {
		t.Fatalf("restored: %+v", got)
	}
}

// The battery data reads (Bat.GetStatus) were removed (owner decision
// 2026-11-23): a BatteryDetails saved by an older version is ignored, the
// other fields are kept, and the next Save no longer writes it.
func TestLoadIgnoresRemovedBatteryDetails(t *testing.T) {
	st := openStore(t)
	for _, v := range []string{"auto", "on", "off", "always"} {
		raw := `{"RetentionDays":365,"BatteryDetails":"` + v + `"}`
		if err := st.SetSetting(Key, raw); err != nil {
			t.Fatal(err)
		}
		got, err := Load(st)
		want := Defaults()
		want.RetentionDays = 365
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: Load = %+v, %v; want defaults with retention 365", v, got, err)
		}
		if err := Save(st, got); err != nil {
			t.Fatal(err)
		}
		if after, _, _ := st.GetSetting(Key); strings.Contains(after, "BatteryDetails") {
			t.Fatalf("%s: saved %s", v, after)
		}
	}
}

// Plan 3 Task 4: the daily update check is on by default; settings saved by
// an older version (no UpdateCheck) load with it on, a stored false stays
// off, a wrongly typed value takes the default.
func TestUpdateCheckDefaultOn(t *testing.T) {
	st := openStore(t)
	for raw, want := range map[string]bool{
		`{"RetentionDays":365}`:                     true,
		`{"RetentionDays":365,"UpdateCheck":false}`: false,
		`{"RetentionDays":365,"UpdateCheck":true}`:  true,
		`{"RetentionDays":365,"UpdateCheck":"no"}`:  true,
		`{"RetentionDays":365,"UpdateCheck":null}`:  true,
	} {
		if err := st.SetSetting(Key, raw); err != nil {
			t.Fatal(err)
		}
		got, err := Load(st)
		if err != nil || got.UpdateCheck != want || got.RetentionDays != 365 {
			t.Fatalf("%s: Load = UpdateCheck %v retention %d, %v; want %v, 365", raw, got.UpdateCheck, got.RetentionDays, err, want)
		}
	}
	off := Defaults()
	off.UpdateCheck = false
	if err := Save(st, off); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(st); got.UpdateCheck {
		t.Fatal("a saved UpdateCheck false loads as true")
	}
}
