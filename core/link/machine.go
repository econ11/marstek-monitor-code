// Package link is the per-station connection state machine (flows §2, v1 LAN-only).
//
// The machine is pure and deterministic: time enters only through the `now`
// parameters, it starts no goroutines or timers, and it returns effects for the
// caller to execute instead of performing them.
package link

import (
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	"github.com/econ11/marstek-monitor-code/core/store"
)

type State string

const (
	Connecting State = "CONNECTING"
	Live       State = "LIVE"
	Stale      State = "STALE"
	NoLink     State = "NO_LINK"
	LikelyDead State = "LIKELY_DEAD"
	Resyncing  State = "RESYNCING"
	Suspended  State = "SUSPENDED"
)

// Diagnosis explains NO_LINK (flows §2.2). v1 has no cloud, so only two exist.
type Diagnosis string

const (
	DiagPCOffline Diagnosis = "pc_offline"
	DiagLANSilent Diagnosis = "lan_silent"
)

// Config holds the flows §2.1 thresholds.
type Config struct {
	Fresh, StaleToNoLink, ConnectTimeout, BootConnectTimeout, ResyncMax time.Duration
}

func DefaultConfig() Config {
	return Config{Fresh: 15 * time.Second, StaleToNoLink: 5 * time.Minute, ConnectTimeout: 30 * time.Second,
		BootConnectTimeout: 90 * time.Second, ResyncMax: 30 * time.Second}
}

type EffectKind int

const (
	// OpenGap: data stopped at At (the last good sample) for Reason.
	OpenGap EffectKind = iota
	// CloseGap: the open gap ended at At (the first new fresh sample).
	CloseGap
	// LogDelay: a sub-gap interruption (STALE → LIVE); data stopped at At, resumed now.
	LogDelay
	// EnteredLikelyDead: station probably ran out; At = estimated station_died_at.
	// Also emitted retroactively at startup (flows §2.3), while State stays CONNECTING.
	EnteredLikelyDead
	// StateChanged: From → To at At.
	StateChanged
	// LogStartDelay: the first sample after start / resume follows a hole
	// longer than Fresh that is not a gap (a reboot or restart under 5 min).
	// At = the last sample before the hole; data resumed now.
	LogStartDelay
)

type Effect struct {
	Kind     EffectKind
	At       time.Time
	Reason   store.GapReason
	From, To State
}

type Machine struct {
	cfg     Config
	profile model.Profile
	state   State
	diag    Diagnosis
	since   time.Time     // when the current state was entered
	timeout time.Duration // CONNECTING timeout for the current CONNECTING episode
	last    *model.Sample // last fresh sample (or the stored one at startup)
	gapOpen bool          // an OpenGap was emitted and not yet closed
	dead    bool          // EnteredLikelyDead was emitted for the open gap
	first   bool          // no sample accepted yet since start / resume
	pending []Effect      // startup effects, emitted on the first call
}

// NewMachine starts in CONNECTING. last is the newest stored sample (nil if
// none); boot selects the longer post-Windows-start connect timeout. If last
// is older than StaleToNoLink, the gap since then is opened (reason
// app_not_running, or likely_dead when DeadCondition holds on last) and
// returned by the first Observe/Tick/Suspend/Resume.
func NewMachine(cfg Config, last *model.Sample, profile model.Profile, now time.Time, boot bool) *Machine {
	m := &Machine{cfg: cfg, profile: profile, state: Connecting, since: now, timeout: cfg.ConnectTimeout, first: true}
	if boot {
		m.timeout = cfg.BootConnectTimeout
	}
	if last != nil {
		l := *last
		m.last = &l
		m.clampLast(now)
		if now.Sub(m.last.At) > cfg.StaleToNoLink {
			reason := store.GapAppNotRunning
			dead := DeadCondition(*m.last, profile, emptyAt(*m.last, profile), now)
			if dead {
				reason = store.GapLikelyDead
			}
			m.pending = append(m.pending, m.openGap(reason))
			if dead {
				m.pending = m.enterDead(m.pending, now)
			}
		}
	}
	return m
}

