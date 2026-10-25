package main

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
)

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

// Options are the choices behind a twin.
type Options struct {
	Color       string // #RRGGBB, or "" to pick one from the label
	Tint        bool   // shift the icon's hue toward the color
	Badge       bool   // draw the label on the icon
	OwnHardware bool   // give the twin its own hardware IDs (macOS)
}

func autoColor(label string) string {
	if c, ok := labelColors[strings.ToLower(label)]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(label)))
	return palette[h.Sum32()%uint32(len(palette))]
}

// parseOptions reads the --color, --no-badge and --share-hardware flags.
func parseOptions(color string, noBadge, shareHardware bool) (Options, error) {
	s := Options{Tint: true, Badge: !noBadge, OwnHardware: !shareHardware}
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
