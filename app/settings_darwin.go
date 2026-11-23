package app

import (
	"os/exec"
	"path/filepath"
)

// notificationSettingsURL opens System Settings → Notifications.
const notificationSettingsURL = "x-apple.systempreferences:com.apple.Notifications-Settings.extension"

// localAbsPath: an absolute path (an export lands in a folder the user
// picked; macOS has no drive letters or UNC paths).
func localAbsPath(path string) bool { return filepath.IsAbs(path) }

// revealFile opens Finder with the file at p selected.
func revealFile(p string) error {
	cmd := exec.Command("open", "-R", p)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
