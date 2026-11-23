package app

import (
	"os"
	"os/exec"
	"path/filepath"
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
	want := `anchor apple generic and certificate leaf[subject.OU] = "ABCDE12345"`
	if got := teamRequirement("ABCDE12345"); got != want {
		t.Fatalf("%s", got)
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
