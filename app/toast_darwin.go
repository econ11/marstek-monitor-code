package app

import (
	"fmt"
	"log/slog"
	"maps"

	"github.com/econ11/marstek-monitor-code/core/notify"
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
//   - attribution → Subtitle;
//   - the tab the click opens → user info "nav" (withNav).
//
// A click comes back through the same Wails response as on Windows, with
// the user info: Notifier.OnClick reads station_id / intent / event_id and
// main routes with ToastRoute at click time (a grid-lost toast of an outage
// still going on opens Overview, an ended one its row in Outages), exactly
// as on Windows.
//
// The logo (logo) is not used: macOS shows the app icon.
// TODO(macos): the "off" logo for trouble toasts would need an attachment.

// AskNotificationPermission asks macOS once, at start, whether the app may
// show notifications; the system prompt appears only while the answer is
// still "not determined". Call it off the UI thread (it waits for the user).
func AskNotificationPermission() {
	ok, err := notifications.New().RequestNotificationAuthorization()
	slog.Info("notifications: authorization", "granted", ok, "err", err)
}

// navKey is the user-info key of the tab the notification's click opens.
const navKey = "nav"

// withNav is a copy of data (the toast's Data, may be nil) with navKey set
// to the tab ToastRoute gives for its intent and event id: the same target
// as a Windows toast click (an outage's open state is decided at click time
// by main, as on Windows).
func withNav(data map[string]any) map[string]any {
	out := maps.Clone(data)
	if out == nil {
		out = map[string]any{}
	}
	kind, _ := data["intent"].(string)
	tag, _ := data["event_id"].(string)
	r := ToastRoute(notify.Kind(kind), tag, 0)
	nav := map[string]any{"view": r.View}
	if r.OutageID != 0 {
		nav["outageId"] = r.OutageID
	}
	if r.Section != "" {
		nav["section"] = r.Section
	}
	out[navKey] = nav
	return out
}

// navFromUserInfo reads a navKey value back (numbers come back from the
// user info as JSON numbers).
func navFromUserInfo(m map[string]any) Navigate {
	var n Navigate
	n.View, _ = m["view"].(string)
	n.Section, _ = m["section"].(string)
	switch v := m["outageId"].(type) {
	case int64:
		n.OutageID = v
	case int:
		n.OutageID = int64(v)
	case float64:
		n.OutageID = int64(v)
	}
	return n
}

func (n *Notifier) push(o notifications.NotificationOptions, attribution, _ string, p toastPush) error {
	o.Data = withNav(o.Data)
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
	ok, err := notifications.New().CheckNotificationAuthorization()
	if err != nil {
		return false, err
	}
	return !ok, nil
}
