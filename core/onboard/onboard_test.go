package onboard

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lantest"
)

func init() {
	scanWaits = []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond}
	probeWait = 200 * time.Millisecond
}

func newClient(t *testing.T) *lan.Client {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return lan.NewClient(conn, 100*time.Millisecond)
}

func info(mac string) lan.DeviceInfo {
	return lan.DeviceInfo{Device: "VenusE 3.0", Ver: 151, BLEMac: mac, WifiMac: "wifi-" + mac, IP: "10.9.9.9"}
}

// recorder logs the time of every Marstek.GetDevice a fake receives.
type recorder struct {
	mu    sync.Mutex
	times []time.Time
}

func (r *recorder) add() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.times = append(r.times, time.Now())
	return len(r.times)
}

func (r *recorder) all() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Time(nil), r.times...)
}

func targetsOf(fs ...*lantest.FakeStation) func() []*net.UDPAddr {
	return func() []*net.UDPAddr {
		var out []*net.UDPAddr
		for _, f := range fs {
			out = append(out, f.Addr)
		}
		return out
	}
}

func TestScanReportsEachDeviceOnce(t *testing.T) {
	a := lantest.StartAt(t, "127.0.0.1:0", info("aaaa"))
	b := lantest.StartAt(t, lantest.OtherHost(t)+":0", info("bbbb"))
	var nb recorder
	// b answers from the 2nd broadcast on; the 1st gets an RPC error (a busy station).
	b.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) {
		if nb.add() == 1 {
			return nil, &lan.RPCError{Code: -32700, Message: "Parse error"}
		}
		return info("bbbb"), nil
	})
	var got []lan.DeviceInfo
	err := Scan(context.Background(), newClient(t), targetsOf(a, b), func(d lan.DeviceInfo) {
		got = append(got, d)
		// found is called right after the broadcast that first saw the device.
		if n := a.Calls("Marstek.GetDevice"); d.BLEMac == "aaaa" && n != 1 {
			t.Errorf("aaaa found after %d broadcasts, want 1", n)
		}
		if n := b.Calls("Marstek.GetDevice"); d.BLEMac == "bbbb" && n != 2 {
			t.Errorf("bbbb found after %d broadcasts, want 2", n)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].BLEMac != "aaaa" || got[1].BLEMac != "bbbb" {
		t.Fatalf("found = %+v, want aaaa then bbbb", got)
	}
	// The address is where the reply came from, not the payload's.
	if got[0].IP != "127.0.0.1" || got[1].IP != b.Addr.IP.String() || !got[1].Sender.Equal(b.Addr.IP) {
		t.Fatalf("addresses = %q %q %v, want the senders", got[0].IP, got[1].IP, got[1].Sender)
	}
	if got[0].Device != "VenusE 3.0" || got[0].Ver != 151 {
		t.Fatalf("model/firmware = %q/%d, want VenusE 3.0/151", got[0].Device, got[0].Ver)
	}
	if na, nb := a.Calls("Marstek.GetDevice"), b.Calls("Marstek.GetDevice"); na != 3 || nb != 3 {
		t.Fatalf("broadcasts = %d, %d; want 3 each", na, nb)
	}
}

func TestScanSkipsDeviceWithoutBLEMac(t *testing.T) {
	a := lantest.StartAt(t, "127.0.0.1:0", info(""))
	b := lantest.StartAt(t, lantest.OtherHost(t)+":0", info("bbbb"))
	var got []string
	if err := Scan(context.Background(), newClient(t), targetsOf(a, b), func(d lan.DeviceInfo) {
		got = append(got, d.BLEMac)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "bbbb" {
		t.Fatalf("found = %q, want only bbbb", got)
	}
}

func TestScanKeepsCallGap(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	var r recorder
	a.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { r.add(); return info("aaaa"), nil })
	if err := Scan(context.Background(), newClient(t), targetsOf(a), func(lan.DeviceInfo) {}); err != nil {
		t.Fatal(err)
	}
	ts := r.all()
	if len(ts) != 3 {
		t.Fatalf("broadcasts = %d, want 3", len(ts))
	}
	for i := 1; i < len(ts); i++ {
		if d := ts[i].Sub(ts[i-1]); d < time.Second {
			t.Fatalf("broadcast %d came %v after the previous, want ≥ 1 s", i+1, d)
		}
	}
}

func TestScanStopsOnCtxCancel(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancelled while the 1st broadcast collects replies.
	a.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { cancel(); return info("aaaa"), nil })
	err := Scan(ctx, newClient(t), targetsOf(a), func(lan.DeviceInfo) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	time.Sleep(1200 * time.Millisecond) // a 2nd broadcast would have gone by now
	if n := a.Calls("Marstek.GetDevice"); n != 1 {
		t.Fatalf("broadcasts = %d, want 1", n)
	}
}

func TestScanStopsOnCtxCancelBetweenBroadcasts(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := Scan(ctx, newClient(t), targetsOf(a), func(lan.DeviceInfo) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := a.Calls("Marstek.GetDevice"); n != 1 {
		t.Fatalf("broadcasts = %d, want 1", n)
	}
}

func TestProbeIPMarstek(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	d, err := ProbeIP(context.Background(), newClient(t), "127.0.0.1", a.Addr.Port)
	if err != nil {
		t.Fatal(err)
	}
	if d.BLEMac != "aaaa" || d.Device != "VenusE 3.0" || d.Ver != 151 || d.IP != "127.0.0.1" || !d.Sender.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("device = %+v", d)
	}
	if n := a.Calls("Marstek.GetDevice"); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
}

func TestProbeIPNotMarstek(t *testing.T) {
	for name, h := range map[string]func(json.RawMessage) (any, *lan.RPCError){
		"foreign result": func(json.RawMessage) (any, *lan.RPCError) { return map[string]any{"foo": 1}, nil },
		"no ble_mac":     func(json.RawMessage) (any, *lan.RPCError) { return map[string]any{"device": "VenusE 3.0"}, nil },
		"no device":      func(json.RawMessage) (any, *lan.RPCError) { return map[string]any{"ble_mac": "aaaa"}, nil },
		"null result":    func(json.RawMessage) (any, *lan.RPCError) { return nil, nil },
		"rpc error": func(json.RawMessage) (any, *lan.RPCError) {
			return nil, &lan.RPCError{Code: -32601, Message: "Method not found"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := lantest.Start(t, info("aaaa"))
			a.Handle("Marstek.GetDevice", h)
			_, err := ProbeIP(context.Background(), newClient(t), "127.0.0.1", a.Addr.Port)
			if !errors.Is(err, ErrNotMarstek) {
				t.Fatalf("err = %v, want ErrNotMarstek", err)
			}
			if n := a.Calls("Marstek.GetDevice"); n != 1 {
				t.Fatalf("calls = %d, want 1 (a clear answer is not retried)", n)
			}
		})
	}
}

// A Marstek under load sometimes answers -32700 Parse error: the 2nd try,
// ≥ 1 s later, decides.
func TestProbeIPRetriesParseError(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	var r recorder
	a.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) {
		if r.add() == 1 {
			return nil, &lan.RPCError{Code: -32700, Message: "Parse error"}
		}
		return info("aaaa"), nil
	})
	d, err := ProbeIP(context.Background(), newClient(t), "127.0.0.1", a.Addr.Port)
	if err != nil || d.BLEMac != "aaaa" {
		t.Fatalf("device, err = %+v, %v; want aaaa", d, err)
	}
	ts := r.all()
	if len(ts) != 2 || ts[1].Sub(ts[0]) < time.Second {
		t.Fatalf("calls at %v, want 2 ≥ 1 s apart", ts)
	}
}

