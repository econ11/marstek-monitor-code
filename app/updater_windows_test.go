package app

import (
	"slices"
	"strings"
	"testing"
)

// The installer gets InstallerArgs and the running exe in PrevExeEnv, so a
// failed update can start the caller again even when it was a hand-copied
// exe outside the install folder (the installer never modifies it).
func TestInstallerCmd(t *testing.T) {
	setup := `C:\Users\u\AppData\Local\Temp\MarstekMonitor-update\marstek-monitor-1.2.3-setup.exe`
	prev := `C:\Users\u\marstek test\marstek-monitor.exe`
	cmd := installerCmd(setup, prev)
	if want := append([]string{setup}, InstallerArgs...); !slices.Equal(cmd.Args, want) {
		t.Fatalf("Args = %q; want %q", cmd.Args, want)
	}
	if cmd.Dir != `C:\Users\u\AppData\Local\Temp\MarstekMonitor-update` {
		t.Fatalf("Dir = %q", cmd.Dir)
	}
	if !slices.Contains(cmd.Env, PrevExeEnv+"="+prev) {
		t.Fatalf("Env has no %s=%s", PrevExeEnv, prev)
	}
	if len(cmd.Env) < 2 {
		t.Fatalf("Env = %q; want the inherited environment too", cmd.Env)
	}
	// An inherited value (this app was itself started by an installer) is
	// replaced, never doubled. No exe known: the variable is not set (the
	// installer then relaunches only a previously installed exe).
	t.Setenv(PrevExeEnv, `C:\stale.exe`)
	if n := countEnv(installerCmd(setup, prev).Env); n != 1 {
		t.Fatalf("%s set %d times; want 1", PrevExeEnv, n)
	}
	if n := countEnv(installerCmd(setup, "").Env); n != 0 {
		t.Fatalf("Env has %s with no exe", PrevExeEnv)
	}
}

func countEnv(env []string) (n int) {
	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), strings.ToUpper(PrevExeEnv)+"=") {
			n++
		}
	}
	return n
}
