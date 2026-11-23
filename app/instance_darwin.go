package app

/*
#include <unistd.h>
#include <limits.h>

// userTempDir writes the per-user temporary folder
// (/var/folders/…/T/) into buf; 0 when it is unavailable.
static size_t userTempDir(char *buf, size_t n) {
	return confstr(_CS_DARWIN_USER_TEMP_DIR, buf, n);
}
*/
import "C"

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Instance is the single-instance guard on macOS: an flock'ed file plus a
// Unix socket the second copy connects to, to make the first one show its
// window (flows 5.2). LaunchServices already brings a running .app to the
// front when it is opened again; this covers the binary started directly
// (the login item, a terminal, -simulate runs).
type Instance struct {
	ln   net.Listener
	lock *os.File
}

// held keeps the first instance reachable for the life of the process: the
// lock is an open file, and a collected *os.File would close it (and drop
// the lock) through its finalizer.
var held *Instance

// bundleID names the fallback folder under ~/Library/Caches.
const bundleID = "com.econ11.marstekmonitor"

// instanceDir holds the lock and the socket (a var so tests can use a
// throw-away folder).
var instanceDir = userInstanceDir

// userInstanceDir is the per-user temporary folder from
// confstr(_CS_DARWIN_USER_TEMP_DIR), else ~/Library/Caches/<bundle id>/.
// Never $TMPDIR or /tmp: a copy started by launchd at login may have no
// $TMPDIR, and two copies looking at different lock files would both run.
func userInstanceDir() (string, error) {
	if d := darwinUserTempDir(); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Caches", bundleID)
	return dir, os.MkdirAll(dir, 0o700)
}

// darwinUserTempDir is confstr(_CS_DARWIN_USER_TEMP_DIR), "" if unavailable.
func darwinUserTempDir() string {
	buf := make([]byte, C.PATH_MAX)
	n := C.userTempDir((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if n == 0 || int(n) > len(buf) {
		return ""
	}
	return filepath.Clean(C.GoString((*C.char)(unsafe.Pointer(&buf[0]))))
}

// FirstInstance takes the lock `<dir>/<name>.lock`. When another copy
// already holds it, that copy is asked to show its window and ok is false:
// the caller must exit. The lock lives until the process ends.
func FirstInstance(name string) (inst *Instance, ok bool, err error) {
	dir, err := instanceDir()
	if err != nil {
		return nil, false, err
	}
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
		slog.Warn("single instance: no socket, a second copy cannot show this window", "err", err)
		ln = nil
	}
	held = &Instance{ln: ln, lock: f}
	return held, true, nil
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
