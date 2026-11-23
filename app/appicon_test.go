package app

import (
	"bytes"
	"encoding/xml"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/econ11/marstek-monitor-code/core/detect"
	"github.com/econ11/marstek-monitor-code/core/notify"
	"golang.org/x/sys/windows"
)

func TestIconForGrid(t *testing.T) {
	for g, want := range map[detect.GridState]appIcon{
		detect.GridOn:           iconOn,
		detect.GridOff:          iconOff,
		detect.GridUnknownState: iconOn, // unknown is shown as the default
	} {
		if got := iconForGrid(g); got != want {
			t.Errorf("iconForGrid(%v) = %q, want %q", g, got, want)
		}
	}
}

func TestIconForToast(t *testing.T) {
	for k, want := range map[notify.Kind]appIcon{
		notify.GridLost:     iconOff,
		notify.LikelyDead:   iconOff,
		notify.LowCharge:    iconOff,
		notify.GridBack:     iconOn,
		notify.Full:         iconOn,
		notify.GridFlapping: iconOn,
		"":                  iconOn, // plain toasts (tray hint)
	} {
		if got := iconForToast(k); got != want {
			t.Errorf("iconForToast(%q) = %q, want %q", k, got, want)
		}
	}
}

// The embedded .ico files are the ones from docs/design/logo: every size the
// README lists, so each DPI finds its own drawing.
func TestEmbeddedICO(t *testing.T) {
	for _, i := range []appIcon{iconOn, iconOff} {
		ico := windowICO(i)
		var sizes []int
		for _, e := range icoEntries(ico) {
			sizes = append(sizes, e.size)
		}
		if got, want := sizes, []int{16, 20, 24, 32, 40, 48, 64, 256}; !slices.Equal(got, want) {
			t.Errorf("%s: sizes %v, want %v", i, got, want)
		}
	}
}

func TestPickICOImage(t *testing.T) {
	ico := windowICO(iconOn)
	for want, px := range map[int]int{16: 16, 20: 20, 24: 24, 32: 32, 40: 40, 48: 48, 64: 64, 256: 256} {
		if e, ok := pickICOImage(ico, px); !ok || e.size != want {
			t.Errorf("pickICOImage(%d) = %d, want %d", px, e.size, want)
		}
	}
	// No exact size: the next larger one is scaled down; above 256 the largest.
	for px, want := range map[int]int{18: 20, 28: 32, 36: 40, 56: 64, 72: 256, 512: 256} {
		if e, ok := pickICOImage(ico, px); !ok || e.size != want {
			t.Errorf("pickICOImage(%d) = %d, want %d", px, e.size, want)
		}
	}
	for _, bad := range [][]byte{nil, {0, 0, 1, 0}, {0, 0, 2, 0, 1, 0}, append([]byte{0, 0, 1, 0, 1, 0}, make([]byte, 16)...)} {
		if _, ok := pickICOImage(bad, 16); ok {
			t.Errorf("pickICOImage(%v) accepted a broken file", bad)
		}
	}
}

// Windows itself turns each embedded drawing into an icon of the asked size.
func TestNewIconPair(t *testing.T) {
	for _, i := range []appIcon{iconOn, iconOff} {
		for _, dpi := range []int{96, 120, 144, 192} {
			p, err := newIconPair(windowICO(i), dpi)
			if err != nil {
				t.Fatalf("%s @%d: %v", i, dpi, err)
			}
			// 16/32 px at 100 %, 20/40 at 125 %, 24/48 at 150 %, 32/64 at 200 %.
			small, big := dpi/6, dpi/3
			if got := iconWidth(t, p.small); got != small {
				t.Errorf("%s @%d: small icon %d px, want %d", i, dpi, got, small)
			}
			if got := iconWidth(t, p.big); got != big {
				t.Errorf("%s @%d: big icon %d px, want %d", i, dpi, got, big)
			}
			p.destroy()
		}
	}
	if _, err := newIconPair([]byte("not an icon"), 96); err == nil {
		t.Error("newIconPair accepted a broken file")
	}
}

