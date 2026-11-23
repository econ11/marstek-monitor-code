package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// PrevExeEnv tells the installer which exe started the update. If the
// install fails, a /relaunch installer starts it again, so a user updating a
// hand-copied exe (outside the install folder) is not left without the app.
// The installer only runs it; it never stops, moves or deletes it
// (build/windows/nsis/project.nsi).
const PrevExeEnv = "MARSTEK_PREV_EXE"

// startInstaller runs the downloaded installer with InstallerArgs (silent,
// per-user, relaunch), detached from this process so it outlives the quit
// that follows. It does not wait for the installer.
func startInstaller(path string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "" // the installer then relaunches only an installed exe
	}
	cmd := installerCmd(path, exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// installerCmd is the installer command: InstallerArgs, run in its folder,
// with this environment and PrevExeEnv set to prevExe (unset when "").
func installerCmd(path, prevExe string) *exec.Cmd {
	cmd := exec.Command(path, InstallerArgs...)
	cmd.Dir = filepath.Dir(path)
	prefix := strings.ToUpper(PrevExeEnv) + "="
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), prefix) {
			cmd.Env = append(cmd.Env, e)
		}
	}
	if prevExe != "" {
		cmd.Env = append(cmd.Env, PrevExeEnv+"="+prevExe)
	}
	return cmd
}
