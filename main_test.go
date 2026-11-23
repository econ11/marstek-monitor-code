package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		quiet   bool
		first   bool
		bg, sim bool
		simDir  string
	}{
		{args: nil},
		{args: []string{"--background"}, bg: true},
		{args: []string{"-simulate-quiet"}, sim: true, quiet: true},
		{args: []string{"-simulate", "-background"}, bg: true, sim: true},
		{args: []string{"-Embedding", "eyJhIjoxfQ=="}}, // toast activation of a stopped app
		{args: []string{"-simulate-dir", `C:\tmp\sim`, "--background"}, bg: true, sim: true, simDir: `C:\tmp\sim`},
		{args: []string{"-simulate-dir"}}, // no folder given: ignored
		{args: []string{"-simulate-quiet", "-simulate-dir", "sim"}, sim: true, quiet: true, simDir: "sim"},
		{args: []string{"-simulate-firstrun"}, sim: true, first: true},
		{args: []string{"-simulate-quiet", "-simulate-firstrun", "-simulate-dir", "sim"}, sim: true, quiet: true, first: true, simDir: "sim"},
		{args: []string{"-simulate-e2e"}, sim: true, first: true}, // the scripted first run is a first run
	} {
		bg, sim, quiet, first, simDir := parseArgs(tc.args)
		if bg != tc.bg || sim != tc.sim || quiet != tc.quiet || first != tc.first || simDir != tc.simDir {
			t.Errorf("parseArgs(%v) = %v, %v, %v, %v, %q; want %v, %v, %v, %v, %q", tc.args, bg, sim, quiet, first, simDir, tc.bg, tc.sim, tc.quiet, tc.first, tc.simDir)
		}
	}
}

// The frontend's scripted first run (frontend/src/views/firstRunE2E.ts) is on
// only with -simulate-e2e: the real app and the other -simulate runs never
// get the flag.
func TestFrontendFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		e2e  bool
	}{
		{nil, false},
		{[]string{"-simulate-firstrun"}, false},
		{[]string{"-simulate"}, false},
		{[]string{"--simulate-e2e", "-simulate-quiet"}, true},
	} {
		f := frontendFlags(tc.args)
		if _, on := f["e2e"]; on != tc.e2e {
			t.Errorf("frontendFlags(%v) = %v; want e2e %v", tc.args, f, tc.e2e)
		}
		if tc.e2e && f["e2e"] != "firstrun" {
			t.Errorf("frontendFlags(%v) = %v; want e2e=firstrun", tc.args, f)
		}
	}
}

func TestSimSilent(t *testing.T) {
	for _, tc := range []struct {
		at   time.Duration
		want bool
	}{
		{0, false},
		{simSilentAfter - time.Second, false},
		{simSilentAfter, true},
		{simSilentAfter + simSilentFor - time.Second, true},
		{simSilentAfter + simSilentFor, false},
		{time.Hour, false},
	} {
		if got := simSilent(tc.at); got != tc.want {
			t.Errorf("simSilent(%v) = %v, want %v", tc.at, got, tc.want)
		}
	}
}

func TestDataDirKeepsSimulateDir(t *testing.T) {
	want := filepath.Join(t.TempDir(), "sim")
	dir, cleanup, err := dataDir(true, want)
	if err != nil || dir != want {
		t.Fatalf("dataDir = %q, %v; want %q", dir, err, want)
	}
	cleanup()
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the folder must survive cleanup: %v", err)
	}
}

func TestSimStatus(t *testing.T) {
	for _, tc := range []struct {
		at                   time.Duration
		soc, ongrid, offgrid float64
	}{
		{0, 95, 0, 0},
		{59 * time.Second, 95, 0, 0},
		{time.Minute, 95, 0, 400}, // grid off
		{2*time.Minute + 59*time.Second, 92, 0, 400},
		{3 * time.Minute, 91, -2492, 0}, // grid back, charging
		{5*time.Minute + 59*time.Second, 99, -2492, 0},
		{6 * time.Minute, 100, 0, 0}, // full, idle
		{time.Hour, 100, 0, 0},
	} {
		soc, on, off := simStatus(tc.at)
		if soc != tc.soc || on != tc.ongrid || off != tc.offgrid {
			t.Errorf("simStatus(%v) = %v, %v, %v; want %v, %v, %v", tc.at, soc, on, off, tc.soc, tc.ongrid, tc.offgrid)
		}
	}
}

// Ruling 35: a -simulate* run never touches the owner's Run entry, neither
// the first-start enable nor the repoint to the installed exe.
func TestAutostartAtStartSkipsSimulate(t *testing.T) {
	old := repointAutostart
	t.Cleanup(func() { repointAutostart = old })
	called := false
	repointAutostart = func() { called = true }
	autostartAtStart(nil, true) // a nil store: enableAutostartOnce must not run either
	if called {
		t.Fatal("repointAutostart called in a simulated run")
	}
}
