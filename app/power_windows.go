package app

// watchSystem: on Windows the events come through WndProc, which main.go
// installs as the WndProcInterceptor.
func (p *PowerEvents) watchSystem() {}
