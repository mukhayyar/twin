package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// paintSize is the canvas paint renders on, matching icon.js.
const paintSize = 1024

// paint is the portable twin of icon.js, for platforms without AppKit: it
// rotates the icon's dominant hue to col (in HSV, pulling saturation toward
// col's) and stamps the label on a pill at the bottom.
func paint(src image.Image, label string, col color.NRGBA, tint, badge bool) *image.NRGBA {
	const S = paintSize
	dst := image.NewNRGBA(image.Rect(0, 0, S, S))
	xdraw.CatmullRom.Scale(dst, fitSquare(src.Bounds(), S), src, src.Bounds(), draw.Over, nil)
	if tint {
		shiftHue(dst, col)
	}
	if badge {
		drawBadge(dst, strings.ReplaceAll(label, "-", " "), col)
	}
	return dst
}

// fitSquare centers r's aspect ratio inside an s×s square.
func fitSquare(r image.Rectangle, s int) image.Rectangle {
	w, h := r.Dx(), r.Dy()
	if w >= h {
		nh := h * s / max(w, 1)
		return image.Rect(0, (s-nh)/2, s, (s+nh)/2)
	}
	nw := w * s / max(h, 1)
	return image.Rect((s-nw)/2, 0, (s+nw)/2, s)
}

func shiftHue(img *image.NRGBA, target color.NRGBA) {
	// Average like CIAreaAverage: transparent pixels count, weighted by alpha.
	var r, g, b float64
	px := img.Pix
	for i := 0; i < len(px); i += 4 {
		a := float64(px[i+3]) / 255
		r += float64(px[i]) * a
		g += float64(px[i+1]) * a
		b += float64(px[i+2]) * a
	}
	avgH, avgS, _ := rgbToHSV(r, g, b)
	if avgS < 0.15 {
		return // near-grayscale: the badge carries the identity
	}
	tH, tS, _ := rgbToHSV(float64(target.R), float64(target.G), float64(target.B))
	delta := tH - avgH
	gain := math.Min(1.8, math.Max(0.6, tS/avgS))

	for i := 0; i < len(px); i += 4 {
		if px[i+3] == 0 {
			continue
		}
		h, s, v := rgbToHSV(float64(px[i]), float64(px[i+1]), float64(px[i+2]))
		nr, ng, nb := hsvToRGB(math.Mod(h+delta+1, 1), math.Min(1, s*gain), v)
		px[i], px[i+1], px[i+2] = nr, ng, nb
	}
}

// rgbToHSV takes channels on any common scale and returns h, s, v in 0…1
// (v relative to that scale's maximum is not needed by callers).
func rgbToHSV(r, g, b float64) (h, s, v float64) {
	mx := math.Max(r, math.Max(g, b))
	d := mx - math.Min(r, math.Min(g, b))
	if d > 0 {
		switch mx {
		case r:
			h = math.Mod((g-b)/d+6, 6)
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
	}
	if mx > 0 {
		s = d / mx
	}
	return h / 6, s, mx / 255
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	f := func(k float64) uint8 {
		x := math.Mod(k+h*6, 6)
		c := v - v*s*math.Max(0, math.Min(x, math.Min(4-x, 1)))
		return uint8(math.Round(c * 255))
	}
	return f(5), f(3), f(1)
}

var badgeFont, _ = opentype.Parse(gobold.TTF)

// drawBadge follows icon.js's geometry: a pill whose bottom sits at 95.5% of
// the height, 20% tall, with a white stroke and a soft shadow.
func drawBadge(img *image.NRGBA, text string, col color.NRGBA) {
	const S = float64(paintSize)
	h, bottom, pad, maxW := 0.20*S, 0.955*S, 0.11*S, 0.94*S

	size := 0.125 * S
	face := newFace(size)
	for measure(face, text)+pad > maxW && size > 0.075*S {
		size -= 0.005 * S
		face = newFace(size)
	}
	for measure(face, text)+pad > maxW && len([]rune(text)) > 1 {
		r := []rune(text)
		text = string(r[:max(len(r)-2, 1)]) + "…"
	}

	textW := measure(face, text)
	w := textW + pad
	x0, y0 := (S-w)/2, bottom-h
	stroke := 0.018 * S

	// Shadow: the pill's mask, blurred, offset down, 35% black.
	shadow := image.NewAlpha(img.Bounds())
	pill(shadow, x0-stroke/2, y0-stroke/2+0.008*S, w+stroke, h+stroke, image.Opaque)
	boxBlur(shadow, paintSize/50) // ≈ 0.02 × S
	draw.DrawMask(img, img.Bounds(), image.NewUniform(color.NRGBA{0, 0, 0, 89}), image.Point{}, shadow, image.Point{}, draw.Over)

	pill(img, x0-stroke/2, y0-stroke/2, w+stroke, h+stroke, image.NewUniform(color.White))
	pill(img, x0+stroke/2, y0+stroke/2, w-stroke, h-stroke, image.NewUniform(col))

	m := face.Metrics()
	capH := float64(m.CapHeight) / 64
	if capH <= 0 {
		capH = 0.7 * size
	}
	d := font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: face,
		Dot: fixed.P(int((S-textW)/2), int(y0+h/2+capH/2))}
	d.DrawString(text)
}

func newFace(size float64) font.Face {
	f, _ := opentype.NewFace(badgeFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	return f
}

func measure(face font.Face, s string) float64 {
	return float64(font.MeasureString(face, s)) / 64
}

// pill fills a fully rounded rectangle onto dst.
func pill(dst draw.Image, x, y, w, h float64, src image.Image) {
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	r := h / 2
	k := r * 0.5523 // cubic approximation of a quarter circle
	f := func(v float64) float32 { return float32(v) }
	z.MoveTo(f(x+r), f(y))
	z.LineTo(f(x+w-r), f(y))
	z.CubeTo(f(x+w-r+k), f(y), f(x+w), f(y+r-k), f(x+w), f(y+r))
	z.CubeTo(f(x+w), f(y+r+k), f(x+w-r+k), f(y+h), f(x+w-r), f(y+h))
	z.LineTo(f(x+r), f(y+h))
	z.CubeTo(f(x+r-k), f(y+h), f(x), f(y+r+k), f(x), f(y+r))
	z.CubeTo(f(x), f(y+r-k), f(x+r-k), f(y), f(x+r), f(y))
	z.ClosePath()
	z.Draw(dst, b, src, image.Point{})
}

// boxBlur approximates a Gaussian with three box passes in each direction.
func boxBlur(a *image.Alpha, radius int) {
	if radius < 1 {
		return
	}
	w, h := a.Rect.Dx(), a.Rect.Dy()
	tmp := make([]uint8, len(a.Pix))
	pass := func(src, dst []uint8, n, count, stride, step int) {
		for line := 0; line < count; line++ {
			base := line * stride
			sum, win := 0, 2*radius+1
			at := func(i int) int { return int(src[base+min(max(i, 0), n-1)*step]) }
			for i := -radius; i <= radius; i++ {
				sum += at(i)
			}
			for i := 0; i < n; i++ {
				dst[base+i*step] = uint8(sum / win)
				sum += at(i+radius+1) - at(i-radius)
			}
		}
	}
	for range 3 {
		pass(a.Pix, tmp, w, h, a.Stride, 1)
		copy(a.Pix, tmp)
		pass(a.Pix, tmp, h, w, 1, a.Stride)
		copy(a.Pix, tmp)
	}
}

// parseHex turns #RRGGBB into a color.
func parseHex(s string) color.NRGBA {
	n, _ := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	return color.NRGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}
}
