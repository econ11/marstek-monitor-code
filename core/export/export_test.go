package export

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

const id = "st1"

var now = time.Date(2026, 11, 22, 12, 0, 0, 0, time.Local)

func f(v float64) *float64 { return &v }

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// write exports the last 60 days in English (',', point) and returns the
// files' records by kind.
func write(t *testing.T, st *store.Store) map[string][][]string {
	t.Helper()
	return writeIn(t, st, FormatFor("en"))
}

func writeIn(t *testing.T, st *store.Store, f Format) map[string][][]string {
	t.Helper()
	dir := t.TempDir()
	from := time.Date(2026, 9, 23, 0, 0, 0, 0, time.Local)
	paths, err := Write(dir, st, id, from, time.Date(2026, 11, 23, 0, 0, 0, 0, time.Local), now, f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"marstek-outages-20260923-20261122.csv", "marstek-events-20260923-20261122.csv", "marstek-charge-20260923-20261122.csv"}
	if len(paths) != 3 {
		t.Fatalf("paths = %v", paths)
	}
	out := map[string][][]string{}
	for i, p := range paths {
		if filepath.Dir(p) != dir || filepath.Base(p) != want[i] {
			t.Fatalf("path %d = %s, want %s in %s", i, p, want[i], dir)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(b, []byte("\xEF\xBB\xBF")) {
			t.Fatalf("%s: no UTF-8 BOM", want[i])
		}
		r := csv.NewReader(bytes.NewReader(b[3:]))
		r.Comma = f.Sep
		recs, err := r.ReadAll()
		if err != nil {
			t.Fatalf("%s: %v", want[i], err)
		}
		out[strings.Split(want[i], "-")[1]] = recs
	}
	return out
}

func rfc(t time.Time) string { return t.In(time.Local).Format(time.RFC3339) }

func TestExportOutagesCSV(t *testing.T) {
	st := openStore(t)
	start := now.Add(-5 * 24 * time.Hour)
	end := start.Add(2 * time.Hour)
	died := start.Add(90 * time.Minute)
	oid, err := st.OpenOutage(store.Outage{StationID: id, Start: start, SOCStart: f(80)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkDied(oid, died); err != nil {
		t.Fatal(err)
	}
	if err := st.CloseOutage(oid, end, f(12.5)); err != nil {
		t.Fatal(err)
	}
	// A 30 min gap inside it: the duration is a lower bound.
	gEnd := start.Add(time.Hour)
	if _, err := st.OpenGap(store.Gap{StationID: id, Start: start.Add(30 * time.Minute), End: &gEnd, Reason: store.GapPCSleep}); err != nil {
		t.Fatal(err)
	}
	// An ongoing one: no end, its duration up to the last sample.
	cur := now.Add(-time.Hour)
	if _, err := st.OpenOutage(store.Outage{StationID: id, Start: cur}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSample(model.Sample{StationID: id, At: now.Add(-10 * time.Minute), Source: model.SourceLAN, Grid: model.GridAbsent}); err != nil {
		t.Fatal(err)
	}
	recs := write(t, st)["outages"]
	if got := strings.Join(recs[0], ","); got != "start,end,duration_min,known_min,at_least,soc_start,soc_end,died_at,start_approx,end_approx" {
		t.Fatalf("header = %q", got)
	}
	if len(recs) != 3 {
		t.Fatalf("records = %v", recs)
	}
	want := []string{rfc(start), rfc(end), "120", "90", "true", "80", "12.5", rfc(died), "false", "false"}
	if strings.Join(recs[1], "|") != strings.Join(want, "|") {
		t.Fatalf("row = %v\nwant  %v", recs[1], want)
	}
	want = []string{rfc(cur), "", "50", "50", "true", "", "", "", "false", "false"}
	if strings.Join(recs[2], "|") != strings.Join(want, "|") {
		t.Fatalf("ongoing row = %v\nwant          %v", recs[2], want)
	}
}

func TestExportEventsSkipsDebugCodes(t *testing.T) {
	st := openStore(t)
	at := now.Add(-2 * time.Hour)
	for i, e := range []store.Event{
		{Code: "link.no_link", Level: "info", Text: "LIVE -> NO_LINK"},
		{Code: "monitor.cadence", Level: "info", Text: "calm"},
		{Code: "conn.lanLost", Level: "warn", Text: "{}"},
		{Code: "link.moved", Level: "info", Text: "station found at 192.168.1.9 (was 192.168.1.8)"},
		{Code: "station.lowCharge", Level: "warn", Text: `{"soc":"19"}`},
	} {
		e.StationID, e.At = id, at.Add(time.Duration(i)*time.Minute)
		if err := st.AddEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	recs := write(t, st)["events"]
	if got := strings.Join(recs[0], ","); got != "at,level,code,text" {
		t.Fatalf("header = %q", got)
	}
	if len(recs) != 3 {
		t.Fatalf("records = %v; want conn.lanLost and station.lowCharge only", recs)
	}
	if want := []string{rfc(at.Add(2 * time.Minute)), "warn", "conn.lanLost", "{}"}; strings.Join(recs[1], "|") != strings.Join(want, "|") {
		t.Fatalf("row = %v, want %v", recs[1], want)
	}
	if recs[2][2] != "station.lowCharge" || recs[2][3] != `{"soc":"19"}` {
		t.Fatalf("row = %v", recs[2])
	}
}

func TestExportChargeRawThenMinute(t *testing.T) {
	st := openStore(t)
	old := now.Add(-40 * 24 * time.Hour).Truncate(time.Minute)
	for i, soc := range []float64{50, 52, 54} {
		if err := st.InsertSample(model.Sample{StationID: id, At: old.Add(time.Duration(i) * 10 * time.Second), Source: model.SourceLAN,
			SOC: f(soc), BatteryW: f(-300), OngridW: f(0), OffgridW: f(300), Grid: model.GridAbsent}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Compact(now); err != nil {
		t.Fatal(err)
	}
	day := now.Add(-24 * time.Hour)
	if err := st.InsertSample(model.Sample{StationID: id, At: day, Source: model.SourceLAN,
		SOC: f(77), BatteryW: f(450.5), OngridW: f(-450.5), OffgridW: f(0), PVW: nil, Grid: model.GridPresent}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSample(model.Sample{StationID: id, At: day.Add(5 * time.Second), Source: model.SourceLAN}); err != nil {
		t.Fatal(err)
	}
	recs := write(t, st)["charge"]
	if got := strings.Join(recs[0], ","); got != "at,soc,soc_min,soc_max,battery_w,ongrid_w,offgrid_w,pv_w,grid,resolution" {
		t.Fatalf("header = %q", got)
	}
	if len(recs) != 4 {
		t.Fatalf("records = %v", recs)
	}
	for i, r := range recs {
		if len(r) != 10 {
			t.Fatalf("record %d has %d cells: %v", i, len(r), r)
		}
	}
	rows := []string{
		strings.Join([]string{rfc(old), "52", "50", "54", "-300", "", "", "", "", "minute"}, "|"),
		strings.Join([]string{rfc(day), "77", "", "", "450.5", "-450.5", "0", "", "present", "raw"}, "|"),
		strings.Join([]string{rfc(day.Add(5 * time.Second)), "", "", "", "", "", "", "", "", "raw"}, "|"),
	}
	for i, want := range rows {
		if got := strings.Join(recs[i+1], "|"); got != want {
			t.Fatalf("row %d = %s\nwant    %s", i+1, got, want)
		}
	}
}

// Rows outside [from, to) are left out; to must be after from.
func TestExportRange(t *testing.T) {
	st := openStore(t)
	for _, at := range []time.Time{now.Add(-70 * 24 * time.Hour), now.Add(-time.Hour)} {
		if err := st.AddEvent(store.Event{StationID: id, At: at, Level: "warn", Code: "conn.lanLost", Text: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	if recs := write(t, st)["events"]; len(recs) != 2 {
		t.Fatalf("records = %v; want the recent one only", recs)
	}
	if _, err := Write(t.TempDir(), st, id, now, now, now, FormatFor("en")); err == nil {
		t.Fatal("empty range: no error")
	}
}

// Ukrainian: «;» between fields and a decimal comma (Excel in a Ukrainian
// locale splits on «;»); a field holding «;» is quoted. Never «-0».
func TestExportUkrainianFormat(t *testing.T) {
	st := openStore(t)
	start := now.Add(-5 * 24 * time.Hour)
	end := start.Add(90*time.Minute + 30*time.Second)
	oid, err := st.OpenOutage(store.Outage{StationID: id, Start: start, SOCStart: f(80.25)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CloseOutage(oid, end, f(-0.004)); err != nil {
		t.Fatal(err)
	}
	if err := st.AddEvent(store.Event{StationID: id, At: start, Level: "warn", Code: "conn.lanLost", Text: `{"a":"x;y"}`}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	from := time.Date(2026, 9, 23, 0, 0, 0, 0, time.Local)
	paths, err := Write(dir, st, id, from, time.Date(2026, 11, 23, 0, 0, 0, 0, time.Local), now, FormatFor("uk"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(paths[0])
	lines := strings.Split(string(b[3:]), "\r\n")
	if lines[0] != "start;end;duration_min;known_min;at_least;soc_start;soc_end;died_at;start_approx;end_approx" {
		t.Fatalf("header = %q", lines[0])
	}
	want := rfc(start) + ";" + rfc(end) + ";90,5;90,5;false;80,25;0;;false;false"
	if lines[1] != want {
		t.Fatalf("row = %q\nwant  %q", lines[1], want)
	}
	b, _ = os.ReadFile(paths[1])
	if lines = strings.Split(string(b[3:]), "\r\n"); lines[1] != rfc(start)+`;warn;conn.lanLost;"{""a"":""x;y""}"` {
		t.Fatalf("event row = %q", lines[1])
	}
	if en, de := FormatFor("en"), FormatFor("de"); en.Sep != ',' || en.DecimalComma || de.Sep != en.Sep || de.DecimalComma {
		t.Fatal("non-uk languages: want «,» and a point")
	}
}

// English never prints «-0» either.
func TestExportNoMinusZero(t *testing.T) {
	if got := FormatFor("en").num(-0.004); got != "0" {
		t.Fatalf("num(-0.004) = %q", got)
	}
	if got := FormatFor("uk").num(-1.234); got != "-1,23" {
		t.Fatalf("uk num(-1.234) = %q", got)
	}
}

func TestExportEventsLegacyCodeShowsInfo(t *testing.T) {
	st := openStore(t)
	if err := st.AddEvent(store.Event{StationID: id, At: now.Add(-time.Hour), Level: "warn", Code: "battery.readsStopped", Text: "{}"}); err != nil {
		t.Fatal(err)
	}
	recs := write(t, st)["events"]
	if len(recs) != 2 || recs[1][1] != "info" || recs[1][2] != "battery.readsStopped" {
		t.Fatalf("records = %v; want the legacy row at level info", recs)
	}
}

// Owner 2026-11-23: the UI names the window of a start that lay in a gap;
// the CSV keeps its format: the observed start and start_approx = true.
func TestExportOutageStartInGapKeepsFormat(t *testing.T) {
	st := openStore(t)
	gStart, gEnd := now.Add(-10*time.Hour), now.Add(-3*time.Hour)
	if _, err := st.OpenGap(store.Gap{StationID: id, Start: gStart, End: &gEnd, Reason: store.GapAppNotRunning}); err != nil {
		t.Fatal(err)
	}
	start, end := gEnd.Add(40*time.Second), now.Add(-time.Hour)
	oid, err := st.OpenOutage(store.Outage{StationID: id, Start: start, Partial: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CloseOutage(oid, end, nil); err != nil {
		t.Fatal(err)
	}
	recs := write(t, st)["outages"]
	if len(recs) != 2 || len(recs[1]) != len(recs[0]) {
		t.Fatalf("records = %v", recs)
	}
	if r := recs[1]; r[0] != rfc(start) || r[4] != "true" || r[8] != "true" || r[9] != "false" {
		t.Fatalf("row = %v, want the observed start %s with at_least and start_approx", r, rfc(start))
	}
}
