package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// useTempAutostartKey (the name the shared tests use) points the autostart
// code at a throw-away folder so a test never touches ~/Library/LaunchAgents.
func useTempAutostartKey(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := autostartDir
	autostartDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { autostartDir = old })
	return dir
}

func TestAutostartDarwin(t *testing.T) {
	dir := useTempAutostartKey(t)

	if on, err := AutostartEnabled(); err != nil || on {
		t.Fatalf("before: %v, %v", on, err)
	}
	exe := "/Applications/Marstek Monitor.app/Contents/MacOS/Marstek & Co"
	if err := SetAutostart(true, exe); err != nil {
		t.Fatal(err)
	}
	if on, err := AutostartEnabled(); err != nil || !on {
		t.Fatalf("after enable: %v, %v", on, err)
	}
	path := filepath.Join(dir, autostartLabel+".plist")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Marstek &amp; Co</string>") || !strings.Contains(string(b), "<string>--background</string>") {
		t.Fatalf("plist:\n%s", b)
	}
	// The system's own checker must accept the file.
	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v\n%s", err, out)
	}
	if err := SetAutostart(false, exe); err != nil {
		t.Fatal(err)
	}
	if err := SetAutostart(false, exe); err != nil { // already off: no error
		t.Fatal(err)
	}
	if on, _ := AutostartEnabled(); on {
		t.Fatal("still enabled after disable")
	}
}

// main repoints autostart at every start: on macOS that must never touch the
// LaunchAgent.
func TestRepointAutostartDarwinNoOp(t *testing.T) {
	dir := useTempAutostartKey(t)
	if d, err := InstalledDir(); d != "" || err != nil {
		t.Fatalf("InstalledDir = %q, %v; want \"\", nil", d, err)
	}
	if changed, err := RepointAutostart("/Applications/x.app/Contents/MacOS/x", "/Applications"); changed || err != nil {
		t.Fatalf("RepointAutostart = %v, %v; want false, nil", changed, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("LaunchAgents folder touched: %v", entries)
	}
}

// A translocated app or one on /Volumes/ gets no LaunchAgent (that path is
// gone after a restart or an eject); turning autostart off still works.
func TestAutostartMoveFirst(t *testing.T) {
	dir := useTempAutostartKey(t)
	for _, exe := range []string{
		"/private/var/folders/x/AppTranslocation/ABC/d/Marstek Monitor.app/Contents/MacOS/marstek-monitor",
		"/Volumes/Marstek Monitor/Marstek Monitor.app/Contents/MacOS/marstek-monitor",
	} {
		if err := SetAutostart(true, exe); !errors.Is(err, ErrAutostartMoveFirst) {
			t.Errorf("%s: %v, want ErrAutostartMoveFirst", exe, err)
		}
		if err := SetAutostart(false, exe); err != nil {
			t.Errorf("%s: turning off: %v", exe, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("LaunchAgent written: %v", entries)
	}
	if err := SetAutostart(true, "/Applications/Marstek Monitor.app/Contents/MacOS/marstek-monitor"); err != nil {
		t.Fatal(err)
	}
}
