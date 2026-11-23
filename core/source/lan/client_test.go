package lan_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lantest"
)

func newClient(t *testing.T, timeout time.Duration) *lan.Client {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return lan.NewClient(conn, timeout)
}

func TestCallRoundTrip(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{BLEMac: "aa"})
	st.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
		return map[string]any{"bat_soc": 72}, nil
	})
	c := newClient(t, 500*time.Millisecond)
	res, err := c.Call(context.Background(), st.Addr, "ES.GetStatus", map[string]any{"id": 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res), `"bat_soc":72`) {
		t.Fatalf("result = %s", res)
	}
	if st.Calls("ES.GetStatus") != 1 {
		t.Fatalf("calls = %d", st.Calls("ES.GetStatus"))
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{})
	st.Handle("Nope", func(json.RawMessage) (any, *lan.RPCError) {
		return nil, &lan.RPCError{Code: -32601, Message: "Method not found"}
	})
	c := newClient(t, 500*time.Millisecond)
	_, err := c.Call(context.Background(), st.Addr, "Nope", nil)
	var rpcErr *lan.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("err = %v", err)
	}
}

func TestCallTimesOutWhenSilent(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{})
	st.Silence(true)
	c := newClient(t, 200*time.Millisecond)
	start := time.Now()
	_, err := c.Call(context.Background(), st.Addr, "ES.GetStatus", nil)
	if !errors.Is(err, lan.ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
	// A loose ceiling: the 200 ms timeout ends the call; a loaded machine
	// (-race, parallel packages) must not make this flaky.
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestCallContextCancel(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{})
	st.Silence(true)
	c := newClient(t, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Call(ctx, st.Addr, "ES.GetStatus", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestCallIgnoresMismatchedIDAndSender(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{})
	rogue, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rogue.Close() })
	c := newClient(t, 250*time.Millisecond)
	clientAddr := c.LocalAddr()

	// Before the real station answers, a different device sends a reply
	// carrying the id of the current call (ids start at 1 and increment).
	var nextID atomic.Int64
	nextID.Store(1)
	st.Handle("ES.GetStatus", func(json.RawMessage) (any, *lan.RPCError) {
		forged := fmt.Sprintf(`{"id":%d,"src":"rogue","result":{"evil":true}}`, nextID.Load())
		rogue.WriteTo([]byte(forged), clientAddr)
		time.Sleep(20 * time.Millisecond)
		return map[string]any{"bat_soc": 72}, nil
	})

	// Case 1: real station replies with a foreign id, rogue replies with the
	// right id from the wrong address. Call must time out, never return evil.
	st.ReplyWithID(999)
	res, err := c.Call(context.Background(), st.Addr, "ES.GetStatus", nil)
	if err == nil || !errors.Is(err, lan.ErrTimeout) {
		t.Fatalf("case 1: res=%s err=%v, want ErrTimeout", res, err)
	}

	// Case 2: rogue forges the next id first, then the real reply arrives.
	nextID.Store(2)
	res, err = c.Call(context.Background(), st.Addr, "ES.GetStatus", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res), "evil") || !strings.Contains(string(res), `"bat_soc":72`) {
		t.Fatalf("case 2: result = %s", res)
	}
}

func TestCallAcceptsID0ParseErrorFromSameSender(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{})
	st.Handle("X", func(json.RawMessage) (any, *lan.RPCError) {
		return nil, &lan.RPCError{Code: -32700, Message: "Parse error"}
	})
	st.ReplyWithID(0)
	c := newClient(t, 300*time.Millisecond)
	_, err := c.Call(context.Background(), st.Addr, "X", nil)
	var rpcErr *lan.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32700 {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverCollectsDevices(t *testing.T) {
	a := lantest.Start(t, lan.DeviceInfo{Device: "VenusE", Ver: 111, BLEMac: "aabbcc000001", IP: "192.168.1.10"})
	b := lantest.Start(t, lan.DeviceInfo{Device: "VenusE", Ver: 111, BLEMac: "aabbcc000002", IP: "192.168.1.11"})
	c := newClient(t, 300*time.Millisecond)
	// A real broadcast reaches both; on loopback we send the same request to
	// each station address instead.
	devs, err := c.DiscoverMulti(context.Background(), []*net.UDPAddr{a.Addr, b.Addr}, 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("devs = %+v", devs)
	}
	got := map[string]lan.DeviceInfo{}
	for _, d := range devs {
		got[d.BLEMac] = d
	}
	if got["aabbcc000001"].IP != "192.168.1.10" || got["aabbcc000002"].Ver != 111 {
		t.Fatalf("got = %+v", got)
	}
}

func TestDiscoverSingleAddr(t *testing.T) {
	a := lantest.Start(t, lan.DeviceInfo{BLEMac: "aabbcc000001"})
	c := newClient(t, 300*time.Millisecond)
	devs, err := c.Discover(context.Background(), a.Addr, 100*time.Millisecond)
	if err != nil || len(devs) != 1 {
		t.Fatalf("devs=%+v err=%v", devs, err)
	}
}

func TestDiscoverToleratesBadTarget(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{BLEMac: "aa"})
	c := newClient(t, 500*time.Millisecond)
	bad := &net.UDPAddr{IP: net.ParseIP("::1"), Port: 1} // udp4 socket cannot send to IPv6
	devs, failed, err := c.DiscoverTargets(context.Background(), []*net.UDPAddr{bad, st.Addr}, 150*time.Millisecond)
	if err != nil || len(devs) != 1 {
		t.Fatalf("devs=%v err=%v", devs, err)
	}
	if len(failed) != 1 || failed[bad.String()] == nil {
		t.Fatalf("failed = %v", failed)
	}
	if _, err := c.DiscoverMulti(context.Background(), []*net.UDPAddr{bad}, 50*time.Millisecond); err == nil {
		t.Fatal("want error when every target fails")
	}
}

