package app

import (
	"errors"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// A test binary has no bundle id: notifications are off without an error
// that would stop the app, and nothing reaches UNUserNotificationCenter.
func TestNotificationsWithoutBundle(t *testing.T) {
	if NotificationsAvailable() {
		t.Skip("this test binary has a bundle id")
	}
	AskNotificationPermission() // must return at once, without the prompt
	if off, err := ToastsOff(""); off || err != nil {
		t.Fatalf("ToastsOff = %v, %v; want false, nil (no «off in macOS» banner)", off, err)
	}
	n := NewNotifier()
	if err := n.push(notifications.NotificationOptions{ID: "x", Title: "t"}, "", "", toastPush{}); !errors.Is(err, errNoBundle) {
		t.Fatalf("push = %v, want errNoBundle", err)
	}
}
