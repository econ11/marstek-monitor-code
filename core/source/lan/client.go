// Package lan implements the Marstek Open API client: JSON-RPC over UDP.
package lan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// DefaultPort is the UDP port the Marstek local API listens on.
const DefaultPort = 30000

// ErrTimeout is returned when no matching reply arrived within the client timeout.
var ErrTimeout = errors.New("lan: timeout")

// RPCError is an error object returned by the station.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("lan: rpc error %d: %s", e.Code, e.Message)
}

// DeviceInfo is the Marstek.GetDevice result.
type DeviceInfo struct {
	Device   string `json:"device"`
	Ver      int    `json:"ver"`
	BLEMac   string `json:"ble_mac"`
	WifiMac  string `json:"wifi_mac"`
	WifiName string `json:"wifi_name"`
	IP       string `json:"ip"`
	// Sender is the source address of the discovery reply (not part of the
	// payload). The payload IP is whatever the device chose to write; Sender
	// is where it actually answers.
	Sender net.IP `json:"-"`
	// Src is the reply envelope's "src" (not part of the payload): the station's
	// model code and BLE MAC, e.g. «VNSE3-0-0a1b2c3d4e5f»; "" if absent.
	Src string `json:"-"`
}

type request struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type response struct {
	ID     json.RawMessage `json:"id"`
	Src    string          `json:"src"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

// Client sends requests over a shared UDP socket. Calls are serialized.
type Client struct {
	conn    net.PacketConn
	timeout time.Duration

	mu     sync.Mutex
	nextID int
	stray  func(from net.Addr, data []byte)
}

// SetStrayHandler registers fn to be called (inside Call, on the caller's goroutine)
// for every datagram received during a Call whose source is not the target address.
// Such datagrams are otherwise ignored. Pass nil to remove.
func (c *Client) SetStrayHandler(fn func(from net.Addr, data []byte)) {
	c.mu.Lock()
	c.stray = fn
	c.mu.Unlock()
}

// NewClient wraps conn; timeout bounds each Call.
func NewClient(conn net.PacketConn, timeout time.Duration) *Client {
	return &Client{conn: conn, timeout: timeout}
}

// LocalAddr returns the client's socket address.
func (c *Client) LocalAddr() net.Addr { return c.conn.LocalAddr() }

func sameAddr(a net.Addr, b *net.UDPAddr) bool {
	u, ok := a.(*net.UDPAddr)
	return ok && u.Port == b.Port && u.IP.Equal(b.IP)
}

func (c *Client) send(addr *net.UDPAddr, method string, params any) (int, error) {
	c.nextID++
	id := c.nextID
	b, err := json.Marshal(request{ID: id, Method: method, Params: params})
	if err != nil {
		return 0, err
	}
	_, err = c.conn.WriteTo(b, addr)
	return id, err
}

// armDeadline sets the read deadline and makes ctx cancellation interrupt reads.
// The returned func must be called when done.
func (c *Client) armDeadline(ctx context.Context, d time.Duration) func() {
	// ctx deadline/cancel is enforced via AfterFunc (not folded into the read
	// deadline) so ctx.Err() is already set when the read is interrupted.
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	stop := context.AfterFunc(ctx, func() { _ = c.conn.SetReadDeadline(time.Unix(1, 0)) })
	return func() { stop(); _ = c.conn.SetReadDeadline(time.Time{}) }
}

func idMatches(raw json.RawMessage, id int) bool {
	var n int
	return json.Unmarshal(raw, &n) == nil && n == id
}

// readErr maps a read failure to ctx error, ErrTimeout or the raw error.
func readErr(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ErrTimeout
	}
	return err
}

// Call sends one request and returns its result, a *RPCError, ErrTimeout or the context error.
// Only replies from addr whose id matches are accepted; a reply with id 0 carrying an error
// (firmware parse-error behaviour) from addr is treated as this call's error.
func (c *Client) Call(ctx context.Context, addr *net.UDPAddr, method string, params any) (json.RawMessage, error) {
	res, _, err := c.CallSrc(ctx, addr, method, params)
	return res, err
}

// CallSrc is Call that also returns the reply envelope's "src" ("" on an error).
func (c *Client) CallSrc(ctx context.Context, addr *net.UDPAddr, method string, params any) (json.RawMessage, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	id, err := c.send(addr, method, params)
	if err != nil {
		return nil, "", err
	}
	defer c.armDeadline(ctx, c.timeout)()
	buf := make([]byte, 64*1024)
	for {
		n, from, err := c.conn.ReadFrom(buf)
		if err != nil {
			return nil, "", readErr(ctx, err)
		}
		if !sameAddr(from, addr) {
			if c.stray != nil {
				c.stray(from, buf[:n])
			}
			continue
		}
		var r response
		if json.Unmarshal(buf[:n], &r) != nil {
			continue
		}
		if !idMatches(r.ID, id) && !(r.Error != nil && idMatches(r.ID, 0)) {
			continue
		}
		if r.Error != nil {
			return nil, "", r.Error
		}
		if len(r.Result) == 0 {
			return json.RawMessage("null"), r.Src, nil
		}
		return r.Result, r.Src, nil
	}
}

// Discover sends Marstek.GetDevice to bcast and collects every device that answers within wait.
func (c *Client) Discover(ctx context.Context, bcast *net.UDPAddr, wait time.Duration) ([]DeviceInfo, error) {
	return c.DiscoverMulti(ctx, []*net.UDPAddr{bcast}, wait)
}

// DiscoverMulti is Discover for several target addresses (a real broadcast reaches many
// devices with one datagram; tests on loopback address each fake station instead).
// Replies from any sender are accepted if the id matches; duplicates by BLEMac are dropped.
// DeviceInfo.Sender is the address each reply came from.
//
// A target that cannot be sent to is skipped; DiscoverMulti fails only if every target fails.
func (c *Client) DiscoverMulti(ctx context.Context, targets []*net.UDPAddr, wait time.Duration) ([]DeviceInfo, error) {
	devs, _, err := c.DiscoverTargets(ctx, targets, wait)
	return devs, err
}

// DiscoverTargets is DiscoverMulti that also reports the targets whose send failed,
// keyed by the target's String().
func (c *Client) DiscoverTargets(ctx context.Context, targets []*net.UDPAddr, wait time.Duration) ([]DeviceInfo, map[string]error, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	params := map[string]string{"ble_mac": "0"}
	ids := map[int]bool{}
	failed := map[string]error{}
	var lastErr error
	for _, t := range targets {
		id, err := c.send(t, "Marstek.GetDevice", params)
		if err != nil {
			failed[t.String()] = err
			lastErr = err
			continue
		}
		ids[id] = true
	}
	if len(ids) == 0 {
		if lastErr == nil {
			lastErr = errors.New("lan: no discovery targets")
		}
		return nil, failed, lastErr
	}
	defer c.armDeadline(ctx, wait)()
	var out []DeviceInfo
	seen := map[string]bool{}
	buf := make([]byte, 64*1024)
	for {
		n, from, err := c.conn.ReadFrom(buf)
		if err != nil {
			if err = readErr(ctx, err); errors.Is(err, ErrTimeout) {
				return out, failed, nil
			}
			return out, failed, err
		}
		var r response
		if json.Unmarshal(buf[:n], &r) != nil || r.Error != nil {
			continue
		}
		var id int
		if json.Unmarshal(r.ID, &id) != nil || !ids[id] {
			continue
		}
		var d DeviceInfo
		if json.Unmarshal(r.Result, &d) != nil {
			continue
		}
		if u, ok := from.(*net.UDPAddr); ok {
			d.Sender = u.IP
		}
		d.Src = r.Src
		key := d.BLEMac
		if key == "" {
			key = d.WifiMac + "|" + d.IP
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
}
