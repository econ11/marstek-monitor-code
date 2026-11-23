// Package settings holds the user's settings: one JSON value under Key in the
// store's settings table, typed, with defaults and validation. A bad stored
// field never stops the app: it falls back to its default alone.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// Key is the settings-table key that holds the JSON.
const Key = "app.settings"

// Settings are the user's settings.
type Settings struct {
	Language       string // "" = Windows language, "uk", "en"
	MinimizeToTray bool
	Notify         notify.Settings
	PollInterval   time.Duration // the active interval; calm polling is max(20 s, this)
	RetentionDays  int           // 0 = keep forever
	// StationMinSOC (percent) overrides model.Profile.MinSOC: the charge the
	// station stops discharging at, used by forecasts, likely-dead and the
	// cutoff (see ApplyProfile).
	StationMinSOC int
	// UpdateCheck: look for a newer version once a day (plan 3, Task 4).
	// Settings saved before it existed load with it on.
	UpdateCheck bool
}

// The choices the UI offers; anything else is invalid.
var (
	LowThresholds    = []float64{10, 20, 30, 40, 50}
	NoLinkAfters     = []time.Duration{5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute, 60 * time.Minute}
	PollIntervals    = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	RetentionChoices = []int{0, 365, 730, 1825}
	Languages        = []string{"", "uk", "en"}
)

// StationMinSOC range, whole percent.
const (
	MinSOCLow  = 5
	MinSOCHigh = 30
)

// NameMax is the longest station name, in runes.
const NameMax = 24

var (
	ErrNameEmpty   = errors.New("station name is empty")
	ErrNameTooLong = fmt.Errorf("station name is longer than %d characters", NameMax)
)

