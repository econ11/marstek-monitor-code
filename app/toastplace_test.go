package app

import (
	"testing"

	"github.com/econ11/marstek-monitor-code/core/notify"
)

func TestToastPlace(t *testing.T) {
	for name, tc := range map[string]struct {
		toast notify.Toast
		want  toastPush
	}{
		// Both toasts of one outage land on the same entry.
		"grid lost": {notify.Toast{Kind: notify.GridLost, Tag: "7", StationID: "mac"},
			toastPush{Tag: "outage-7", Group: "mac"}},
		"grid back replaces it": {notify.Toast{Kind: notify.GridBack, Tag: "7", StationID: "mac"},
			toastPush{Tag: "outage-7", Group: "mac"}},
		"another outage": {notify.Toast{Kind: notify.GridLost, Tag: "8", StationID: "mac"},
			toastPush{Tag: "outage-8", Group: "mac"}},
		"untagged kinds: one entry per kind": {notify.Toast{Kind: notify.Full, StationID: "mac"},
			toastPush{Tag: "full", Group: "mac"}},
		// Quiet hours / do not disturb: no banner.
		"silenced": {notify.Toast{Kind: notify.GridLost, Tag: "7", StationID: "mac", Silent: true},
			toastPush{Tag: "outage-7", Group: "mac", SuppressPopup: true}},
		// Low charge always pops, even if a caller marks it silent.
		// "connection restored" replaces "no connection": same link- tag.
		"no link": {notify.Toast{Kind: notify.NoLink, Tag: "link-1763700000", StationID: "mac"},
			toastPush{Tag: "link-1763700000", Group: "mac"}},
		"no link restored": {notify.Toast{Kind: notify.NoLinkRestored, Tag: "link-1763700000", StationID: "mac"},
			toastPush{Tag: "link-1763700000", Group: "mac"}},
		"low charge is never silenced": {notify.Toast{Kind: notify.LowCharge, StationID: "mac", Silent: true},
			toastPush{Tag: "low_charge", Group: "mac"}},
	} {
		if got := toastPlace(tc.toast); got != tc.want {
			t.Errorf("%s: toastPlace = %+v, want %+v", name, got, tc.want)
		}
	}
}
