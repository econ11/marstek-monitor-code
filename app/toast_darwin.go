package app

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/mac"
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
// A click comes back through the same Wails response as on Windows, with
// the toast's Data as user info: Notifier.OnClick reads station_id / intent
// / event_id and
// main routes with ToastRoute at click time (a grid-lost toast of an outage
// still going on opens Overview, an ended one its row in Outages), exactly
// as on Windows.
//
// The logo (logo) is not used: macOS shows the app icon.
// TODO(macos): the "off" logo for trouble toasts would need an attachment.

// NotificationsAvailable: the app runs from a bundle with a bundle id, which
// UNUserNotificationCenter needs. A bare binary (wails3 build, a dev run)
// has none: the Wails notification service would fail to start and take
// the app down with it, so main leaves it out and notifications are off.
func NotificationsAvailable() bool {
	bundleOnce.Do(func() { hasBundleID = mac.GetBundleID() != "" })
	return hasBundleID
}

var (
	bundleOnce  sync.Once
	hasBundleID bool
	errNoBundle = errors.New("notification: no bundle id (not run from the .app), notifications are off")
)

// AskNotificationPermission asks macOS once, at start, whether the app may
// show notifications; the system prompt appears only while the answer is
// still "not determined". Call it off the UI thread (it waits for the user).
// Nothing without a bundle id.
func AskNotificationPermission() {
	if !NotificationsAvailable() {
		slog.Warn("notifications: off, the app does not run from its bundle")
		return
	}
	ok, err := notifications.New().RequestNotificationAuthorization()
	slog.Info("notifications: authorization", "granted", ok, "err", err)
}

func (n *Notifier) push(o notifications.NotificationOptions, attribution, _ string, p toastPush) error {
	if !NotificationsAvailable() {
		return errNoBundle
	}
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

// ToastsOff reports whether macOS blocks the app's notifications. It only
// checks (the settings screen calls it at every focus); the prompt is asked
// once at start (AskNotificationPermission). Not determined yet counts as
// off until the user answers.
func ToastsOff(string) (bool, error) {
	if !NotificationsAvailable() {
		return false, nil // nothing to switch on in macOS settings
	}
	ok, err := notifications.New().CheckNotificationAuthorization()
	if err != nil {
		return false, err
	}
	return !ok, nil
}
