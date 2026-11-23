package model

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

func fp(v float64) *float64 { return &v }

func load(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func eq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestParseESStatusFixtures(t *testing.T) {
	cases := []struct {
		file string
		soc  *float64
		batW *float64
		grid GridSignal
	}{
		{"es_status_grid_on_idle.json", fp(100), fp(0), GridPresent},
		{"es_status_grid_off_kettle.json", fp(99), fp(-2239), GridAbsent},
		{"es_status_grid_on_charging.json", fp(93), fp(2492), GridPresent},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			at := time.Unix(100, 0)
			s, err := NewSample("st1", at, "lan", load(t, c.file))
			if err != nil {
				t.Fatal(err)
			}
			if !eq(s.SOC, c.soc) {
				t.Errorf("SOC = %v, want %v", s.SOC, c.soc)
			}
			if !eq(s.BatteryW, c.batW) {
				t.Errorf("BatteryW = %v, want %v", s.BatteryW, c.batW)
			}
			if s.Grid != c.grid {
				t.Errorf("Grid = %v, want %v", s.Grid, c.grid)
			}
			if s.StationID != "st1" || !s.At.Equal(at) || s.Source != "lan" || len(s.Raw) == 0 {
				t.Errorf("metadata not filled: %+v", s)
			}
		})
	}
}

func TestParseESStatusNullsAndMissing(t *testing.T) {
	for _, raw := range []string{string(load(t, "es_status_nulls.json")), `{"bat_soc":null}`, `{}`} {
		s, err := NewSample("x", time.Now(), "lan", json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if s.SOC != nil || s.BatteryW != nil || s.Grid != GridUnknown || s.OngridW != nil || s.OffgridW != nil || s.PVW != nil {
			t.Errorf("%s: want all unknown, got %+v", raw, s)
		}
	}
}

func TestParseESStatusInvalid(t *testing.T) {
	if _, err := ParseESStatus(json.RawMessage(`[1]`)); err == nil {
		t.Error("want error for non-object")
	}
	if _, err := ParseESStatus(nil); err == nil {
		t.Error("want error for empty")
	}
}

func TestGridFromStatus(t *testing.T) {
	cases := []struct {
		on, off *float64
		want    GridSignal
	}{
		{fp(-2492), fp(0), GridPresent},
		{fp(-2404), fp(295), GridPresent},
		{fp(0), fp(2246), GridAbsent},
		{fp(0), fp(15), GridPresent},
		{fp(0), fp(21), GridAbsent},
		{fp(-15), fp(300), GridAbsent},
		{fp(0), nil, GridUnknown},
		{nil, fp(420), GridUnknown},
		{fp(500), nil, GridUnknown},
		{nil, nil, GridUnknown},
	}
	for i, c := range cases {
		if got := GridFromStatus(ESStatus{OngridW: c.on, OffgridW: c.off}); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestBatteryPower(t *testing.T) {
	cases := []struct {
		name string
		es   ESStatus
		want *float64
	}{
		{"discharge", ESStatus{OngridW: fp(0), OffgridW: fp(2246), PVW: fp(0)}, fp(-2246)},
		{"charge", ESStatus{OngridW: fp(-2492), OffgridW: fp(0)}, fp(2492)},
		{"charge with load, pv nil", ESStatus{OngridW: fp(-2404), OffgridW: fp(295), PVW: nil}, fp(2109)},
		{"fallback bat_power", ESStatus{OngridW: nil, BatW: fp(100)}, fp(-100)},
		{"all nil", ESStatus{}, nil},
	}
	for _, c := range cases {
		if got := BatteryPower(c.es); !eq(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if got := BatteryPower(ESStatus{OngridW: fp(0), OffgridW: fp(0)}); got == nil || math.Signbit(*got) {
		t.Errorf("idle must be +0, got %v", got)
	}
}

func TestParseESStatusOneBadFieldKeepsOthers(t *testing.T) {
	es, err := ParseESStatus(json.RawMessage(`{"bat_soc":"x","ongrid_power":-300}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if es.SOC != nil {
		t.Errorf("SOC = %v, want nil", *es.SOC)
	}
	if !eq(es.OngridW, fp(-300)) {
		t.Errorf("OngridW = %v, want -300", es.OngridW)
	}
	if es.CapWh != nil || es.PVW != nil || es.OffgridW != nil || es.BatW != nil {
		t.Errorf("missing fields must stay nil: %+v", es)
	}
}

func TestParseESStatusNullResult(t *testing.T) {
	es, err := ParseESStatus(json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if es != (ESStatus{}) {
		t.Errorf("want ESStatus{}, got %+v", es)
	}
}

func TestGridFromStatusEdges(t *testing.T) {
	cases := []struct {
		name    string
		on, off *float64
		want    GridSignal
	}{
		{"on 5, off nil", fp(5), nil, GridUnknown},
		{"on 0, off -30", fp(0), fp(-30), GridPresent},
		{"on -20 is not present by charging", fp(-20), nil, GridUnknown},
		{"off 20 is present by idle", fp(0), fp(20), GridPresent},
		{"off 21 at on 0 is absent", fp(0), fp(21), GridAbsent},
	}
	for _, c := range cases {
		if got := GridFromStatus(ESStatus{OngridW: c.on, OffgridW: c.off}); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestBatteryPowerFallbackWhenOffgridNil(t *testing.T) {
	got := BatteryPower(ESStatus{OngridW: fp(-300), OffgridW: nil, BatW: fp(250)})
	if !eq(got, fp(-250)) {
		t.Fatalf("got %v, want -250", got)
	}
}

func TestBatteryPowerNilWhenANeededFieldIsUnreadable(t *testing.T) {
	for _, raw := range []string{
		`{"ongrid_power":0,"offgrid_power":300,"pv_power":"x"}`,
		`{"ongrid_power":"x","offgrid_power":300,"bat_power":250}`,
		`{"ongrid_power":0,"offgrid_power":[1],"bat_power":250}`,
		`{"ongrid_power":-300,"bat_power":"x"}`,
	} {
		s, err := NewSample("x", time.Now(), "lan", json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if s.BatteryW != nil {
			t.Errorf("%s: BatteryW = %v, want nil", raw, *s.BatteryW)
		}
	}
	// A bad bat_power is not needed when ongrid and offgrid are known.
	s, _ := NewSample("x", time.Now(), "lan", json.RawMessage(`{"ongrid_power":0,"offgrid_power":300,"bat_power":"x"}`))
	if !eq(s.BatteryW, fp(-300)) {
		t.Errorf("BatteryW = %v, want -300", s.BatteryW)
	}
}
