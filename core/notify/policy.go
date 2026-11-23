// Package notify decides which toasts to send, when, and whether they pop up
// or land silently in the notification centre (flows spec 2.7 and 5.3).
// Pure logic: time comes only from the `now` parameters, no goroutines, not
// safe for concurrent use (the caller serializes).
package notify

import (
	"maps"
	"strconv"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/model"
)

// Kind is a toast category.
type Kind string

const (
	GridLost     Kind = "grid_lost"
	GridBack     Kind = "grid_back"
	GridFlapping Kind = "grid_flapping"
	LowCharge    Kind = "low_charge"
	Full         Kind = "full"
	LikelyDead   Kind = "likely_dead"
	// NoLink: the station has not answered for Settings.NoLinkAfter (off by
	// default: in an outage the router may lose power too).
	NoLink Kind = "no_link"
	// NoLinkRestored replaces a NoLink toast (same Tag) once data is back.
	// It has no switch of its own: it follows NoLink.
	NoLinkRestored Kind = "no_link_restored"
	// Test is sent only by the Settings button, never by the policy; it has
	// no switch and ignores quiet hours and "do not disturb".
	Test Kind = "test"
	// Update: a newer version of the app is out (plan 3, Task 4). It has no
	// switch: it is shown whenever the update check finds one, once per
	// version, but quiet hours and "do not disturb" silence it (UpdateToast).
	Update Kind = "update"
)

// Kinds lists every kind with an on/off switch (settings accept Enabled
// entries only for these); a task adding a switchable kind adds it here.
// NoLinkRestored, Test and Update have no switch (see their comments). A stored
// grid_flapping entry is accepted but has no effect: grid_flapping follows
// grid_lost.
var Kinds = []Kind{GridLost, GridBack, GridFlapping, LowCharge, Full, LikelyDead, NoLink}

const (
	// maxEventAge: toasts only come from live data, an event older than this
	// is catch-up and must not pop (flows 2.7, D5).
	maxEventAge = 2 * time.Minute
	// linkQuiet: no link toasts for this long after start / resume, so Wi-Fi
	// still coming up is not reported as "no link".
	linkQuiet = 2 * time.Minute
	// flapWindow and flapCount: this many grid transitions within the window
	// collapse into one grid_flapping toast; stability for the same window
	// ends the flapping state.
	flapWindow = 10 * time.Minute
	flapCount  = 3
	// lowRearmMargin: low charge re-arms at SOC >= threshold + 5 points.
	lowRearmMargin = 5.0
	// fullSOC / fullRearmSOC: "full" fires at 100 and re-arms below 95.
	fullSOC        = 100.0
	fullRearmSOC   = 95.0
	defaultMorning = "07:00"
)

// DNDManualUntil is the sentinel for "until I turn it on again": a date so far
// away it never expires in practice.
var DNDManualUntil = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// Toast is one notification to show. StationID is filled for sample-driven
// toasts; OnPower / OnLikelyDead receive no station id, so the caller (which
// owns the per-station policy) sets it. Tag is the outage id for
// grid_lost/grid_back so the second replaces the first in the notification
// centre.
type Toast struct {
	Kind      Kind
	StationID string
	Tag       string
	At        time.Time
	Silent    bool
	Args      map[string]string
}

// Settings configures the policy. Quiet times are "HH:MM" local wall-clock;
// end <= start means the interval crosses midnight. A nil Enabled map or a
// missing key means the kind is enabled. NoLinkAfter is how long the link
// must be down before a no_link toast.
type Settings struct {
	LowThreshold float64
	QuietEnabled bool
	QuietStart   string
	QuietEnd     string
	NoLinkAfter  time.Duration
	Enabled      map[Kind]bool
}

type lastGrid struct {
	known bool
	kind  Kind
	tag   string
	at    time.Time
	args  map[string]string
}

// Policy holds the re-arm / flapping / DND state for one station.
type Policy struct {
	s        Settings
	quietEnd time.Time // link-toast quiet period ends here (start / resume + 2 min)
	dnd      *time.Time

	lowArmed  bool
	fullArmed bool
	// seeded: a reading has been seen since start / resume. The first one
	// cannot show the charge REACHING 100%, so it only disarms "full".
	seeded bool

	transitions []time.Time
	flapping    bool
	last        lastGrid // last accepted grid transition

	// The current no-link episode (OnLink): open while no new data has
	// arrived since it began. linkSince makes the Tag, linkLast is the last
	// data before it, linkSent says the NoLink toast went out.
	linkOpen  bool
	linkSince time.Time
	linkLast  time.Time
	linkSent  bool

	// crossings: thresholds crossed since the last Crossings call.
	crossings []Kind
}

