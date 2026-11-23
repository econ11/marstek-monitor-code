// Package onboard finds a station for the first-run screen: a LAN scan, a
// check of an IP typed by hand, and the check of that IP's format. It only
// finds; adding the station (store) is the caller's.
//
// Every call keeps the station's ≥ 1 s gap between two requests (it drops
// back-to-back calls, docs/dev-notes.md). The gap between separate Scan and
// ProbeIP calls is the caller's.
package onboard

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
)

var (
	ErrNoAnswer   = errors.New("onboard: no answer")
	ErrNotMarstek = errors.New("onboard: not a Marstek station")
	ErrBadIP      = errors.New("onboard: not a unicast IPv4")
)

// scanWaits: how long each of the 3 discovery broadcasts collects replies
// (8 s in all; like monitor.discoverWait, a var so tests can shorten it).
var scanWaits = []time.Duration{2 * time.Second, 2 * time.Second, 4 * time.Second}

// probeWait bounds each of ProbeIP's 2 tries (a var so tests can shorten it).
var probeWait = 2500 * time.Millisecond

// callGap is the least time between two requests to the station.
const callGap = time.Second

// codeParseError is the JSON-RPC "Parse error" a Marstek under load sometimes
// answers with (docs/dev-notes.md): it is retried, not taken as the answer.
const codeParseError = -32700

// Scan sends 3 discovery broadcasts (DiscoverMulti with wait = scanWaits[i])
// and calls found at once, after each broadcast, for every device with a BLE
// MAC not seen before (a reply without one has no identity and is skipped).
// The device's IP and Sender are the address its reply came from (the
// payload IP is not trusted, as in the monitor). Broadcasts start ≥ 1 s
// apart. A cancelled ctx stops the scan with ctx.Err(); a broadcast that
// fails (no reply collected and an error) is skipped, and Scan fails, with
// the last error, only if all 3 did.
func Scan(ctx context.Context, c *lan.Client, targets func() []*net.UDPAddr, found func(lan.DeviceInfo)) error {
	seen := map[string]bool{}
	// last: no earlier than the previous broadcast went out. DiscoverMulti
	// starts its wait after sending, so when it ran its full wait its return
	// minus the wait is a time after the send (time.Now before the call would
	// be before it). When it returned early (a read error), its return is.
	var last time.Time
	var callErr error
	sent := false
	for _, w := range scanWaits {
		if !last.IsZero() {
			if err := sleep(ctx, time.Until(last.Add(callGap))); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		start := time.Now()
		devs, err := c.DiscoverMulti(ctx, targets(), w)
		last = time.Now()
		if last.Sub(start) >= w {
			last = last.Add(-w)
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			callErr = err
		}
		// A read error may still come with the replies collected before it.
		if err == nil || len(devs) > 0 {
			sent = true
		}
		for _, d := range devs {
			ip := d.Sender.To4()
			if d.BLEMac == "" || ip == nil || seen[d.BLEMac] {
				continue
			}
			seen[d.BLEMac] = true
			d.Sender, d.IP = ip, ip.String()
			found(d)
		}
	}
	if !sent {
		return callErr
	}
	return nil
}

// ProbeIP asks ip:port for Marstek.GetDevice (params.ble_mac "0") up to 2
// times, each bounded by probeWait (2.5 s). A reply with device and ble_mac
// is the station: its IP and Sender are ip, Src the reply envelope's
// "src". A reply without them, or with an RPC error, is ErrNotMarstek at
// once. A -32700 Parse error (a busy
// Marstek) is retried 1 s after that reply; a timeout, or a failure to
// send, is retried 1 s after the 1st request (at once with probeWait 2.5 s,
// so the check ends within 5 s). When the last try gets no answer, a
// -32700 or a send failure, the result is ErrNoAnswer (Ruling 32: worth
// trying again). ip must parse as IPv4 (ErrBadIP); whether it is a unicast
// address is ValidIPv4's job, so tests may probe loopback. Besides those
// three errors only ctx.Err() is returned.
func ProbeIP(ctx context.Context, c *lan.Client, ip string, port int) (lan.DeviceInfo, error) {
	p := net.ParseIP(strings.TrimSpace(ip)).To4()
	if p == nil {
		return lan.DeviceInfo{}, ErrBadIP
	}
	addr := &net.UDPAddr{IP: p, Port: port}
	var err error
	for try := 0; try < 2; try++ {
		start := time.Now()
		var d lan.DeviceInfo
		d, err = probeOnce(ctx, c, addr)
		if err == nil {
			d.Sender, d.IP = p, p.String()
			return d, nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return lan.DeviceInfo{}, cerr
		}
		var rpc *lan.RPCError
		replied := errors.As(err, &rpc)
		if (replied && rpc.Code != codeParseError) || errors.Is(err, ErrNotMarstek) {
			return lan.DeviceInfo{}, ErrNotMarstek
		}
		if try == 0 {
			// After a reply the gap counts from the reply. start is taken
			// before the send, so a margin keeps a short try (tests) ≥ 1 s.
			wait := time.Until(start.Add(callGap + gapMargin))
			if replied {
				wait = callGap
			}
			if serr := sleep(ctx, wait); serr != nil {
				return lan.DeviceInfo{}, serr
			}
		}
	}
	return lan.DeviceInfo{}, ErrNoAnswer
}

// gapMargin pads a call gap measured from a time taken before the send.
const gapMargin = 10 * time.Millisecond

// probeOnce is one Marstek.GetDevice to addr: the device, ErrNotMarstek for
// a reply that is not a station's, or the Call error (a timeout is
// lan.ErrTimeout or context.DeadlineExceeded of the probeWait bound).
func probeOnce(ctx context.Context, c *lan.Client, addr *net.UDPAddr) (lan.DeviceInfo, error) {
	cctx, cancel := context.WithTimeout(ctx, probeWait)
	defer cancel()
	raw, src, err := c.CallSrc(cctx, addr, "Marstek.GetDevice", map[string]string{"ble_mac": "0"})
	if err != nil {
		return lan.DeviceInfo{}, err
	}
	var d lan.DeviceInfo
	if json.Unmarshal(raw, &d) != nil || d.Device == "" || d.BLEMac == "" {
		return lan.DeviceInfo{}, ErrNotMarstek
	}
	d.Src = src
	return d, nil
}

// ValidIPv4 parses an address typed by hand (spaces around it are ignored).
// Only a dotted unicast IPv4 passes: not 0.0.0.0, 255.255.255.255, loopback,
// multicast, x.x.x.255 or x.x.x.0 (the broadcast and network address of a
// /24, the usual home LAN); anything else is ErrBadIP. The result is 4 bytes
// long.
func ValidIPv4(s string) (net.IP, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ":") { // IPv6, an IPv4-mapped IPv6 or ip:port
		return nil, ErrBadIP
	}
	ip := net.ParseIP(s).To4()
	if ip == nil || ip.IsUnspecified() || ip.Equal(net.IPv4bcast) || ip.IsLoopback() || ip.IsMulticast() || ip[3] == 255 || ip[3] == 0 {
		return nil, ErrBadIP
	}
	return ip, nil
}

// sleep waits d (nothing if d ≤ 0) or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
