package monitor

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/econ11/marstek-monitor-code/core/energy"
	"github.com/econ11/marstek-monitor-code/core/health"
	"github.com/econ11/marstek-monitor-code/core/history"
	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/link"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// reportMargin: outages and gaps are read this far around the period, so the
// bounds of an outage that crosses the period edge are judged correctly.
const reportMargin = 24 * time.Hour

// OutagesReport builds the "Outages" tab for the local days fromDay..toDay
// ("2006-01-02", inclusive).
func (m *Monitor) OutagesReport(fromDay, toDay string) (history.Report, error) {
	now := m.clock.Now()
	p, err := history.PeriodFor(fromDay, toDay, now)
	if err != nil {
		return history.Report{}, err
	}
	m.mu.Lock()
	id, live, lastAt := m.station.ID, m.machine.State() == link.Live, now
	if m.last != nil {
		lastAt = m.last.At
	}
	m.mu.Unlock()

	outs, err := m.d.Store.Outages(id, p.From.Add(-reportMargin), p.To.Add(reportMargin))
	if err != nil {
		return history.Report{}, err
	}
	// Gaps from BoundarySlack before the earliest outage start: a gap that
	// ended just before it makes that start approximate. Up to now: rows use the
	// whole outage (Known, EndApprox), which can end long after the period.
	from := p.From.Add(-reportMargin)
	for _, o := range outs {
		if o.Start.Before(from) {
			from = o.Start
		}
	}
	gaps, err := m.d.Store.Gaps(id, from.Add(-history.BoundarySlack), now.Add(time.Second))
	if err != nil {
		return history.Report{}, err
	}
	var since *time.Time
	if t, ok, err := m.d.Store.ObservedSince(id); err != nil {
		return history.Report{}, err
	} else if ok {
		since = &t
	}
	minSOC := func(a, b time.Time) *float64 {
		v, err := m.d.Store.MinSOC(id, a, b)
		if err != nil {
			slog.Warn("monitor: min soc", "err", err)
			return nil
		}
		return v
	}
	return history.BuildReport(outs, gaps, minSOC, since, p, now, lastAt, live), nil
}

// EnergyReport builds the energy of the local days fromDay..toDay
// ("2006-01-02", inclusive) from the stored minutes. The last minute or two
// before now are not rolled up yet (rollupEnergy) and are missing.
func (m *Monitor) EnergyReport(fromDay, toDay string) (energy.Report, error) {
	now := m.clock.Now()
	p, err := history.PeriodFor(fromDay, toDay, now)
	if err != nil {
		return energy.Report{}, err
	}
	m.mu.Lock()
	id, next, failed := m.station.ID, m.energyNext, m.energyFailed
	m.mu.Unlock()
	hours, err := m.d.Store.EnergyHours(id, p.From, p.To)
	if err != nil {
		return energy.Report{}, err
	}
	var since *time.Time
	if t, ok, err := m.d.Store.ObservedSince(id); err != nil {
		return energy.Report{}, err
	} else if ok {
		since = &t
	}
	r := energy.BuildReport(hours, since, p, now)
	if t, ok, err := m.d.Store.FirstEnergyMinute(id); err != nil {
		return energy.Report{}, err
	} else if ok {
		r.EnergySince = &t
	}
	// Not started yet, or more behind than the normal lag of a minute or two,
	// and only while it is making progress: a rollup whose last attempt
	// failed is stuck, and the tab then shows the gaps as they are.
	r.Backfilling = !failed && (next.IsZero() || now.Sub(next) > energyBehind)
	return r, nil
}

// PowerSeries builds the battery power chart of the local days fromDay..toDay
// ("2006-01-02", inclusive): per minute for one day, per hour for more, at
// most the last energy.MaxPowerDays days (bounded store query). Minutes come
// from the samples; hours and the peak from the energy rollup, so the last
// minute or two before now are not in them yet.
func (m *Monitor) PowerSeries(fromDay, toDay string) (energy.PowerSeries, error) {
	now := m.clock.Now()
	p, err := history.PeriodFor(fromDay, toDay, now)
	if err != nil {
		return energy.PowerSeries{}, err
	}
	m.mu.Lock()
	id := m.station.ID
	m.mu.Unlock()
	w := energy.PowerWindowFor(p)
	from, to := w.Query(now)
	buckets, err := m.d.Store.PowerBuckets(id, from, to, w.Step)
	if err != nil {
		return energy.PowerSeries{}, err
	}
	peakW, peakAt, err := m.d.Store.PeakDischarge(id, from, to)
	if err != nil {
		return energy.PowerSeries{}, err
	}
	outs, err := m.d.Store.Outages(id, from, to)
	if err != nil {
		return energy.PowerSeries{}, err
	}
	return energy.BuildPowerSeries(w, buckets, outs, peakW, peakAt, now), nil
}

