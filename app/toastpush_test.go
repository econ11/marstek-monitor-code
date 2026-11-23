package app

import (
	"strings"
	"testing"

	"github.com/econ11/marstek-monitor-code/core/notify"
	"github.com/go-ole/go-ole"
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

// The whole string must arrive, emoji included: ole.NewHString loses one
// trailing unit per surrogate pair, which is what broke the toast XML.
func TestNewHStringKeepsFullLength(t *testing.T) {
	for _, s := range []string{"", "ascii", "Зникло світло", "🕯️ Зникло світло 🔋🪫</toast>"} {
		h, err := newHString(s)
		if err != nil {
			t.Fatalf("newHString(%q): %v", s, err)
		}
		got := h.String()
		_ = ole.DeleteHString(h)
		if got != s {
			t.Errorf("newHString(%q) holds %q", s, got)
		}
	}
}

func TestClipUnits(t *testing.T) {
	if got := clipUnits("outage-7", 64); got != "outage-7" {
		t.Errorf("short: %q", got)
	}
	if got := clipUnits(strings.Repeat("я", 70), 64); got != strings.Repeat("я", 64) {
		t.Errorf("long: %d runes", len([]rune(got)))
	}
	// A surrogate pair is never split: 63 units + one emoji (2 units) = 63.
	if got := clipUnits(strings.Repeat("a", 63)+"🔋", 64); got != strings.Repeat("a", 63) {
		t.Errorf("pair at the limit: %q", got)
	}
}
