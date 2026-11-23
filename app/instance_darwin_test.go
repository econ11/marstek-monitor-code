package app

import (
	"os"
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
