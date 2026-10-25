package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Plan describes one clone operation.
type Plan struct {
	Src     App
	Label   string
	Name    string // <App>-<Label>
	Dest    string // the twin itself: a bundle, .desktop file or shortcut
	ID      string // bundle identifier (macOS) or desktop file ID (Linux)
	Profile string
	Exists  bool   // Dest is already there and will be replaced
	Color   string // badge/tint color, #RRGGBB
	Tint    bool
	Badge   bool

	OwnHardware bool // the twin gets its own hardware IDs (macOS)
	BringData   bool // the twin starts with a copy of the original's data

	ExistingIsClone bool // what's at Dest was made by twin
}

func newPlan(src App, rawLabel string, style Options) Plan {
	label := sanitizeLabel(rawLabel)
	name := src.Name + "-" + label
	color := style.Color
	if color == "" {
		color = autoColor(label)
	}
	p := Plan{
		Src:     src,
		Label:   label,
		Name:    name,
		Profile: filepath.Join(profilesRoot(), name),
		Color:   color,
		Tint:    style.Tint,
		Badge:   style.Badge,

		OwnHardware: style.OwnHardware,
		BringData:   style.BringData,
	}
	p.Dest, p.ID = twinTarget(src, name, label)
	_, err := os.Stat(p.Dest)
	p.Exists = err == nil
	p.ExistingIsClone = p.Exists && isTwin(p.Dest)
	return p
}

// twinHash keeps per-twin paths short: Unix socket paths max out at 104
// bytes on macOS and 108 on Linux.
func twinHash(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return fmt.Sprintf("%08x", h.Sum32())
}

func profilesRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".app_profiles")
}

// sanitizeLabel keeps labels valid inside bundle identifiers and file names:
// whitespace runs become "-", anything but [A-Za-z0-9-] is dropped.
func sanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range strings.Join(strings.Fields(s), "-") {
		if r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Clone"
	}
	return b.String()
}

// paintStep is the icon step's title on every platform. When it warns, the
// twin kept the original icon.
const paintStep = "Paint icon & badge"

// step is one stage of a clone. A returned error aborts the clone; a warning
// is shown but the clone carries on.
type step struct {
	Title string
	Run   func(Plan) (warning string, err error)
}

// run executes a command and folds its output into the error on failure.
func run(name string, args ...string) error {
	_, err := output(name, args...)
	return err
}

func output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", filepath.Base(name), msg)
	}
	return string(out), nil
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	return path
}
