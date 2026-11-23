//go:build windows

package app

import (
	"strings"
	"testing"

	"github.com/go-ole/go-ole"
)

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