func TestProbeIPNoAnswer(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	a.Silence(true)
	_, err := ProbeIP(context.Background(), newClient(t), "127.0.0.1", a.Addr.Port)
	if !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("err = %v, want ErrNoAnswer", err)
	}
	if n := a.Calls("Marstek.GetDevice"); n != 2 {
		t.Fatalf("calls = %d, want 2", n)
	}
}

func TestProbeIPStopsOnCtxCancel(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	a.Silence(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ProbeIP(ctx, newClient(t), "127.0.0.1", a.Addr.Port); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestProbeIPBadIP(t *testing.T) {
	if _, err := ProbeIP(context.Background(), newClient(t), "abc", 30000); !errors.Is(err, ErrBadIP) {
		t.Fatalf("err = %v, want ErrBadIP", err)
	}
}

func TestValidIPv4RejectsNonUnicast(t *testing.T) {
	for _, s := range []string{"192.168.1.255", "255.255.255.255", "127.0.0.1", "224.0.0.1", "192.168.1", "abc",
		"0.0.0.0", "", "::1", "::ffff:192.168.1.47", "192.168.1.47:30000", "239.255.255.250", "192.168.1.0", "10.0.0.0"} {
		if ip, err := ValidIPv4(s); !errors.Is(err, ErrBadIP) {
			t.Errorf("ValidIPv4(%q) = %v, %v; want ErrBadIP", s, ip, err)
		}
	}
	for _, s := range []string{"192.168.1.47", " 192.168.1.20 ", "10.0.0.1"} {
		ip, err := ValidIPv4(s)
		if err != nil || ip.To4() == nil || len(ip) != net.IPv4len {
			t.Errorf("ValidIPv4(%q) = %v, %v; want a 4-byte IPv4", s, ip, err)
		}
	}
}

// Ruling 32: a -32700 on the last try is a busy station, worth trying again
// later, not a foreign device.
func TestProbeIPParseErrorOnLastTryIsNoAnswer(t *testing.T) {
	for name, first := range map[string]bool{"parse error twice": true, "timeout then parse error": false} {
		t.Run(name, func(t *testing.T) {
			a := lantest.Start(t, info("aaaa"))
			var r recorder
			a.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) {
				if r.add() == 1 && !first {
					time.Sleep(150 * time.Millisecond) // past the client's 100 ms
				}
				return nil, &lan.RPCError{Code: -32700, Message: "Parse error"}
			})
			_, err := ProbeIP(context.Background(), newClient(t), "127.0.0.1", a.Addr.Port)
			if !errors.Is(err, ErrNoAnswer) {
				t.Fatalf("err = %v, want ErrNoAnswer", err)
			}
			if n := len(r.all()); n != 2 {
				t.Fatalf("calls = %d, want 2", n)
			}
		})
	}
}

// Tries shorter than 1 s (here 100 ms) still keep the 1 s gap after a timeout.
func TestProbeIPKeepsGapAfterTimeout(t *testing.T) {
	a := lantest.Start(t, info("aaaa"))
	var r recorder
	a.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) {
		r.add()
		time.Sleep(150 * time.Millisecond) // every reply comes after the client's 100 ms
		return info("aaaa"), nil
	})
	if _, err := ProbeIP(context.Background(), newClient(t), "127.0.0.1", a.Addr.Port); !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("err = %v, want ErrNoAnswer", err)
	}
	ts := r.all()
	if len(ts) != 2 || ts[1].Sub(ts[0]) < time.Second {
		t.Fatalf("calls at %v, want 2 ≥ 1 s apart", ts)
	}
}
