package store

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
)

// Minute averages come from the raw samples (the last 30 days) and from
// samples_minute.bat_w_avg (older); a minute whose samples have no battery
// power is a nil value, never 0; minutes without samples are not returned.
func TestStorePowerBucketsMinutes(t *testing.T) {
	s, _ := openTemp(t)
	h := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	add := func(at time.Time, w *float64) {
		t.Helper()
		if err := s.InsertSample(model.Sample{StationID: "st1", At: at, Source: model.SourceLAN, BatteryW: w, Grid: model.GridPresent}); err != nil {
			t.Fatal(err)
		}
	}
	// 10:00: −400 and −200 → −300. 10:01: unknown power. 10:02: nothing. 10:03: +500.
	add(h, f(-400))
	add(h.Add(30*time.Second), f(-200))
	add(h.Add(time.Minute), nil)
	add(h.Add(3*time.Minute), f(500))
	add(h.Add(3*time.Minute+20*time.Second), f(500))
	s.InsertSample(model.Sample{StationID: "st2", At: h.Add(4 * time.Minute), Source: model.SourceLAN, BatteryW: f(9)})
	// Aggregated minutes at 09:58 (unknown) and 09:59 (older data, after Compact).
	if _, err := s.db.Exec(`INSERT INTO samples_minute(station_id,minute,bat_w_avg) VALUES('st1',?,?),('st1',?,NULL)`,
		ms(h.Add(-time.Minute)), -100, ms(h.Add(-2*time.Minute))); err != nil {
		t.Fatal(err)
	}
	got, err := s.PowerBuckets("st1", h.Add(-2*time.Minute), h.Add(time.Hour), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	wants := []PowerBucket{
		{h.Add(-2 * time.Minute), nil},
		{h.Add(-time.Minute), f(-100)},
		{h, f(-300)},
		{h.Add(time.Minute), nil},
		{h.Add(3 * time.Minute), f(500)},
	}
	checkBuckets(t, got, wants)
}

// An hour is its net energy in the rollup over its covered time: no raw
// sample is read. An hour without rolled-up minutes has no bucket (never 0).
// The two 03:00 hours of 25.10.2026 in Kyiv (00:00 and 01:00 UTC) stay apart.
func TestStorePowerBucketsHoursFromRollup(t *testing.T) {
	s, _ := openTemp(t)
	h := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) // 03:00 EEST
	var mins []EnergyMinute
	// 00:xx UTC: 30 covered minutes discharging 600 W (10 Wh each) → −600 W.
	for i := 0; i < 30; i++ {
		mins = append(mins, EnergyMinute{At: h.Add(time.Duration(i) * time.Minute), DisWh: 10, Covered: time.Minute})
	}
	// 01:xx UTC (03:00 EET, the repeated hour): 10 Wh charged, 4 Wh discharged over 2 × 30 s → +360 W.
	mins = append(mins,
		EnergyMinute{At: h.Add(time.Hour), ChgWh: 10, Covered: 30 * time.Second},
		EnergyMinute{At: h.Add(time.Hour + time.Minute), DisWh: 4, Covered: 30 * time.Second})
	// 02:xx UTC: nothing. 03:xx UTC: idle minutes, covered: a measured 0 W.
	mins = append(mins, EnergyMinute{At: h.Add(3 * time.Hour), Covered: time.Minute})
	if err := s.UpsertEnergyMinutes("st1", mins); err != nil {
		t.Fatal(err)
	}
	// Raw samples are not read for hours.
	if err := s.InsertSample(model.Sample{StationID: "st1", At: h.Add(2 * time.Hour), Source: model.SourceLAN, BatteryW: f(999)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.PowerBuckets("st1", h, h.Add(4*time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	checkBuckets(t, got, []PowerBucket{{h, f(-600)}, {h.Add(time.Hour), f(360)}, {h.Add(3 * time.Hour), f(0)}})
}

func checkBuckets(t *testing.T, got, wants []PowerBucket) {
	t.Helper()
	if len(got) != len(wants) {
		t.Fatalf("buckets = %+v, want %d", got, len(wants))
	}
	for i, w := range wants {
		g := got[i]
		if !g.At.Equal(w.At) || (g.W == nil) != (w.W == nil) || (g.W != nil && math.Abs(*g.W-*w.W) > 1e-9) {
			t.Fatalf("bucket %d = {%v %v}, want {%v %v}", i, g.At, ptrStr(g.W), w.At, ptrStr(w.W))
		}
	}
}

func ptrStr(p *float64) any {
	if p == nil {
		return "nil"
	}
	return *p
}

// The query is bounded: a window longer than MaxPowerSpan, or a bucket that
// is neither a minute nor an hour, is refused without reading anything.
func TestStorePowerBucketsBounded(t *testing.T) {
	s, _ := openTemp(t)
	h := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	if _, err := s.PowerBuckets("st1", h.Add(-MaxPowerSpan-time.Minute), h, time.Hour); !errors.Is(err, ErrPowerSpan) {
		t.Fatalf("over the span: err = %v, want ErrPowerSpan", err)
	}
	if _, err := s.PowerBuckets("st1", h.Add(-MaxPowerSpan), h, time.Hour); err != nil {
		t.Fatalf("at the span: err = %v", err)
	}
	if _, err := s.PowerBuckets("st1", h.Add(-time.Hour), h, 10*time.Second); err == nil {
		t.Fatal("10 s bucket accepted")
	}
	if _, err := s.PowerBuckets("st1", h, h.Add(-time.Hour), time.Minute); err == nil {
		t.Fatal("reversed window accepted")
	}
}

// The highest discharge in the window with its time; nil without discharge.
func TestStorePeakDischarge(t *testing.T) {
	s, _ := openTemp(t)
	h := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	w1, w2, w3 := 412.0, 812.0, 999.0
	a1, a2, a3 := h.Add(5*time.Second), h.Add(time.Minute+7*time.Second), h.Add(2*time.Hour)
	if err := s.UpsertEnergyMinutes("st1", []EnergyMinute{
		{At: h, DisWh: 1, Covered: time.Minute, PeakDisW: &w1, PeakAt: &a1},
		{At: h.Add(time.Minute), DisWh: 2, Covered: time.Minute, PeakDisW: &w2, PeakAt: &a2},
		{At: h.Add(2 * time.Minute), ChgWh: 3, Covered: time.Minute},
		{At: h.Add(2 * time.Hour), DisWh: 9, Covered: time.Minute, PeakDisW: &w3, PeakAt: &a3}, // outside
	}); err != nil {
		t.Fatal(err)
	}
	w, at, err := s.PeakDischarge("st1", h, h.Add(time.Hour))
	if err != nil || w == nil || *w != 812 || at == nil || !at.Equal(a2) {
		t.Fatalf("peak = %v at %v, %v; want 812 W at 10:01:07", w, at, err)
	}
	if w, at, err := s.PeakDischarge("st1", h.Add(2*time.Minute), h.Add(time.Hour)); err != nil || w != nil || at != nil {
		t.Fatalf("no discharge: peak = %v at %v, %v; want nil", w, at, err)
	}
}
