package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
)

// probeMethods are polled in this order on every tick. Bat.GetStatus is deliberately
// absent: on some firmwares it reboots the station, and the app never calls it
// (owner decision 2026-11-23, docs/dev-notes.md).
var probeMethods = []string{"ES.GetStatus", "ES.GetMode", "EM.GetStatus", "Wifi.GetStatus"}

// Config drives Run.
type Config struct {
	Addr     *net.UDPAddr   // station address
	Info     lan.DeviceInfo // station identity (written as the first JSONL line)
	Interval time.Duration  // tick period
	Gap      time.Duration  // pause between calls within a tick
	Console  io.Writer      // human-readable output; may be nil
	Summary  time.Duration  // console summary period (default 5 min)
	// RediscoverAfter is how long calls must keep timing out before Rediscover is tried (default 60 s).
	RediscoverAfter time.Duration
	// Rediscover returns the station's current address (matched by ble_mac); may be nil.
	Rediscover func(ctx context.Context) (*net.UDPAddr, error)
}

type line struct {
	T       string          `json:"t"`
	Method  string          `json:"method"`
	Ms      *float64        `json:"ms,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Timeout bool            `json:"timeout,omitempty"`
	Tick    int             `json:"tick,omitempty"`
	// SincePrevMs is the time from the end of the previous call (any method) to the start of this one;
	// PrevOutcome is that call's outcome: ok, timeout or error. Both help to test the "back-to-back calls overload the station" hypothesis.
	SincePrevMs *float64 `json:"since_prev_ms,omitempty"`
	PrevOutcome string   `json:"prev,omitempty"`
}

type stats struct {
	samples, timeouts, errs int
	last                    map[string]string
}

func (s *stats) noteStatus(res json.RawMessage) {
	var m map[string]json.RawMessage
	if json.Unmarshal(res, &m) != nil {
		return
	}
	for _, k := range []string{"bat_soc", "ongrid_power", "offgrid_power"} {
		if v, ok := m[k]; ok {
			s.last[k] = string(v)
		}
	}
}

func (s *stats) String() string {
	v := func(k string) string {
		if x, ok := s.last[k]; ok {
			return x
		}
		return "?"
	}
	return fmt.Sprintf("samples=%d timeouts=%d errors=%d bat_soc=%s ongrid_power=%s offgrid_power=%s",
		s.samples, s.timeouts, s.errs, v("bat_soc"), v("ongrid_power"), v("offgrid_power"))
}

// Run polls the station until ctx is cancelled, appending one JSON line per call to w.
// It never stops on timeouts or errors; it returns nil on cancellation.
func Run(ctx context.Context, c *lan.Client, cfg Config, w io.Writer) error {
	if cfg.Summary <= 0 {
		cfg.Summary = 5 * time.Minute
	}
	if cfg.RediscoverAfter <= 0 {
		cfg.RediscoverAfter = 60 * time.Second
	}
	emit := func(l line) error {
		b, err := json.Marshal(l)
		if err != nil {
			return err
		}
		_, err = w.Write(append(b, '\n')) // single write per line: nothing is buffered
		return err
	}
	cprintf := func(f string, a ...any) {
		if cfg.Console != nil {
			fmt.Fprintf(cfg.Console, f, a...)
		}
	}
	infoJSON, _ := json.Marshal(cfg.Info)
	if err := emit(line{T: time.Now().Format(time.RFC3339Nano), Method: "Marstek.GetDevice", Result: infoJSON}); err != nil {
		return err
	}
	cprintf("station: device=%q ver=%d ble_mac=%s ip=%s\n", cfg.Info.Device, cfg.Info.Ver, cfg.Info.BLEMac, cfg.Info.IP)

	c.SetStrayHandler(func(from net.Addr, data []byte) {
		if len(data) > 200 {
			data = data[:200]
		}
		cprintf("stray datagram from %s: %s\n", from, data)
		res, _ := json.Marshal(map[string]string{"from": from.String(), "data": string(data)})
		_ = emit(line{T: time.Now().Format(time.RFC3339Nano), Method: "stray", Result: res})
	})
	defer c.SetStrayHandler(nil)

	st := &stats{last: map[string]string{}}
	firstTick := true
	tickN := 0
	prevOutcome := ""
	var prevEnd time.Time
	addr := cfg.Addr
	var failSince time.Time // start of the current no-reply streak
	nextSummary := time.Now().Add(cfg.Summary)

	for {
		tickN++
		tickStart := time.Now()
		for mi, method := range probeMethods {
			if mi > 0 && cfg.Gap > 0 {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(cfg.Gap):
				}
			}
			start := time.Now()
			res, err := c.Call(ctx, addr, method, map[string]int{"id": 0})
			if ctx.Err() != nil {
				return nil
			}
			ms := float64(time.Since(start).Microseconds()) / 1000
			l := line{T: start.Format(time.RFC3339Nano), Method: method, Ms: &ms, Tick: tickN, PrevOutcome: prevOutcome}
			if !prevEnd.IsZero() {
				sp := float64(start.Sub(prevEnd).Microseconds()) / 1000
				l.SincePrevMs = &sp
			}
			prevEnd = start.Add(time.Since(start))
			var re *lan.RPCError
			switch {
			case err == nil:
				l.Result = res
				st.samples++
				failSince = time.Time{}
				if method == "ES.GetStatus" {
					st.noteStatus(res)
				}
			case errors.Is(err, lan.ErrTimeout):
				l.Timeout = true
				st.timeouts++
				if failSince.IsZero() {
					failSince = start
				}
			default:
				l.Error = err.Error()
				st.errs++
				if !errors.As(err, &re) { // transport error: counts as no reply
					if failSince.IsZero() {
						failSince = start
					}
				} else {
					failSince = time.Time{} // an RPC error proves the station is alive
				}
			}
			switch {
			case l.Timeout:
				prevOutcome = "timeout"
			case l.Error != "":
				prevOutcome = "error"
			default:
				prevOutcome = "ok"
			}
			if werr := emit(l); werr != nil {
				cprintf("write error: %v\n", werr)
			}
		}
		if firstTick {
			firstTick = false
			if st.samples == 0 && st.errs == 0 {
				cprintf("WARNING: all %d calls of the first tick timed out. The station is not answering at %s: check the IP, that the local API is enabled in the Marstek app, and that UDP port 30000 is not blocked by the firewall or used by another program. The probe keeps running and logging.\n", len(probeMethods), addr)
			}
		}
		if !failSince.IsZero() && time.Since(failSince) >= cfg.RediscoverAfter && cfg.Rediscover != nil {
			cprintf("no replies for %s, re-running discovery\n", cfg.RediscoverAfter)
			na, err := cfg.Rediscover(ctx)
			if ctx.Err() != nil {
				return nil
			}
			if err == nil && na != nil && (!na.IP.Equal(addr.IP) || na.Port != addr.Port) {
				cprintf("station moved: %s -> %s\n", addr, na)
				addr = na
			}
			failSince = time.Now() // retry no more often than every RediscoverAfter
		}
		if time.Now().After(nextSummary) {
			cprintf("%s %s\n", time.Now().Format("15:04:05"), st)
			nextSummary = time.Now().Add(cfg.Summary)
		}
		// Next tick: Interval after this tick started, but never sooner than Gap after the last
		// call ended. After an overrun the schedule restarts from here (no burst of catch-up ticks).
		next := tickStart.Add(cfg.Interval)
		if min := prevEnd.Add(cfg.Gap); next.Before(min) {
			next = min
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Until(next)):
		}
	}
}