func (m *Machine) State() State { return m.state }

// SetFresh sets how old the last sample may be and still count as fresh: 3 ×
// the current poll interval, which changes with the poll cadence. STALE →
// NO_LINK still follows StaleToNoLink after that.
func (m *Machine) SetFresh(d time.Duration) { m.cfg.Fresh = d }

// SetProfile replaces the station profile (the user changed its minimum
// charge); the likely-dead condition uses it from the next check on.
func (m *Machine) SetProfile(p model.Profile) { m.profile = p }

// Profile is the station profile in use.
func (m *Machine) Profile() model.Profile { return m.profile }

// Diagnosis is set only in NO_LINK; otherwise "".
func (m *Machine) Diagnosis() Diagnosis {
	if m.state != NoLink {
		return ""
	}
	return m.diag
}

// Observe feeds a sample. Samples that are not fresh at now, or older than the
// last one seen, are ignored.
func (m *Machine) Observe(s model.Sample, now time.Time) []Effect {
	effs := m.drain()
	m.clampLast(now)
	if m.state == Suspended || now.Sub(s.At) > m.cfg.Fresh || (m.last != nil && s.At.Before(m.last.At)) {
		return effs
	}
	prev, first := m.last, m.first
	m.last, m.first = &s, false
	if m.state == Live || m.state == Resyncing {
		return effs
	}
	// CONNECTING, STALE, NO_LINK, LIKELY_DEAD: data is back.
	if m.gapOpen {
		m.gapOpen, m.dead = false, false
		effs = append(effs, Effect{Kind: CloseGap, At: s.At})
		return m.to(effs, Resyncing, now)
	}
	if prev != nil {
		switch {
		case first && s.At.Sub(prev.At) > m.cfg.Fresh:
			// The hole spans the time the app was not running.
			effs = append(effs, Effect{Kind: LogStartDelay, At: prev.At})
		case m.state == Stale:
			effs = append(effs, Effect{Kind: LogDelay, At: prev.At})
		}
	}
	return m.to(effs, Live, now)
}

// Tick advances time-driven transitions; pcOnline feeds the NO_LINK diagnosis.
func (m *Machine) Tick(now time.Time, pcOnline bool) []Effect {
	effs := m.drain()
	m.clampLast(now)
	if pcOnline {
		m.diag = DiagLANSilent
	} else {
		m.diag = DiagPCOffline
	}
	switch m.state {
	case Connecting:
		if now.Sub(m.since) < m.timeout {
			return effs
		}
		if m.last == nil {
			return m.to(effs, NoLink, now)
		}
		if m.age(now) <= m.cfg.Fresh+m.cfg.StaleToNoLink {
			return m.to(effs, Stale, now)
		}
		return m.lose(effs, now)
	case Live, Resyncing:
		if m.age(now) > m.cfg.Fresh {
			return m.to(effs, Stale, now)
		}
		if m.state == Resyncing && now.Sub(m.since) >= m.cfg.ResyncMax {
			return m.to(effs, Live, now)
		}
	case Stale:
		if m.age(now) > m.cfg.Fresh+m.cfg.StaleToNoLink {
			return m.lose(effs, now)
		}
	case NoLink:
		if m.last != nil && DeadCondition(*m.last, m.profile, emptyAt(*m.last, m.profile), now) {
			return m.to(m.enterDead(effs, now), LikelyDead, now)
		}
	}
	return effs
}

// Suspend: the PC is going to sleep. Opens a pc_sleep gap unless one is open.
func (m *Machine) Suspend(now time.Time) []Effect {
	effs := m.drain()
	m.clampLast(now)
	if m.state == Suspended {
		return effs
	}
	if !m.gapOpen && m.last != nil {
		effs = append(effs, m.openGap(store.GapPCSleep))
	}
	return m.to(effs, Suspended, now)
}

