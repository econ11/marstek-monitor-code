// Package lansim is an in-process fake Marstek station, for the tests (through
// lantest) and for the app's -simulate mode. It does not import "testing", so
// it can be linked into the app.
package lansim

import (
	"encoding/json"
	"net"
	"sort"
	"sync"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
)

// FakeStation answers Marstek JSON-RPC over UDP on 127.0.0.1.
type FakeStation struct {
	Addr *net.UDPAddr

	conn net.PacketConn
	mu   sync.Mutex
	h    map[string]func(json.RawMessage) (any, *lan.RPCError)
	n    map[string]int
	mute bool
	fid  any
	hasF bool
	src  string // the reply envelope's "src"
	done chan struct{}
}

// Listen starts a station on 127.0.0.1:0 that answers Marstek.GetDevice with
// info by default; the caller must Close it.
func Listen(info lan.DeviceInfo) (*FakeStation, error) { return ListenAt("127.0.0.1:0", info) }

// ListenAt is Listen on a given loopback address ("127.0.0.2:0" is another
// host as far as the client can tell).
func ListenAt(addr string, info lan.DeviceInfo) (*FakeStation, error) {
	conn, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, err
	}
	f := &FakeStation{
		Addr: conn.LocalAddr().(*net.UDPAddr),
		conn: conn,
		h:    map[string]func(json.RawMessage) (any, *lan.RPCError){},
		n:    map[string]int{},
		src:  "fake-station",
		done: make(chan struct{}),
	}
	f.Handle("Marstek.GetDevice", func(json.RawMessage) (any, *lan.RPCError) { return info, nil })
	go func() { defer close(f.done); f.serve() }()
	return f, nil
}

// Close stops the station and waits for its goroutine.
func (f *FakeStation) Close() {
	f.conn.Close()
	<-f.done
}

// Handle registers (or replaces) the handler for method.
func (f *FakeStation) Handle(method string, h func(params json.RawMessage) (any, *lan.RPCError)) {
	f.mu.Lock()
	f.h[method] = h
	f.mu.Unlock()
}

// Silence stops (on=true) or resumes replying. Requests are still counted.
func (f *FakeStation) Silence(on bool) {
	f.mu.Lock()
	f.mute = on
	f.mu.Unlock()
}

// SetSrc sets the "src" every reply carries (a real station: model code and
// BLE MAC, «VNSE3-0-0a1b2c3d4e5f»; by default "fake-station").
func (f *FakeStation) SetSrc(src string) {
	f.mu.Lock()
	f.src = src
	f.mu.Unlock()
}

// ReplyWithID makes the next reply carry id instead of the id of the request.
func (f *FakeStation) ReplyWithID(id any) {
	f.mu.Lock()
	f.fid, f.hasF = id, true
	f.mu.Unlock()
}

// Calls reports how many requests for method were received.
func (f *FakeStation) Calls(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n[method]
}

// Methods reports every method received so far, sorted.
func (f *FakeStation) Methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.n))
	for m := range f.n {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func (f *FakeStation) serve() {
	buf := make([]byte, 64*1024)
	for {
		n, from, err := f.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(buf[:n], &req) != nil {
			continue
		}
		f.mu.Lock()
		f.n[req.Method]++
		h, mute := f.h[req.Method], f.mute
		f.mu.Unlock()
		if mute {
			continue
		}
		var result any
		var rpcErr *lan.RPCError
		if h == nil {
			rpcErr = &lan.RPCError{Code: -32601, Message: "Method not found"}
		} else {
			result, rpcErr = h(req.Params)
		}
		var id any = req.ID
		f.mu.Lock()
		if f.hasF {
			id, f.hasF = f.fid, false
		}
		src := f.src
		f.mu.Unlock()
		reply := map[string]any{"id": id, "src": src}
		if rpcErr != nil {
			reply["error"] = rpcErr
		} else {
			reply["result"] = result
		}
		if b, err := json.Marshal(reply); err == nil {
			_, _ = f.conn.WriteTo(b, from)
		}
	}
}
