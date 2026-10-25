package main

import (
	_ "embed"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed icon.js
var iconScript []byte

var (
	// labelColors gives the common labels fixed, recognizable colors.
	labelColors = map[string]string{
		"work":     "#0A84FF",
		"personal": "#30D158",
		"hobby":    "#FF9F0A",
		"school":   "#BF5AF2",
	}
	// palette colors any other label, picked by hash so it stays stable.
	palette = []string{"#FF375F", "#40C8E0", "#5E5CE6", "#FFD60A", "#FF453A", "#63E6BE", "#AC8E68", "#64D2FF"}

	namedColors = map[string]string{
		"blue": "#0A84FF", "green": "#30D158", "orange": "#FF9F0A", "purple": "#BF5AF2",
		"pink": "#FF375F", "teal": "#40C8E0", "indigo": "#5E5CE6", "yellow": "#FFD60A",
		"red": "#FF453A", "mint": "#63E6BE", "brown": "#AC8E68", "cyan": "#64D2FF",
	}
	hexColor = regexp.MustCompile(`^#?[0-9A-Fa-f]{6}$`)
)

// IconStyle controls how a clone's icon is repainted.
type IconStyle struct {
	Color string // #RRGGBB, or "" to pick one from the label
	Tint  bool   // shift the icon's hue toward the color
	Badge bool   // draw the label on the icon
}

func autoColor(label string) string {
	if c, ok := labelColors[strings.ToLower(label)]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(label)))
	return palette[h.Sum32()%uint32(len(palette))]
}

// parseIconStyle reads the --color and --no-badge flags.
func parseIconStyle(color string, noBadge bool) (IconStyle, error) {
	s := IconStyle{Tint: true, Badge: !noBadge}
	switch c := strings.ToLower(color); {
	case c == "" || c == "auto":
	case c == "none" || c == "off":
		s.Tint = false
	case namedColors[c] != "":
		s.Color = namedColors[c]
	case hexColor.MatchString(c):
		s.Color = "#" + strings.ToUpper(strings.TrimPrefix(c, "#"))
	default:
		return s, fmt.Errorf("unknown color %q (use auto, none, a name like blue, or #RRGGBB)", color)
	}
	return s, nil
}

func paintIcon(p Plan) (string, error) {
	if !p.Tint && !p.Badge {
		return "", nil
	}
	dir, err := os.MkdirTemp("", "twin-icon-")
	if err != nil {
		return err.Error(), nil
	}
	defer os.RemoveAll(dir)

	script := filepath.Join(dir, "icon.js")
	iconset := filepath.Join(dir, "TwinIcon.iconset")
	if err := os.WriteFile(script, iconScript, 0o644); err != nil {
		return err.Error(), nil
	}
	if err := os.Mkdir(iconset, 0o755); err != nil {
		return err.Error(), nil
	}
	// Resolve symlinks, or the icon comes back with an alias arrow.
	src, err := filepath.EvalSymlinks(p.Src.Path)
	if err != nil {
		return err.Error(), nil
	}
	if err := run("osascript", "-l", "JavaScript", script, src, p.Label, p.Color, flag01(p.Tint), flag01(p.Badge), iconset); err != nil {
		return err.Error(), nil
	}

	res := filepath.Join(p.Dest, "Contents", "Resources")
	info := filepath.Join(p.Dest, "Contents", "Info.plist")
	if err := os.MkdirAll(res, 0o755); err != nil {
		return err.Error(), nil
	}
	if err := run("iconutil", "-c", "icns", iconset, "-o", filepath.Join(res, "TwinIcon.icns")); err != nil {
		return err.Error(), nil
	}
	if err := run("plutil", "-replace", "CFBundleIconFile", "-string", "TwinIcon", info); err != nil {
		return err.Error(), nil
	}
	// Without this, macOS keeps showing the asset-catalog icon.
	_ = run("plutil", "-remove", "CFBundleIconName", info)
	now := time.Now()
	_ = os.Chtimes(p.Dest, now, now)
	return "", nil
}

func flag01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
