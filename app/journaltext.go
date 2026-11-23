package app

import (
	"strconv"
	"time"

	"github.com/econ11/marstek-monitor-code/app/i18n"
	"github.com/econ11/marstek-monitor-code/app/trayicon"
	"github.com/econ11/marstek-monitor-code/core/journal"
)

// journalText is a journal entry's sentence in the app's language, as the
// Errors tab shows it (frontend errorsModel rowOf/argsOf): its copy key
// filled with its args:
//   - at → {time}; start / end → {start} / {end} («14:05» on the entry's day,
//     else «20.11 14:05» / «20 Nov 14:05»; the end against the start);
//   - from / to → {from} / {to}, local HH:MM:SS;
//   - duration_ms → {duration} (the short form); seconds → {duration} (seconds
//     under a minute, else the short form).
//
// An arg that can't be read is left out, so its {placeholder} shows instead of
// a wrong value. The events CSV writes it in its «text» column
// (export.Format.EventText).
func journalText(e journal.Entry) string {
	kv := make([]string, 0, 2*len(e.Args))
	for k, v := range e.Args {
		switch k {
		case "at", "start", "end", "duration_ms", "from", "to", "seconds":
		default:
			kv = append(kv, k, v)
		}
	}
	stamp := func(k string) (time.Time, bool) {
		t, err := time.Parse(time.RFC3339, e.Args[k])
		return t, err == nil
	}
	if t, ok := stamp("at"); ok {
		kv = append(kv, "time", entryTime(t, e.At))
	}
	ref := e.At
	if t, ok := stamp("start"); ok {
		kv = append(kv, "start", entryTime(t, e.At))
		ref = t
	}
	if t, ok := stamp("end"); ok {
		kv = append(kv, "end", entryTime(t, ref))
	}
	if ms, err := strconv.ParseInt(e.Args["duration_ms"], 10, 64); err == nil {
		kv = append(kv, "duration", trayicon.ShortDuration(time.Duration(ms)*time.Millisecond))
	}
	// A short link drop: the clock times to the second, the duration in seconds
	// under a minute, else the short form.
	if t, ok := stamp("from"); ok {
		kv = append(kv, "from", t.Local().Format("15:04:05"))
	}
	if t, ok := stamp("to"); ok {
		kv = append(kv, "to", t.Local().Format("15:04:05"))
	}
	if s, err := strconv.ParseInt(e.Args["seconds"], 10, 64); err == nil {
		if s < 60 {
			kv = append(kv, "duration", i18n.T("duration.short.s", "s", strconv.FormatInt(max(s, 0), 10)))
		} else {
			kv = append(kv, "duration", trayicon.ShortDuration(time.Duration(s)*time.Second))
		}
	}
	return i18n.T(e.Key, kv...)
}

// entryTime is «14:05» on ref's local day, else the short date and the time
// (uk «20.11 14:05», en «20 Nov 14:05»), as the Errors tab's timeText.
func entryTime(t, ref time.Time) string {
	t, ref = t.Local(), ref.Local()
	ty, tm, td := t.Date()
	ry, rm, rd := ref.Date()
	if ty == ry && tm == rm && td == rd {
		return hhmm(t)
	}
	layout := "02.01" // uk
	if i18n.Lang() != "uk" {
		layout = "2 Jan" // en-GB
	}
	return t.Format(layout) + " " + hhmm(t)
}
