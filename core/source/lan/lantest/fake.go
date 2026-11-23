// Package lantest provides an in-process fake Marstek station for tests.
package lantest

import (
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
