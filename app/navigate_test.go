package app

import "testing"

// Task 20c (Task 7 carry-over): a toast click before the frontend listens
// (--background, or Windows starting the app for the click) is not lost: the
// newest route waits for Ready and is delivered once; after Ready a click is
// emitted at once.
func TestNavigateQueuedUntilFrontendReady(t *testing.T) {
	var sent []Navigate
	s := &NavigateService{Emit: func(n Navigate) { sent = append(sent, n) }}

	s.Push(Navigate{View: "overview"})
	s.Push(Navigate{View: "outages", OutageID: 42}) // the newest click wins
	if len(sent) != 0 {
		t.Fatalf("emitted before the frontend was ready: %+v", sent)
	}
	got := s.Ready()
	if got == nil || *got != (Navigate{View: "outages", OutageID: 42}) {
		t.Fatalf("Ready = %+v, want the newest click (outages 42)", got)
	}
	if again := s.Ready(); again != nil {
		t.Fatalf("second Ready = %+v, want nil (delivered once)", again)
	}
	s.Push(Navigate{View: "settings", Section: "notifications"})
	if len(sent) != 1 || sent[0] != (Navigate{View: "settings", Section: "notifications"}) {
		t.Fatalf("after Ready: emitted %+v, want the click at once", sent)
	}
	if p := s.Ready(); p != nil {
		t.Fatalf("an emitted click was also kept: %+v", p)
	}
}

func TestNavigateReadyWithoutClick(t *testing.T) {
	s := &NavigateService{Emit: func(Navigate) {}}
	if p := s.Ready(); p != nil {
		t.Fatalf("Ready = %+v, want nil", p)
	}
}
