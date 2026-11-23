package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The lock folder is the per-user temp folder, never /tmp, and does not
// depend on $TMPDIR (a login item started by launchd may have none).
func TestInstanceDirIgnoresTMPDIR(t *testing.T) {
	want, err := userInstanceDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"", "/tmp", "/private/tmp/elsewhere"} {
		t.Setenv("TMPDIR", v)
		got, err := userInstanceDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("TMPDIR=%q: dir %q, want %q", v, got, want)
		}
		for _, bad := range []string{"/tmp", "/private/tmp"} {
			if got == bad || strings.HasPrefix(got, bad+"/") {
				t.Errorf("TMPDIR=%q: dir %q is under %s", v, got, bad)
			}
		}
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Fatalf("dir %q: %v", want, err)
	}
}

// A second FirstInstance with the same name sees the lock, reaches the
// first copy through the socket and gets ok == false.
func TestFirstInstanceSecondLaunch(t *testing.T) {
	dir := t.TempDir()
	old, oldHeld := instanceDir, held
	instanceDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { instanceDir, held = old, oldHeld })

	first, ok, err := FirstInstance("T")
	if err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	if held != first {
		t.Fatal("first instance is not held for the life of the process")
	}
	shown := make(chan struct{}, 1)
	first.OnSecondLaunch(func() { shown <- struct{}{} })
	defer first.ln.Close()
	defer first.lock.Close()

	if _, ok, err := FirstInstance("T"); err != nil || ok {
		t.Fatalf("second: ok=%v err=%v, want false, nil", ok, err)
	}
	select {
	case <-shown:
	case <-time.After(5 * time.Second):
		t.Fatal("the first copy was not asked to show its window")
	}
}

// Without confstr the lock folder is ~/Library/Caches/<bundle id>/, owner
// only, also when it already existed with wider rights.
func TestInstanceDirFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := userTempDir
	userTempDir = func() string { return "" }
	t.Cleanup(func() { userTempDir = old })

	want := filepath.Join(home, "Library", "Caches", bundleID)
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(want, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := userInstanceDir()
	if err != nil || got != want {
		t.Fatalf("dir %q, %v; want %q", got, err, want)
	}
	fi, err := os.Stat(got)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v, %v; want 0700", fi.Mode().Perm(), err)
	}
	// A fresh home: created as 0700.
	t.Setenv("HOME", t.TempDir())
	got, err = userInstanceDir()
	if fi, _ := os.Stat(got); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("new dir %q: %v", got, err)
	}
}

// A socket path over the sun_path limit is shortened the same way for every
// copy; one that cannot fit gives "" (no socket, the lock still works).
func TestSockPath(t *testing.T) {
	if got := sockPath("/a", "MarstekMonitor"); got != "/a/MarstekMonitor.sock" {
		t.Errorf("short dir: %q", got)
	}
	long := "/" + strings.Repeat("d", 85)
	got := sockPath(long, "MarstekMonitorSimulate")
	if len(got) > maxSockPath || !strings.HasPrefix(got, long+"/mm-") || !strings.HasSuffix(got, ".sock") {
		t.Errorf("long dir: %q (%d bytes)", got, len(got))
	}
	if again := sockPath(long, "MarstekMonitorSimulate"); again != got {
		t.Errorf("not stable: %q vs %q", again, got)
	}
	if other := sockPath(long, "MarstekMonitor"); other == got {
		t.Errorf("two names share a socket: %q", other)
	}
	if got := sockPath("/"+strings.Repeat("d", 110), "x"); got != "" {
		t.Errorf("too long: %q, want \"\"", got)
	}
}

// A second copy with a shortened socket still reaches the first.
func TestFirstInstanceLongDir(t *testing.T) {
	base, err := os.MkdirTemp("", "mm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	// The full name must not fit (> 103 bytes) and the short one must.
	const name = "MarstekMonitorSimulate"
	want := maxSockPath - len("/mm-12345678.sock") // the longest dir for the short name
	if len(base)+2 > want {
		t.Skip("temp folder too long for this test:", base)
	}
	dir := filepath.Join(base, strings.Repeat("d", want-len(base)-1))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if n := len(filepath.Join(dir, name+".sock")); n <= maxSockPath {
		t.Fatalf("setup: full socket path %d bytes fits", n)
	}
	old, oldHeld := instanceDir, held
	instanceDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { instanceDir, held = old, oldHeld })
	first, ok, err := FirstInstance("MarstekMonitorSimulate")
	if err != nil || !ok || first.ln == nil {
		t.Fatalf("first: ok=%v err=%v listener=%v", ok, err, first != nil && first.ln != nil)
	}
	defer first.ln.Close()
	defer first.lock.Close()
	shown := make(chan struct{}, 1)
	first.OnSecondLaunch(func() { shown <- struct{}{} })
	if _, ok, err := FirstInstance("MarstekMonitorSimulate"); err != nil || ok {
		t.Fatalf("second: ok=%v err=%v", ok, err)
	}
	select {
	case <-shown:
	case <-time.After(5 * time.Second):
		t.Fatal("the first copy was not asked to show its window")
	}
}
