package main

import (
	"encoding/json"
	"math"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lansim"
)

// Simulated scenario (-simulate), one pass, times from process start:
// 0:00 grid on, idle at 95%; 1:00 grid off, the house draws ~400 W;
// 3:00 grid back, charging at 2492 W until 100% (about 6:00), then idle;
// 6:30 the station stops answering for 6 minutes (data goes stale, then the
// link is declared lost), 12:30 it answers again.
const (
	simGridOffAfter = time.Minute
	simGridOffFor   = 2 * time.Minute
	simStartSOC     = 95.0
	simSilentAfter  = 6*time.Minute + 30*time.Second
	simSilentFor    = 6 * time.Minute
)

// simSilent reports whether the station ignores requests at this time.
func simSilent(el time.Duration) bool {
	return el >= simSilentAfter && el < simSilentAfter+simSilentFor
}

// startSimulator starts an in-process fake Venus E on loopback that plays
// the scenario above. The real station is never contacted.
func startSimulator() (*lansim.FakeStation, error) {
	info := lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: "5100000000ff", WifiMac: "simulated"}
	f, err := lansim.Listen(info)
	if err != nil {
		return nil, err
	}
	// Like a real station it reports its bare IP; the port reaches the
	// monitor through the discovery targets.
	info.IP = f.Addr.IP.String()
	f.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { return info, nil })
	start := time.Now()
	// The station is silent to discovery too, as a switched-off one would be.
	// The timers only flip a flag, so they may outlive Close.
	time.AfterFunc(simSilentAfter, func() { f.Silence(simSilent(time.Since(start))) })
	time.AfterFunc(simSilentAfter+simSilentFor, func() { f.Silence(simSilent(time.Since(start))) })
	f.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
		soc, ongrid, offgrid := simStatus(time.Since(start))
		return map[string]any{"bat_soc": soc, "bat_cap": 5120, "pv_power": 0,
			"ongrid_power": ongrid, "offgrid_power": offgrid}, nil
	})
	return f, nil
}

// simStatus is the station's state at the given time into the scenario:
// -1% every 30 s without grid, +1% every 20 s while charging.
func simStatus(el time.Duration) (soc, ongrid, offgrid float64) {
	off := el - simGridOffAfter
	switch {
	case off < 0:
		return simStartSOC, 0, 0
	case off < simGridOffFor:
		return simStartSOC - math.Floor(off.Seconds()/30), 0, 400
	}
	low := simStartSOC - math.Floor(simGridOffFor.Seconds()/30)
	soc = math.Min(100, low+math.Floor((off-simGridOffFor).Seconds()/20))
	if soc < 100 {
		return soc, -2492, 0
	}
	return 100, 0, 0
}