// SettingJournalSeen holds when the user last opened the journal (RFC 3339).
const SettingJournalSeen = "journal.seen_at"

// readJournal reads the journal events of station id in [from, to), and its
// gaps when withGaps (gap entries are info: they never count as new).
func (m *Monitor) readJournal(id string, from, to, now time.Time, withGaps bool) ([]journal.Entry, error) {
	var events []store.Event
	for _, code := range journal.Codes {
		es, err := m.d.Store.Events(id, code, from, to)
		if err != nil {
			return nil, err
		}
		events = append(events, es...)
	}
	if !withGaps {
		return journal.Build(events, nil, now), nil
	}
	gaps, err := m.d.Store.Gaps(id, from, to)
	if err != nil {
		return nil, err
	}
	return journal.Build(events, gaps, now), nil
}

// Journal returns the journal entries in [from, to), newest first.
func (m *Monitor) Journal(from, to time.Time) ([]journal.Entry, error) {
	m.mu.Lock()
	id := m.station.ID
	m.mu.Unlock()
	return m.readJournal(id, from, to, m.clock.Now(), true)
}

// JournalCounts are the whole-period totals behind the Errors filters, and
// New: the warnings and errors newer than the time the caller keeps as the
// last visit.
type JournalCounts struct{ All, Errors, Warnings, Connection, New int }

func countJournal(es []journal.Entry, newSince time.Time) JournalCounts {
	c := JournalCounts{All: len(es), New: journal.NewSince(es, newSince)}
	for _, e := range es {
		switch e.Level {
		case journal.Error:
			c.Errors++
		case journal.Warning:
			c.Warnings++
		}
		if e.Group == journal.Connection || e.Group == journal.App {
			c.Connection++
		}
	}
	return c
}