var (
	procGetIconInfo  = windows.NewLazySystemDLL("user32.dll").NewProc("GetIconInfo")
	procGetObject    = windows.NewLazySystemDLL("gdi32.dll").NewProc("GetObjectW")
	procDeleteObject = windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject")
)

// iconWidth reads the width of the icon's bitmaps.
func iconWidth(t *testing.T, h windows.Handle) int {
	t.Helper()
	if h == 0 {
		t.Fatal("no icon")
	}
	var ii struct {
		fIcon             int32
		xHot, yHot        uint32
		hbmMask, hbmColor uintptr
	}
	if r, _, err := procGetIconInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&ii))); r == 0 {
		t.Fatalf("GetIconInfo: %v", err)
	}
	defer procDeleteObject.Call(ii.hbmMask)
	defer procDeleteObject.Call(ii.hbmColor)
	var bm struct {
		typ, width, height, widthBytes int32
		planes, bitsPixel              uint16
		bits                           uintptr
	}
	if r, _, err := procGetObject.Call(ii.hbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		t.Fatalf("GetObject: %v", err)
	}
	return int(bm.width)
}

func TestWriteToastLogos(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with space", "icons")
	logos, err := writeToastLogos(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []appIcon{iconOn, iconOff} {
		u, err := url.Parse(logos[i])
		if err != nil || u.Scheme != "file" {
			t.Fatalf("%s: %q is not a file URI", i, logos[i])
		}
		path := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", i, err)
		}
		if !bytes.Equal(got, toastPNG(i)) || !bytes.HasPrefix(got, []byte("\x89PNG")) {
			t.Errorf("%s: %s does not hold the toast logo", i, path)
		}
	}
	// Written again on the next start: same names, no error.
	if again, err := writeToastLogos(dir); err != nil || again[iconOff] != logos[iconOff] {
		t.Errorf("second write: %v, %v", again, err)
	}
}

func TestToastXMLLogo(t *testing.T) {
	logo := "file:///C:/Users/a%20b/icons/toast-off.png?x=1&y=<2>"
	doc, err := toastXML("Заголовок", "", "", logo, "x", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, "&y=<2>") {
		t.Errorf("logo URI not escaped: %s", doc)
	}
	var got toastDoc
	if err := xml.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatal(err)
	}
	img := got.Binding.Image
	if img == nil || img.Placement != "appLogoOverride" || img.Src != logo {
		t.Errorf("image = %+v, want appLogoOverride %q", img, logo)
	}

	doc, err = toastXML("Заголовок", "", "", "", "x", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, "<image") {
		t.Errorf("no logo, yet an image: %s", doc)
	}
}

func TestNotifierLogo(t *testing.T) {
	n := &Notifier{}
	if got := n.logo(notify.GridLost); got != "" {
		t.Errorf("logo before SetLogoDir = %q", got)
	}
	if err := n.SetLogoDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := n.logo(notify.GridLost); !strings.HasSuffix(got, "/toast-off.png") {
		t.Errorf("grid_lost logo = %q", got)
	}
	if got := n.logo(notify.GridBack); !strings.HasSuffix(got, "/toast-on.png") {
		t.Errorf("grid_back logo = %q", got)
	}
}

// Task 20c (icon review): an icon that cannot be made is retried with every
// snapshot, but logged once, not at the poll rate.
func TestWindowIconFailureLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	w := &windowIcon{want: iconOff, newPair: func([]byte, int) (iconPair, error) {
		return iconPair{}, errors.New("CreateIconFromResourceEx failed")
	}}
	for i := 0; i < 3; i++ {
		w.show(1, 96)
	}
	if n := strings.Count(buf.String(), "window icon"); n != 1 {
		t.Fatalf("log has %d window icon entries, want 1:\n%s", n, buf.String())
	}
	if w.shown == iconOff {
		t.Fatal("a failed icon counts as shown")
	}
}
