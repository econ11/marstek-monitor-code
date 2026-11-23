package monitor

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/model"
)

func fp(v float64) *float64 { return &v }

func calmInput() cadenceInput {
	return cadenceInput{Live: true, Grid: detect.GridOn, Mode: detect.Idle,
		SOCSpan: 0, SOCKnown: true, SinceStart: 3 * time.Minute,
		BatteryW: fp(0), LastGrid: model.GridPresent}
}

func TestCadenceRules(t *testing.T) {
	cases := []struct {
		name string
		edit func(*cadenceInput)
		want Cadence
	}{
		{"all calm conditions", func(*cadenceInput) {}, Calm},
		{"SOC span exactly 1 p.p.", func(in *cadenceInput) { in.SOCSpan = 1 }, Calm},
		{"warmup just over", func(in *cadenceInput) { in.SinceStart = ActiveWarmup }, Calm},
		{"grid off", func(in *cadenceInput) { in.Grid = detect.GridOff }, Active},
		{"grid unknown", func(in *cadenceInput) { in.Grid = detect.GridUnknownState }, Active},
		{"charging", func(in *cadenceInput) { in.Mode = detect.Charging; in.BatteryW = fp(400) }, Active},
		{"discharging", func(in *cadenceInput) { in.Mode = detect.Discharging; in.BatteryW = fp(-400) }, Active},
		{"cutoff", func(in *cadenceInput) { in.Mode = detect.Cutoff }, Active},
		{"mode unknown", func(in *cadenceInput) { in.Mode = detect.ModeUnknown; in.BatteryW = nil }, Active},
		{"pending grid change", func(in *cadenceInput) { in.PendingGrid = true }, Active},
		{"pending mode change", func(in *cadenceInput) { in.PendingMode = true }, Active},
		{"SOC span 1.5 p.p.", func(in *cadenceInput) { in.SOCSpan = 1.5 }, Active},
		{"SOC unknown", func(in *cadenceInput) { in.SOCKnown = false }, Active},
		{"in warmup (1 min)", func(in *cadenceInput) { in.SinceStart = time.Minute }, Active},
		{"link not live", func(in *cadenceInput) { in.Live = false }, Active},
		{"power above the deadband", func(in *cadenceInput) { in.BatteryW = fp(-21) }, Active},
		{"power unknown", func(in *cadenceInput) { in.BatteryW = nil }, Active},
		{"last sample shows grid absent", func(in *cadenceInput) { in.LastGrid = model.GridAbsent }, Active},
		{"last sample grid unknown", func(in *cadenceInput) { in.LastGrid = model.GridUnknown }, Active},
	}
	for _, c := range cases {
		in := calmInput()
		c.edit(&in)
		if got := cadenceFor(in); got != c.want {
			t.Errorf("%s: cadence = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPollIntervalCalmNeverFasterThanActive(t *testing.T) {
	for _, c := range []struct{ poll, active, calm time.Duration }{
		{5 * time.Second, 5 * time.Second, 20 * time.Second},
		{10 * time.Second, 10 * time.Second, 20 * time.Second},
		{30 * time.Second, 30 * time.Second, 30 * time.Second},
	} {
		st := openStore(t)
		seedStation(t, st, "127.0.0.1")
		d := deps(st, newClient(t), nil, newClock(t0))
		d.PollInterval = c.poll
		m, err := New(d)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.pollInterval(); got != c.active {
			t.Errorf("poll %v, Active: %v, want %v", c.poll, got, c.active)
		}
		m.cadence = Calm
		if got := m.pollInterval(); got != c.calm {
			t.Errorf("poll %v, Calm: %v, want %v", c.poll, got, c.calm)
		}
	}
}

// At the floor with no power flowing the sample's grid is unknown (Ruling 12),
// yet nothing changes while the grid stays confirmed on: Calm.
func TestCadenceCalmAtTheFloorWithGridOn(t *testing.T) {
	cases := []struct {
		name string
		edit func(*cadenceInput)
		want Cadence
	}{
		{"idle at MinSOC+1, floor-unknown grid", func(in *cadenceInput) {
			in.LastGrid, in.FloorIdle = model.GridUnknown, true
		}, Calm},
		{"cutoff at MinSOC, floor-unknown grid", func(in *cadenceInput) {
			in.Mode, in.LastGrid, in.FloorIdle = detect.Cutoff, model.GridUnknown, true
		}, Calm},
		{"floor-unknown grid, grid not confirmed on", func(in *cadenceInput) {
			in.LastGrid, in.FloorIdle, in.Grid = model.GridUnknown, true, detect.GridOff
		}, Active},
		{"floor-unknown grid, grid unknown", func(in *cadenceInput) {
			in.LastGrid, in.FloorIdle, in.Grid = model.GridUnknown, true, detect.GridUnknownState
		}, Active},
	}
	for _, c := range cases {
		in := calmInput()
		c.edit(&in)
		if got := cadenceFor(in); got != c.want {
			t.Errorf("%s: cadence = %v, want %v", c.name, got, c.want)
		}
	}
}
