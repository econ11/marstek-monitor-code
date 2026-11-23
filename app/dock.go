package app

import "sync"

// The macOS Dock icon (macOS port, owner decision (a)): the app lives in the
// menu bar, and the Dock icon is there only while the window is open. While
// shown it carries the grid state like the Windows taskbar button
// (iconForGrid). The rule is here so it is tested on every platform;
// setDock applies it on macOS and does nothing elsewhere.

type dockPolicy int

const (
	dockHidden dockPolicy = iota // accessory: no Dock icon, no app menu
	dockShown                    // regular: Dock icon with the grid state
)

// dockFor is the Dock policy for the window's visibility.
func dockFor(windowVisible bool) dockPolicy {
	if windowVisible {
		return dockShown
	}
	return dockHidden
}

// dock tracks the window's visibility and the grid icon and hands each
// change that shows on screen to apply, in order. Before start (the app not
// running yet) it only records.
type dock struct {
	mu      sync.Mutex
	apply   func(dockPolicy, appIcon)
	open    bool
	icon    appIcon
	started bool
	// What apply last showed, to skip repeats.
	shown     dockPolicy
	shownIcon appIcon
}

func newDock(apply func(dockPolicy, appIcon)) *dock {
	return &dock{apply: apply, icon: iconOn}
}

// start applies the recorded state once the app runs.
func (d *dock) start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.started = true
	d.push(true)
}

// setOpen records whether the window is open. Any goroutine.
func (d *dock) setOpen(open bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.open = open
	d.push(false)
}

// setIcon records the grid icon; it shows only while the window is open.
// Any goroutine.
func (d *dock) setIcon(i appIcon) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.icon = i
	d.push(false)
}

// push calls apply when what the Dock shows changes (always when force).
// apply runs under d.mu, so calls keep their order.
func (d *dock) push(force bool) {
	if !d.started {
		return
	}
	p := dockFor(d.open)
	if !force && p == d.shown && (p == dockHidden || d.icon == d.shownIcon) {
		return
	}
	d.apply(p, d.icon)
	d.shown, d.shownIcon = p, d.icon
}
