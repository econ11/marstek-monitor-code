package energy

import (
	"time"

	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// MaxPowerDays: the battery power chart of a longer period shows only its
// last MaxPowerDays local days (store.PowerBuckets is bounded by them).
const MaxPowerDays = 31

// PowerWindow is the span the power chart draws: whole local days from
// Start (local midnight) to End (the local midnight after the last day, even
// for today), per minute for one day and per hour for more. Clipped: the
// period was longer than MaxPowerDays and only its last days are drawn.
type PowerWindow struct {
	Start, End time.Time
	Step       time.Duration
	Clipped    bool
}

// PowerWindowFor is the window of a report period (history.PeriodFor).
func PowerWindowFor(p history.Period) PowerWindow {
	// The last day is the one holding the period's last instant: To is the
	// midnight after it, or now when the period reaches today.
	last := p.To.Add(-time.Nanosecond).In(time.Local)
	if !p.To.After(p.From) {
		last = p.From.In(time.Local)
	}
	end := time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, time.Local)
	w := PowerWindow{Start: p.From, End: end, Step: time.Hour}
	if earliest := time.Date(last.Year(), last.Month(), last.Day()-(MaxPowerDays-1), 0, 0, 0, 0, time.Local); w.Start.Before(earliest) {
		w.Start, w.Clipped = earliest, true
	}
	if s := w.Start.In(time.Local); end.Equal(time.Date(s.Year(), s.Month(), s.Day()+1, 0, 0, 0, 0, time.Local)) {
		w.Step = time.Minute
	}
	return w
}

// Query is the range to read for the window: up to now at most (a second
// past it, so the newest sample is in), never longer than store.MaxPowerSpan.
func (w PowerWindow) Query(now time.Time) (from, to time.Time) {
	to = w.End
	if n := now.Add(time.Second); n.Before(to) {
		to = n
	}
	if to.Before(w.Start) {
		to = w.Start
	}
	return w.Start, to
}

// PowerPoint is one minute (or hour, from its start) of the chart: W the
// average battery power (+ charge, − discharge), nil when unknown — the line
// breaks, it is never drawn as 0.
type PowerPoint struct {
	At time.Time
	W  *float64
}

// PowerBand is an outage clipped to the window; an ongoing one ends at now.
type PowerBand struct {
	Start, End time.Time
	Ongoing    bool
}

// PowerSeries is the «Потужність батареї» chart of a period. Points are
// ascending; a missing minute (hour) has no point at all. PeakW/PeakAt: the
// highest discharge in the window (energy_minute.peak_dis_w), nil without
// discharge.
type PowerSeries struct {
	Start, End time.Time
	Step       time.Duration
	Clipped    bool
	Points     []PowerPoint
	PeakW      *float64
	PeakAt     *time.Time
	Outages    []PowerBand
}

// BuildPowerSeries turns the stored buckets, the outages around the window
// and the window's peak into the chart.
func BuildPowerSeries(w PowerWindow, buckets []store.PowerBucket, outs []store.Outage, peakW *float64, peakAt *time.Time, now time.Time) PowerSeries {
	s := PowerSeries{Start: w.Start, End: w.End, Step: w.Step, Clipped: w.Clipped, PeakW: peakW, PeakAt: peakAt,
		Points: make([]PowerPoint, 0, len(buckets)), Outages: []PowerBand{}}
	for _, b := range buckets {
		s.Points = append(s.Points, PowerPoint{At: b.At, W: b.W})
	}
	for _, o := range outs {
		end, ongoing := now, o.End == nil
		if !ongoing && o.End.Before(now) {
			end = *o.End
		}
		a, e := o.Start, end
		if a.Before(w.Start) {
			a = w.Start
		}
		if e.After(w.End) {
			e = w.End
		}
		if e.After(a) {
			s.Outages = append(s.Outages, PowerBand{Start: a, End: e, Ongoing: ongoing})
		}
	}
	return s
}
