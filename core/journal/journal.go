// Package journal builds the user's event journal (the "Errors" tab). In v1
// the station sends no error codes over LAN (spec §9 item 3), so the journal
// is the app's connection events, events derived from the station's data and
// the app's own absences (data gaps). Internal debug events (link.*,
// monitor.cadence) stay in the database and never reach the journal: Build
// keeps only the codes listed here.
package journal

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

// Level is an entry's severity.
type Level string

const (
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
)

// Group is what an entry is about (the "connection" filter shows Connection
// and App).
type Group string

const (
	Connection Group = "connection"
	Station    Group = "station"
	App        Group = "app"
)

// Entry is one journal row. Key and TodoKey are copy keys ("what happened"
// and "what to do"; TodoKey "" = no advice) filled with Args. ID is
// "<code>:<event id>" for events and "app.gap:<gap id>" for gaps. Code is the
// station's error code: always "" in v1.
type Entry struct {
	ID      string
	At      time.Time
	Level   Level
	Group   Group
	Key     string
	TodoKey string
	Args    map[string]string
	Code    string
}

// Event codes (store.Event.Code) the monitor writes for the journal. Text is
// the JSON object of the entry's Args; the stored level is info | warn | error.
const (
	CodeAPISilent   = "conn.apiSilent"
	CodeLANLost     = "conn.lanLost"
	CodeLANRestored = "conn.lanRestored"
	CodeIPChanged   = "conn.ipChanged"
	// CodeLinkDropped: a short interruption (STALE → LIVE, no gap). Args from,
	// to (RFC 3339) and seconds; Info, no advice.
	CodeLinkDropped = "conn.linkDropped"
	CodeLikelyOut   = "station.likelyOut"
	CodeCutoff      = "station.cutoff"
	CodeLowCharge   = "station.lowCharge"
)

// Codes of the removed battery data reads (Bat.GetStatus, removed by the
// owner's decision of 2026-11-23, Ruling 57). Nothing writes them any more;
// they stay readable so the rows an older version stored still show, with
// their text and no advice (the settings the advice named are gone).
const (
	legacyBatReadsStopped  = "battery.readsStopped"
	legacyBatReadsResumed  = "battery.readsResumed"
	legacyChargeBlocked    = "station.chargeBlocked"
	legacyChargeAllowed    = "station.chargeAllowed"
	legacyDischargeBlocked = "station.dischargeBlocked"
	legacyDischargeAllowed = "station.dischargeAllowed"
	legacyFwUnverified     = "battery.fwUnverified"
	legacyFwVerified       = "battery.fwVerified"
)

// Codes lists every journal event code (a reader asks the store per code).
var Codes = []string{CodeAPISilent, CodeLANLost, CodeLANRestored, CodeIPChanged, CodeLinkDropped, CodeLikelyOut, CodeCutoff, CodeLowCharge,
	legacyBatReadsStopped, legacyBatReadsResumed, legacyChargeBlocked, legacyChargeAllowed, legacyDischargeBlocked,
	legacyDischargeAllowed, legacyFwUnverified, legacyFwVerified}

// IsLegacyCode reports a code of the removed battery data reads.
func IsLegacyCode(code string) bool {
	switch code {
	case legacyBatReadsStopped, legacyBatReadsResumed, legacyChargeBlocked, legacyChargeAllowed, legacyDischargeBlocked,
		legacyDischargeAllowed, legacyFwUnverified, legacyFwVerified:
		return true
	}
	return false
}

type kind struct {
	key, todo string
	level     Level // when the stored level is not one of info | warn | error
	group     Group
}

var kinds = map[string]kind{
	CodeAPISilent:          {"conn.event.apiSilent", "conn.event.apiSilent.todo", Warning, Connection},
	CodeLANLost:            {"conn.event.lanLost.noFallback", "conn.event.lanLost.noFallback.todo", Warning, Connection},
	CodeLANRestored:        {"conn.event.lanRestored", "", Info, Connection},
	CodeIPChanged:          {"conn.event.ipChanged", "", Info, Connection},
	CodeLinkDropped:        {"journal.linkDropped", "", Info, Connection},
	CodeLikelyOut:          {"journal.likelyOut", "journal.likelyOut.todo", Warning, Station},
	CodeCutoff:             {"journal.cutoff", "journal.cutoff.todo", Warning, Station},
	CodeLowCharge:          {"journal.lowCharge", "journal.lowCharge.todo", Warning, Station},
	legacyBatReadsStopped:  {"journal.batReadsStopped", "", Info, App},
	legacyBatReadsResumed:  {"journal.batReadsResumed", "", Info, App},
	legacyChargeBlocked:    {"journal.chargeBlocked", "", Info, Station},
	legacyChargeAllowed:    {"journal.chargeAllowed", "", Info, Station},
	legacyDischargeBlocked: {"journal.dischargeBlocked", "", Info, Station},
	legacyDischargeAllowed: {"journal.dischargeAllowed", "", Info, Station},
	legacyFwUnverified:     {"journal.fwUnverified", "", Info, Station},
	legacyFwVerified:       {"journal.fwVerified", "", Info, Station},
}

