package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	destRoot   = "/Applications"
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// wrapperTemplate replaces the clone's main executable. %[1]s is the clone
// name, %[2]s the original executable name.
const wrapperTemplate = `#!/usr/bin/env bash
PROFILE_DIR="$HOME/.app_profiles/%[1]s"
mkdir -p "$PROFILE_DIR"

# Redirect HOME so local config files stay completely isolated.
# Cocoa ignores HOME for NSHomeDirectory()/~/Library; CFFIXED_USER_HOME is what it honors.
export HOME="$PROFILE_DIR"
export CFFIXED_USER_HOME="$PROFILE_DIR"
exec "$(dirname "$0")/%[2]s.real" "$@"
`

// Plan describes one clone operation.
type Plan struct {
	Src      App
	Label    string
	Name     string // <App>-<Label>
	Dest     string
	Profile  string
	BundleID string
	Exists   bool   // Dest is already there and will be replaced
	Color    string // badge/tint color, #RRGGBB
	Tint     bool
	Badge    bool

	ExistingIsClone bool // the app at Dest was made by twin
}

func newPlan(src App, rawLabel string, style IconStyle) Plan {
	label := sanitizeLabel(rawLabel)
	name := src.Name + "-" + label
	home, _ := os.UserHomeDir()
	dest := filepath.Join(destRoot, name+".app")
	_, err := os.Stat(dest)
	existing, _ := readApp(dest)
	color := style.Color
	if color == "" {
		color = autoColor(label)
	}
	return Plan{
		Src:             src,
		Label:           label,
		Name:            name,
		Dest:            dest,
		Profile:         filepath.Join(home, ".app_profiles", name),
		BundleID:        src.BundleID + "." + label,
		Exists:          err == nil,
		Color:           color,
		Tint:            style.Tint,
		Badge:           style.Badge,
		ExistingIsClone: existing.IsClone,
	}
}

// sanitizeLabel keeps labels valid inside bundle identifiers:
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

// step is one stage of a clone. A returned error aborts the clone; a warning
// is shown but the clone carries on.
type step struct {
	Title string
	Run   func(Plan) (warning string, err error)
}

var steps = []step{
	{"Copy app bundle", copyBundle},
	{"Update bundle identifier", updateInfoPlist},
	{"Paint icon & badge", paintIcon},
	{"Inject isolated profile launcher", injectLauncher},
	{"Clear quarantine & re-sign", resign},
	{"Register with Launch Services", register},
}

func copyBundle(p Plan) (string, error) {
	if p.Exists {
		if err := os.RemoveAll(p.Dest); err != nil {
			return "", err
		}
	}
	// Some bundles (e.g. Safari) are symlinks; copy what they point to.
	src, err := filepath.EvalSymlinks(p.Src.Path)
	if err != nil {
		return "", err
	}
	return "", run("ditto", src, p.Dest)
}

func updateInfoPlist(p Plan) (string, error) {
	info := filepath.Join(p.Dest, "Contents", "Info.plist")
	if err := run("plutil", "-replace", "CFBundleIdentifier", "-string", p.BundleID, info); err != nil {
		return "", err
	}
	return "", run("plutil", "-replace", "CFBundleName", "-string", p.Name, info)
}

func injectLauncher(p Plan) (string, error) {
	if err := os.MkdirAll(p.Profile, 0o755); err != nil {
		return "", err
	}
	exe := filepath.Join(p.Dest, "Contents", "MacOS", p.Src.Exec)
	if err := os.Rename(exe, exe+".real"); err != nil {
		return "", err
	}
	return "", os.WriteFile(exe, []byte(fmt.Sprintf(wrapperTemplate, p.Name, p.Src.Exec)), 0o755)
}

func resign(p Plan) (string, error) {
	var warnings []string
	if err := run("xattr", "-cr", p.Dest); err != nil {
		warnings = append(warnings, err.Error())
	}
	if err := run("codesign", "--force", "--deep", "--sign", "-", p.Dest); err != nil {
		warnings = append(warnings, err.Error())
	}
	return strings.Join(warnings, "; "), nil
}

func register(p Plan) (string, error) {
	if err := run(lsregister, "-f", p.Dest); err != nil {
		return err.Error(), nil
	}
	return "", nil
}

// run executes a command and folds its output into the error on failure.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", filepath.Base(name), msg)
	}
	return nil
}