// NewPolicy creates a policy; link toasts are held back for 2 min after startedAt.
func NewPolicy(s Settings, startedAt time.Time) *Policy {
	return &Policy{s: s, quietEnd: startedAt.Add(linkQuiet), lowArmed: true, fullArmed: true}
}

// SetSettings replaces the settings. Re-arm, flapping, DND and quiet-period
// state is kept: changing a setting is not a restart.
// Enabled is cloned so the caller's later map edits do not leak in.
func (p *Policy) SetSettings(s Settings) {
	s.Enabled = maps.Clone(s.Enabled)
	p.s = s
}

// SetDND sets "do not disturb" until the given time; nil turns it off.
func (p *Policy) SetDND(until *time.Time) {
	if until == nil {
		p.dnd = nil
		return
	}
	u := *until
	p.dnd = &u
}

// DNDUntil returns the active DND deadline or nil.
func (p *Policy) DNDUntil() *time.Time {
	if p.dnd == nil {
		return nil
	}
	u := *p.dnd
	return &u
}

// OnResume starts the 2 min link-toast quiet period after system resume. The
// next reading is again a first one: what happened during sleep was not seen.
func (p *Policy) OnResume(now time.Time) {
	p.quietEnd = now.Add(linkQuiet)
	p.seeded = false
}

// DNDOption maps a tray menu option to a DND deadline. "morning" is the end of
// quiet hours when enabled, else the next 07:00. "manual" is DNDManualUntil.
// Unknown options return nil.
func DNDOption(opt string, now time.Time, s Settings) *time.Time {
	var t time.Time
	switch opt {
	case "1h":
		t = now.Add(time.Hour)
	case "3h":
		t = now.Add(3 * time.Hour)
	case "8h":
		t = now.Add(8 * time.Hour)
	case "morning":
		hhmm := defaultMorning
		if s.QuietEnabled {
			if _, _, ok := parseHHMM(s.QuietEnd); ok {
				hhmm = s.QuietEnd
			}
		}
		h, m, _ := parseHHMM(hhmm)
		t = time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
	case "manual":
		t = DNDManualUntil
	default:
		return nil
	}
	return &t
}

// OnPower handles a confirmed grid event. outageID becomes the toast Tag.
// If flapping had expired unnoticed, the pending grid_lost is emitted first.
// The 2 min rule is measured from ev.ConfirmedAt (ev.At when it is not set):
// a change first seen just before the station went silent and confirmed
// minutes later is still a live observation. The toast keeps ev.At.
// ev.AcrossGap: GridBack is not toasted (only remembered), GridLost carries
// Args["approx"] = "1", Args["to"] = ev.At and, when known, Args["from"] =
// ev.GapFrom (both RFC 3339 in now's zone): the grid went off between them.
func (p *Policy) OnPower(ev detect.PowerEvent, outageID int64, live bool, now time.Time) []Toast {
	args := map[string]string{}
	if ev.SOC != nil {
		args["soc"] = strconv.FormatFloat(*ev.SOC, 'f', 0, 64)
	}
	k := GridLost
	if ev.Kind == detect.GridBack {
		k = GridBack
	}
	tag := strconv.FormatInt(outageID, 10)
	seen := ev.ConfirmedAt
	if seen.IsZero() {
		seen = ev.At
	}
	if ev.AcrossGap && k == GridLost {
		// The grid was not observed up to the change: the start is approximate.
		// It lies between the last observation before the gap and the first
		// observation of the outage (RFC 3339, so the toast can name the day).
		args["approx"] = "1"
		args["to"] = ev.At.In(now.Location()).Format(time.RFC3339)
		if !ev.GapFrom.IsZero() {
			args["from"] = ev.GapFrom.In(now.Location()).Format(time.RFC3339)
		}
	}
	// GridBack across a data gap: when the grid returned is unknown, so it is
	// not toasted (global constraints), but the state is remembered below.
	if !live || now.Sub(seen) >= maxEventAge || ev.AcrossGap && k == GridBack {
		// No toast, but the true grid state must still be remembered, or a
		// later flapping expiry could report an outage that already ended.
		if !p.last.known || !ev.At.Before(p.last.at) {
			p.last = lastGrid{known: true, kind: k, tag: tag, at: ev.At, args: args}
		}
		return nil
	}
	out := p.expireFlapping(now)

	kept := p.transitions[:0]
	for _, t := range p.transitions {
		if ev.At.Sub(t) < flapWindow {
			kept = append(kept, t)
		}
	}
	p.transitions = append(kept, ev.At)
	p.last = lastGrid{known: true, kind: k, tag: tag, at: ev.At, args: args}

	// If the flapping toast is disabled, do not collapse: otherwise real
	// lost/back toasts would vanish with nothing replacing them.
	if p.flappingEnabled() {
		if p.flapping {
			return out
		}
		if len(p.transitions) >= flapCount {
			p.flapping = true
			return append(out, p.emit(Toast{Kind: GridFlapping, At: ev.At, Args: args}, now)...)
		}
	}
	return append(out, p.emit(Toast{Kind: k, Tag: tag, At: ev.At, Args: args}, now)...)
}

