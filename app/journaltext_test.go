package app

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/store"
)

func TestJournalTextLinkDropped(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("uk") })
	from := time.Date(2026, 11, 22, 12, 0, 10, 0, time.Local)
	entry := func(secs string, to time.Time) journal.Entry {
		return journal.Entry{ID: "conn.linkDropped:1", At: to, Level: journal.Info, Group: journal.Connection, Key: "journal.linkDropped",
			Args: map[string]string{"from": journal.Stamp(from), "to": journal.Stamp(to), "seconds": secs}}
	}
	cases := []struct {
		lang, secs string
		to         time.Time
		want       string
	}{
		{"uk", "40", from.Add(40 * time.Second), "Станція не відповідала 40 с (12:00:10–12:00:50)"},
		{"en", "40", from.Add(40 * time.Second), "The station didn't respond for 40 s (12:00:10–12:00:50)"},
		{"uk", "150", from.Add(150 * time.Second), "Станція не відповідала 2 хв (12:00:10–12:02:40)"},
		{"en", "150", from.Add(150 * time.Second), "The station didn't respond for 2 min (12:00:10–12:02:40)"},
	}
	for _, c := range cases {
		i18n.SetLang(c.lang)
		if got := journalText(entry(c.secs, c.to)); got != c.want {
			t.Errorf("%s %ss: %q, want %q", c.lang, c.secs, got, c.want)
		}
	}
}

// The events CSV is built from journal.Codes only (no gap entries), so every
// key an entry of those codes can carry must exist in both Go catalogues.
func TestJournalTextKeysExistInCatalogues(t *testing.T) {
	for _, code := range journal.Codes {
		es := journal.Build([]store.Event{{ID: 1, StationID: "st", At: time.Now(), Level: "info", Code: code, Text: "{}"}}, nil, time.Now())
		if len(es) != 1 {
			t.Fatalf("%s: %d entries", code, len(es))
		}
		for lang, cat := range map[string]map[string]string{"uk": i18n.UK, "en": i18n.EN} {
			if _, ok := cat[es[0].Key]; !ok {
				t.Errorf("%s: key %s missing in %s", code, es[0].Key, lang)
			}
		}
	}
}
