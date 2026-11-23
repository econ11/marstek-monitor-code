package app

import "sync"

// NavigateService hands a toast click's tab (Navigate) to the frontend. A
// click can come before the frontend listens for NavigateEvent: started with
// --background the window has no page yet, and Windows starts a stopped app
// for the click. Such a route waits here until the frontend calls Ready, and
// is delivered once; later clicks go out as NavigateEvent at once.
type NavigateService struct {
	// Emit sends NavigateEvent; main sets it before the app runs.
	Emit func(Navigate)

	mu      sync.Mutex
	ready   bool
	pending *Navigate // the newest click before Ready
}

// Push delivers the route of a toast click, or keeps it (the newest one
// only) until Ready. Any goroutine.
//
//wails:ignore
func (s *NavigateService) Push(n Navigate) {
	s.mu.Lock()
	if !s.ready || s.Emit == nil {
		s.pending = &n
		s.mu.Unlock()
		return
	}
	emit := s.Emit
	s.mu.Unlock()
	emit(n)
}

// Ready is called by the frontend once it listens for NavigateEvent. It
// returns the route of a click made before (nil if none), only once.
// Ready is never reset: after a WebView reload (dev only; the app never
// reloads its page) a click before the new page listens is emitted and lost.
// Accepted for v1.
func (s *NavigateService) Ready() *Navigate {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	p := s.pending
	s.pending = nil
	return p
}
