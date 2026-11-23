package app

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// Instance is the single-instance guard on macOS: an flock'ed file plus a
// Unix socket the second copy connects to, to make the first one show its
// window (flows 5.2). LaunchServices already brings a running .app to the
// front when it is opened again; this covers the binary started directly
// (autostart, a terminal, -simulate runs).
type Instance struct {
	ln   net.Listener
	lock *os.File
}

// instanceDir holds the lock and the socket: the per-user $TMPDIR (a var so
// tests can use a throw-away folder).
var instanceDir = os.TempDir

// FirstInstance takes the lock `<TMPDIR>/<name>.lock`. When another copy
// already holds it, that copy is asked to show its window and ok is false:
// the caller must exit. The lock lives until the process ends.
func FirstInstance(name string) (inst *Instance, ok bool, err error) {
	dir := instanceDir()
	sock := filepath.Join(dir, name+".sock")
	f, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, err
		}
		if c, err := net.Dial("unix", sock); err == nil {
			_ = c.Close() // connecting is the whole message
		}
		return nil, false, nil
	}
	// We hold the lock: a socket file left over is from a copy that died.
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		// Still the only copy; a second one just cannot reach us.
		slog.Warn("single instance: no socket", "err", err)
	}
	return &Instance{ln: ln, lock: f}, true, nil
}

// OnSecondLaunch calls fn each time another copy of the app is started.
func (i *Instance) OnSecondLaunch(fn func()) {
	if i.ln == nil {
		return
	}
	go func() {
		for {
			c, err := i.ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
			fn()
		}
	}()
}
