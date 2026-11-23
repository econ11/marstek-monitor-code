package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "monitor.db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func f(v float64) *float64 { return &v }

func TestSampleRoundTripPreservesNil(t *testing.T) {
	s, _ := openTemp(t)
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	in := model.Sample{
		StationID: "st1", At: at, Source: model.SourceLAN,
		SOC: nil, BatteryW: f(-420), OngridW: f(0), OffgridW: f(420), PVW: nil,
		Grid: model.GridAbsent, Raw: json.RawMessage(`{"bat_soc":null}`),
	}
	if err := s.InsertSample(in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Samples("st1", at.Add(-time.Second), at.Add(time.Second))
	if err != nil || len(got) != 1 {
		t.Fatalf("samples: %v %v", got, err)
	}
	g := got[0]
	if g.SOC != nil || g.PVW != nil {
		t.Errorf("nil became non-nil: soc=%v pv=%v", g.SOC, g.PVW)
	}
	if g.BatteryW == nil || *g.BatteryW != -420 || g.OngridW == nil || *g.OngridW != 0 {
		t.Errorf("values lost: %+v", g)
	}
	if !g.At.Equal(at) || g.Grid != model.GridAbsent || g.Source != model.SourceLAN || string(g.Raw) != `{"bat_soc":null}` {
		t.Errorf("fields differ: %+v", g)
	}
	last, ok, err := s.LastSample("st1")
	if err != nil || !ok || !last.At.Equal(at) {
		t.Errorf("LastSample: %+v %v %v", last, ok, err)
	}
	if _, ok, _ := s.LastSample("none"); ok {
		t.Error("LastSample for unknown station should be !ok")
	}
	since, ok, err := s.ObservedSince("st1")
	if err != nil || !ok || !since.Equal(at) {
		t.Errorf("ObservedSince: %v %v %v", since, ok, err)
	}
}

func TestSingleOpenOutagePerStation(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Now().UTC()
	id, err := s.OpenOutage(Outage{StationID: "st1", Start: now, SOCStart: f(80)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenOutage(Outage{StationID: "st1", Start: now.Add(time.Minute)}); err == nil {
		t.Fatal("second open outage must fail")
	}
	if _, err := s.OpenOutage(Outage{StationID: "st2", Start: now}); err != nil {
		t.Fatalf("other station must be allowed: %v", err)
	}
	cur, ok, err := s.CurrentOutage("st1")
	if err != nil || !ok || cur.ID != id || cur.End != nil || *cur.SOCStart != 80 {
		t.Fatalf("current: %+v %v %v", cur, ok, err)
	}
	if err := s.MarkDied(id, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseOutage(id, now.Add(2*time.Hour), f(12)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.CurrentOutage("st1"); ok {
		t.Error("no open outage expected after close")
	}
	if _, err := s.OpenOutage(Outage{StationID: "st1", Start: now.Add(3 * time.Hour)}); err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
	all, _ := s.Outages("st1", now.Add(-time.Hour), now.Add(10*time.Hour))
	if len(all) != 2 || all[0].DiedAt == nil || all[0].End == nil || *all[0].SOCEnd != 12 {
		t.Fatalf("outages: %+v", all)
	}
}

func TestOutagesIntersectRange(t *testing.T) {
	s, _ := openTemp(t)
	start := time.Date(2026, 11, 19, 23, 30, 0, 0, time.UTC)
	end := time.Date(2026, 11, 20, 1, 10, 0, 0, time.UTC)
	id, _ := s.OpenOutage(Outage{StationID: "st1", Start: start})
	if err := s.CloseOutage(id, end, nil); err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	got, err := s.Outages("st1", today, today.Add(24*time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("want overlap with today: %+v %v", got, err)
	}
	if got, _ := s.Outages("st1", end, end.Add(time.Hour)); len(got) != 0 {
		t.Errorf("outage ending exactly at from must not match: %+v", got)
	}
	// open outage intersects any later range
	s.OpenOutage(Outage{StationID: "st1", Start: today.Add(5 * time.Hour)})
	if got, _ := s.Outages("st1", today.Add(48*time.Hour), today.Add(72*time.Hour)); len(got) != 1 {
		t.Errorf("open outage should match later range: %+v", got)
	}
}

func TestGapsAndEvents(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	id, err := s.OpenGap(Gap{StationID: "st1", Start: now, Reason: GapPCSleep})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CloseGap(id, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g, err := s.Gaps("st1", now.Add(30*time.Minute), now.Add(2*time.Hour))
	if err != nil || len(g) != 1 || g[0].Reason != "pc_sleep" || g[0].End == nil {
		t.Fatalf("gaps: %+v %v", g, err)
	}
	if err := s.AddEvent(Event{StationID: "st1", At: now, Level: "warn", Code: "x", Text: "t", Source: "lan"}); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)
	if n != 1 {
		t.Errorf("events=%d", n)
	}
}

func TestStationsAndSettings(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	st := Station{ID: "st1", Name: "Venus", Model: "VenusE 3.0", BLEMac: "aa", WifiMac: "bb", IP: "1.2.3.4", Firmware: 147, CreatedAt: now}
	if err := s.UpsertStation(st); err != nil {
		t.Fatal(err)
	}
	st.IP = "5.6.7.8"
	if err := s.UpsertStation(st); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.StationByBLEMac("aa")
	if err != nil || !ok || got.IP != "5.6.7.8" || !got.CreatedAt.Equal(now) {
		t.Fatalf("by mac: %+v %v %v", got, ok, err)
	}
	if _, ok, _ := s.StationByBLEMac("zz"); ok {
		t.Error("unknown mac found")
	}
	if all, _ := s.Stations(); len(all) != 1 {
		t.Errorf("stations: %+v", all)
	}
	if _, ok, _ := s.GetSetting("k"); ok {
		t.Error("missing setting reported present")
	}
	s.SetSetting("k", "1")
	s.SetSetting("k", "2")
	if v, ok, _ := s.GetSetting("k"); !ok || v != "2" {
		t.Errorf("setting: %q %v", v, ok)
	}
}

func TestReopenKeepsData(t *testing.T) {
	s, p := openTemp(t)
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s.InsertSample(model.Sample{StationID: "st1", At: at, Source: model.SourceLAN, SOC: f(55)})
	s.SetSetting("a", "b")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, _ := s2.Samples("st1", at.Add(-time.Hour), at.Add(time.Hour))
	if len(got) != 1 || *got[0].SOC != 55 {
		t.Fatalf("data lost: %+v", got)
	}
	if v, ok, _ := s2.GetSetting("a"); !ok || v != "b" {
		t.Error("setting lost")
	}
}

func TestOpenIsDurable(t *testing.T) {
	s, _ := openTemp(t)
	var sync int
	if err := s.db.QueryRow(`PRAGMA synchronous`).Scan(&sync); err != nil || sync != 2 {
		t.Errorf("synchronous=%d err=%v, want 2 (FULL)", sync, err)
	}
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode=%q err=%v, want wal", mode, err)
	}
}

func TestSetGapReason(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	id, err := s.OpenGap(Gap{StationID: "st1", Start: now, Reason: GapNoLink})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetGapReason(id, GapLikelyDead); err != nil {
		t.Fatal(err)
	}
	g, err := s.Gaps("st1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 1 || g[0].Reason != GapLikelyDead || g[0].End != nil {
		t.Fatalf("gaps = %+v, want one open likely_dead gap", g)
	}
	if err := s.SetGapReason(id+100, GapLikelyDead); err == nil {
		t.Fatal("SetGapReason on a missing gap: want error")
	}
}

func TestEventsByCodeInRange(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	add := func(st, code string, at time.Time) {
		t.Helper()
		if err := s.AddEvent(Event{StationID: st, At: at, Level: "info", Code: code, Text: code + " text", Source: "monitor"}); err != nil {
			t.Fatal(err)
		}
	}
	add("st1", "link.delayed", now.Add(2*time.Minute))
	add("st1", "link.delayed", now.Add(time.Minute))
	add("st1", "link.live", now.Add(time.Minute))
	add("st1", "link.delayed", now.Add(time.Hour)) // outside the range
	add("st2", "link.delayed", now.Add(time.Minute))
	ev, err := s.Events("st1", "link.delayed", now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 || !ev[0].At.Equal(now.Add(time.Minute)) || !ev[1].At.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("events = %+v, want the two delays of st1 in range, ascending", ev)
	}
	if e := ev[0]; e.StationID != "st1" || e.Level != "info" || e.Code != "link.delayed" || e.Text != "link.delayed text" || e.Source != "monitor" || e.ID == 0 {
		t.Fatalf("event = %+v", e)
	}
}

func TestStoreRenameStation(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	st := Station{ID: "st1", Name: "Venus", Model: "VenusE 3.0", BLEMac: "aa", IP: "1.2.3.4", CreatedAt: now}
	if err := s.UpsertStation(st); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameStation("st1", "Дача"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.StationByBLEMac("aa")
	if err != nil || !ok || got.Name != "Дача" || got.IP != "1.2.3.4" || got.Model != "VenusE 3.0" {
		t.Fatalf("after rename: %+v %v %v", got, ok, err)
	}
	if err := s.RenameStation("nope", "X"); err == nil {
		t.Error("renaming an unknown station succeeded")
	}
}

func TestStoreMinSOC(t *testing.T) {
	s, _ := openTemp(t)
	base := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	if v, err := s.MinSOC("st", base, base.Add(time.Hour)); err != nil || v != nil {
		t.Fatalf("MinSOC without data = %v, %v; want nil", v, err)
	}
	for i, soc := range []*float64{f(60), nil, f(55), f(70)} {
		if err := s.InsertSample(model.Sample{StationID: "st", At: base.Add(time.Duration(i) * time.Minute), Source: model.SourceLAN, SOC: soc}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO samples_minute(station_id,minute,soc_avg,soc_min,soc_max) VALUES(?,?,?,?,?),(?,?,?,?,?)`,
		"st", ms(base.Add(-30*time.Minute)), 50, 41, 58, "other", ms(base.Add(-20*time.Minute)), 10, 5, 12); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		from, to time.Time
		want     *float64
	}{
		{base, base.Add(time.Hour), f(55)},                                         // raw only; a nil SOC is skipped
		{base.Add(-time.Hour), base.Add(time.Hour), f(41)},                         // aggregated minute is lower
		{base.Add(-30*time.Minute + 30*time.Second), base.Add(time.Minute), f(41)}, // a minute that overlaps from
		{base.Add(3 * time.Minute), base.Add(time.Hour), f(70)},
		{base.Add(5 * time.Minute), base.Add(time.Hour), nil},
	}
	for i, c := range cases {
		v, err := s.MinSOC("st", c.from, c.to)
		if err != nil {
			t.Fatal(err)
		}
		if (v == nil) != (c.want == nil) || v != nil && *v != *c.want {
			t.Fatalf("case %d: MinSOC = %v, want %v", i, v, c.want)
		}
	}
}

func TestStoreEnergyHoursSums(t *testing.T) {
	s, _ := openTemp(t)
	h := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	var ms []EnergyMinute
	for i := 0; i < 60; i++ {
		m := EnergyMinute{At: h.Add(time.Duration(i) * time.Minute), ChgWh: 1, DisWh: 2, DisOutageWh: 0.5, PVChgWh: 0.25, Covered: 50 * time.Second}
		if i == 17 || i == 40 {
			w, at := float64(500+i), h.Add(time.Duration(i)*time.Minute+3*time.Second)
			m.PeakDisW, m.PeakAt = &w, &at
		}
		ms = append(ms, m)
	}
	ms = append(ms, EnergyMinute{At: h.Add(time.Hour), ChgWh: 9, Covered: time.Minute}) // next hour
	if err := s.UpsertEnergyMinutes("st1", ms); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEnergyMinutes("st1", ms[:1]); err != nil { // replace, not duplicate
		t.Fatal(err)
	}
	if err := s.UpsertEnergyMinutes("st2", ms[:1]); err != nil {
		t.Fatal(err)
	}
	got, err := s.EnergyHours("st1", h, h.Add(time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("hours = %+v, %v; want one", got, err)
	}
	g := got[0]
	if !g.At.Equal(h) || g.ChgWh != 60 || g.DisWh != 120 || g.DisOutageWh != 30 || g.PVChgWh != 15 || g.Covered != 50*time.Minute {
		t.Fatalf("hour = %+v", g)
	}
	if g.PeakDisW == nil || *g.PeakDisW != 540 || g.PeakAt == nil || !g.PeakAt.Equal(h.Add(40*time.Minute+3*time.Second)) {
		t.Fatalf("peak = %v at %v, want 540 at 10:40:03", g.PeakDisW, g.PeakAt)
	}
	all, _ := s.EnergyHours("st1", h, h.Add(2*time.Hour))
	if len(all) != 2 || all[1].ChgWh != 9 || all[1].PeakDisW != nil || all[1].PeakAt != nil {
		t.Fatalf("two hours = %+v", all)
	}
	last, ok, err := s.LastEnergyMinute("st1")
	if err != nil || !ok || !last.Equal(h.Add(time.Hour)) {
		t.Fatalf("LastEnergyMinute = %v %v %v", last, ok, err)
	}
	if _, ok, _ := s.LastEnergyMinute("none"); ok {
		t.Fatal("LastEnergyMinute for unknown station should be !ok")
	}
	if wh, err := s.TotalDischargedWh("st1"); err != nil || wh != 120 {
		t.Fatalf("TotalDischargedWh = %v %v, want 120", wh, err)
	}
	if wh, err := s.TotalDischargedWh("none"); err != nil || wh != 0 {
		t.Fatalf("TotalDischargedWh(none) = %v %v, want 0", wh, err)
	}
}

// An existing v1 database with data (the owner's live one) is upgraded on
// Open: user_version 2, the energy table exists, nothing is lost; a second
// Open changes nothing.
func TestMigrationV2KeepsData(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	for _, q := range []string{schemaV1, `PRAGMA user_version = 1`,
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,battery_w,grid) VALUES('st1',%d,'lan',55,-300,2)`, at.UnixMilli()),
		fmt.Sprintf(`INSERT INTO outages(station_id,start) VALUES('st1',%d)`, at.UnixMilli()),
		`INSERT INTO settings(key,value) VALUES('app.settings','{}')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	for i := 0; i < 2; i++ {
		s, err := Open(p)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != len(migrations) {
			t.Fatalf("user_version = %d, want %d", v, len(migrations))
		}
		got, err := s.Samples("st1", at, at.Add(time.Second))
		if err != nil || len(got) != 1 || *got[0].SOC != 55 {
			t.Fatalf("samples = %+v, %v", got, err)
		}
		if _, ok, err := s.CurrentOutage("st1"); err != nil || !ok {
			t.Fatalf("open outage lost: %v %v", ok, err)
		}
		if v, ok, _ := s.GetSetting("app.settings"); !ok || v != "{}" {
			t.Fatalf("settings lost: %q %v", v, ok)
		}
		if err := s.UpsertEnergyMinutes("st1", []EnergyMinute{{At: at, DisWh: 1, Covered: time.Minute}}); err != nil {
			t.Fatalf("energy table: %v", err)
		}
		s.Close()
	}
}

// A populated v2 database (the owner's) is upgraded to v3 in place: its
// samples, energy minutes and settings stay, the estimates table is usable,
// and opening again changes nothing.
func TestMigrationV3KeepsData(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v2.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	for _, q := range []string{schemaV1, schemaV2, `PRAGMA user_version = 2`,
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,battery_w,grid) VALUES('st1',%d,'lan',55,-300,2)`, at.UnixMilli()),
		fmt.Sprintf(`INSERT INTO energy_minute(station_id,minute,chg_wh,dis_wh,dis_out_wh,pv_chg_wh,covered_ms) VALUES('st1',%d,0,7,0,0,60000)`, at.UnixMilli()),
		`INSERT INTO settings(key,value) VALUES('app.settings','{}')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	e := CapacityEstimate{Start: at, End: at.Add(3 * time.Hour), Wh: 5000, DeltaSOC: 60}
	for i := 0; i < 2; i++ {
		s, err := Open(p)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != len(migrations) {
			t.Fatalf("user_version = %d, want %d", v, len(migrations))
		}
		if got, err := s.Samples("st1", at, at.Add(time.Second)); err != nil || len(got) != 1 {
			t.Fatalf("samples = %+v, %v", got, err)
		}
		if wh, err := s.TotalDischargedWh("st1"); err != nil || wh != 7 {
			t.Fatalf("discharged = %v, %v; want 7", wh, err)
		}
		if v, ok, _ := s.GetSetting("app.settings"); !ok || v != "{}" {
			t.Fatalf("settings lost: %q %v", v, ok)
		}
		if i == 0 {
			if err := s.UpsertCapacityEstimates("st1", []CapacityEstimate{e}); err != nil {
				t.Fatalf("estimates table: %v", err)
			}
		}
		if got, err := s.CapacityEstimates("st1"); err != nil || len(got) != 1 || got[0] != e {
			t.Fatalf("estimates = %+v, %v", got, err)
		}
		s.Close()
	}
}

// Estimates are kept per station, ascending by start; one with the same start
// replaces the stored one (a discharge that went on since the last pass).
func TestStoreCapacityEstimates(t *testing.T) {
	s, _ := openTemp(t)
	at := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	a := CapacityEstimate{Start: at.Add(48 * time.Hour), End: at.Add(51 * time.Hour), Wh: 5000, DeltaSOC: 55}
	b := CapacityEstimate{Start: at, End: at.Add(3 * time.Hour), Wh: 5100, DeltaSOC: 60}
	if err := s.UpsertCapacityEstimates("st1", []CapacityEstimate{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCapacityEstimates("st2", []CapacityEstimate{b}); err != nil {
		t.Fatal(err)
	}
	longer := a
	longer.End, longer.Wh, longer.DeltaSOC = a.End.Add(time.Hour), 5050, 70
	if err := s.UpsertCapacityEstimates("st1", []CapacityEstimate{longer}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCapacityEstimates("st1", nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.CapacityEstimates("st1")
	if err != nil || len(got) != 2 || got[0] != b || got[1] != longer {
		t.Fatalf("estimates = %+v, %v; want [b, longer]", got, err)
	}
	if got, err := s.CapacityEstimates("none"); err != nil || len(got) != 0 {
		t.Fatalf("unknown station: %+v, %v", got, err)
	}
}

// BenchmarkEnergyChunk reads one 6 h rollup chunk out of a synthetic 30-day
// database (5 s samples with raw JSON): the battery-only query against the
// full one.
//
//	go test -run '^$' -bench EnergyChunk ./core/store/
func BenchmarkEnergyChunk(b *testing.B) {
	p := filepath.Join(b.TempDir(), "big.db")
	s, err := Open(p)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	end := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	start := end.Add(-30 * 24 * time.Hour)
	raw := `{"id":0,"bat_soc":63,"bat_cap":5120,"pv_power":0,"ongrid_power":0,"offgrid_power":312,"total_pv_energy":0,"total_grid_output_energy":1234,"total_grid_input_energy":5678,"total_load_energy":91011}`
	tx, _ := s.db.Begin()
	st, _ := tx.Prepare(`INSERT INTO samples(station_id,at,source,soc,battery_w,ongrid_w,offgrid_w,pv_w,grid,raw) VALUES('st1',?,'lan',63,-312,0,312,0,2,?)`)
	n := 0
	for at := start; at.Before(end); at = at.Add(5 * time.Second) {
		if _, err := st.Exec(at.UnixMilli(), raw); err != nil {
			b.Fatal(err)
		}
		n++
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	from := start.Add(10 * 24 * time.Hour)
	to := from.Add(6*time.Hour + time.Minute)
	b.Logf("%d samples in the database", n)
	b.Run("EnergySamples", func(b *testing.B) {
		for range b.N {
			if got, err := s.EnergySamples("st1", from, to); err != nil || len(got) < 4320 {
				b.Fatal(len(got), err)
			}
		}
	})
	b.Run("Samples", func(b *testing.B) {
		for range b.N {
			if got, err := s.Samples("st1", from, to); err != nil || len(got) < 4320 {
				b.Fatal(len(got), err)
			}
		}
	})
}

// --- retention (Task 16) ---

// RF4: Prune deletes only what is older than the bound. The open outage and
// the open gap that started 400 days ago stay (never touched), so does an
// outage that started before the bound but ended after it; the closed ones
// of 400 days ago, the old event, minute and sample go; the recent ones stay.
func TestPruneKeepsOpenRowsAndRecentData(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	old, recent := now.Add(-400*24*time.Hour), now.Add(-10*24*time.Hour)
	before := now.Add(-365 * 24 * time.Hour)
	if _, err := s.OpenOutage(Outage{StationID: "st1", Start: old, SOCStart: f(80)}); err != nil {
		t.Fatal(err)
	}
	oldEnd := old.Add(time.Hour)
	if _, err := s.OpenOutage(Outage{StationID: "st2", Start: old, End: &oldEnd}); err != nil {
		t.Fatal(err)
	}
	across := before.Add(time.Hour)
	if _, err := s.OpenOutage(Outage{StationID: "st2", Start: before.Add(-time.Hour), End: &across}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenGap(Gap{StationID: "st1", Start: old, Reason: GapAppNotRunning}); err != nil {
		t.Fatal(err)
	}
	earlyEnd := old.Add(-4 * time.Hour) // before the open outage: nothing needs it
	if _, err := s.OpenGap(Gap{StationID: "st1", Start: old.Add(-5 * time.Hour), End: &earlyEnd, Reason: GapPCSleep}); err != nil {
		t.Fatal(err)
	}
	// Closed gaps the kept outages still need: one inside the open outage,
	// one ending a minute before the straddling outage starts (its start is
	// approximate); one ending long before that goes.
	inEnd := old.Add(3 * time.Hour)
	if _, err := s.OpenGap(Gap{StationID: "st1", Start: old.Add(2 * time.Hour), End: &inEnd, Reason: GapPCSleep}); err != nil {
		t.Fatal(err)
	}
	nearEnd, farEnd := before.Add(-61*time.Minute), before.Add(-3*time.Hour)
	if _, err := s.OpenGap(Gap{StationID: "st2", Start: before.Add(-2 * time.Hour), End: &nearEnd, Reason: GapAppNotRunning}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenGap(Gap{StationID: "st2", Start: before.Add(-4 * time.Hour), End: &farEnd, Reason: GapAppNotRunning}); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{old, recent} {
		if err := s.AddEvent(Event{StationID: "st1", At: at, Level: "warn", Code: "conn.lanLost", Text: "{}"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO samples_minute(station_id,minute,soc_avg) VALUES('st1',?,50)`, ms(at)); err != nil {
			t.Fatal(err)
		}
		if err := s.InsertSample(model.Sample{StationID: "st1", At: at, Source: model.SourceLAN, SOC: f(50)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"st1", "st2"} {
		if err := s.Prune(id, before); err != nil {
			t.Fatal(err)
		}
	}
	far, end := time.UnixMilli(0), now.Add(time.Hour)
	if os, _ := s.Outages("st1", far, end); len(os) != 1 || os[0].End != nil || !os[0].Start.Equal(old) {
		t.Fatalf("st1 outages = %+v; want the open one", os)
	}
	if os, _ := s.Outages("st2", far, end); len(os) != 1 || !os[0].End.Equal(across) {
		t.Fatalf("st2 outages = %+v; want only the one ending after the bound", os)
	}
	if gs, _ := s.Gaps("st1", far, end); len(gs) != 2 || gs[0].End != nil || !gs[1].End.Equal(inEnd) {
		t.Fatalf("st1 gaps = %+v; want the open one and the one inside the open outage", gs)
	}
	if gs, _ := s.Gaps("st2", far, end); len(gs) != 1 || !gs[0].End.Equal(nearEnd) {
		t.Fatalf("st2 gaps = %+v; want the one just before the straddling outage", gs)
	}
	if es, _ := s.Events("st1", "conn.lanLost", far, end); len(es) != 1 || !es[0].At.Equal(recent) {
		t.Fatalf("events = %+v; want the recent one", es)
	}
	if ps, _ := s.MinuteSeries("st1", far, end); len(ps) != 1 || !ps[0].At.Equal(recent) {
		t.Fatalf("minutes = %+v; want the recent one", ps)
	}
	if xs, _ := s.Samples("st1", far, end); len(xs) != 1 || !xs[0].At.Equal(recent) {
		t.Fatalf("samples = %+v; want the recent one", xs)
	}
}

// Lifetime cycles never shrink: the discharged energy of the pruned minutes
// moves to SettingDischargedPrunedWh in the same transaction; a second Prune
// adds nothing, and each station keeps its own count.
func TestPruneKeepsCycleTotal(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	old, yesterday := now.Add(-400*24*time.Hour), now.Add(-24*time.Hour)
	before := now.Add(-365 * 24 * time.Hour)
	if err := s.UpsertEnergyMinutes("st1", []EnergyMinute{
		{At: old, DisWh: 3000, Covered: time.Minute}, {At: yesterday, DisWh: 2000, Covered: time.Minute}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEnergyMinutes("st2", []EnergyMinute{{At: old, DisWh: 700, Covered: time.Minute}}); err != nil {
		t.Fatal(err)
	}
	total := func(when, id string, want float64) {
		t.Helper()
		if v, err := s.TotalDischargedWh(id); err != nil || v != want {
			t.Fatalf("%s: %s total = %v, %v; want %v", when, id, v, err, want)
		}
	}
	total("before", "st1", 5000)
	for i := 1; i <= 2; i++ {
		if err := s.Prune("st1", before); err != nil {
			t.Fatal(err)
		}
		total(fmt.Sprintf("after prune %d", i), "st1", 5000)
		total(fmt.Sprintf("after prune %d", i), "st2", 700)
	}
	if first, ok, _ := s.FirstEnergyMinute("st1"); !ok || !first.Equal(yesterday) {
		t.Fatalf("first minute = %v %v; want yesterday's", first, ok)
	}
	if err := s.Prune("st2", before); err != nil {
		t.Fatal(err)
	}
	total("st2 pruned", "st1", 5000)
	total("st2 pruned", "st2", 700)
	if _, ok, _ := s.FirstEnergyMinute("st2"); ok {
		t.Fatal("st2 minutes not pruned")
	}
}

// An unreadable counter is an error, never a silent 0: Prune then deletes
// nothing (the energy is not lost).
func TestPruneBadCounterDeletesNothing(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	if err := s.UpsertEnergyMinutes("st1", []EnergyMinute{{At: now.Add(-400 * 24 * time.Hour), DisWh: 3000}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(SettingDischargedPrunedWh, "lots"); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune("st1", now.Add(-365*24*time.Hour)); err == nil {
		t.Fatal("prune with a bad counter: no error")
	}
	if _, err := s.TotalDischargedWh("st1"); err == nil {
		t.Fatal("total with a bad counter: no error")
	}
	if _, ok, _ := s.FirstEnergyMinute("st1"); !ok {
		t.Fatal("minutes deleted despite the error")
	}
}

// Estimates older than the bound go, except the newest month's (health is
// read from it): a station without a deep discharge for over a year keeps
// its last health figure.
func TestPruneKeepsNewestEstimates(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.Local)
	before := now.Add(-365 * 24 * time.Hour)
	m1 := time.Date(2025, 9, 3, 10, 0, 0, 0, time.Local)
	m2a, m2b := time.Date(2025, 10, 2, 10, 0, 0, 0, time.Local), time.Date(2025, 10, 20, 10, 0, 0, 0, time.Local)
	es := []CapacityEstimate{
		{Start: m1, End: m1.Add(3 * time.Hour), Wh: 5100, DeltaSOC: 60},
		{Start: m2a, End: m2a.Add(3 * time.Hour), Wh: 5050, DeltaSOC: 60},
		{Start: m2b, End: m2b.Add(3 * time.Hour), Wh: 5000, DeltaSOC: 60},
	}
	if err := s.UpsertCapacityEstimates("st1", es); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune("st1", before); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CapacityEstimates("st1"); len(got) != 2 || !got[0].Start.Equal(m2a) || !got[1].Start.Equal(m2b) {
		t.Fatalf("estimates = %+v; want October's two", got)
	}
	fresh := now.Add(-10 * 24 * time.Hour)
	if err := s.UpsertCapacityEstimates("st1", []CapacityEstimate{{Start: fresh, End: fresh.Add(time.Hour), Wh: 4900, DeltaSOC: 55}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune("st1", before); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CapacityEstimates("st1"); len(got) != 1 || !got[0].Start.Equal(fresh) {
		t.Fatalf("estimates = %+v; want the fresh one only", got)
	}
}

// A second close of an outage or a gap is refused and keeps the first end;
// an unknown id is still sql.ErrNoRows.
func TestCloseOutageTwiceKeepsFirstEnd(t *testing.T) {
	s, _ := openTemp(t)
	at := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	id, err := s.OpenOutage(Outage{StationID: "st1", Start: at, SOCStart: f(80)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CloseOutage(id, at.Add(time.Hour), f(60)); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseOutage(id, at.Add(2*time.Hour), f(40)); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("second CloseOutage = %v, want ErrAlreadyClosed", err)
	}
	got, err := s.Outages("st1", at, at.Add(3*time.Hour))
	if err != nil || len(got) != 1 || !got[0].End.Equal(at.Add(time.Hour)) || *got[0].SOCEnd != 60 {
		t.Fatalf("outages = %+v, %v; want the first end (11:00, 60%%)", got, err)
	}
	if err := s.CloseOutage(id+100, at, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("CloseOutage(unknown) = %v, want sql.ErrNoRows", err)
	}

	gid, err := s.OpenGap(Gap{StationID: "st1", Start: at, Reason: GapNoLink})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CloseGap(gid, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseGap(gid, at.Add(2*time.Hour)); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("second CloseGap = %v, want ErrAlreadyClosed", err)
	}
	gs, err := s.Gaps("st1", at, at.Add(3*time.Hour))
	if err != nil || len(gs) != 1 || !gs[0].End.Equal(at.Add(time.Hour)) {
		t.Fatalf("gaps = %+v, %v; want the first end (11:00)", gs, err)
	}
	if err := s.CloseGap(gid+100, at); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("CloseGap(unknown) = %v, want sql.ErrNoRows", err)
	}
}

// A station has at most one open gap; CurrentGap returns it.
func TestOneOpenGapPerStation(t *testing.T) {
	s, _ := openTemp(t)
	at := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	if _, ok, err := s.CurrentGap("st1"); err != nil || ok {
		t.Fatalf("CurrentGap(empty) = %v, %v; want none", ok, err)
	}
	id, err := s.OpenGap(Gap{StationID: "st1", Start: at, Reason: GapPCSleep})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenGap(Gap{StationID: "st1", Start: at.Add(time.Minute), Reason: GapNoLink}); err == nil {
		t.Fatal("a second open gap must fail")
	}
	if _, err := s.OpenGap(Gap{StationID: "st2", Start: at, Reason: GapNoLink}); err != nil {
		t.Fatalf("another station must be allowed: %v", err)
	}
	if g, ok, err := s.CurrentGap("st1"); err != nil || !ok || g.ID != id || g.End != nil || g.Reason != GapPCSleep {
		t.Fatalf("CurrentGap = %+v, %v, %v; want gap %d", g, ok, err, id)
	}
	if err := s.CloseGap(id, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.CurrentGap("st1"); ok {
		t.Fatal("no open gap expected after close")
	}
	if _, err := s.OpenGap(Gap{StationID: "st1", Start: at.Add(2 * time.Hour), Reason: GapNoLink}); err != nil {
		t.Fatalf("open after close: %v", err)
	}
}

// v4Database writes a v4 database (Task 15b) at p with a row in every table
// and these gaps (hours before at):
//   - st1: closed 20..19, open from 5 and from 2, a raw sample at 4;
//   - st2: open from 7 (its only one), a raw sample at 5;
//   - st3: open from 6 and from 3, no measurement between (st2's at 5
//     does not count), a raw sample at 1, after the newest;
//   - st4: open from 6 and from 2, an aggregated minute at 4.5 and a raw
//     sample at 4.
func v4Database(t *testing.T, p string, at time.Time) {
	t.Helper()
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := at.UnixMilli()
	h := time.Hour.Milliseconds()
	for _, q := range []string{schemaV1, schemaV2, schemaV3, schemaV4, `PRAGMA user_version = 4`,
		fmt.Sprintf(`INSERT INTO stations(id,name,model,ble_mac,wifi_mac,ip,firmware,created_at) VALUES('st1','Venus','VenusE 3.0','aa','bb','1.2.3.4',151,%d)`, a-48*h),
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,battery_w,grid,raw) VALUES('st1',%d,'lan',55,-300,2,'{"x":1}')`, a),
		fmt.Sprintf(`INSERT INTO samples_minute(station_id,minute,soc_avg,soc_min,soc_max,bat_w_avg,grid_off_sec) VALUES('st1',%d,50,49,51,-200,60)`, a-40*24*h),
		fmt.Sprintf(`INSERT INTO outages(station_id,start,"end",soc_start,soc_end) VALUES('st1',%d,%d,90,70)`, a-10*h, a-9*h),
		fmt.Sprintf(`INSERT INTO outages(station_id,start,soc_start) VALUES('st1',%d,60)`, a-h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,"end",reason) VALUES('st1',%d,%d,'pc_sleep')`, a-20*h, a-19*h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st1',%d,'app_not_running')`, a-5*h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st1',%d,'no_link')`, a-2*h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st2',%d,'pc_sleep')`, a-7*h),
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,grid) VALUES('st1',%d,'lan',40,1)`, a-4*h),
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,grid) VALUES('st2',%d,'lan',40,1)`, a-5*h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st3',%d,'app_not_running')`, a-6*h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st3',%d,'no_link')`, a-3*h),
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,grid) VALUES('st3',%d,'lan',40,1)`, a-h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st4',%d,'pc_sleep')`, a-6*h),
		fmt.Sprintf(`INSERT INTO gaps(station_id,start,reason) VALUES('st4',%d,'no_link')`, a-2*h),
		fmt.Sprintf(`INSERT INTO samples_minute(station_id,minute,soc_avg,grid_off_sec) VALUES('st4',%d,40,0)`, a-4*h-h/2),
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,grid) VALUES('st4',%d,'lan',40,1)`, a-4*h),
		fmt.Sprintf(`INSERT INTO events(station_id,at,level,code,text,source) VALUES('st1',%d,'warn','station.lowCharge','{}','lan')`, a),
		`INSERT INTO settings(key,value) VALUES('app.settings','{}')`,
		fmt.Sprintf(`INSERT INTO energy_minute(station_id,minute,chg_wh,dis_wh,dis_out_wh,pv_chg_wh,covered_ms) VALUES('st1',%d,0,7,0,0,60000)`, a),
		fmt.Sprintf(`INSERT INTO capacity_estimates(station_id,start,"end",wh,dsoc) VALUES('st1',%d,%d,5000,60)`, a-30*h, a-27*h),
		fmt.Sprintf(`INSERT INTO bat_status(station_id,at,soc,temp_c,remaining_wh,rated_wh,chg_ok,dis_ok) VALUES('st1',%d,55,21,2800,5120,1,1)`, a),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// A filled v4 database (every table, two open gaps) is upgraded to v5 in
// place (Ruling 38): an older open gap is closed at the first measurement
// after its start (raw sample or aggregated minute of that station), at most
// the newest one's start, and at the newest one's start without any; the
// newest stays open, a station's only open gap stays, nothing else changes;
// opening again changes nothing.
func TestMigrationV5KeepsData(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v4.db")
	at := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	v4Database(t, p, at)
	want := map[string]int{"stations": 1, "samples": 5, "samples_minute": 2, "outages": 2, "gaps": 8, "events": 1,
		"settings": 1, "energy_minute": 1, "capacity_estimates": 1, "bat_status": 1}

	for i := 0; i < 2; i++ {
		s, err := Open(p)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		t.Cleanup(func() { s.Close() }) // also on a failure; a second Close is harmless
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != 5 {
			t.Fatalf("user_version = %d, want 5", v)
		}
		for tb, w := range want {
			var n int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + tb).Scan(&n); err != nil || n != w {
				t.Fatalf("%s: %d rows, %v; want %d", tb, n, err, w)
			}
		}
		gs, err := s.Gaps("st1", at.Add(-24*time.Hour), at)
		if err != nil || len(gs) != 3 {
			t.Fatalf("gaps = %+v, %v", gs, err)
		}
		if !gs[0].End.Equal(at.Add(-19*time.Hour)) || gs[0].Reason != GapPCSleep {
			t.Fatalf("closed gap changed: %+v", gs[0])
		}
		if gs[1].End == nil || !gs[1].End.Equal(at.Add(-4*time.Hour)) || gs[1].Reason != GapAppNotRunning {
			t.Fatalf("older open gap = %+v, want closed at the sample after it (05:00)", gs[1])
		}
		for _, c := range []struct {
			station             string
			older, newest, ends time.Duration // before at
		}{
			{"st3", 6 * time.Hour, 3 * time.Hour, 3 * time.Hour},                // no measurement between: the newest start
			{"st4", 6 * time.Hour, 2 * time.Hour, 4*time.Hour + 30*time.Minute}, // the minute before the sample
		} {
			gs, err := s.Gaps(c.station, at.Add(-24*time.Hour), at)
			if err != nil || len(gs) != 2 || !gs[0].Start.Equal(at.Add(-c.older)) || gs[0].End == nil ||
				!gs[0].End.Equal(at.Add(-c.ends)) || gs[1].End != nil || !gs[1].Start.Equal(at.Add(-c.newest)) {
				t.Fatalf("%s gaps = %+v, %v; want the older closed at -%v, the newest from -%v open", c.station, gs, err, c.ends, c.newest)
			}
		}
		if g, ok, err := s.CurrentGap("st1"); err != nil || !ok || g.ID != gs[2].ID || !g.Start.Equal(at.Add(-2*time.Hour)) || g.Reason != GapNoLink {
			t.Fatalf("open gap = %+v, %v, %v; want the newest (no_link, 07:00)", g, ok, err)
		}
		if g, ok, err := s.CurrentGap("st2"); err != nil || !ok || !g.Start.Equal(at.Add(-7*time.Hour)) {
			t.Fatalf("st2 open gap = %+v, %v, %v; want kept", g, ok, err)
		}
		if _, err := s.OpenGap(Gap{StationID: "st1", Start: at, Reason: GapNoLink}); err == nil {
			t.Fatal("v5 must refuse a second open gap")
		}
		if o, ok, err := s.CurrentOutage("st1"); err != nil || !ok || *o.SOCStart != 60 {
			t.Fatalf("open outage = %+v, %v, %v", o, ok, err)
		}
		if got, err := s.Samples("st1", at, at.Add(time.Second)); err != nil || len(got) != 1 || string(got[0].Raw) != `{"x":1}` {
			t.Fatalf("samples = %+v, %v", got, err)
		}
		if wh, err := s.TotalDischargedWh("st1"); err != nil || wh != 7 {
			t.Fatalf("discharged = %v, %v; want 7", wh, err)
		}
		s.Close()
	}
}

// SOCBuckets averages the SOC of the raw samples and the aggregated minutes
// (soc_avg) together per bucket, ascending; a bucket with only unknown SOC
// has a nil SOC, and nothing outside [from, to) or of another station counts.
func TestSOCBuckets(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -31).Truncate(time.Hour)
	// Compacted: the first two minutes of that hour (avg 10 and 20).
	for i, soc := range []float64{10, 10, 20, 20} {
		if err := s.InsertSample(model.Sample{StationID: "st1", At: old.Add(time.Duration(i) * 30 * time.Second),
			Source: model.SourceLAN, SOC: f(soc)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Compact(now); err != nil {
		t.Fatal(err)
	}
	for _, x := range []model.Sample{
		{StationID: "st1", At: old.Add(30 * time.Minute), SOC: f(60)}, // raw in the hour of the minutes
		{StationID: "st1", At: old.Add(time.Hour), SOC: nil},          // unknown only
		{StationID: "st1", At: old.Add(2*time.Hour + time.Minute), SOC: f(70)},
		{StationID: "st1", At: old.Add(2*time.Hour + 2*time.Minute), SOC: f(80)},
		{StationID: "st2", At: old.Add(2 * time.Hour), SOC: f(1)},
		{StationID: "st1", At: old.Add(3 * time.Hour), SOC: f(99)}, // at to: excluded
	} {
		x.Source = model.SourceLAN
		if err := s.InsertSample(x); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.SOCBuckets("st1", old, old.Add(3*time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := []SOCBucket{{At: old, SOC: f(30)}, {At: old.Add(time.Hour)}, {At: old.Add(2 * time.Hour), SOC: f(75)}}
	if len(got) != len(want) {
		t.Fatalf("buckets = %+v, want %d", got, len(want))
	}
	for i := range want {
		if !got[i].At.Equal(want[i].At) || (got[i].SOC == nil) != (want[i].SOC == nil) || got[i].SOC != nil && *got[i].SOC != *want[i].SOC {
			t.Fatalf("bucket %d = %v %v, want %v %v", i, got[i].At, got[i].SOC, want[i].At, want[i].SOC)
		}
	}
	if got[0].At.Location() != time.UTC {
		t.Errorf("bucket time in %v, want UTC", got[0].At.Location())
	}
}

// The bat_status table (schema_v4.sql) is no longer written or read (the
// app does not call Bat.GetStatus, owner decision 2026-11-23); rows an older
// version stored still follow the retention: Prune deletes those older than
// the bound, of that station only.
func TestPruneDeletesOldBatStatusRows(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	before := now.AddDate(0, 0, -365)
	for _, r := range []struct {
		station string
		at      time.Time
	}{{"st1", before.Add(-time.Hour)}, {"st1", before.Add(time.Hour)}, {"st2", before.Add(-time.Hour)}} {
		if _, err := s.db.Exec(`INSERT INTO bat_status(station_id,at,soc,remaining_wh) VALUES(?,?,100,5120)`, r.station, ms(r.at)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prune("st1", before); err != nil {
		t.Fatal(err)
	}
	for station, want := range map[string]int{"st1": 1, "st2": 1} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM bat_status WHERE station_id=?`, station).Scan(&n); err != nil || n != want {
			t.Fatalf("%s: %d rows, %v; want %d", station, n, err, want)
		}
	}
	var at int64
	if err := s.db.QueryRow(`SELECT at FROM bat_status WHERE station_id='st1'`).Scan(&at); err != nil || at != ms(before.Add(time.Hour)) {
		t.Fatalf("kept %d, %v; want the newer row", at, err)
	}
}

// A filled v3 database is upgraded to the latest version in place: samples
// and capacity estimates are kept, and v4's bat_status exists but stays
// empty (the app no longer writes it); opening again changes nothing.
func TestMigrationV4KeepsData(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v3.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	for _, q := range []string{schemaV1, schemaV2, schemaV3, `PRAGMA user_version = 3`,
		fmt.Sprintf(`INSERT INTO samples(station_id,at,source,soc,battery_w,grid) VALUES('st1',%d,'lan',55,-300,2)`, at.UnixMilli()),
		fmt.Sprintf(`INSERT INTO capacity_estimates(station_id,start,"end",wh,dsoc) VALUES('st1',%d,%d,5000,60)`, at.UnixMilli(), at.UnixMilli()+1),
		`INSERT INTO settings(key,value) VALUES('app.settings','{}')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	for i := 0; i < 2; i++ {
		s, err := Open(p)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != len(migrations) {
			t.Fatalf("user_version = %d, want %d", v, len(migrations))
		}
		if got, err := s.Samples("st1", at, at.Add(time.Second)); err != nil || len(got) != 1 {
			t.Fatalf("samples = %+v, %v", got, err)
		}
		if got, err := s.CapacityEstimates("st1"); err != nil || len(got) != 1 {
			t.Fatalf("estimates = %+v, %v", got, err)
		}
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM bat_status`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("bat_status: %d rows, %v; want the empty table", n, err)
		}
		s.Close()
	}
}
