package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
void marstekObservePower(void);
*/
import "C"

import "sync"

// Wails has events.Mac.ApplicationWillSleep / DidWake, but runs each handler
// on a fresh goroutine, with the same reordering risk as on Windows
// (power.go). The NSWorkspace observers feed the queue directly instead.

var (
	powerMu      sync.Mutex
	powerTargets []*PowerEvents
	powerOnce    sync.Once
)

// watchSystem subscribes p to the system's sleep / wake notifications.
func (p *PowerEvents) watchSystem() {
	powerMu.Lock()
	powerTargets = append(powerTargets, p)
	powerMu.Unlock()
	powerOnce.Do(func() { C.marstekObservePower() })
}

//export marstekPowerEvent
func marstekPowerEvent(sleep C.int) {
	powerMu.Lock()
	defer powerMu.Unlock()
	for _, p := range powerTargets {
		p.post(sleep != 0) // never blocks the main thread
	}
}