// gapKeys: the gaps that are the app's own absences. Other gaps (no_link,
// likely_dead) are told by their connection and station events.
var gapKeys = map[store.GapReason]string{
	store.GapAppNotRunning: "journal.appOff",
	store.GapPCSleep:       "journal.pcSleep",
	store.GapAppUpdate:     "journal.appUpdate",
}

// GapCode prefixes the ID of a gap entry.
const GapCode = "app.gap"

func levelOf(stored string, def Level) Level {
	switch stored {
	case "info":
		return Info
	case "warn":
		return Warning
	case "error":
		return Error
	}
	return def
}

// Build turns events and gaps into journal entries, newest first. Events with
// other codes are skipped; app_not_running and pc_sleep gaps become App
// entries with Args start / end (RFC 3339, UTC; an open gap ends now).
func Build(events []store.Event, gaps []store.Gap, now time.Time) []Entry {
	var rs []Entry
	for _, e := range events {
		k, ok := kinds[e.Code]
		if !ok {
			continue
		}
		var args map[string]string
		if e.Text != "" {
			if err := json.Unmarshal([]byte(e.Text), &args); err != nil {
				args = nil // an unreadable text still leaves the entry
			}
		}
		rs = append(rs, Entry{ID: e.Code + ":" + strconv.FormatInt(e.ID, 10), At: e.At,
			Level: entryLevel(e, k), Group: k.group, Key: k.key, TodoKey: k.todo, Args: args})
	}
	for _, g := range gaps {
		key, ok := gapKeys[g.Reason]
		if !ok {
			continue
		}
		end := now
		if g.End != nil {
			end = *g.End
		}
		rs = append(rs, Entry{ID: GapCode + ":" + strconv.FormatInt(g.ID, 10), At: g.Start, Level: Info, Group: App,
			Key: key, Args: map[string]string{"start": Stamp(g.Start), "end": Stamp(end)}})
	}
	// Newest first; the same instant by ID, descending: a total order, so an
	// (At, ID) cursor pages without skipping or repeating.
	sort.Slice(rs, func(i, j int) bool { return rs[i].Before(CursorOf(rs[j])) })
	return rs
}

// NewSince counts the entries at warning level or above newer than seen.
func NewSince(entries []Entry, seen time.Time) int {
	n := 0
	for _, e := range entries {
		if e.Level != Info && e.At.After(seen) {
			n++
		}
	}
	return n
}

// Stamp formats a time argument (RFC 3339, UTC), as Build does for gaps.
func Stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// entryLevel is the level an event shows at: a legacy code and a link drop are
// always Info, whatever their row stored (the row itself is left as it is).
func entryLevel(e store.Event, k kind) Level {
	if IsLegacyCode(e.Code) || e.Code == CodeLinkDropped {
		return Info
	}
	return levelOf(e.Level, k.level)
}

// Cursor marks a position in the journal order (newest first; ties at the
// same At ordered by ID, descending). A page continues strictly after it.
type Cursor struct {
	At time.Time
	ID string
}

// CursorOf is the cursor at entry e.
func CursorOf(e Entry) Cursor { return Cursor{At: e.At, ID: e.ID} }

// Before reports whether e comes before c in the journal order, i.e. is newer
// (or the same instant with a greater ID).
func (e Entry) Before(c Cursor) bool {
	if !e.At.Equal(c.At) {
		return e.At.After(c.At)
	}
	return IDGreater(e.ID, c.ID)
}

// After reports whether e comes after c in the journal order (older).
func (e Entry) After(c Cursor) bool {
	if !e.At.Equal(c.At) {
		return e.At.Before(c.At)
	}
	return IDGreater(c.ID, e.ID)
}

// IDGreater orders two entry IDs ("<code>:<number>"): with the same code by
// the number (so "x:10" is above "x:9"), else by the code. A total order.
func IDGreater(a, b string) bool {
	ac, an := splitID(a)
	bc, bn := splitID(b)
	if ac != bc {
		return ac > bc
	}
	if an != bn {
		return an > bn
	}
	return a > b
}

func splitID(id string) (code string, n int64) {
	i := strings.LastIndex(id, ":")
	if i < 0 {
		return id, 0
	}
	n, _ = strconv.ParseInt(id[i+1:], 10, 64)
	return id[:i], n
}