// Tick expires flapping by time alone, without waiting for a grid event. If
// the grid is still off once the 10 min of stability have passed, the
// suppressed outage is reported now as one grid_lost with the outage's Tag.
// Exception to the 2 min age rule: At is the time of that last transition and
// the toast is deliberately not rejected as old, because it is the stable
// state being reported, not catch-up. Callers should tick only while the link
// is live. Task 10 calls it about once a second.
func (p *Policy) Tick(now time.Time) []Toast { return p.expireFlapping(now) }

// flappingEnabled: grid_flapping is on exactly when grid_lost is.
func (p *Policy) flappingEnabled() bool { return p.enabled(GridFlapping) }

// enabled applies the Enabled map. Kinds without a switch of their own
// follow another one: grid_flapping follows grid_lost, no_link_restored
// follows no_link. A missing entry means on.
func (p *Policy) enabled(k Kind) bool {
	switch k {
	case GridFlapping:
		k = GridLost
	case NoLinkRestored:
		k = NoLink
	}
	on, ok := p.s.Enabled[k]
	return !ok || on
}

// OnLink handles the no-connection toast; call it on every tick. noLink:
// the link is NO_LINK or LIKELY_DEAD; noLinkSince: when that state was
// entered; lastAt: the time of the newest sample (zero if none); soc: its
// charge. now is local time.
//
// NoLink is sent once per episode, when now − noLinkSince ≥ NoLinkAfter and
// the 2 min after start / resume are over. noLinkSince is this call's: after
// a sleep the monitor re-enters NO_LINK, so only awake time without a link
// counts. Tag "link-"+unix(noLinkSince of the episode's first call); Args
// duration (now − lastAt, a Go duration string), time (lastAt, "15:04") and
// lastAt (RFC 3339, so the toast can name the day). A zero noLinkSince is
// ignored. An episode lasts until new data arrives (lastAt moves past the
// last data before it), so suspend and reconnecting do not end it. Then
// NoLinkRestored is sent with the same Tag, only if NoLink was: Args
// duration (time without data) and soc.
func (p *Policy) OnLink(noLink bool, noLinkSince, lastAt time.Time, soc *float64, now time.Time) []Toast {
	if noLink && noLinkSince.IsZero() {
		return nil // the monitor has not seen NO_LINK begin
	}
	if noLink {
		if !p.linkOpen || lastAt.After(p.linkLast) {
			p.linkOpen, p.linkSince, p.linkLast, p.linkSent = true, noLinkSince, lastAt, false
		}
		// NoLinkAfter 0 (unset) is off: there is no threshold to report.
		if p.linkSent || p.s.NoLinkAfter <= 0 || now.Sub(noLinkSince) < p.s.NoLinkAfter || now.Before(p.quietEnd) {
			return nil
		}
		args := map[string]string{}
		if !lastAt.IsZero() {
			args["duration"] = now.Sub(lastAt).Truncate(time.Second).String()
			args["time"] = lastAt.In(now.Location()).Format("15:04")
			args["lastAt"] = lastAt.In(now.Location()).Format(time.RFC3339)
		}
		out := p.emit(Toast{Kind: NoLink, Tag: p.linkTag(), At: now, Args: args}, now)
		p.linkSent = len(out) > 0
		return out
	}
	if !p.linkOpen || !lastAt.After(p.linkLast) {
		return nil // no episode, or no new data yet
	}
	p.linkOpen = false
	if !p.linkSent {
		return nil
	}
	p.linkSent = false
	args := map[string]string{}
	if !p.linkLast.IsZero() {
		args["duration"] = lastAt.Sub(p.linkLast).Truncate(time.Second).String()
	}
	if soc != nil {
		args["soc"] = strconv.FormatFloat(*soc, 'f', 0, 64)
	}
	return p.emit(Toast{Kind: NoLinkRestored, Tag: p.linkTag(), At: now, Args: args}, now)
}

func (p *Policy) linkTag() string { return "link-" + strconv.FormatInt(p.linkSince.Unix(), 10) }

