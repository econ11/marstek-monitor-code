package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseTeamID(t *testing.T) {
	for name, tc := range map[string]struct{ out, want string }{
		"developer id": {"Executable=/A.app/Contents/MacOS/a\nIdentifier=com.x\nAuthority=Developer ID Application: X (ABCDE12345)\nTeamIdentifier=ABCDE12345\n", "ABCDE12345"},
		"ad-hoc":       {"Identifier=com.x\nSignature=adhoc\nTeamIdentifier=not set\n", ""},
		"not set":      {"Identifier=com.x\nTeamIdentifier=not set\n", ""},
		"junk team":    {"TeamIdentifier=abc\n", ""},
		"empty":        {"", ""},
	} {
		if got := parseTeamID([]byte(tc.out)); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

const attachPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>system-entities</key>
	<array>
		<dict>
			<key>content-hint</key><string>GUID_partition_scheme</string>
			<key>dev-entry</key><string>/dev/disk7</string>
		</dict>
		<dict>
			<key>content-hint</key><string>Apple_HFS</string>
			<key>dev-entry</key><string>/dev/disk7s1</string>
			<key>mount-point</key><string>/Volumes/Marstek Monitor</string>
		</dict>
	</array>
</dict>
</plist>`

func TestParseAttachPlist(t *testing.T) {
	mount, dev, err := parseAttachPlist([]byte(attachPlist))
	if err != nil || mount != "/Volumes/Marstek Monitor" || dev != "/dev/disk7" {
		t.Fatalf("mount %q dev %q err %v", mount, dev, err)
	}
	// Cut off: no mount point, but the disk to detach is still found.
	cut := attachPlist[:strings.Index(attachPlist, "<key>mount-point")]
	mount, dev, _ = parseAttachPlist([]byte(cut))
	if mount != "" {
		t.Fatalf("mount %q from a cut plist", mount)
	}
	if dev == "" {
		dev = firstDevEntry([]byte(cut))
	}
	if dev != "/dev/disk7" {
		t.Fatalf("dev %q, want /dev/disk7", dev)
	}
}

func TestTeamRequirement(t *testing.T) {
	want := `anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists` +
		` and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "ABCDE12345"`
	if got := teamRequirement("ABCDE12345"); got != want {
		t.Fatalf("%s", got)
	}
	// codesign must accept the text as a requirement (a syntax error is exit 1,
	// an unmet one exit 3): checked against an ad-hoc bundle.
	app := filepath.Join(t.TempDir(), "A.app")
	makeAdhocApp(t, app)
	err := exec.Command("/usr/bin/codesign", "--verify", "-R="+teamRequirement("ABCDE12345"), app).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("codesign with the requirement: %v, want exit 3 (requirement not met)", err)
	}
}

// makeAdhocApp builds a minimal ad-hoc signed bundle at path.
func makeAdhocApp(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>test.marstek.update</string>
<key>CFBundleExecutable</key><string>t</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "Contents", "MacOS", "t"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", path).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v %s", err, out)
	}
}

// The real tools on a real dmg: mount read-only, find the app, copy it with
// no quarantine, read its (empty) Team ID, refuse to verify it, detach.
func TestRealMacTools(t *testing.T) {
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("no hdiutil")
	}
	root := t.TempDir()
	src := filepath.Join(root, "src")
	app := filepath.Join(src, "Marstek Monitor.app")
	makeAdhocApp(t, app)
	// A quarantine flag on the source must not reach the copy.
	_ = exec.Command("/usr/bin/xattr", "-w", "com.apple.quarantine", "0081;00000000;test;", filepath.Join(app, "Contents", "Info.plist")).Run()
	dmg := filepath.Join(root, "u.dmg")
	if out, err := exec.Command("/usr/bin/hdiutil", "create", "-quiet", "-fs", "HFS+", "-volname", "MarstekTest",
		"-srcfolder", src, "-ov", dmg).CombinedOutput(); err != nil {
		t.Skipf("hdiutil create: %v %s", err, out)
	}

	var r realMac
	mount, err := r.Attach(dmg)
	if err != nil {
		t.Fatal(err)
	}
	detached := false
	t.Cleanup(func() {
		if !detached {
			_ = r.Detach(mount)
		}
	})
	if !strings.HasPrefix(mount, "/Volumes/") {
		t.Fatalf("mount %q", mount)
	}
	got, err := dmgApp(mount)
	if err != nil || filepath.Base(got) != "Marstek Monitor.app" {
		t.Fatalf("dmgApp = %q, %v", got, err)
	}
	if writableDir(mount) {
		t.Error("a read-only mount counts as writable")
	}

	dst := filepath.Join(root, "copy", "Marstek Monitor.app")
	if err := r.Copy(got, dst); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command("/usr/bin/xattr", "-r", "-l", dst).CombinedOutput(); strings.Contains(string(out), "com.apple.quarantine") {
		t.Errorf("the copy is quarantined:\n%s", out)
	}
	if team, err := r.TeamID(dst); err != nil || team != "" {
		t.Errorf("ad-hoc TeamID = %q, %v; want \"\", nil", team, err)
	}
	if err := r.Verify(dst); err == nil {
		t.Error("Verify passed with no team")
	}
	if err := (realMac{team: "ABCDE12345"}).Verify(dst); err == nil {
		t.Error("an ad-hoc app passed the Developer ID requirement")
	}
	if canSelfUpdate(realMac{}, dst, writableDir) {
		t.Error("an ad-hoc app may update itself")
	}

	// The swap refuses an ad-hoc running app before touching it.
	running := filepath.Join(root, "Applications", "Marstek Monitor.app")
	makeAdhocApp(t, running)
	if err := r.Detach(mount); err != nil {
		t.Fatal(err)
	}
	detached = true
	if _, err := macInstall(r, dmg, running); err == nil {
		t.Fatal("macInstall swapped an ad-hoc app")
	}
	if _, err := os.Stat(running + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old made: %v", err)
	}
	if !writableDir(filepath.Dir(running)) {
		t.Error("a temp folder is not writable")
	}
	out, _ := exec.Command("/usr/bin/hdiutil", "info").Output()
	if strings.Contains(string(out), dmg) {
		t.Errorf("the dmg is still attached:\n%s", out)
	}
}

// A bare test binary is not in a bundle: it can never update itself.
func TestCanInstallHereOutsideBundle(t *testing.T) {
	if canInstallHere() {
		t.Fatal("a test binary may update itself")
	}
	if _, err := runningBundle(); err == nil {
		t.Fatal("a test binary is inside a bundle?")
	}
}

// After the swap the new app gets this run's -simulate* flags back (the
// local update test must not restart on the real data folder); nothing else.
func TestRelaunchArgs(t *testing.T) {
	got := relaunchArgs([]string{"--background", "-simulate-quiet", "-simulate-dir", "/tmp/x y", "-psn_0_1"})
	want := []string{"-simulate-quiet", "-simulate-dir", "/tmp/x y"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q, want %q", got, want)
	}
	if got := relaunchArgs([]string{"--background"}); len(got) != 0 {
		t.Fatalf("a normal run: %q", got)
	}
}

// The relaunch script passes the flags through as separate arguments, opens
// even when the old process does not exit in time, and logs a failed open;
// checked with stand-ins for open and logger.
func TestRelaunchScript(t *testing.T) {
	run := func(pid string, ticks int, openExit int, flags ...string) (opened, logged string) {
		t.Helper()
		dir := t.TempDir()
		openOut, logOut := filepath.Join(dir, "open"), filepath.Join(dir, "log")
		// The stand-in for open is a shell function, so "$@" are its words.
		script := "openf() { printf '%s\\n' \"$@\" >" + openOut + "; return " + strconv.Itoa(openExit) + "; }\n" +
			strings.ReplaceAll(strings.ReplaceAll(relaunchScript, "/usr/bin/open", "openf"), "/usr/bin/logger -t marstek-monitor", "printf '%s\\n' >>"+logOut)
		args := append([]string{"-c", script, "x", pid, "/A b.app", strconv.Itoa(ticks)}, flags...)
		_ = exec.Command("/bin/sh", args...).Run()
		o, _ := os.ReadFile(openOut)
		l, _ := os.ReadFile(logOut)
		return string(o), string(l)
	}
	opened, logged := run("999999", 300, 0, "-simulate-quiet", "-simulate-dir", "/t m")
	if opened != "/A b.app\n--args\n-simulate-quiet\n-simulate-dir\n/t m\n" || logged != "" {
		t.Fatalf("flags: open %q, log %q", opened, logged)
	}
	// This test process never exits: after 2 ticks the app opens anyway.
	opened, logged = run(strconv.Itoa(os.Getpid()), 2, 0)
	if opened != "/A b.app\n" || !strings.Contains(logged, "still runs") {
		t.Fatalf("timeout: open %q, log %q", opened, logged)
	}
	_, logged = run("999999", 300, 1)
	if !strings.Contains(logged, "could not open") {
		t.Fatalf("failed open not logged: %q", logged)
	}
}
