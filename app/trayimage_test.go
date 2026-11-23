package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

func decodeTrayPNG(t *testing.T, i appIcon, px int) *image.NRGBA {
	t.Helper()
	b, err := trayPNG(i, px)
	if err != nil {
		t.Fatalf("trayPNG(%s, %d): %v", i, px, err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("png.Decode(%s, %d): %v", i, px, err)
	}
	return toNRGBA(img)
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

func TestTrayPNGSizes(t *testing.T) {
	for _, i := range []appIcon{iconOn, iconOff} {
		for _, px := range []int{16, 20, 24, 32, 40, 48, 64} {
			img := decodeTrayPNG(t, i, px)
			if b := img.Bounds(); b.Dx() != px || b.Dy() != px {
				t.Errorf("%s %d: bounds %v", i, px, b)
				continue
			}
			if a := img.NRGBAAt(0, 0).A; a > 2 { // the 16 px frame has alpha 1 in the corner
				t.Errorf("%s %d: corner alpha %d, want ~0", i, px, a)
			}
			if a := img.NRGBAAt(px/2, px/2).A; a != 255 {
				t.Errorf("%s %d: centre alpha %d, want 255", i, px, a)
			}
		}
	}
}

// boxDown downsamples to n×n by averaging blocks (colour weighted by alpha).
func boxDown(src *image.NRGBA, n int) *image.NRGBA {
	k := src.Bounds().Dx() / n
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a float64
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
					p := src.NRGBAAt(x*k+dx, y*k+dy)
					al := float64(p.A)
					r += float64(p.R) * al
					g += float64(p.G) * al
					b += float64(p.B) * al
					a += al
				}
			}
			if a > 0 {
				out.SetNRGBA(x, y, color.NRGBA{uint8(r/a + .5), uint8(g/a + .5), uint8(b/a + .5), uint8(a/float64(k*k) + .5)})
			}
		}
	}
	return out
}

func diffs(a, b *image.NRGBA) (dA, dRGB float64) {
	n := a.Bounds().Dx()
	var sa, srgb float64
	var cnt int
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			p, q := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			sa += math.Abs(float64(p.A) - float64(q.A))
			if p.A > 128 && q.A > 128 {
				srgb += (math.Abs(float64(p.R)-float64(q.R)) + math.Abs(float64(p.G)-float64(q.G)) + math.Abs(float64(p.B)-float64(q.B))) / 3
				cnt++
			}
		}
	}
	if cnt == 0 {
		cnt = 1
	}
	return sa / float64(n*n), srgb / float64(cnt)
}

func flipV(src *image.NRGBA) *image.NRGBA {
	n := src.Bounds().Dx()
	out := image.NewNRGBA(src.Bounds())
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			out.SetNRGBA(x, n-1-y, src.NRGBAAt(x, y))
		}
	}
	return out
}

func TestTrayPNGMatchesLargeFrame(t *testing.T) {
	for _, i := range []appIcon{iconOn, iconOff} {
		big := boxDown(decodeTrayPNG(t, i, 256), 32)
		got := decodeTrayPNG(t, i, 32)
		dA, dRGB := diffs(got, big)
		if dA >= 24 || dRGB >= 40 {
			t.Errorf("%s: mean |dA| %.1f (<24), mean |dRGB| %.1f (<40)", i, dA, dRGB)
		}
		fA, fRGB := diffs(flipV(got), big)
		t.Logf("%s: straight dA %.2f dRGB %.2f; flipped dA %.2f dRGB %.2f", i, dA, dRGB, fA, fRGB)
		if fRGB < 2*dRGB {
			t.Errorf("%s: flipped |dRGB| %.1f is not at least 2x the straight %.1f", i, fRGB, dRGB)
		}
	}
}

func bmpFrame(w, h int32, bpp uint16, hdr int) []byte {
	n := 0
	if w > 0 && w < 4096 {
		n = int(w) * int(w) * 4
	}
	b := make([]byte, 40+n)
	binary.LittleEndian.PutUint32(b[0:], uint32(hdr))
	binary.LittleEndian.PutUint32(b[4:], uint32(w))
	binary.LittleEndian.PutUint32(b[8:], uint32(h))
	binary.LittleEndian.PutUint16(b[12:], 1)
	binary.LittleEndian.PutUint16(b[14:], bpp)
	return b
}

func TestTrayPNGBadInput(t *testing.T) {
	good := bmpFrame(2, 4, 32, 40)
	if _, err := decodeICOFrame(good); err != nil {
		t.Fatalf("good frame: %v", err)
	}
	for name, f := range map[string][]byte{
		"nil":            nil,
		"truncated":      good[:20],
		"header only":    good[:40],
		"short pixels":   good[:len(good)-1],
		"bpp 24":         bmpFrame(2, 4, 24, 40),
		"height not 2w":  bmpFrame(2, 3, 32, 40),
		"header size 12": bmpFrame(2, 4, 32, 12),
		"negative w":     bmpFrame(-2, -4, 32, 40),
		"huge":           bmpFrame(1<<30, 1<<31-2, 32, 40),
	} {
		if img, err := decodeICOFrame(f); err == nil {
			t.Errorf("%s: want error, got %v", name, img.Bounds())
		}
	}
}

// A 2x2 32-bit BMP (XOR + AND mask, stored bottom-up as BGRA) decodes to the
// exact NRGBA: B/R swapped, alpha untouched, top row first.
func TestDecodeICOFrameExact(t *testing.T) {
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:], 40)
	binary.LittleEndian.PutUint32(hdr[4:], 2)
	binary.LittleEndian.PutUint32(hdr[8:], 4) // 2 x width
	binary.LittleEndian.PutUint16(hdr[12:], 1)
	binary.LittleEndian.PutUint16(hdr[14:], 32)
	px := []byte{ // bottom row first, BGRA
		0x10, 0x20, 0x30, 0xFF, 0x40, 0x50, 0x60, 0x80,
		0x01, 0x02, 0x03, 0xFF, 0x04, 0x05, 0x06, 0x00,
	}
	img, err := decodeICOFrame(append(hdr, px...))
	if err != nil {
		t.Fatal(err)
	}
	got := toNRGBA(img)
	want := []byte{ // top row, then bottom row, RGBA
		0x03, 0x02, 0x01, 0xFF, 0x06, 0x05, 0x04, 0x00,
		0x30, 0x20, 0x10, 0xFF, 0x60, 0x50, 0x40, 0x80,
	}
	if got.Rect.Dx() != 2 || got.Rect.Dy() != 2 || !bytes.Equal(got.Pix, want) {
		t.Errorf("pix = % x, want % x", got.Pix, want)
	}
}
