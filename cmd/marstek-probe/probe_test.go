package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lantest"
)

func startStation(t *testing.T) *lantest.FakeStation {
	f := lantest.Start(t, lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: "aabbcc", IP: "127.0.0.1"})
	for _, m := range probeMethods {
		f.Handle(m, func(json.RawMessage) (any, *lan.RPCError) {
			return map[string]any{"id": 0, "bat_soc": 55, "ongrid_power": 120, "offgrid_power": 0}, nil
		})
	}
	return f
}

func runProbe(t *testing.T, f *lantest.FakeStation, d time.Duration, mod func(*Config)) []map[string]any {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := lan.NewClient(conn, 60*time.Millisecond)
	path := filepath.Join(t.TempDir(), "p.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Addr: f.Addr, Info: lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: "aabbcc"}, Interval: 50 * time.Millisecond}
	if mod != nil {
		mod(&cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if err := Run(ctx, c, cfg, file); err != nil {
		t.Fatal(err)
	}
	file.Close()
	rf, _ := os.Open(path)
	defer rf.Close()
	var lines []map[string]any
	sc := bufio.NewScanner(rf)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad jsonl line %q: %v", sc.Text(), err)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestProbeRecordsEachMethod(t *testing.T) {
	f := startStation(t)
	lines := runProbe(t, f, 400*time.Millisecond, nil)
	if lines[0]["method"] != "Marstek.GetDevice" {
		t.Fatalf("first line = %v", lines[0])
	}
	seen := map[string]bool{}
	for _, l := range lines[1:] {
		if l["result"] == nil || l["ms"] == nil || l["t"] == nil {
			t.Fatalf("incomplete line %v", l)
		}
		seen[l["method"].(string)] = true
	}
	for _, m := range probeMethods {
		if !seen[m] {
			t.Errorf("no line for %s", m)
		}
	}
	if n := f.Calls("Bat.GetStatus"); n != 0 {
		t.Errorf("Bat.GetStatus called %d times", n)
	}
}

func TestProbeLogsTimeoutsAndKeepsGoing(t *testing.T) {
	f := startStation(t)
	f.Silence(true)
	lines := runProbe(t, f, 500*time.Millisecond, nil)
	n := 0
	for _, l := range lines[1:] {
		if l["timeout"] == true {
			n++
		}
	}
	if n < 4 {
		t.Fatalf("want >=4 timeout lines, got %d (%d lines)", n, len(lines))
	}
}

func TestProbeRediscoversMovedStation(t *testing.T) {
	old := startStation(t)
	old.Silence(true)
	moved := startStation(t)
	lines := runProbe(t, old, 900*time.Millisecond, func(c *Config) {
		c.RediscoverAfter = 100 * time.Millisecond
		c.Rediscover = func(context.Context) (*net.UDPAddr, error) { return moved.Addr, nil }
	})
	if moved.Calls("ES.GetStatus") == 0 {
		t.Fatalf("probe never switched to the new address (%d lines)", len(lines))
	}
}

func TestProbeLogsStrayDatagram(t *testing.T) {
	f := startStation(t)
	f.Silence(true)
	other, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := lan.NewClient(conn, 100*time.Millisecond)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				_, _ = other.WriteTo([]byte(`{"id":1,"result":{"x":1}}`), conn.LocalAddr())
			}
		}
	}()
	var buf bytes.Buffer
	var console bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	cfg := Config{Addr: f.Addr, Interval: 50 * time.Millisecond, Console: &console}
	if err := Run(ctx, c, cfg, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"method":"stray"`) || !strings.Contains(console.String(), "stray datagram") {
		t.Fatalf("no stray record:\n%s\n%s", buf.String(), console.String())
	}
	if !strings.Contains(console.String(), "first tick timed out") {
		t.Fatalf("no first-tick diagnostic:\n%s", console.String())
	}
}

func TestProbeGapAndSequenceFields(t *testing.T) {
	f := startStation(t)
	lines := runProbe(t, f, 700*time.Millisecond, func(c *Config) { c.Gap = 40 * time.Millisecond; c.Interval = 300 * time.Millisecond })
	var second map[string]any
	for _, l := range lines[1:] {
		if l["method"] == "ES.GetMode" {
			second = l
			break
		}
	}
	if second == nil {
		t.Fatal("no ES.GetMode line")
	}
	if sp, _ := second["since_prev_ms"].(float64); sp < 35 {
		t.Errorf("since_prev_ms = %v, want >= gap", second["since_prev_ms"])
	}
	if second["prev"] != "ok" || second["tick"] != float64(1) {
		t.Errorf("prev/tick = %v/%v", second["prev"], second["tick"])
	}
}

func TestProbeGapAppliesAcrossTicks(t *testing.T) {
	f := startStation(t)
	// Interval shorter than a tick (3 gaps of 60ms) forces overruns.
	lines := runProbe(t, f, 1200*time.Millisecond, func(c *Config) { c.Gap = 60 * time.Millisecond; c.Interval = 20 * time.Millisecond })
	n := 0
	for _, l := range lines[1:] {
		if l["method"] == "ES.GetStatus" && l["tick"].(float64) >= 2 {
			n++
			if sp, _ := l["since_prev_ms"].(float64); sp < 55 {
				t.Errorf("tick %v first call since_prev_ms=%v, want >= gap", l["tick"], sp)
			}
		}
	}
	if n < 2 {
		t.Fatalf("only %d later ticks", n)
	}
}
