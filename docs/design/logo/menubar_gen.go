//go:build ignore

// menubar_gen draws the macOS menu-bar template icons from the bolt of the
// small app-icon master (appicon-on-small.svg) and writes:
//
//	docs/design/logo/menubar-on.svg, menubar-off.svg   vector masters (22 pt artboard)
//	app/assets/menubar-{on,off}-{22,44}.png            black + alpha, 1x and 2x
//
// Run from the repository root: go run docs/design/logo/menubar_gen.go
// Flags: -stroke (pt, the "off" outline), -out (a folder to write into
// instead of the repository, for trying variants).
//
// The glyph is the outer silhouette of the app icon's bolt: upper blade, seam
// bar and lower blade as one shape. A template image is one colour, so the
// green seam cannot show; the thin tile-coloured lines on both sides of the
// seam (under a pixel at 22 px) are closed. "on" (grid present, also unknown)
// fills the bolt; "off" (no grid) strokes it inside its outline, so both
// states have the same silhouette. The stroke is 1 pt (1 px at 22 px, 2 px
// at 44 px): the blades are only about 4.8 pt wide, and a 1.5 pt stroke
// leaves a hole under 2 pt that closes up in the narrow lower blade. The
// glyph is 16 pt tall with 3 pt padding top and bottom, centred on a 22 pt
// artboard: Wails 3.1 scales the tray image to the menu bar thickness
// (22 pt), so a 22 pt image is shown 1:1 (an 18 pt one was stretched to a
// soft 20 pt glyph, taller than the neighbouring icons; checked on a Mac
// 2026-11-23, docs/macos-spike.md).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type pt struct{ x, y float64 }

// line through two points of an edge of the master (viewBox 256).
type line struct{ a, b pt }

func intersect(l, m line) pt {
	x1, y1, x2, y2 := l.a.x, l.a.y, l.b.x, l.b.y
	x3, y3, x4, y4 := m.a.x, m.a.y, m.b.x, m.b.y
	d := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / d
	return pt{x1 + t*(x2-x1), y1 + t*(y2-y1)}
}

// Edges of appicon-on-small.svg (straight parts; the 2-unit corner rounds
// are about a tenth of a pixel at 22 px and are left out).
var (
	upperTop   = line{pt{158.77, 27.59}, pt{92.49, 41.68}}
	upperRight = line{pt{133.48, 109.08}, pt{160.09, 29.07}}
	upperLeft  = line{pt{89.9, 43.99}, pt{63.29, 124}}
	seamTop    = line{pt{69.6, 132.23}, pt{183.07, 108.12}}
	seamBottom = line{pt{72.93, 147.88}, pt{186.4, 123.77}}
	lowerRight = line{pt{136.26, 220.87}, pt{192.28, 131.79}}
	lowerBot   = line{pt{95.3, 233.97}, pt{133.27, 223.11}}
	lowerLeft  = line{pt{122.52, 146.92}, pt{94.01, 232.62}}
)

// bolt is the outline of upper blade + seam bar + lower blade, the blades
// stretched to the bar (closing the gaps), clockwise (y down), master units.
func bolt() []pt {
	return []pt{
		intersect(upperTop, upperLeft),    // top-left of the upper blade
		intersect(upperTop, upperRight),   // the top tip
		intersect(upperRight, seamTop),    // the upper blade meets the bar
		seamTop.b,                         // bar, top-right
		seamBottom.b,                      // bar, bottom-right
		intersect(seamBottom, lowerRight), // top-right of the lower blade
		intersect(lowerRight, lowerBot),   // the bottom tip
		intersect(lowerBot, lowerLeft),
		intersect(seamBottom, lowerLeft), // the lower blade leaves the bar
		seamBottom.a,                     // bar, bottom-left
		seamTop.a,                        // bar, top-left
		intersect(seamTop, upperLeft),    // bottom-left of the upper blade
	}
}

const (
	artboard = 22.0 // pt
	pad      = 3.0  // pt, top and bottom
)

