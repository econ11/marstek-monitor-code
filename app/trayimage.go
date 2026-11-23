package app

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"sync"
)

// The tray shows the app icon (docs/superpowers/specs/2026-11-23-tray-icon-design.md).
// The embedded .ico files hold BMP frames (16..64 px) and a PNG frame (256 px);
// the tray takes PNG bytes, so the frame is decoded here. No Windows API, so it
// also builds elsewhere.

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

type trayKey struct {
	icon appIcon
	px   int
}

var trayPNGCache sync.Map // trayKey -> []byte; the embedded icons never change

// trayPNG is the PNG of the .ico frame that pickICOImage picks for px.
// The returned slice is shared (cached): the caller must not modify it.
func trayPNG(i appIcon, px int) ([]byte, error) {
	key := trayKey{i, px}
	if b, ok := trayPNGCache.Load(key); ok {
		return b.([]byte), nil
	}
	ico := windowICO(i)
	e, ok := pickICOImage(ico, px)
	if !ok {
		return nil, errors.New("tray icon: not an .ico file")
	}
	img, err := decodeICOFrame(ico[e.offset : e.offset+e.length])
	if err != nil {
		return nil, fmt.Errorf("tray icon %s %d px: %w", i, px, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	trayPNGCache.Store(key, b)
	return b, nil
}

// decodeICOFrame decodes one image of an .ico: a PNG, or a 32-bit BMP
// (BITMAPINFOHEADER, height = 2 x width for XOR + AND mask). The BMP is stored
// bottom-up as straight-alpha BGRA; the AND mask is ignored.
func decodeICOFrame(frame []byte) (image.Image, error) {
	if bytes.HasPrefix(frame, pngSignature) {
		return png.Decode(bytes.NewReader(frame))
	}
	if len(frame) < 40 {
		return nil, errors.New("ico frame: truncated header")
	}
	if hs := binary.LittleEndian.Uint32(frame[0:]); hs != 40 {
		return nil, fmt.Errorf("ico frame: header size %d, want 40", hs)
	}
	w := int(int32(binary.LittleEndian.Uint32(frame[4:])))
	h := int(int32(binary.LittleEndian.Uint32(frame[8:])))
	bpp := binary.LittleEndian.Uint16(frame[14:])
	if bpp != 32 {
		return nil, fmt.Errorf("ico frame: %d bpp, want 32", bpp)
	}
	if w <= 0 || w > 1024 || h != 2*w {
		return nil, fmt.Errorf("ico frame: size %dx%d, want height = 2 x width", w, h)
	}
	if compression := binary.LittleEndian.Uint32(frame[16:]); compression != 0 {
		return nil, fmt.Errorf("ico frame: compression %d, want 0", compression)
	}
	px := frame[40:]
	if len(px) < w*w*4 {
		return nil, errors.New("ico frame: truncated pixels")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, w))
	for y := 0; y < w; y++ {
		row := px[(w-1-y)*w*4:] // bottom-up
		out := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			out[4*x], out[4*x+1], out[4*x+2], out[4*x+3] = row[4*x+2], row[4*x+1], row[4*x], row[4*x+3]
		}
	}
	return img, nil
}