func (p *Policy) expireFlapping(now time.Time) []Toast {
	if !p.flapping || !p.last.known || now.Sub(p.last.at) < flapWindow {
		return nil
	}
	p.flapping = false
	p.transitions = nil
	if p.last.kind != GridLost {
		return nil
	}
	return p.emit(Toast{Kind: GridLost, Tag: p.last.tag, At: p.last.at, Args: p.last.args}, now)
}

// OnSample produces low_charge (only while discharging, with hysteresis) and
// full (with re-arm below 95%). Re-arming happens even when not live. The
// first reading after start / resume never fires "full": a station that
// already sits at 100% did not reach it while we were watching.
func (p *Policy) OnSample(s model.Sample, mode detect.BatteryMode, live bool, now time.Time) []Toast {
	var out []Toast
	if live {
		out = p.expireFlapping(now)
	}
	if s.SOC == nil {
		return out
	}
	soc := *s.SOC
	if soc >= p.s.LowThreshold+lowRearmMargin {
		p.lowArmed = true
	}
	if soc < fullRearmSOC {
		p.fullArmed = true
	}
	if !p.seeded {
		p.seeded = true
		if soc >= fullSOC {
			p.fullArmed = false // already full: nothing was reached
		}
	}
	if !live || now.Sub(s.At) >= maxEventAge {
		return out
	}
	if p.lowArmed && mode == detect.Discharging && soc < p.s.LowThreshold {
		p.lowArmed = false
		p.crossings = append(p.crossings, LowCharge)
		out = append(out, p.emit(Toast{
			Kind: LowCharge, StationID: s.StationID, At: s.At,
			Args: map[string]string{
				"soc":       strconv.FormatFloat(soc, 'f', 0, 64),
				"threshold": strconv.FormatFloat(p.s.LowThreshold, 'f', 0, 64),
			},
		}, now)...)
	}
	if p.fullArmed && soc >= fullSOC {
		p.fullArmed = false
		out = append(out, p.emit(Toast{Kind: Full, StationID: s.StationID, At: s.At}, now)...)
	}
	return out
}

// Crossings returns the thresholds crossed since the last call (low_charge,
// by the same live, discharging, re-armed rule as its toast) whether or not
// their toast is switched on, and forgets them. The journal logs these.
func (p *Policy) Crossings() []Kind {
	c := p.crossings
	p.crossings = nil
	return c
}

// OnLikelyDead toasts only when the condition was detected live (flows 11 Q4)
// and not within 2 min of start / resume.
func (p *Policy) OnLikelyDead(at time.Time, socBefore *float64, detectedLive bool, now time.Time) []Toast {
	if !detectedLive || now.Before(p.quietEnd) {
		return nil
	}
	args := map[string]string{}
	if socBefore != nil {
		args["soc"] = strconv.FormatFloat(*socBefore, 'f', 0, 64)
	}
	return p.emit(Toast{Kind: LikelyDead, At: at, Args: args}, now)
}

// emit applies the Enabled map and the silent rule. Low charge always pops.
func (p *Policy) emit(t Toast, now time.Time) []Toast {
	if !p.enabled(t.Kind) {
		return nil
	}
	t.Silent = t.Kind != LowCharge && (p.dndActive(now) || p.inQuiet(now))
	return []Toast{t}
}

func (p *Policy) dndActive(now time.Time) bool { return p.dnd != nil && now.Before(*p.dnd) }

// inQuiet uses the wall-clock of now's own location; the caller passes local time.
func (p *Policy) inQuiet(now time.Time) bool { return inQuiet(p.s, now) }

func inQuiet(s Settings, now time.Time) bool {
	if !s.QuietEnabled {
		return false
	}
	sh, sm, ok1 := parseHHMM(s.QuietStart)
	eh, em, ok2 := parseHHMM(s.QuietEnd)
	if !ok1 || !ok2 {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	start, end := sh*60+sm, eh*60+em
	if start < end {
		return cur >= start && cur < end
	}
	return cur >= start || cur < end // crosses midnight (end <= start)
}

func parseHHMM(s string) (h, m int, ok bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// UpdateToast is the "version is available" toast (plan 3, Task 4): Args
// version. It is not a policy event (the update check runs without a
// station), so the caller passes the settings and the DND deadline; Enabled
// does not apply (no switch), quiet hours and an active DND make it silent.
func UpdateToast(version string, s Settings, dnd *time.Time, now time.Time) Toast {
	silent := dnd != nil && now.Before(*dnd) || inQuiet(s, now)
	return Toast{Kind: Update, At: now, Silent: silent, Args: map[string]string{"version": version}}
}
