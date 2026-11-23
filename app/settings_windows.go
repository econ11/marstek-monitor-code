package app

import (
	"os/exec"
	"path/filepath"
	"syscall"
)

// notificationSettingsURL opens Settings → System → Notifications.
const notificationSettingsURL = "ms-settings:notifications"

// localAbsPath: an absolute path on a local drive letter (no UNC or \\?\
// path: the check itself would reach the network).
func localAbsPath(path string) bool {
	vol := filepath.VolumeName(path)
	return len(vol) == 2 && vol[1] == ':' && filepath.IsAbs(path)
}

// revealFile opens Explorer with the file at p selected.
func revealFile(p string) error {
	// explorer wants /select,"<path>" as one argument with the quotes
	// inside it, which exec's own quoting does not produce.
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // explorer's exit code means nothing
	return nil
}