// Resume: the PC woke up; reconnect with the normal (non-boot) timeout.
func (m *Machine) Resume(now time.Time) []Effect {
	effs := m.drain()
	if m.state != Suspended {
		return effs
	}
	m.timeout, m.first = m.cfg.ConnectTimeout, true
	return m.to(effs, Connecting, now)
}

// DeadCondition is flows §2.3: grid absent in last && (SOC ≤ MinSOC+5 || emptyAt ≤ now).
// The "output cut off by protection" clause is not exposed by the v1 API.
func DeadCondition(last model.Sample, profile model.Profile, emptyAt *time.Time, now time.Time) bool {
	if last.Grid != model.GridAbsent {
		return false
	}
	if last.SOC != nil && *last.SOC <= profile.MinSOC+5 {
		return true
	}
	return emptyAt != nil && !emptyAt.After(now)
}

// emptyAt forecasts when s's discharge reaches MinSOC ("До розряду"), or nil
// when unknowable (not discharging, unknown SOC/power or capacity).
func emptyAt(s model.Sample, p model.Profile) *time.Time {
	if s.SOC == nil || s.BatteryW == nil || *s.BatteryW >= 0 || p.NominalWh <= 0 {
		return nil
	}
	leftWh := (*s.SOC - p.MinSOC) / 100 * p.NominalWh
	if leftWh < 0 {
		leftWh = 0
	}
	t := s.At.Add(time.Duration(leftWh / -*s.BatteryW * float64(time.Hour)))
	return &t
}

// lose handles data loss past StaleToNoLink: NO_LINK or LIKELY_DEAD, opening
// the gap from the last sample unless one is already open.
func (m *Machine) lose(effs []Effect, now time.Time) []Effect {
	dead := DeadCondition(*m.last, m.profile, emptyAt(*m.last, m.profile), now)
	if !m.gapOpen {
		reason := store.GapNoLink
		if dead {
			reason = store.GapLikelyDead
		}
		effs = append(effs, m.openGap(reason))
	}
	if dead {
		return m.to(m.enterDead(effs, now), LikelyDead, now)
	}
	return m.to(effs, NoLink, now)
}

// enterDead emits EnteredLikelyDead once per open gap (reset by CloseGap).
func (m *Machine) enterDead(effs []Effect, now time.Time) []Effect {
	if m.dead {
		return effs
	}
	m.dead = true
	return append(effs, Effect{Kind: EnteredLikelyDead, At: m.deathAt(now)})
}

// clampLast handles a wall clock that went backwards (e.g. RTC drift or an NTP
// correction across a restart): a last sample later than now is moved to now,
// so new fresh samples are not rejected as out-of-order and data age (and any
// gap opened from last) never lies in the future.
func (m *Machine) clampLast(now time.Time) {
	if m.last != nil && m.last.At.After(now) {
		l := *m.last
		l.At = now
		m.last = &l
	}
}

func (m *Machine) openGap(reason store.GapReason) Effect {
	m.gapOpen = true
	return Effect{Kind: OpenGap, At: m.last.At, Reason: reason}
}

// deathAt estimates station_died_at: min(last + forecast, now), else the last sample.
func (m *Machine) deathAt(now time.Time) time.Time {
	if e := emptyAt(*m.last, m.profile); e != nil {
		if e.After(now) {
			return now
		}
		return *e
	}
	return m.last.At
}

func (m *Machine) age(now time.Time) time.Duration { return now.Sub(m.last.At) }

func (m *Machine) to(effs []Effect, s State, now time.Time) []Effect {
	if s == m.state {
		return effs
	}
	effs = append(effs, Effect{Kind: StateChanged, At: now, From: m.state, To: s})
	m.state, m.since = s, now
	return effs
}

func (m *Machine) drain() []Effect {
	effs := m.pending
	m.pending = nil
	return effs
}
