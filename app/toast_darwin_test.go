package app

import (
	"testing"
	"time"

	"github.com/econ11/marstek-monitor-code/core/notify"
)

// deliverData is the Data a monitor toast carries (Notifier.Deliver).
func deliverData(t notify.Toast) map[string]any {
	return map[string]any{"station_id": t.StationID, "intent": string(t.Kind), "event_id": toastTag(t)}
}

// The notification's user info carries the tab its click opens under "nav":
// the same target the Windows toast click routes to (ToastRoute), so both
// platforms open the same tab.
func TestToastNavTarget(t *testing.T) {
	at := time.Date(2026, 11, 24, 14, 5, 0, 0, time.Local)
	for name, tc := range map[string]struct {
		toast notify.Toast
		want  Navigate
	}{
		"grid lost → outages": {notify.Toast{Kind: notify.GridLost, Tag: "7", StationID: "s", At: at},
			Navigate{View: "outages", OutageID: 7}},
		"low charge → overview": {notify.Toast{Kind: notify.LowCharge, StationID: "s", At: at},
			Navigate{View: "overview"}},
		"update → about": {notify.Toast{Kind: notify.Update, At: at},
			Navigate{View: "settings", Section: "about"}},
	} {
		data := deliverData(tc.toast)
		windows := ToastRoute(tc.toast.Kind, toastTag(tc.toast), 0)
		if windows != tc.want {
			t.Fatalf("%s: the Windows route is %+v, want %+v (test out of date)", name, windows, tc.want)
		}
		got, ok := withNav(data)[navKey].(map[string]any)
		if !ok {
			t.Fatalf("%s: no %q in %v", name, navKey, withNav(data))
		}
		if nav := navFromUserInfo(got); nav != windows {
			t.Errorf("%s: nav %+v, Windows route %+v", name, nav, windows)
		}
		if _, had := data[navKey]; had {
			t.Errorf("%s: the caller's Data was changed", name)
		}
	}
	// A plain toast (tray hint, test toast without Data) opens Overview.
	if nav := navFromUserInfo(withNav(nil)[navKey].(map[string]any)); nav != (Navigate{View: "overview"}) {
		t.Errorf("plain toast: %+v", nav)
	}
}
