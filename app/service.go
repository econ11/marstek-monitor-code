package app

import (
	"fmt"
	"sync"
	"time"

	"github.com/econ11/marstek-monitor-code/core/energy"
	"github.com/econ11/marstek-monitor-code/core/health"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/monitor"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SnapshotEvent is the Go -> JS event carrying a monitor.Snapshot after
// every change.
const SnapshotEvent = "snapshot"

func init() {
	application.RegisterEvent[monitor.Snapshot](SnapshotEvent)
}

// Service is the scaffold service the placeholder frontend pings.
type Service struct{}

// Ping lets the frontend verify the Go <-> JS bridge works.
func (s *Service) Ping() string {
	return "pong"
}

// SeriesResponse is the charge chart: points plus outage and gap bands.
type SeriesResponse struct {
	Points []monitor.SeriesPoint
	Bands  []monitor.Band
}

// MonitorService is bound to the frontend. Until a station is found there is
// no monitor: Snapshot is then the zero value (no station).
type MonitorService struct {
	mu sync.RWMutex
	m  *monitor.Monitor
}

// SetMonitor attaches the running monitor.
//
//wails:ignore
func (s *MonitorService) SetMonitor(m *monitor.Monitor) {
	s.mu.Lock()
	s.m = m
	s.mu.Unlock()
}

func (s *MonitorService) monitor() *monitor.Monitor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m
}

// Snapshot returns the latest state.
func (s *MonitorService) Snapshot() monitor.Snapshot {
	if m := s.monitor(); m != nil {
		return m.Latest()
	}
	return monitor.Snapshot{}
}

// SOCSeries returns the charge chart for "24h", "7d" or "30d".
func (s *MonitorService) SOCSeries(rangeKey string) (SeriesResponse, error) {
	m := s.monitor()
	if m == nil {
		return SeriesResponse{}, monitor.ErrNoStation
	}
	points, bands, err := m.SOCSeries(rangeKey)
	return SeriesResponse{Points: points, Bands: bands}, err
}

// OutagesReport returns the "Outages" tab for the local days fromDay..toDay
// ("2006-01-02", inclusive).
func (s *MonitorService) OutagesReport(fromDay, toDay string) (history.Report, error) {
	m := s.monitor()
	if m == nil {
		return history.Report{}, monitor.ErrNoStation
	}
	return m.OutagesReport(fromDay, toDay)
}

// EnergyReport returns the battery energy for the local days fromDay..toDay
// ("2006-01-02", inclusive).
func (s *MonitorService) EnergyReport(fromDay, toDay string) (energy.Report, error) {
	m := s.monitor()
	if m == nil {
		return energy.Report{}, monitor.ErrNoStation
	}
	return m.EnergyReport(fromDay, toDay)
}

// PowerSeries returns the battery power chart of the "Energy" tab for the
// local days fromDay..toDay ("2006-01-02", inclusive).
func (s *MonitorService) PowerSeries(fromDay, toDay string) (energy.PowerSeries, error) {
	m := s.monitor()
	if m == nil {
		return energy.PowerSeries{}, monitor.ErrNoStation
	}
	return m.PowerSeries(fromDay, toDay)
}

// Battery returns the "Battery" tab: capacity estimates, estimated cycles and
// the largest discharge so far.
func (s *MonitorService) Battery() (health.Summary, error) {
	m := s.monitor()
	if m == nil {
		return health.Summary{}, monitor.ErrNoStation
	}
	return m.Battery()
}

// JournalPageView is one page of the "Errors" tab: the entries (newest
// first), whether older ones follow, and the filter counts of the whole period.
type JournalPageView struct {
	Entries []journal.Entry
	More    bool
	Counts  monitor.JournalCounts
}

