// Package lantest provides an in-process fake Marstek station for tests.
package lantest

import (
	"net"
	"testing"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
	"github.com/econ11/marstek-monitor-code/core/source/lan/lansim"
)

// FakeStation answers Marstek JSON-RPC over UDP on 127.0.0.1.
type FakeStation = lansim.FakeStation

// Start listens on 127.0.0.1:0 and answers Marstek.GetDevice with info by
// default; the station is closed when the test ends.
func Start(t testing.TB, info lan.DeviceInfo) *FakeStation {
	t.Helper()
	f, err := lansim.Listen(info)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

// StartAt is Start on a given loopback address (see lansim.ListenAt).
func StartAt(t testing.TB, addr string, info lan.DeviceInfo) *FakeStation {
	t.Helper()
	f, err := lansim.ListenAt(addr, info)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

// OtherHost is an IPv4 address on this machine other than 127.0.0.1, for a
// fake station that must look like another host: 127.0.0.2 where the whole
// 127/8 is loopback (Windows, Linux); on macOS, where lo0 has 127.0.0.1 only
// (127.0.0.2 needs `sudo ifconfig lo0 alias`), the machine's own address on
// an interface that is up. Packets to it from 127.0.0.1 never leave the
// machine. Without one the test is skipped.
func OtherHost(t testing.TB) string {
	t.Helper()
	if c, err := net.ListenPacket("udp4", "127.0.0.2:0"); err == nil {
		c.Close()
		return "127.0.0.2"
	}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			if c, err := net.ListenPacket("udp4", n.IP.String()+":0"); err == nil {
				c.Close()
				return n.IP.String()
			}
		}
	}
	t.Skip("no second IPv4 address on this machine (127.0.0.2 or an interface address)")
	return ""
}
