package journal

import (
	"strconv"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func ev(id int64, at time.Time, level, code, text string) store.Event {
	return store.Event{ID: id, StationID: "st", At: at, Level: level, Code: code, Text: text, Source: "monitor"}
}

func TestBuildMapsCodesAndLevels(t *testing.T) {
	cases := []struct {
		code, level, text string
		key, todo         string
		lvl               Level
		grp               Group
		args              map[string]string
	}{
		{CodeAPISilent, "warn", "{}", "conn.event.apiSilent", "conn.event.apiSilent.todo", Warning, Connection, nil},
		{CodeLANLost, "warn", "{}", "conn.event.lanLost.noFallback", "conn.event.lanLost.noFallback.todo", Warning, Connection, nil},
		{CodeLANRestored, "info", `{"duration_ms":"360000"}`, "conn.event.lanRestored", "", Info, Connection, map[string]string{"duration_ms": "360000"}},
		{CodeIPChanged, "info", `{"ip":"192.168.1.7"}`, "conn.event.ipChanged", "", Info, Connection, map[string]string{"ip": "192.168.1.7"}},
		{CodeLikelyOut, "warn", `{"at":"2026-11-22T11:00:00Z","soc":"12"}`, "journal.likelyOut", "journal.likelyOut.todo", Warning, Station,
			map[string]string{"at": "2026-11-22T11:00:00Z", "soc": "12"}},
		{CodeCutoff, "warn", `{"soc":"10"}`, "journal.cutoff", "journal.cutoff.todo", Warning, Station, map[string]string{"soc": "10"}},
		{CodeLowCharge, "warn", `{"soc":"19"}`, "journal.lowCharge", "journal.lowCharge.todo", Warning, Station, map[string]string{"soc": "19"}},
		{CodeLowCharge, "error", `{"soc":"19"}`, "journal.lowCharge", "journal.lowCharge.todo", Error, Station, map[string]string{"soc": "19"}},
		{legacyBatReadsStopped, "warn", "{}", "journal.batReadsStopped", "", Info, App, nil},
		{legacyBatReadsResumed, "info", "{}", "journal.batReadsResumed", "", Info, App, nil},
		{legacyChargeBlocked, "info", `{"soc":"3"}`, "journal.chargeBlocked", "", Info, Station, map[string]string{"soc": "3"}},
		{legacyChargeAllowed, "info", "{}", "journal.chargeAllowed", "", Info, Station, nil},
		{legacyDischargeBlocked, "info", `{"soc":"10"}`, "journal.dischargeBlocked", "", Info, Station, map[string]string{"soc": "10"}},
		{legacyDischargeAllowed, "info", "{}", "journal.dischargeAllowed", "", Info, Station, nil},
		{legacyFwUnverified, "info", `{"firmware":"152"}`, "journal.fwUnverified", "", Info, Station, map[string]string{"firmware": "152"}},
		{legacyFwVerified, "info", `{"firmware":"152"}`, "journal.fwVerified", "", Info, Station, map[string]string{"firmware": "152"}},
	}
	for i, c := range cases {
		got := Build([]store.Event{ev(int64(i+1), t0, c.level, c.code, c.text)}, nil, t0)
		if len(got) != 1 {
			t.Fatalf("%s: %d entries, want 1", c.code, len(got))
		}
		e := got[0]
		wantID := c.code + ":" + strconv.FormatInt(int64(i+1), 10)
		if e.ID != wantID || e.Key != c.key || e.TodoKey != c.todo || e.Level != c.lvl || e.Group != c.grp || e.Code != "" || !e.At.Equal(t0) {
			t.Errorf("%s: entry = %+v, want id %s key %s todo %q level %s group %s", c.code, e, wantID, c.key, c.todo, c.lvl, c.grp)
		}
		if len(e.Args) != len(c.args) {
			t.Errorf("%s: args = %v, want %v", c.code, e.Args, c.args)
		}
		for k, v := range c.args {
			if e.Args[k] != v {
				t.Errorf("%s: args = %v, want %v", c.code, e.Args, c.args)
			}
		}
	}
}

func TestBuildSkipsLegacyLinkEvents(t *testing.T) {
	evs := []store.Event{
		ev(1, t0, "info", "link.no_link", "LIVE -> NO_LINK"),
		ev(2, t0, "info", "link.delayed", "data delayed since …"),
		ev(3, t0, "info", "link.likely_dead", "station probably ran out at …"),
		ev(4, t0, "info", "monitor.cadence", "calm"),
	}
	if got := Build(evs, nil, t0); len(got) != 0 {
		t.Fatalf("entries = %+v, want none", got)
	}
}

func TestBuildAppGaps(t *testing.T) {
	start := time.Date(2026, 11, 22, 1, 20, 0, 0, time.UTC)
	end := time.Date(2026, 11, 22, 2, 40, 0, 0, time.UTC)
	sleepEnd := t0.Add(-30 * time.Minute)
	gaps := []store.Gap{
		{ID: 7, StationID: "st", Start: start, End: &end, Reason: store.GapAppNotRunning},
		{ID: 8, StationID: "st", Start: t0.Add(-2 * time.Hour), End: &sleepEnd, Reason: store.GapPCSleep},
		{ID: 9, StationID: "st", Start: t0.Add(-time.Hour), Reason: store.GapNoLink},
		{ID: 10, StationID: "st", Start: t0.Add(-time.Hour), Reason: store.GapLikelyDead},
	}
	got := Build(nil, gaps, t0)
	if len(got) != 2 {
		t.Fatalf("entries = %+v, want 2 (app_not_running, pc_sleep)", got)
	}
	// Newest first.
	sl, off := got[0], got[1]
	if off.ID != "app.gap:7" || off.Key != "journal.appOff" || off.TodoKey != "" || off.Level != Info || off.Group != App ||
		!off.At.Equal(start) || off.Args["start"] != "2026-11-22T01:20:00Z" || off.Args["end"] != "2026-11-22T02:40:00Z" {
		t.Errorf("app_not_running entry = %+v", off)
	}
	if sl.ID != "app.gap:8" || sl.Key != "journal.pcSleep" || sl.Group != App || sl.Level != Info ||
		sl.Args["end"] != sleepEnd.Format(time.RFC3339) {
		t.Errorf("pc_sleep entry = %+v", sl)
	}
}

func TestBuildOpenAppGapEndsNow(t *testing.T) {
	gaps := []store.Gap{{ID: 3, StationID: "st", Start: t0.Add(-time.Hour), Reason: store.GapAppNotRunning}}
	got := Build(nil, gaps, t0)
	if len(got) != 1 || got[0].Args["end"] != t0.Format(time.RFC3339) {
		t.Fatalf("entries = %+v, want one ending now", got)
	}
}

func TestBuildNewestFirst(t *testing.T) {
	evs := []store.Event{
		ev(1, t0, "warn", CodeAPISilent, "{}"),
		ev(2, t0.Add(6*time.Minute), "info", CodeLANRestored, `{"duration_ms":"360000"}`),
		ev(3, t0.Add(6*time.Minute), "warn", CodeLowCharge, `{"soc":"19"}`),
	}
	g := store.Gap{ID: 1, Start: t0.Add(time.Minute), Reason: store.GapPCSleep}
	end := t0.Add(2 * time.Minute)
	g.End = &end
	got := Build(evs, []store.Gap{g}, t0.Add(time.Hour))
	var ids []string
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	want := []string{CodeLowCharge + ":3", CodeLANRestored + ":2", "app.gap:1", CodeAPISilent + ":1"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func TestBuildBadArgsKeepsEntry(t *testing.T) {
	got := Build([]store.Event{ev(1, t0, "warn", CodeCutoff, "not json")}, nil, t0)
	if len(got) != 1 || got[0].Key != "journal.cutoff" || len(got[0].Args) != 0 {
		t.Fatalf("entries = %+v, want one without args", got)
	}
}

func TestNewSinceCountsWarningsOnly(t *testing.T) {
	es := []Entry{
		{At: t0.Add(-time.Minute), Level: Warning}, // before seen
		{At: t0, Level: Warning},                   // at seen: not new
		{At: t0.Add(time.Minute), Level: Info},
		{At: t0.Add(2 * time.Minute), Level: Warning},
		{At: t0.Add(3 * time.Minute), Level: Error},
	}
	if n := NewSince(es, t0); n != 2 {
		t.Fatalf("NewSince = %d, want 2", n)
	}
	if n := NewSince(es, time.Time{}); n != 4 {
		t.Fatalf("NewSince(zero) = %d, want 4", n)
	}
}

func TestLegacyCodesAreInfoWhateverStored(t *testing.T) {
	var evs []store.Event
	for i, code := range []string{legacyBatReadsStopped, legacyBatReadsResumed, legacyChargeBlocked, legacyChargeAllowed,
		legacyDischargeBlocked, legacyDischargeAllowed, legacyFwUnverified, legacyFwVerified} {
		evs = append(evs, ev(int64(i+1), t0.Add(time.Minute), []string{"warn", "error", "info"}[i%3], code, "{}"))
	}
	es := Build(evs, nil, t0)
	if len(es) != len(evs) {
		t.Fatalf("entries = %d, want %d", len(es), len(evs))
	}
	for _, e := range es {
		if e.Level != Info || e.TodoKey != "" {
			t.Errorf("%s: level %s todo %q, want info and no advice", e.ID, e.Level, e.TodoKey)
		}
	}
	if n := NewSince(es, t0); n != 0 {
		t.Fatalf("NewSince = %d, want 0", n)
	}
}

func TestLinkDroppedEntry(t *testing.T) {
	args := `{"from":"2026-11-22T12:00:10Z","to":"2026-11-22T12:00:50Z","seconds":"40"}`
	for _, level := range []string{"info", "warn", "error"} { // a stored warn or error still shows Info
		got := Build([]store.Event{ev(7, t0.Add(time.Minute), level, CodeLinkDropped, args)}, nil, t0)
		if len(got) != 1 {
			t.Fatalf("%s: %d entries, want 1", level, len(got))
		}
		e := got[0]
		if e.ID != "conn.linkDropped:7" || e.Key != "journal.linkDropped" || e.TodoKey != "" || e.Level != Info || e.Group != Connection {
			t.Errorf("%s: entry = %+v", level, e)
		}
		if e.Args["from"] != "2026-11-22T12:00:10Z" || e.Args["to"] != "2026-11-22T12:00:50Z" || e.Args["seconds"] != "40" || len(e.Args) != 3 {
			t.Errorf("%s: args = %v", level, e.Args)
		}
		if n := NewSince(got, t0); n != 0 {
			t.Errorf("%s: NewSince = %d, want 0", level, n)
		}
	}
	found := false
	for _, c := range Codes {
		found = found || c == CodeLinkDropped
	}
	if !found {
		t.Error("CodeLinkDropped is not in Codes")
	}
}

func TestIDOrderNumericWithinCode(t *testing.T) {
	at := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	es := Build([]store.Event{
		{ID: 9, At: at, Code: CodeIPChanged, Level: "info"},
		{ID: 10, At: at, Code: CodeIPChanged, Level: "info"},
		{ID: 2, At: at, Code: CodeLANRestored, Level: "info"},
	}, nil, at)
	got := []string{es[0].ID, es[1].ID, es[2].ID}
	if got[0] != "conn.lanRestored:2" || got[1] != "conn.ipChanged:10" || got[2] != "conn.ipChanged:9" {
		t.Fatalf("order = %v", got)
	}
	c := CursorOf(es[1])
	if !es[0].Before(c) || !es[2].After(c) || es[1].Before(c) || es[1].After(c) {
		t.Fatal("cursor not consistent with the order")
	}
}
