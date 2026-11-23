package app

import (
	"bytes"
	"image/png"
	"testing"
)

// Both Dock images are embedded at 1024 px (a 256 px source looked soft on
// Retina) and differ.
func TestDockImages(t *testing.T) {
	on, off := dockPNG(iconOn), dockPNG(iconOff)
	for name, b := range map[string][]byte{"on": on, "off": off} {
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r := img.Bounds(); r.Dx() != 1024 || r.Dy() != 1024 {
			t.Errorf("%s: %v, want 1024 x 1024", name, r)
		}
	}
	if bytes.Equal(on, off) {
		t.Error("on and off are the same image")
	}
}
