package app

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

// The PC froze before "suspend" was handled: at wake both events are waiting.
// They must still be handled as suspend, then resume, every time.
func TestPowerEventsKeepOrder(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []string
	)
	gate := make(chan struct{}) // the handler cannot run until the "wake"
	done := make(chan struct{}, 8)
	note := func(s string) func() {
		return func() {
			<-gate
			mu.Lock()
			calls = append(calls, s)
			mu.Unlock()
			done <- struct{}{}
		}
	}
	p := NewPowerEvents(note("suspend"), note("resume"))

	const (
		pbtResumeSuspend = 7  // user-triggered resume: ignored, AUTOMATIC already came
		pbtPowerStatus   = 10 // battery / AC change: not ours
	)
	for _, w := range []uintptr{pbtAPMSuspend, pbtAPMResumeAutomatic, pbtResumeSuspend, pbtPowerStatus,
		pbtAPMSuspend, pbtAPMResumeAutomatic} {
		if ret, handled := p.WndProc(0, wmPowerBroadcast, w, 0); ret != 0 || handled {
			t.Fatalf("WndProc consumed the message (wParam %d)", w)
		}
	}
	p.WndProc(0, 0x0010 /* WM_CLOSE */, pbtAPMSuspend, 0) // other messages are not power events

	close(gate)
	for i := 0; i < 4; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("power events were not handled")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"suspend", "resume", "suspend", "resume"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("handled %v, want %v", calls, want)
	}
}
