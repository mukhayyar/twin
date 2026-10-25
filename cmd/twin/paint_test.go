package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestHSVRoundTrip(t *testing.T) {
	for _, c := range [][3]uint8{{88, 101, 242}, {255, 159, 10}, {0, 0, 0}, {255, 255, 255}, {48, 209, 88}} {
		h, s, v := rgbToHSV(float64(c[0]), float64(c[1]), float64(c[2]))
		r, g, b := hsvToRGB(h, s, v)
		if r != c[0] || g != c[1] || b != c[2] {
			t.Errorf("round trip %v → %v", c, [3]uint8{r, g, b})
		}
	}
}

func TestPaintKeepsSizeAndBadge(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	out := paint(src, "Client-Acme-International-Holdings", parseHex("#FF375F"), true, true)
	if out.Bounds().Dx() != paintSize || out.Bounds().Dy() != paintSize {
		t.Fatalf("size = %v", out.Bounds())
	}
	// The badge's fill color must appear near the bottom center.
	if c := out.NRGBAAt(paintSize/2, paintSize*86/100); c.A == 0 {
		t.Errorf("no badge at bottom center: %v", c)
	}
}

// TWIN_PAINT_SAMPLE=src.png renders src as a Work twin next to it, for eyeballing.
func TestPaintSample(t *testing.T) {
	in := os.Getenv("TWIN_PAINT_SAMPLE")
	if in == "" {
		t.Skip("set TWIN_PAINT_SAMPLE to a PNG")
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	for label, hex := range map[string]string{"Work": "#0A84FF", "Hobby": "#FF9F0A", "Client-Acme-International": "#FF375F"} {
		out, _ := os.Create(filepath.Join(filepath.Dir(in), "go-"+label+".png"))
		png.Encode(out, paint(src, label, parseHex(hex), true, true))
		out.Close()
	}
}
