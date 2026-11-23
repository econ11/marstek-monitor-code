package app

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
)

// The Mac one-click update (Task 6): the real macTools for macInstall
// (macupdate.go), startInstaller and canInstallHere.

// realMac runs hdiutil, ditto and codesign. team is the running app's Team
// ID: Verify accepts only an app signed by it.
type realMac struct{ team string }

// toolOutput runs name with args and returns stdout; stderr goes into the
// error.
func toolOutput(name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Attach mounts dmg read-only, hidden from Finder, and returns its mount
// point. If hdiutil mounted it but its plist cannot be read, the image is
// detached again before the error.
func (realMac) Attach(dmg string) (string, error) {
	out, err := toolOutput("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-plist", dmg)
	if err != nil {
		return "", err
	}
	mount, dev, perr := parseAttachPlist(out)
	if perr == nil && mount != "" {
		return mount, nil
	}
	// Mounted, but where is unknown: do not leave it attached.
	if dev == "" {
		dev = firstDevEntry(out)
	}
	if dev != "" {
		if _, derr := toolOutput("/usr/bin/hdiutil", "detach", "-force", dev); derr != nil {
			slog.Warn("update: detach after an unreadable attach", "dev", dev, "err", derr)
		}
	}
	if perr == nil {
		perr = errors.New("no mount point")
	}
	return "", fmt.Errorf("hdiutil attach: %w", perr)
}

// Detach unmounts the image, forcing it if a plain detach fails.
func (realMac) Detach(mount string) error {
	if _, err := toolOutput("/usr/bin/hdiutil", "detach", mount); err != nil {
		if _, ferr := toolOutput("/usr/bin/hdiutil", "detach", "-force", mount); ferr != nil {
			return errors.Join(err, ferr)
		}
	}
	return nil
}

// Copy copies the bundle with ditto, with no quarantine flag: the download
// was verified, and a quarantined app would be translocated at its first
// start and could not update itself again. ditto --noqtn alone keeps a flag
// a file inside the image already had (checked on macOS 26), so the copy is
// cleared afterwards and checked.
func (realMac) Copy(src, dst string) error {
	if _, err := toolOutput("/usr/bin/ditto", "--noqtn", src, dst); err != nil {
		return err
	}
	// -d on a file without the attribute is an error; what counts is the check.
	_, _ = toolOutput("/usr/bin/xattr", "-r", "-d", "com.apple.quarantine", dst)
	out, err := toolOutput("/usr/bin/xattr", "-r", "-l", dst)
	if err != nil {
		return err
	}
	if bytes.Contains(out, []byte("com.apple.quarantine")) {
		return errors.New("ditto: the copy is still quarantined")
	}
	return nil
}

// teamIDPattern is an Apple Team ID: 10 upper-case letters and digits.
var teamIDPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// Verify checks the signature and, in the same call, that the app is signed
// by a Developer ID of the running app's team.
func (r realMac) Verify(app string) error {
	if !teamIDPattern.MatchString(r.team) {
		return fmt.Errorf("codesign: no team to verify against (%q)", r.team)
	}
	_, err := toolOutput("/usr/bin/codesign", "--verify", "--deep", "--strict",
		"-R="+teamRequirement(r.team), app)
	return err
}

// teamRequirement is the code requirement «a Developer ID Application
// certificate of team»: Apple's anchor, the Developer ID intermediate
// (1.2.840.113635.100.6.2.6), a Developer ID Application leaf
// (1.2.840.113635.100.6.1.13) and the team in its OU. An App Store or
// development certificate of the same team does not pass.
func teamRequirement(team string) string {
	return `anchor apple generic` +
		` and certificate 1[field.1.2.840.113635.100.6.2.6] exists` +
		` and certificate leaf[field.1.2.840.113635.100.6.1.13] exists` +
		` and certificate leaf[subject.OU] = "` + team + `"`
}

// TeamID is the app's Team ID from codesign -dv, "" when it is ad-hoc
// signed or not signed at all.
func (realMac) TeamID(app string) (string, error) {
	out, err := exec.Command("/usr/bin/codesign", "-dv", "--verbose=2", app).CombinedOutput()
	if err != nil {
		if bytes.Contains(out, []byte("not signed at all")) {
			return "", nil
		}
		return "", fmt.Errorf("codesign -dv: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseTeamID(out), nil
}

// parseTeamID reads TeamIdentifier from codesign -dv output; "" for an
// ad-hoc signature or «not set».
func parseTeamID(out []byte) string {
	adhoc, team := false, ""
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "TeamIdentifier":
			team = v
		case "Signature":
			adhoc = v == "adhoc"
		}
	}
	if adhoc || !teamIDPattern.MatchString(team) {
		return ""
	}
	return team
}

// parseAttachPlist reads the first mount-point and the whole-disk dev-entry
// (/dev/diskN) from `hdiutil attach -plist` output.
func parseAttachPlist(out []byte) (mount, dev string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(out))
	dec.Strict = false
	var key string
	var inKey, inString bool
	for {
		tok, terr := dec.Token()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			return mount, dev, fmt.Errorf("plist: %w", terr)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inKey, inString = t.Name.Local == "key", t.Name.Local == "string"
		case xml.EndElement:
			if t.Name.Local == "string" {
				key = ""
			}
			inKey, inString = false, false
		case xml.CharData:
			s := strings.TrimSpace(string(t))
			switch {
			case inKey:
				key = s
			case inString && key == "mount-point" && mount == "":
				mount = s
			case inString && key == "dev-entry" && wholeDisk.MatchString(s) && dev == "":
				dev = s
			}
		}
	}
	return mount, dev, nil
}

