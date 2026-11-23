package monitor

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/journal"
	"github.com/econ11/marstek-monitor-code/core/settings"
	"github.com/econ11/marstek-monitor-code/core/source/lan"
)

// Owner decision 2026-11-23 (Ruling 57): the monitor never sends
// Bat.GetStatus. The fake station records every method it receives over a
// long fake-clock run: a first run (discovery), saved settings from an older
// version that asked for battery reads, LIVE polling, an outage, a station
// that goes silent (rediscovery), a PC sleep and wake, and a restart. Only
// ES.GetStatus and Marstek.GetDevice may appear.
func TestMonitorNeverSendsBatGetStatus(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	// Saved by an older version: battery reads on.
	if err := st.SetSetting(settings.Key, `{"PollInterval":2000000000,"BatteryDetails":"on"}`); err != nil {
		t.Fatal(err)
	}
	s, err := settings.Load(st)
	if err != nil {
		t.Fatal(err)
	}
	if s.PollInterval != 2*time.Second {
		t.Fatalf("poll = %v; the other saved fields must be kept", s.PollInterval)
	}
	clk := newClock(t0)
	d := deps(st, newClient(t), fs.Addr, clk)
	d.PollInterval = s.PollInterval
	m, err := New(d) // first run: discovers the station
	if err != nil {
		t.Fatal(err)
	}
	m.Configure(ConfigFor(s))
	stop := run(t, m, clk)

	clk.advance(t, time.Second, 25*time.Minute) // LIVE, calm and active polling
	fs.gridOff(true)                            // an outage
	clk.advance(t, time.Second, 12*time.Minute)
	fs.gridOff(false)
	clk.advance(t, time.Second, 12*time.Minute)
	fs.Silence(true) // the station goes silent: backoff and rediscovery
	clk.advance(t, time.Second, 3*time.Minute)
	fs.Silence(false)
	clk.advance(t, time.Second, 12*time.Minute)
	m.Suspend() // PC sleep and wake
	clk.advance(t, time.Second, 5*time.Minute)
	m.Resume()
	clk.advance(t, time.Second, 25*time.Minute)
	stop()

	// A restart on the same store.
	clk.Advance(30 * time.Second)
	m2, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	m2.Configure(ConfigFor(s))
	run(t, m2, clk)
	clk.advance(t, time.Second, 25*time.Minute)

	if n := fs.Calls("Bat.GetStatus"); n != 0 {
		t.Fatalf("Bat.GetStatus sent %d times, want never", n)
	}
	for _, method := range fs.Methods() {
		if !slices.Contains([]string{"ES.GetStatus", "Marstek.GetDevice"}, method) {
			t.Fatalf("methods sent = %v; want only ES.GetStatus and Marstek.GetDevice", fs.Methods())
		}
	}
	if fs.Calls("ES.GetStatus") < 100 || fs.Calls("Marstek.GetDevice") < 2 {
		t.Fatalf("polls %d, discovery %d: the run did not poll and rediscover", fs.Calls("ES.GetStatus"), fs.Calls("Marstek.GetDevice"))
	}
}

// A rediscovery that finds the station at the same IP with another firmware
// (an update or a rollback) stores it, so the row stays current; that is no
// IP change and gets no journal entry.
func TestMonitorRediscoveryKeepsFirmwareCurrent(t *testing.T) {
	st := openStore(t)
	fs := startStation(t, testMac)
	seedStation(t, st, fs.Addr.IP.String()) // firmware 151
	fs.info.Ver = 152
	// The polls fail (an RPC error), so rediscovery runs; GetDevice answers.
	fs.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
		return nil, &lan.RPCError{Code: -32700, Message: "Parse error"}
	})
	clk := newClock(t0)
	m, err := New(deps(st, newClient(t), fs.Addr, clk))
	if err != nil {
		t.Fatal(err)
	}
	run(t, m, clk)
	clk.advance(t, time.Second, 90*time.Second)
	if fs.Calls("Marstek.GetDevice") == 0 {
		t.Fatal("no rediscovery")
	}
	all, err := st.Stations()
	if err != nil || len(all) != 1 || all[0].Firmware != 152 || all[0].IP != fs.Addr.IP.String() {
		t.Fatalf("stations = %+v, %v; want firmware 152 at the same IP", all, err)
	}
	if got := m.Latest().Station.Firmware; got != 152 {
		t.Fatalf("snapshot firmware = %d, want 152", got)
	}
	if _, codes := journalCodes(t, m, clk); slices.Contains(codes, journal.CodeIPChanged) {
		t.Fatalf("journal = %v; a firmware change at the same IP is not an IP change", codes)
	}
}
