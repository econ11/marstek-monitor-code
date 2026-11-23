package app

import "log/slog"

const (
	wmPowerBroadcast      = 0x0218
	pbtAPMSuspend         = 4
	pbtAPMResumeAutomatic = 18 // sent on every resume
)

// PowerEvents hands sleep and wake to the monitor in the order Windows sent
// them. Wails' own power events run each handler on a fresh goroutine: when
// the PC freezes before the "suspend" goroutine gets to run, both run at wake
// in either order, and "resume, then suspend" would leave the monitor asleep.
// Here the message loop itself queues the events and one goroutine handles
// them one by one.
type PowerEvents struct {
	queue chan bool // true = suspend, false = resume
}

// NewPowerEvents starts the handling goroutine. On Windows install WndProc as
// application.Options.Windows.WndProcInterceptor; on macOS it subscribes to
// the NSWorkspace sleep / wake notifications itself (power_darwin.go).
func NewPowerEvents(suspend, resume func()) *PowerEvents {
	p := &PowerEvents{queue: make(chan bool, 16)}
	go func() {
		for sleep := range p.queue {
			if sleep {
				suspend()
			} else {
				resume()
			}
		}
	}()
	p.watchSystem()
	return p
}

// WndProc sees every window message of the app, on the UI thread, in order;
// it only looks at WM_POWERBROADCAST and never consumes a message.
func (p *PowerEvents) WndProc(_ uintptr, msg uint32, wParam, _ uintptr) (uintptr, bool) {
	if msg == wmPowerBroadcast {
		switch wParam {
		case pbtAPMSuspend:
			p.post(true)
		case pbtAPMResumeAutomatic:
			p.post(false)
		}
	}
	return 0, false
}

// post never blocks the UI thread.
func (p *PowerEvents) post(sleep bool) {
	select {
	case p.queue <- sleep:
	default:
		slog.Warn("power: event queue full, dropped", "suspend", sleep)
	}
}
