package app

import (
	"sync"
	"testing"
)

type dockCall struct {
	p    dockPolicy
	icon appIcon
}

type dockRecorder struct {
	mu    sync.Mutex
	calls []dockCall
}

func (r *dockRecorder) apply(p dockPolicy, i appIcon) bool {
	r.mu.Lock()
	r.calls = append(r.calls, dockCall{p, i})
	r.mu.Unlock()
	return true
}

func (r *dockRecorder) take() []dockCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.calls
	r.calls = nil
	return c
}

// The Dock icon is there exactly while the window is open (macOS port,
// owner decision (a)); a grid change while it is hidden is kept for the
// next open, and nothing is applied before the app runs.
func TestDockPolicy(t *testing.T) {
	if dockFor(true) != dockShown || dockFor(false) != dockHidden {
		t.Fatalf("dockFor: true→%v false→%v", dockFor(true), dockFor(false))
	}

	var r dockRecorder
	d := newDock(r.apply)
	d.setOpen(true) // before the app runs: recorded only
	d.setIcon(iconOff)
	if c := r.take(); len(c) != 0 {
		t.Fatalf("applied before start: %v", c)
	}
	d.start()
	if c := r.take(); len(c) != 1 || c[0] != (dockCall{dockShown, iconOff}) {
		t.Fatalf("start: %v, want shown/off", c)
	}

	d.setIcon(iconOn) // grid back while the window is open: the icon changes
	if c := r.take(); len(c) != 1 || c[0] != (dockCall{dockShown, iconOn}) {
		t.Fatalf("grid on while open: %v", c)
	}
	d.setIcon(iconOn) // same state: nothing to do
	if c := r.take(); len(c) != 0 {
		t.Fatalf("repeated icon applied again: %v", c)
	}

	d.setOpen(false) // closed to the menu bar
	if c := r.take(); len(c) != 1 || c[0].p != dockHidden {
		t.Fatalf("close: %v, want hidden", c)
	}
	d.setIcon(iconOff) // grid lost while hidden: recorded, the Dock stays hidden
	if c := r.take(); len(c) != 0 {
		t.Fatalf("grid change while hidden applied: %v", c)
	}
	d.setOpen(false)
	if c := r.take(); len(c) != 0 {
		t.Fatalf("second close applied again: %v", c)
	}

	d.setOpen(true) // opened from the menu bar: the Dock shows the latest icon
	if c := r.take(); len(c) != 1 || c[0] != (dockCall{dockShown, iconOff}) {
		t.Fatalf("open after hidden change: %v, want shown/off", c)
	}
}

// A state the Dock could not show (no image) is not remembered: the next
// change tries it again instead of leaving the old icon up.
func TestDockRetriesUnshownState(t *testing.T) {
	var calls []dockCall
	ok := false
	d := newDock(func(p dockPolicy, i appIcon) bool {
		calls = append(calls, dockCall{p, i})
		return ok || p == dockHidden
	})
	d.setOpen(true)
	d.start() // shown/on fails
	d.setIcon(iconOn)
	if len(calls) != 2 {
		t.Fatalf("calls %v: an unshown state was taken as shown", calls)
	}
	ok = true
	d.setIcon(iconOn)
	d.setIcon(iconOn) // now shown: no more calls
	if len(calls) != 3 || calls[2] != (dockCall{dockShown, iconOn}) {
		t.Fatalf("calls %v, want 3 ending in shown/on", calls)
	}
}