// Defaults returns the default settings; each call returns a fresh Enabled map.
func Defaults() Settings {
	return Settings{
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
}

// ApplyProfile returns p with MinSOC replaced by the user's setting. A zero
// StationMinSOC (a zero-value Settings, never a valid stored one) keeps the
// profile's own MinSOC.
func (s Settings) ApplyProfile(p model.Profile) model.Profile {
	if s.StationMinSOC == 0 {
		return p
	}
	p.MinSOC = float64(s.StationMinSOC)
	return p
}

// Load reads the settings. A missing key or unparsable JSON gives Defaults();
// each field that is missing, wrongly typed or invalid takes its default
// (with a warning), the rest is kept. Only a store error is returned.
func Load(st *store.Store) (Settings, error) {
	s := Defaults()
	raw, ok, err := st.GetSetting(Key)
	if err != nil {
		return s, err
	}
	if !ok {
		return s, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		slog.Warn("settings: unreadable, using defaults", "err", err)
		return s, nil
	}
	field(top, "Language", &s.Language, validLanguage)
	field(top, "MinimizeToTray", &s.MinimizeToTray, nil)
	field(top, "PollInterval", &s.PollInterval, validPoll)
	field(top, "RetentionDays", &s.RetentionDays, validRetention)
	field(top, "StationMinSOC", &s.StationMinSOC, validMinSOC)
	field(top, "UpdateCheck", &s.UpdateCheck, nil)
	if r, ok := top["Notify"]; ok {
		var nm map[string]json.RawMessage
		if err := json.Unmarshal(r, &nm); err != nil {
			slog.Warn("settings: bad field, using default", "field", "Notify", "err", err)
		} else {
			n := &s.Notify
			field(nm, "LowThreshold", &n.LowThreshold, validLow)
			field(nm, "QuietEnabled", &n.QuietEnabled, nil)
			field(nm, "QuietStart", &n.QuietStart, validHHMM)
			field(nm, "QuietEnd", &n.QuietEnd, validHHMM)
			field(nm, "NoLinkAfter", &n.NoLinkAfter, validNoLink)
			mergeEnabled(nm, n.Enabled)
		}
	}
	return s, nil
}

// mergeEnabled copies into dst (the default map) only known kinds stored as a
// real true/false, so a stored {"full":false} keeps no_link off. Anything else
// is ignored with a warning.
func mergeEnabled(nm map[string]json.RawMessage, dst map[notify.Kind]bool) {
	r, ok := nm["Enabled"]
	if !ok {
		return
	}
	var em map[string]json.RawMessage
	if err := json.Unmarshal(r, &em); err != nil || em == nil {
		slog.Warn("settings: bad field, using default", "field", "Notify.Enabled", "err", err)
		return
	}
	for k, v := range em {
		lit := string(bytes.TrimSpace(v))
		if !slices.Contains(notify.Kinds, notify.Kind(k)) || lit != "true" && lit != "false" {
			slog.Warn("settings: bad Enabled entry, ignored", "kind", k, "value", lit)
			continue
		}
		dst[notify.Kind(k)] = lit == "true"
	}
}

// field decodes m[name] into dst if present, well typed, not null and valid.
func field[T any](m map[string]json.RawMessage, name string, dst *T, valid func(T) error) {
	r, ok := m[name]
	if !ok {
		return
	}
	var v T
	err := json.Unmarshal(r, &v)
	if err == nil && bytes.Equal(bytes.TrimSpace(r), []byte("null")) {
		err = errors.New("null")
	}
	if err == nil && valid != nil {
		err = valid(v)
	}
	if err != nil {
		slog.Warn("settings: bad field, using default", "field", name, "err", err)
		return
	}
	*dst = v
}

// Save validates s and stores it; an invalid s leaves the stored value as it was.
func Save(st *store.Store, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return st.SetSetting(Key, string(b))
}

// Validate reports every invalid field.
func (s Settings) Validate() error {
	n := s.Notify
	return errors.Join(
		named("Language", validLanguage(s.Language)),
		named("PollInterval", validPoll(s.PollInterval)),
		named("RetentionDays", validRetention(s.RetentionDays)),
		named("StationMinSOC", validMinSOC(s.StationMinSOC)),
		named("Notify.LowThreshold", validLow(n.LowThreshold)),
		named("Notify.QuietStart", validHHMM(n.QuietStart)),
		named("Notify.QuietEnd", validHHMM(n.QuietEnd)),
		named("Notify.NoLinkAfter", validNoLink(n.NoLinkAfter)),
	)
}

func named(field string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("settings: %s: %w", field, err)
}

// ValidName checks a station name after trimming spaces.
func ValidName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameEmpty
	}
	if utf8.RuneCountInString(name) > NameMax {
		return ErrNameTooLong
	}
	return nil
}

func oneOf[T comparable](choices []T, v T) error {
	if slices.Contains(choices, v) {
		return nil
	}
	return fmt.Errorf("%v is not one of %v", v, choices)
}

func validLanguage(v string) error      { return oneOf(Languages, v) }
func validPoll(v time.Duration) error   { return oneOf(PollIntervals, v) }
func validNoLink(v time.Duration) error { return oneOf(NoLinkAfters, v) }
func validRetention(v int) error        { return oneOf(RetentionChoices, v) }
func validLow(v float64) error          { return oneOf(LowThresholds, v) }

func validMinSOC(v int) error {
	if v < MinSOCLow || v > MinSOCHigh {
		return fmt.Errorf("%d is outside %d..%d", v, MinSOCLow, MinSOCHigh)
	}
	return nil
}

// hhmm is strict: two digits each ("7:00" is rejected, though time.Parse
// with "15:04" accepts it).
var hhmm = regexp.MustCompile(`^\d\d:\d\d$`)

func validHHMM(v string) error {
	if !hhmm.MatchString(v) {
		return fmt.Errorf("%q is not HH:MM", v)
	}
	h, m := int(v[0]-'0')*10+int(v[1]-'0'), int(v[3]-'0')*10+int(v[4]-'0')
	if h > 23 || m > 59 {
		return fmt.Errorf("%q is outside 00:00..23:59", v)
	}
	return nil
}
