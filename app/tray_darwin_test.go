package app

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/econ11/marstek-monitor-code/core/detect"
)

// The menu-bar images are template images: black with alpha only, square,
// 22 px and 44 px, and on/off differ.
func TestMenubarImages(t *testing.T) {
	for _, i := range []appIcon{iconOn, iconOff} {
		for _, px := range []int{16, 32} {
			b, err := trayImage(i, px)
			if err != nil {
				t.Fatalf("trayImage(%s, %d): %v", i, px, err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("%s %d: %v", i, px, err)
			}
			want := menubarPx(px)
			if r := img.Bounds(); r.Dx() != want || r.Dy() != want {
				t.Errorf("%s %d: %v, want %d x %d", i, px, r, want, want)
			}
			opaque := 0
			for y := 0; y < want; y++ {
				for x := 0; x < want; x++ {
					r, g, bl, a := img.At(x, y).RGBA()
					if r|g|bl != 0 {
						t.Fatalf("%s %d: pixel %d,%d is not black", i, px, x, y)
					}
					if a > 0x8000 {
						opaque++
					}
				}
			}
			if opaque == 0 {
				t.Errorf("%s %d: empty", i, px)
			}
		}
	}
	on, _ := trayImage(iconOn, 32)
	off, _ := trayImage(iconOff, 32)
	if bytes.Equal(on, off) {
		t.Error("on and off are the same image")
	}
}

// The tray asks for the @2x image on macOS, and the grid rule is iconForGrid's.
func TestMenubarPick(t *testing.T) {
	if got := menubarPx(trayIconPx(systemDPI())); got != 44 {
		t.Errorf("menu bar image %d px, want 44", got)
	}
	if got := menubarPx(16); got != 22 {
		t.Errorf("menubarPx(16) = %d, want 22", got)
	}
	off, _ := trayImage(iconForGrid(detect.GridOff), 32)
	want, _ := menubarFS.ReadFile("assets/menubar-off-44.png")
	if !bytes.Equal(off, want) {
		t.Error("grid off does not show menubar-off")
	}
	for _, g := range []detect.GridState{detect.GridOn, detect.GridUnknownState} {
		b, _ := trayImage(iconForGrid(g), 32)
		want, _ := menubarFS.ReadFile("assets/menubar-on-44.png")
		if !bytes.Equal(b, want) {
			t.Errorf("grid %v does not show menubar-on", g)
		}
	}
}