var (
	wholeDisk    = regexp.MustCompile(`^/dev/disk\d+$`)
	devEntryText = regexp.MustCompile(`/dev/disk\d+\b`)
)

// firstDevEntry finds a /dev/diskN in output that did not parse.
func firstDevEntry(out []byte) string {
	return string(devEntryText.Find(out))
}

// runningBundle is the .app the running executable is in
// (<X>.app/Contents/MacOS/<exe>), symlinks resolved.
func runningBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	app := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(app, ".app") {
		return "", fmt.Errorf("update: %s is not inside an app bundle", exe)
	}
	return app, nil
}

// writableDir: a file can be created in dir (a real write, not the mode
// bits: ACLs and read-only volumes count).
func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".marstek-write-test-")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// canInstallHere: this copy can replace itself (canSelfUpdate over the real
// tools): a Developer ID app, not translocated, not on /Volumes/, in a
// folder it can write. Otherwise only «Завантажити» is offered.
func canInstallHere() bool {
	app, err := runningBundle()
	if err != nil {
		return false
	}
	return canSelfUpdate(realMac{}, app, writableDir)
}

// swapped is set once an update replaced the bundle: this process now runs
// from <app>.old, which a second install would delete. Only a restart
// clears it.
var swapped atomic.Bool

// startInstaller installs the downloaded dmg over the running app
// (macInstall) and starts a detached shell that opens the new app once this
// process has exited; the caller then quits. A second call before the
// restart is refused.
func startInstaller(dmg string) error {
	if swapped.Load() {
		return errors.New("update: already installed, waiting for the restart")
	}
	app, err := runningBundle()
	if err != nil {
		return err
	}
	if !canSelfUpdate(realMac{}, app, writableDir) {
		return errors.New("update: this copy cannot replace itself")
	}
	team, err := realMac{}.TeamID(app)
	if err != nil {
		return err
	}
	newApp, err := macInstall(realMac{team: team}, dmg, app)
	if err != nil {
		return err
	}
	swapped.Store(true)
	// The pid, the path and the flags are arguments, never shell text. The
	// new app gets this run's -simulate* flags back (the local update test
	// must not restart on the real data folder); a normal run has none, and
	// the app opens as by a double click.
	args := append([]string{"marstek-relaunch", strconv.Itoa(os.Getpid()), newApp, strconv.Itoa(relaunchWaitTicks)},
		relaunchArgs(os.Args[1:])...)
	cmd := exec.Command("/bin/sh", append([]string{"-c", relaunchScript}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		// The new app is in place; it opens at the next start by hand.
		return fmt.Errorf("update: installed, but the relaunch did not start: %w", err)
	}
	return cmd.Process.Release()
}

// relaunchWaitTicks × 0.2 s is how long the relaunch waits for the old
// process: 60 s, then it opens the new app anyway.
const relaunchWaitTicks = 300

// relaunchScript waits for the old process ($1) to exit, at most $3 ticks of
// 0.2 s, then opens the new app ($2), with the remaining arguments as its
// flags when there are any. A wait that runs out or an open that fails goes
// to the system log (logger -t marstek-monitor; Console.app).
const relaunchScript = `pid=$1; app=$2; ticks=$3; shift 3
i=0
while kill -0 "$pid" 2>/dev/null; do
  if [ "$i" -ge "$ticks" ]; then
    /usr/bin/logger -t marstek-monitor "update: the old process $pid still runs after the wait; opening the new app anyway"
    break
  fi
  sleep 0.2; i=$((i+1))
done
if [ $# -gt 0 ]; then /usr/bin/open "$app" --args "$@"; else /usr/bin/open "$app"; fi
rc=$?
if [ "$rc" -ne 0 ]; then
  /usr/bin/logger -t marstek-monitor "update: could not open the new app $app (open exit $rc); start it by hand"
fi
exit "$rc"`

// relaunchArgs keeps the -simulate* flags of args (and the folder after
// -simulate-dir); everything else is dropped.
func relaunchArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch strings.TrimLeft(args[i], "-") {
		case "simulate", "simulate-quiet", "simulate-firstrun", "simulate-e2e":
			out = append(out, args[i])
		case "simulate-dir":
			if i+1 < len(args) {
				out = append(out, args[i], args[i+1])
				i++
			}
		}
	}
	return out
}

// cleanAfterUpdate removes what an update left beside the running app (the
// swapped-out <app>.old and other runs' temp folders). Called at start.
func cleanAfterUpdate() {
	app, err := runningBundle()
	if err != nil {
		return // a bare binary (dev): nothing to clean
	}
	if err := cleanOldBundle(app); err != nil {
		slog.Info("update: clean the old bundle", "err", err)
	}
}