// toPoints maps p onto the 22 pt artboard: 16 pt tall, centred.
func toPoints(p []pt) []pt {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, q := range p {
		minX, maxX = math.Min(minX, q.x), math.Max(maxX, q.x)
		minY, maxY = math.Min(minY, q.y), math.Max(maxY, q.y)
	}
	s := (artboard - 2*pad) / (maxY - minY)
	ox := artboard/2 - (minX+maxX)/2*s
	oy := pad - minY*s
	out := make([]pt, len(p))
	for i, q := range p {
		out[i] = pt{q.x*s + ox, q.y*s + oy}
	}
	return out
}

// inside: even-odd crossing test.
func inside(p []pt, q pt) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		a, b := p[i], p[j]
		if (a.y > q.y) != (b.y > q.y) && q.x < (b.x-a.x)*(q.y-a.y)/(b.y-a.y)+a.x {
			in = !in
		}
	}
	return in
}

// edgeDist is the distance from q to the outline of p.
func edgeDist(p []pt, q pt) float64 {
	d := math.Inf(1)
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		dx, dy := b.x-a.x, b.y-a.y
		t := math.Max(0, math.Min(1, ((q.x-a.x)*dx+(q.y-a.y)*dy)/(dx*dx+dy*dy)))
		d = math.Min(d, math.Hypot(q.x-a.x-t*dx, q.y-a.y-t*dy))
	}
	return d
}

// covers: inside the bolt and, for an outline (stroke > 0), within stroke of
// its edge: the inner stroke with round joins, as the SVG master draws it.
func covers(p []pt, stroke float64, q pt) bool {
	return inside(p, q) && (stroke == 0 || edgeDist(p, q) < stroke)
}

// render rasterizes at px x px with 16 x 16 samples per pixel: black, the
// coverage in alpha.
func render(p []pt, stroke float64, px int) *image.NRGBA {
	const ss = 16
	k := float64(px) / artboard
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			hit := 0
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					q := pt{(float64(x) + (float64(i)+0.5)/ss) / k, (float64(y) + (float64(j)+0.5)/ss) / k}
					if covers(p, stroke, q) {
						hit++
					}
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, uint8((hit*255 + ss*ss/2) / (ss * ss))})
		}
	}
	return img
}

func pathData(p []pt) string {
	var b strings.Builder
	for i, q := range p {
		c := "L"
		if i == 0 {
			c = "M"
		}
		fmt.Fprintf(&b, "%s%.3f %.3f", c, q.x, q.y)
	}
	return b.String() + "Z"
}

func svg(p []pt, stroke float64, title string) string {
	head := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 22 22" width="22" height="22"><title>` + title + `</title>`
	d := pathData(p)
	if stroke == 0 {
		return head + `<path d="` + d + `" fill="#000"/></svg>` + "\n"
	}
	// Inner stroke: twice the width, clipped to the shape.
	return head + `<defs><clipPath id="bolt"><path d="` + d + `"/></clipPath></defs>` +
		fmt.Sprintf(`<path d="%s" fill="none" stroke="#000" stroke-width="%g" stroke-linejoin="round" clip-path="url(#bolt)"/></svg>`, d, 2*stroke) + "\n"
}

func writeFile(path string, b []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	stroke := flag.Float64("stroke", 1.0, "outline width of the off state, pt")
	out := flag.String("out", ".", "repository root (or a scratch folder)")
	flag.Parse()

	p := toPoints(bolt())
	states := []struct {
		name, title string
		stroke      float64
	}{
		{"on", "Marstek Monitor menu bar: grid on (filled bolt)", 0},
		{"off", "Marstek Monitor menu bar: grid off (outlined bolt)", *stroke},
	}
	for _, st := range states {
		writeFile(filepath.Join(*out, "docs/design/logo/menubar-"+st.name+".svg"), []byte(svg(p, st.stroke, st.title)))
		for _, px := range []int{22, 44} {
			var buf strings.Builder
			if err := png.Encode(&buf, render(p, st.stroke, px)); err != nil {
				log.Fatal(err)
			}
			writeFile(filepath.Join(*out, fmt.Sprintf("app/assets/menubar-%s-%d.png", st.name, px)), []byte(buf.String()))
		}
	}
}
