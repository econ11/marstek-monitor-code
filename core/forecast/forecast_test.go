package forecast

import (
	"math"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

var venus = model.Profile{Model: "VenusE 3.0", NominalWh: 5120, MaxChargeW: 1750, MaxDischargeW: 2560, MinSOC: 10}

func near(t *testing.T, want, got time.Duration) {
	t.Helper()
	if math.Abs((want - got).Seconds()) > 60 {
		t.Fatalf("want %v ± 1m, got %v", want, got)
	}
}

func delta(t *testing.T, want, got, eps float64) {
	t.Helper()
	if math.Abs(want-got) > eps {
		t.Fatalf("want %v ± %v, got %v", want, eps, got)
	}
}

func TestToEmptyVenusE3(t *testing.T) {
	d, ok := ToEmpty(72, venus, 420)
	if !ok {
		t.Fatal("ok=false")
	}
	near(t, 7*time.Hour+33*time.Minute, d)
}

func TestToEmptyEdges(t *testing.T) {
	for _, w := range []float64{0, -5, math.NaN()} {
		if _, ok := ToEmpty(72, venus, w); ok {
			t.Fatalf("w=%v: want ok=false", w)
		}
	}
	if _, ok := ToEmpty(72, model.Profile{}, 420); ok {
		t.Fatal("unknown profile: want ok=false")
	}
	d, ok := ToEmpty(5, venus, 420)
	if !ok || d != 0 {
		t.Fatalf("below MinSOC: want 0,true got %v,%v", d, ok)
	}
}

func TestToFullTaper(t *testing.T) {
	d, ok := ToFull(63, venus, 1750)
	if !ok {
		t.Fatal("ok=false")
	}
	near(t, 1*time.Hour+22*time.Minute, d)
}

func TestToFullEdges(t *testing.T) {
	for _, w := range []float64{0, -5, math.NaN()} {
		if _, ok := ToFull(63, venus, w); ok {
			t.Fatalf("w=%v: want ok=false", w)
		}
	}
	if _, ok := ToFull(63, model.Profile{}, 1750); ok {
		t.Fatal("unknown profile: want ok=false")
	}
	if d, ok := ToFull(100, venus, 1750); !ok || d != 0 {
		t.Fatalf("full: want 0,true got %v,%v", d, ok)
	}
	// Only the taper part: 5% of 5120 = 256 Wh at 500 W.
	d, ok := ToFull(95, venus, 1000)
	if !ok {
		t.Fatal("ok=false")
	}
	near(t, time.Duration(256.0/500*float64(time.Hour)), d)
}

func TestFillToEmpty72(t *testing.T) {
	delta(t, 0.6889, FillToEmpty(72, venus), 1e-3)
	if v := FillToEmpty(5, venus); v != 0 {
		t.Fatalf("below min: %v", v)
	}
	if v := FillToEmpty(72, model.Profile{}); v != 0 {
		t.Fatalf("unknown profile: %v", v)
	}
}

func TestFillToFull(t *testing.T) {
	delta(t, 0.63, FillToFull(63), 1e-9)
	if FillToFull(120) != 1 || FillToFull(-3) != 0 {
		t.Fatal("not clamped")
	}
}

func TestFillNoLight(t *testing.T) {
	delta(t, 0.25, FillNoLight(time.Hour, 4*time.Hour), 1e-9)
	if FillNoLight(time.Hour, 0) != 0 {
		t.Fatal("zero elapsed")
	}
	if FillNoLight(5*time.Hour, 4*time.Hour) != 1 {
		t.Fatal("not clamped")
	}
}

func TestSmootherWarmup(t *testing.T) {
	t0 := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	s := NewSmoother(5 * time.Minute)
	s.Add(t0, 400)
	s.Add(t0.Add(30*time.Second), 500)
	if _, _, ok := s.Value(t0.Add(30 * time.Second)); ok {
		t.Fatal("30 s: want ok=false")
	}
	s.Add(t0.Add(61*time.Second), 600)
	w, span, ok := s.Value(t0.Add(61 * time.Second))
	if !ok {
		t.Fatal("61 s: want ok=true")
	}
	delta(t, 500, w, 1e-9)
	if span != 61*time.Second {
		t.Fatalf("span %v", span)
	}
}

func TestSmootherDropsOld(t *testing.T) {
	t0 := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	s := NewSmoother(5 * time.Minute)
	s.Add(t0, 1000)
	s.Add(t0.Add(6*time.Minute), 200)
	s.Add(t0.Add(7*time.Minute), 400)
	w, _, ok := s.Value(t0.Add(7 * time.Minute))
	if !ok {
		t.Fatal("ok=false")
	}
	delta(t, 300, w, 1e-9)
}

func TestLastsForNoHistory(t *testing.T) {
	if _, ok := LastsFor(72, venus, nil); ok {
		t.Fatal("nil avg: want ok=false")
	}
	w := 420.0
	d, ok := LastsFor(72, venus, &w)
	if !ok {
		t.Fatal("ok=false")
	}
	near(t, 7*time.Hour+33*time.Minute, d)
}

func TestAvgOutageDischargeW(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	f := func(v float64) *float64 { return &v }
	e1 := now.Add(-1 * time.Hour)
	eShort := now.Add(-3*time.Hour + 10*time.Minute)
	eOld := now.Add(-40*24*time.Hour + time.Hour)
	outs := []store.Outage{
		{Start: now.Add(-2 * time.Hour), End: &e1},         // 1 h, counts
		{Start: now.Add(-3 * time.Hour), End: &eShort},     // 10 min, excluded
		{Start: now.Add(-40 * 24 * time.Hour), End: &eOld}, // older than 30 days
	}
	samples := []model.Sample{
		{At: now.Add(-100 * time.Minute), BatteryW: f(-400)},
		{At: now.Add(-90 * time.Minute), BatteryW: f(-600)},
		{At: now.Add(-80 * time.Minute), BatteryW: f(-10)}, // dead zone
		{At: now.Add(-70 * time.Minute), BatteryW: nil},
		{At: now.Add(-175 * time.Minute), BatteryW: f(-3000)}, // inside the short outage
		{At: now.Add(-40*24*time.Hour + 10*time.Minute), BatteryW: f(-2000)},
		{At: now, BatteryW: f(500)},
	}
	avg := AvgOutageDischargeW(samples, outs)
	if avg == nil {
		t.Fatal("nil")
	}
	delta(t, 500, *avg, 1e-9)

	if AvgOutageDischargeW(samples, nil) != nil {
		t.Fatal("no outages: want nil")
	}
	if AvgOutageDischargeW(nil, outs) != nil {
		t.Fatal("no samples: want nil")
	}
}

func TestTinyPowerHasNoForecast(t *testing.T) {
	for _, w := range []float64{0.5, 1e-9} {
		if d, ok := ToEmpty(72, venus, w); ok {
			t.Errorf("ToEmpty w=%v: want ok=false, got %v", w, d)
		}
		if d, ok := ToFull(63, venus, w); ok {
			t.Errorf("ToFull w=%v: want ok=false, got %v", w, d)
		}
		if d, ok := LastsFor(72, venus, &w); ok {
			t.Errorf("LastsFor w=%v: want ok=false, got %v", w, d)
		}
	}
	if _, ok := ToEmpty(72, venus, 1); !ok {
		t.Error("ToEmpty at 1 W: want ok=true")
	}
}
