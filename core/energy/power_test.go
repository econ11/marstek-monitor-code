package energy

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/store"
)

// One day is drawn per minute, 00:00 to the next local midnight (25 h on
// 25.10.2026); a longer period per hour; one longer than MaxPowerDays only
// its last MaxPowerDays local days, with Clipped set.
func TestPowerWindow(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 22, 14)
	cases := []struct {
		name, from, to string
		start, end     time.Time
		step           time.Duration
		clipped        bool
	}{
		{"today", "2026-11-22", "2026-11-22", lt(2026, 11, 22, 0), lt(2026, 11, 23, 0), time.Minute, false},
		{"a past day", "2026-11-20", "2026-11-20", lt(2026, 11, 20, 0), lt(2026, 11, 21, 0), time.Minute, false},
		{"DST day", "2026-10-25", "2026-10-25", lt(2026, 10, 25, 0), lt(2026, 10, 26, 0), time.Minute, false},
		{"7 days", "2026-11-16", "2026-11-22", lt(2026, 11, 16, 0), lt(2026, 11, 23, 0), time.Hour, false},
		{"31 days", "2026-10-23", "2026-11-22", lt(2026, 10, 23, 0), lt(2026, 11, 23, 0), time.Hour, false},
		{"60 days", "2026-09-24", "2026-11-22", lt(2026, 10, 23, 0), lt(2026, 11, 23, 0), time.Hour, true},
	}
	for _, c := range cases {
		w := PowerWindowFor(period(t, c.from, c.to, now))
		if !w.Start.Equal(c.start) || !w.End.Equal(c.end) || w.Step != c.step || w.Clipped != c.clipped {
			t.Errorf("%s: window = %v..%v step %v clipped %v, want %v..%v %v %v", c.name, w.Start, w.End, w.Step, w.Clipped, c.start, c.end, c.step, c.clipped)
		}
		// The query reads up to now at most, and never more than the store allows.
		from, to := w.Query(now)
		if to.After(now.Add(time.Second)) || to.Sub(from) > store.MaxPowerSpan || from.Before(w.Start) {
			t.Errorf("%s: query %v..%v", c.name, from, to)
		}
	}
	if d := lt(2026, 10, 26, 0).Sub(lt(2026, 10, 25, 0)); d != 25*time.Hour {
		t.Fatalf("25.10 is %v long", d)
	}
}

// Points keep a missing value nil; outage bands are clipped to the window,
// an ongoing one up to now; the peak is kept.
func TestBuildPowerSeries(t *testing.T) {
	kyiv(t)
	now := lt(2026, 11, 22, 14)
	w := PowerWindowFor(period(t, "2026-11-22", "2026-11-22", now))
	m := lt(2026, 11, 22, 9)
	buckets := []store.PowerBucket{
		{At: m, W: f(-412)},
		{At: m.Add(time.Minute), W: nil},
		{At: m.Add(2 * time.Minute), W: f(-300)},
		{At: m.Add(3 * time.Minute), W: f(250)},
	}
	peakAt := m.Add(10 * time.Second)
	outs := []store.Outage{
		{Start: lt(2026, 11, 21, 22), End: tp(lt(2026, 11, 22, 1))}, // started yesterday
		{Start: lt(2026, 11, 22, 13)},                               // ongoing
	}
	s := BuildPowerSeries(w, buckets, outs, f(812), &peakAt, now)
	if s.Step != time.Minute || !s.Start.Equal(w.Start) || !s.End.Equal(w.End) || len(s.Points) != 4 {
		t.Fatalf("series = %+v", s)
	}
	if p := s.Points[0]; p.W == nil || *p.W != -412 || !p.At.Equal(m) {
		t.Fatalf("point 0 = %v at %v, want −412 W at 09:00", p.W, p.At)
	}
	if p := s.Points[1]; p.W != nil {
		t.Fatalf("point 1 = %v, want nil (no invented 0)", *p.W)
	}
	if s.PeakW == nil || *s.PeakW != 812 || s.PeakAt == nil || !s.PeakAt.Equal(peakAt) {
		t.Fatalf("peak = %v at %v", s.PeakW, s.PeakAt)
	}
	if len(s.Outages) != 2 {
		t.Fatalf("outages = %+v", s.Outages)
	}
	if o := s.Outages[0]; !o.Start.Equal(w.Start) || !o.End.Equal(lt(2026, 11, 22, 1)) || o.Ongoing {
		t.Fatalf("outage 0 = %+v, want 00:00–01:00", o)
	}
	if o := s.Outages[1]; !o.Start.Equal(lt(2026, 11, 22, 13)) || !o.End.Equal(now) || !o.Ongoing {
		t.Fatalf("outage 1 = %+v, want 13:00–now, ongoing", o)
	}
}