// journalPeriod turns the local days into the read range (to is nudged past
// now: what is logged this very moment counts).
func journalPeriod(fromDay, toDay string) (from, to time.Time, err error) {
	now := time.Now()
	p, err := history.PeriodFor(fromDay, toDay, now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to = p.To
	if !to.Before(now) {
		to = now.Add(time.Second)
	}
	return p.From, to, nil
}

// parseCursor reads the (At, ID) a page continues from; an empty At means none.
func parseCursor(at, id string) (*journal.Cursor, error) {
	if at == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, fmt.Errorf("journal cursor: %w", err)
	}
	return &journal.Cursor{At: t, ID: id}, nil
}

// JournalPage returns up to limit "Errors" entries for the local days
// fromDay..toDay ("2006-01-02", inclusive) under filter (all|errors|warnings|
// connection), newest first, after the cursor
// (beforeAt RFC 3339 + beforeID; "" = from the newest). Counts are of the whole
// period; Counts.New counts warnings and errors after newSince ("" = never).
func (s *MonitorService) JournalPage(fromDay, toDay string, beforeAt, beforeID string, limit int, filter, newSince string) (JournalPageView, error) {
	var seen time.Time // "" = never seen
	if newSince != "" {
		t, err := time.Parse(time.RFC3339Nano, newSince)
		if err != nil {
			return JournalPageView{}, fmt.Errorf("journal newSince: %w", err)
		}
		seen = t
	}
	cur, err := parseCursor(beforeAt, beforeID)
	if err != nil {
		return JournalPageView{}, err
	}
	m := s.monitor()
	if m == nil {
		return JournalPageView{}, monitor.ErrNoStation
	}
	from, to, err := journalPeriod(fromDay, toDay)
	if err != nil {
		return JournalPageView{}, err
	}
	es, more, counts, err := m.JournalPage(from, to, cur, limit, filter, seen)
	return JournalPageView{Entries: es, More: more, Counts: counts}, err
}

// JournalSinceView is what JournalSince returns: the newer entries and the
// counts of the whole period (JournalPageView.Counts).
type JournalSinceView struct {
	Entries []journal.Entry
	Counts  monitor.JournalCounts
}

// JournalSince returns the entries under filter strictly newer than the
// cursor, newest first, with the counts of the whole period (New against newSince).
func (s *MonitorService) JournalSince(fromDay, toDay string, afterAt, afterID, filter, newSince string) (JournalSinceView, error) {
	cur, err := parseCursor(afterAt, afterID)
	if err != nil {
		return JournalSinceView{}, err
	}
	var seen time.Time
	if newSince != "" {
		if seen, err = time.Parse(time.RFC3339Nano, newSince); err != nil {
			return JournalSinceView{}, fmt.Errorf("journal newSince: %w", err)
		}
	}
	m := s.monitor()
	if m == nil {
		return JournalSinceView{}, monitor.ErrNoStation
	}
	if cur == nil {
		return JournalSinceView{}, fmt.Errorf("journal since: no cursor")
	}
	from, to, err := journalPeriod(fromDay, toDay)
	if err != nil {
		return JournalSinceView{}, err
	}
	es, counts, err := m.JournalSince(from, to, *cur, filter, seen)
	return JournalSinceView{Entries: es, Counts: counts}, err
}

// MarkJournalSeen records that the user has seen the journal up to upTo
// (RFC 3339: the At of the newest entry on screen); a warning logged after
// it stays new.
func (s *MonitorService) MarkJournalSeen(upTo string) error {
	t, err := time.Parse(time.RFC3339Nano, upTo)
	if err != nil {
		return fmt.Errorf("journal seen: %w", err)
	}
	m := s.monitor()
	if m == nil {
		return monitor.ErrNoStation
	}
	return m.MarkJournalSeen(t)
}

// DismissGapCard closes the "while the app was not running" card for good.
func (s *MonitorService) DismissGapCard(gapID int64) error {
	m := s.monitor()
	if m == nil {
		return monitor.ErrNoStation
	}
	return m.DismissGapCard(gapID)
}