// journalFilter keeps the entries of a filter: "" / "all", "errors",
// "warnings" or "connection" (the connection events and the app's absences).
func journalFilter(es []journal.Entry, filter string) ([]journal.Entry, error) {
	var keep func(journal.Entry) bool
	switch filter {
	case "", "all":
		return es, nil
	case "errors":
		keep = func(e journal.Entry) bool { return e.Level == journal.Error }
	case "warnings":
		keep = func(e journal.Entry) bool { return e.Level == journal.Warning }
	case "connection":
		keep = func(e journal.Entry) bool { return e.Group == journal.Connection || e.Group == journal.App }
	default:
		return nil, fmt.Errorf("monitor: unknown journal filter %q", filter)
	}
	out := make([]journal.Entry, 0, len(es))
	for _, e := range es {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out, nil
}

// JournalPage returns up to limit entries of [from, to) under filter,
// strictly after the cursor before (nil = from the newest), newest first,
// whether more follow, and the counts of the whole period (not of the page or
// the filter). newSince is when the user last saw the journal (zero = never).
func (m *Monitor) JournalPage(from, to time.Time, before *journal.Cursor, limit int, filter string, newSince time.Time) (entries []journal.Entry, more bool, counts JournalCounts, err error) {
	es, err := m.Journal(from, to)
	if err != nil {
		return nil, false, JournalCounts{}, err
	}
	counts = countJournal(es, newSince)
	if es, err = journalFilter(es, filter); err != nil {
		return nil, false, JournalCounts{}, err
	}
	if before != nil {
		i := sort.Search(len(es), func(i int) bool { return es[i].After(*before) })
		es = es[i:]
	}
	if limit > 0 && len(es) > limit {
		es, more = es[:limit], true
	}
	return es, more, counts, nil
}

// JournalSince returns the entries of [from, to) under filter strictly newer
// than the cursor after, newest first, and the counts of the whole period
// (as JournalPage: not of the filter), so the caller keeps its chips right.
func (m *Monitor) JournalSince(from, to time.Time, after journal.Cursor, filter string, newSince time.Time) ([]journal.Entry, JournalCounts, error) {
	es, err := m.Journal(from, to)
	if err != nil {
		return nil, JournalCounts{}, err
	}
	counts := countJournal(es, newSince)
	if es, err = journalFilter(es, filter); err != nil {
		return nil, JournalCounts{}, err
	}
	i := sort.Search(len(es), func(i int) bool { return !es[i].Before(after) })
	return es[:i], counts, nil
}

// MarkJournalSeen records that the user has seen the journal up to upTo, the
// time of the newest entry on screen: NewEvents then counts only the
// warnings after it (one logged meanwhile stays new), and the snapshot is
// published. The mark never moves back and never past now.
func (m *Monitor) MarkJournalSeen(upTo time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	// Clamped at mark time, not fetch time: that differs only for an entry
	// ahead of the clock (the clock set back), where a warning logged between
	// the fetch and the mark would count as seen. Accepted.
	seen := upTo
	if seen.After(now) {
		seen = now
	}
	// A stored mark ahead of the clock (the clock was set back since) is
	// replaced, else it would hide new warnings until the clock caught up.
	if m.seenAt != nil && !m.seenAt.After(now) && !seen.After(*m.seenAt) {
		return nil
	}
	if err := m.d.Store.SetSetting(SettingJournalSeen, seen.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	m.seenAt = &seen
	m.refreshNewEvents()
	m.publish(m.snapshot(now))
	return nil
}

// loadJournalSeen reads journal.seen_at; an unreadable value counts as never.
func (m *Monitor) loadJournalSeen() error {
	v, ok, err := m.d.Store.GetSetting(SettingJournalSeen)
	if err != nil {
		return fmt.Errorf("monitor: settings: %w", err)
	}
	if !ok {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		slog.Warn("monitor: bad journal.seen_at", "value", v)
		return nil
	}
	m.seenAt = &t
	return nil
}

// refreshNewEvents recounts the warnings since seenAt. It runs only when the
// count can change (a warning logged, a gap opened or closed, the journal
// seen), never per tick. Caller holds m.mu.
func (m *Monitor) refreshNewEvents() {
	m.journalReads++
	now := m.clock.Now()
	var from, seen time.Time // never seen: everything kept counts
	if m.seenAt != nil {
		from, seen = *m.seenAt, *m.seenAt
	}
	es, err := m.readJournal(m.station.ID, from, now.Add(time.Minute), now, false)
	if err != nil {
		slog.Warn("monitor: journal", "err", err)
		return
	}
	m.newEvents = journal.NewSince(es, seen)
}

// SettingMaxDelta holds the largest SOC drop (points) of any discharge run
// seen so far (estimateCapacity), for «найбільший розряд поки N п.п.».
const SettingMaxDelta = "health.max_dsoc"

// Battery builds the "Battery" tab: the stored capacity estimates, the cycles
// from the lifetime discharged energy and the largest discharge so far.
func (m *Monitor) Battery() (health.Summary, error) {
	m.mu.Lock()
	id, p := m.station.ID, m.profile
	m.mu.Unlock()
	es, err := m.d.Store.CapacityEstimates(id)
	if err != nil {
		return health.Summary{}, err
	}
	// An unreadable lifetime counter (store.SettingDischargedPrunedWh) hides
	// only the cycles: the rest of the tab still loads. Prune stays stopped
	// (fail-closed) until it is fixed.
	dis, err := m.d.Store.TotalDischargedWh(id)
	cyclesOK := err == nil
	if err != nil {
		slog.Warn("monitor: lifetime discharge unreadable, cycles hidden", "err", err)
	}
	s := health.Summarize(es, dis, m.storedMaxDelta(), p)
	if !cyclesOK {
		s.Cycles = nil
	}
	return s, nil
}

// storedMaxDelta reads SettingMaxDelta; 0 when unset or unreadable (the next
// capacity pass writes it again).
func (m *Monitor) storedMaxDelta() float64 {
	v, ok, err := m.d.Store.GetSetting(SettingMaxDelta)
	if err != nil || !ok {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || f < 0 {
		return 0
	}
	return f
}
