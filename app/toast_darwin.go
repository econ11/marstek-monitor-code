package app

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// On macOS the toasts go through the Wails notification service
// (UNUserNotificationCenter), which needs the app to run from a bundle with
// a bundle identifier (bin/Marstek Monitor.app). What the Windows toast does
// by hand (toastpush.go) maps onto UNNotificationRequest:
//
//   - Tag + Group → the request identifier: a later toast with the same pair
//     replaces the earlier one ("grid back" replaces "grid lost");
//   - Group → ThreadID (one stack per station in Notification Centre);
//   - SuppressPopup → interruption level "passive" and no sound. Note that a
//     passive notification still shows a banner while the app is frontmost
//     (the Wails delegate's willPresent asks for one);
//   - attribution → Subtitle.
//
// The logo (logo) is not used: macOS shows the app icon.
// TODO(macos): the "off" logo for trouble toasts would need an attachment.

var notifyAuthOnce sync.Once

// askNotifications asks once per run for permission; macOS shows its prompt
// only while the answer is still "not determined".
func askNotifications() {
	notifyAuthOnce.Do(func() {
		ok, err := notifications.New().RequestNotificationAuthorization()
		slog.Info("notifications: authorization", "granted", ok, "err", err)
	})
}

func (n *Notifier) push(o notifications.NotificationOptions, attribution, _ string, p toastPush) error {
	askNotifications()
	o.Title = n.TitlePrefix + o.Title
	if p.Tag != "" {
		o.ID = p.Group + "/" + p.Tag
	}
	o.ThreadID = p.Group
	if attribution != "" && attribution != appName {
		o.Subtitle = attribution
	}
	if p.SuppressPopup {
		o.InterruptionLevel = "passive"
		o.Sound = &notifications.NotificationSound{Silent: true}
	}
	if err := n.svc.SendNotification(o); err != nil {
		return fmt.Errorf("notification: %w", err)
	}
	return nil
}

// ToastsOff reports whether macOS blocks the app's notifications. Asking
// (not just checking) keeps a "not determined" state from counting as off:
// it brings up the system prompt instead.
func ToastsOff(string) (bool, error) {
	ok, err := notifications.New().RequestNotificationAuthorization()
	if err != nil {
		return false, err
	}
	return !ok, nil
}
