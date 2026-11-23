package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
)

func TestCompactAggregatesOldSamples(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	base := now.AddDate(0, 0, -31).Truncate(time.Minute)
	for i := 0; i < 120; i++ {
		soc := float64(i)
		grid := model.GridPresent
		if i < 12 { // whole first minute without grid
			grid = model.GridAbsent
		}
		x := model.Sample{StationID: "st1", At: base.Add(time.Duration(i) * 5 * time.Second), Source: model.SourceLAN,
			SOC: &soc, BatteryW: f(float64(-100 * (i%12 + 1))), Grid: grid}
		if err := s.InsertSample(x); err != nil {
			t.Fatal(err)
		}
	}
	s.InsertSample(model.Sample{StationID: "st1", At: now.Add(-time.Hour), Source: model.SourceLAN, SOC: f(70)})

	if err := s.Compact(now); err != nil {
		t.Fatal(err)
	}
	pts, err := s.MinuteSeries("st1", base, base.Add(time.Hour))
	if err != nil || len(pts) != 10 {
		t.Fatalf("points=%d err=%v", len(pts), err)
	}
	p0 := pts[0]
	if !p0.At.Equal(base) || *p0.SOCMin != 0 || *p0.SOCMax != 11 || *p0.SOCAvg != 5.5 || *p0.BatWAvg != -650 {
		t.Errorf("p0: at=%v min=%v max=%v avg=%v bat=%v", p0.At, *p0.SOCMin, *p0.SOCMax, *p0.SOCAvg, *p0.BatWAvg)
	}
	if p0.GridOffSec != 60 || pts[1].GridOffSec != 0 {
		t.Errorf("grid off: %d %d", p0.GridOffSec, pts[1].GridOffSec)
	}
	if *pts[9].SOCMax != 119 {
		t.Errorf("p9 max: %v", *pts[9].SOCMax)
	}
	if old, _ := s.Samples("st1", base.Add(-time.Hour), base.Add(time.Hour)); len(old) != 0 {
		t.Errorf("raw old samples not removed: %d", len(old))
	}
	if got, _ := s.Samples("st1", now.Add(-2*time.Hour), now); len(got) != 1 {
		t.Errorf("fresh sample touched: %d", len(got))
	}
	if err := s.Compact(now); err != nil { // idempotent
		t.Fatal(err)
	}
	if pts2, _ := s.MinuteSeries("st1", base, base.Add(time.Hour)); len(pts2) != 10 {
		t.Errorf("second compact changed points: %d", len(pts2))
	}
	if since, ok, _ := s.ObservedSince("st1"); !ok || !since.Equal(base) {
		t.Errorf("ObservedSince after compact: %v %v", since, ok)
	}
}

func TestCompactNullsOldRaw(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"bat_soc":50}`)
	s.InsertSample(model.Sample{StationID: "st1", At: now.AddDate(0, 0, -8), Source: model.SourceLAN, SOC: f(50), Raw: raw})
	s.InsertSample(model.Sample{StationID: "st1", At: now.AddDate(0, 0, -6), Source: model.SourceLAN, SOC: f(51), Raw: raw})
	if err := s.Compact(now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Samples("st1", now.AddDate(0, 0, -10), now)
	if len(got) != 2 {
		t.Fatalf("samples=%d", len(got))
	}
	if got[0].Raw != nil || *got[0].SOC != 50 {
		t.Errorf("8d raw should be nil, sample kept: %+v", got[0])
	}
	if string(got[1].Raw) != string(raw) {
		t.Errorf("6d raw lost: %s", got[1].Raw)
	}
}

// In Calm the samples come every 20 s: each holds its grid state up to
// energy.MaxStep (60 s), so a minute without the grid counts 60 s, not 45.
func TestCompactCapsSpanAtMaxStep(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	base := now.AddDate(0, 0, -31).Truncate(time.Minute)
	for i := 0; i < 6; i++ { // two minutes, three samples each
		grid := model.GridAbsent
		if i >= 3 {
			grid = model.GridPresent
		}
		if err := s.InsertSample(model.Sample{StationID: "st1", At: base.Add(time.Duration(i) * 20 * time.Second),
			Source: model.SourceLAN, SOC: f(50), Grid: grid}); err != nil {
			t.Fatal(err)
		}
	}
	// A 5-minute hole after a sample without the grid counts only MaxStep.
	for _, d := range []time.Duration{10 * time.Minute, 15 * time.Minute} {
		if err := s.InsertSample(model.Sample{StationID: "st1", At: base.Add(d), Source: model.SourceLAN,
			SOC: f(50), Grid: model.GridAbsent}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Compact(now); err != nil {
		t.Fatal(err)
	}
	pts, err := s.MinuteSeries("st1", base, base.Add(time.Hour))
	if err != nil || len(pts) != 4 {
		t.Fatalf("points = %+v, %v", pts, err)
	}
	if pts[0].GridOffSec != 60 || pts[1].GridOffSec != 0 || pts[2].GridOffSec != 60 {
		t.Fatalf("grid off = %d %d %d, want 60 0 60", pts[0].GridOffSec, pts[1].GridOffSec, pts[2].GridOffSec)
	}
}