// m-2: the address a reply came from is reported next to the payload, which
// a device is free to fill with anything.
func TestDiscoverReportsSender(t *testing.T) {
	a := lantest.Start(t, lan.DeviceInfo{BLEMac: "aabbcc000001", IP: "10.9.8.7:1234"})
	c := newClient(t, 300*time.Millisecond)
	devs, err := c.Discover(context.Background(), a.Addr, 100*time.Millisecond)
	if err != nil || len(devs) != 1 {
		t.Fatalf("devs=%+v err=%v", devs, err)
	}
	if !devs[0].Sender.Equal(a.Addr.IP) || devs[0].IP != "10.9.8.7:1234" {
		t.Fatalf("sender = %v, payload ip = %q; want %v and the payload untouched", devs[0].Sender, devs[0].IP, a.Addr.IP)
	}
}

// The fake records every method it receives (the monitor's guard against
// Bat.GetStatus reads them) and answers an unknown one with "Method not found".
func TestFakeRecordsMethods(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{BLEMac: "aa"})
	c := newClient(t, 500*time.Millisecond)
	if _, err := c.Call(context.Background(), st.Addr, "Marstek.GetDevice", map[string]string{"ble_mac": "0"}); err != nil {
		t.Fatal(err)
	}
	_, err := c.Call(context.Background(), st.Addr, "Foo.Bar", nil)
	var rpcErr *lan.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("unknown method: err = %v; want -32601", err)
	}
	if got := fmt.Sprint(st.Methods()); got != "[Foo.Bar Marstek.GetDevice]" {
		t.Fatalf("methods = %s", got)
	}
}

func TestReplySrcReported(t *testing.T) {
	st := lantest.Start(t, lan.DeviceInfo{Device: "VenusE 3.0", BLEMac: "0a1b2c3d4e5f"})
	st.SetSrc("VNSE3-0-0a1b2c3d4e5f")
	c := newClient(t, 300*time.Millisecond)
	_, src, err := c.CallSrc(context.Background(), st.Addr, "Marstek.GetDevice", map[string]string{"ble_mac": "0"})
	if err != nil || src != "VNSE3-0-0a1b2c3d4e5f" {
		t.Fatalf("CallSrc: src %q, %v", src, err)
	}
	devs, err := c.DiscoverMulti(context.Background(), []*net.UDPAddr{st.Addr}, 150*time.Millisecond)
	if err != nil || len(devs) != 1 || devs[0].Src != "VNSE3-0-0a1b2c3d4e5f" {
		t.Fatalf("discover: %+v, %v", devs, err)
	}
}

func TestDiscoverSendsBLEMacZero(t *testing.T) {
	info := lan.DeviceInfo{Device: "VenusE 3.0", BLEMac: "aabbccddeeff"}
	st := lantest.Start(t, info)
	got := make(chan json.RawMessage, 1)
	st.Handle("Marstek.GetDevice", func(p json.RawMessage) (any, *lan.RPCError) {
		select {
		case got <- p:
		default:
		}
		return info, nil
	})
	c := newClient(t, 300*time.Millisecond)
	if _, err := c.Discover(context.Background(), st.Addr, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	var params struct {
		BLEMac *string `json:"ble_mac"`
	}
	select {
	case p := <-got:
		if err := json.Unmarshal(p, &params); err != nil {
			t.Fatalf("params %s: %v", p, err)
		}
	default:
		t.Fatal("no Marstek.GetDevice request seen")
	}
	if params.BLEMac == nil || *params.BLEMac != "0" {
		t.Fatalf("params.ble_mac = %v, want \"0\"", params.BLEMac)
	}
}
