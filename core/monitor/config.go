package monitor

import (
	"maps"
	"strings"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/store"
)

// Config is the part of the user's settings the monitor applies live.
// Poll is the Active interval (Calm is max(CalmPoll, Poll)); 0 keeps the
// current one. StationMinSOC overrides the profile's MinSOC (0 = the model's
// own, settings.ApplyProfile). RetentionDays is kept for pruning (0 = keep
// forever).
type Config struct {
	Notify        notify.Settings
	Poll          time.Duration
	RetentionDays int
	StationMinSOC int
}

// ConfigFor picks the monitor's part out of the user's settings.
func ConfigFor(s settings.Settings) Config {
	return Config{Notify: s.Notify, Poll: s.PollInterval, RetentionDays: s.RetentionDays, StationMinSOC: s.StationMinSOC}
}

// profileFor is the model's profile with the user's minimum charge.
func profileFor(stationModel string, minSOC int) model.Profile {
	return settings.Settings{StationMinSOC: minSOC}.ApplyProfile(model.ProfileFor(stationModel))
}

// Configure applies changed settings without a restart. The next poll is
// due at the new interval from the last one (calls stay ≥ minCallGap apart,
// see nextAction); a failing link keeps its backoff count, now counted from
// the new interval. Freshness moves to 3 × the new interval. The policy keeps
// its re-arm, flapping and DND state. Any goroutine: everything happens
// under m.mu, the lock the poll loop holds while it uses the policy.
func (m *Monitor) Configure(c Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	if c.Poll > 0 {
		m.poll = c.Poll
	}
	if !m.pollStart.IsZero() {
		if m.fails > 0 {
			m.nextPoll = m.pollStart.Add(m.backoff(m.fails))
		} else {
			m.nextPoll = m.pollStart.Add(m.pollInterval())
		}
	}
	m.machine.SetFresh(freshFor(m.pollInterval()))

	c.Notify.Enabled = maps.Clone(c.Notify.Enabled)
	m.d.Notify = c.Notify // SetDND reads the morning end from it
	m.policy.SetSettings(c.Notify)
	m.d.RetentionDays = c.RetentionDays

	m.d.StationMinSOC = c.StationMinSOC
	m.profile = profileFor(m.station.Model, c.StationMinSOC)
	m.machine.SetProfile(m.profile)

	m.publish(m.snapshot(now))
}

// RetentionBound is the oldest time kept with a retention of days (> 0):
// the local midnight that many local days before today, so the oldest day
// kept is whole in the History and Energy views (a 23 or 25 h day counts as
// one).
func RetentionBound(now time.Time, days int) time.Time {
	return localMidnight(now).AddDate(0, 0, -days)
}

// pruneJob is what a prune needs: read under m.mu, run outside it.
type pruneJob struct {
	station string
	days    int // RetentionDays; 0 = keep forever
}

// pruneArgs is the prune of the current retention. Caller holds m.mu.
func (m *Monitor) pruneArgs() pruneJob {
	return pruneJob{station: m.station.ID, days: m.d.RetentionDays}
}

// run deletes the station's data older than the retention
// (store.Prune); nothing with days 0. It reports whether a prune ran.
// Store work: not under m.mu.
func (p pruneJob) run(st *store.Store, now time.Time) (bool, error) {
	if p.days <= 0 {
		return false, nil
	}
	return true, st.Prune(p.station, RetentionBound(now, p.days))
}

// PruneNow applies the current retention at once (the user confirmed a
// shorter one; Configure first) instead of at the next daily maintenance.
// The store work runs outside m.mu (Ruling 39), after any daily
// maintenance in progress (maintMu); then the battery figures and the
// journal's new-event count are re-read and the snapshot published.
func (m *Monitor) PruneNow() error {
	m.maintMu.Lock() // not in the middle of the daily maintenance
	defer m.maintMu.Unlock()
	m.mu.Lock()
	p := m.pruneArgs()
	m.mu.Unlock()
	pruned, err := p.run(m.d.Store, m.clock.Now())
	if err != nil {
		return err
	}
	if pruned {
		m.refreshBattery(m.clock.Now()) // estimates may be gone
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshNewEvents()
	m.publish(m.snapshot(m.clock.Now()))
	return nil
}

// RenameStation sets the user's name for the station (trimmed, checked by
// settings.ValidName) and publishes it.
func (m *Monitor) RenameStation(name string) error {
	if err := settings.ValidName(name); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.d.Store.RenameStation(m.station.ID, name); err != nil {
		return err
	}
	m.station.Name = name
	m.publish(m.snapshot(m.clock.Now()))
	return nil
}
