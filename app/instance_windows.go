package app

import (
	"golang.org/x/sys/windows"
)

// Instance is the single-instance guard: a named mutex plus an event the
// second copy signals to make the first one show its window (flows 5.2).
type Instance struct {
	event windows.Handle
}

// FirstInstance takes the mutex `Local\<name>`. When another copy already
// holds it, that copy is asked to show its window and ok is false: the caller
// must exit. The handles live until the process ends.
func FirstInstance(name string) (inst *Instance, ok bool, err error) {
	mutexName, err := windows.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, false, err
	}
	eventName, err := windows.UTF16PtrFromString(`Local\` + name + `.show`)
	if err != nil {
		return nil, false, err
	}
	_, mErr := windows.CreateMutex(nil, false, mutexName)
	if mErr != nil && mErr != windows.ERROR_ALREADY_EXISTS {
		return nil, false, mErr
	}
	// Auto-reset event; CreateEvent opens it if the other copy made it first.
	ev, eErr := windows.CreateEvent(nil, 0, 0, eventName)
	if ev == 0 {
		return nil, false, eErr
	}
	if mErr == windows.ERROR_ALREADY_EXISTS {
		// This process was started by the user, so it may hand the right to
		// take the foreground to the first copy.
		_, _, _ = procAllowSetForegroundWindow.Call(asfwAny)
		err = windows.SetEvent(ev)
		_ = windows.CloseHandle(ev)
		return nil, false, err
	}
	return &Instance{event: ev}, true, nil
}

// OnSecondLaunch calls fn each time another copy of the app is started.
func (i *Instance) OnSecondLaunch(fn func()) {
	go func() {
		for {
			if r, err := windows.WaitForSingleObject(i.event, windows.INFINITE); err != nil || r != windows.WAIT_OBJECT_0 {
				return
			}
			fn()
		}
	}()
}

const asfwAny = ^uintptr(0) // ASFW_ANY

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")
