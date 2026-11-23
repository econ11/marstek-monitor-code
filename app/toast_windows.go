package app

import (
	"encoding/base64"
	"encoding/json"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows/registry"
)

// push shows the toast described by o, with the logo image (file URI, may be
// empty), at the given place; p.AppID and p.XML are filled in here.
func (n *Notifier) push(o notifications.NotificationOptions, attribution, logo string, p toastPush) error {
	o.Title = n.TitlePrefix + o.Title
	// The click comes back through the Wails service, which expects its own
	// launch payload.
	payload, err := json.Marshal(notifications.NotificationPayload{
		Action: notifications.DefaultActionIdentifier, Options: o})
	if err != nil {
		return err
	}
	// No banner also means no sound.
	p.XML, err = toastXML(o.Title, o.Body, attribution, logo, base64.StdEncoding.EncodeToString(payload), p.SuppressPopup)
	if err != nil {
		return err
	}
	p.AppID = application.Get().Config().Name
	n.iconOnce.Do(saveToastIcon)
	return pushToast(p)
}

// saveToastIcon writes the app icon where the toast registration (IconUri,
// set by the Wails service) expects it; without the file toasts show a blank
// logo.
func saveToastIcon() {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Classes\AppUserModelId\`+application.Get().Config().Name, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	path, _, err := k.GetStringValue("IconUri")
	if err != nil || path == "" {
		return
	}
	if icon, err := application.NewIconFromResource(w32.GetModuleHandle(""), 3); err == nil {
		_ = w32.SaveHIconAsPNG(icon, path)
	}
}

// AskNotificationPermission: Windows has no permission prompt for toasts
// (macOS asks at start, toast_darwin.go).
func AskNotificationPermission() {}
