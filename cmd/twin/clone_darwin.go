package main

import (
	_ "embed"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	destRoot   = "/Applications"
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// libtwinisolate closes the channels HOME redirection leaves open (/tmp,
// temp and cache folders, preferences, keychain items); see isolate/isolate.m.
//
//go:embed isolate/libtwinisolate.dylib
var isolateLib []byte

// launcher is the script that replaces a twin's main executable. Every twin
// gets the same isolation; no app is special-cased.
func launcher(p Plan) string {
	return `#!/usr/bin/env bash
PROFILE_DIR="$HOME/.app_profiles/` + bashEscape(p.Name) + `"
mkdir -p "$PROFILE_DIR/Library"

# macOS finds the login keychain under $HOME, so keep it reachable before HOME
# moves. Keychain items stay separate: libtwinisolate gives them twin names.
[ -e "$PROFILE_DIR/Library/Keychains" ] || ln -s "$HOME/Library/Keychains" "$PROFILE_DIR/Library/Keychains"

# Isolate everything else. HOME and CFFIXED_USER_HOME move files and ~/Library;
# libtwinisolate covers /tmp, temp and cache folders, preferences and keychain items.
export TWIN_NAME="` + bashEscape(p.Name) + `" TWIN_LABEL="` + p.Label + `" TWIN_ID="` + bashEscape(p.ID) + `" TWIN_ORIG_ID="` + bashEscape(p.Src.ID) + `"
export TWIN_ROOT="/private/tmp/twin.$UID.` + twinHash(p.Name) + `" TWIN_CACHE="$PROFILE_DIR/Library/Caches/TwinDarwin"
export TMPDIR="$TWIN_ROOT/T/"
export HOME="$PROFILE_DIR"
export CFFIXED_USER_HOME="$PROFILE_DIR"
export DYLD_INSERT_LIBRARIES="$(dirname "$0")/libtwinisolate.dylib"
exec "$(dirname "$0")/` + bashEscape(p.Src.Exec) + `.real" "$@"
`
}

// bashEscape makes s safe inside a double-quoted bash string.
func bashEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`").Replace(s)
}

// twinHash keeps TWIN_ROOT short: Unix socket paths max out at 104 bytes.
func twinHash(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return fmt.Sprintf("%08x", h.Sum32())
}

var steps = []step{
	{"Copy app bundle", copyBundle},
	{"Update bundle identifier", updateInfoPlist},
	{paintStep, paintIcon},
	{"Inject isolated profile launcher", injectLauncher},
	{"Clear quarantine & re-sign", resign},
	{"Register with Launch Services", register},
}

func twinTarget(src App, name, label string) (dest, id string) {
	return filepath.Join(destRoot, name+".app"), src.ID + "." + label
}

func isTwin(dest string) bool {
	a, ok := readApp(dest)
	return ok && a.IsClone
}

func previewRows(p Plan) []string {
	return []string{"App", tildify(p.Dest), "Bundle", p.ID, "Profile", tildify(p.Profile)}
}

// cleanup removes a half-built twin after a failed step.
func cleanup(p Plan) { os.RemoveAll(p.Dest) }

func openTwin(p Plan) error { return exec.Command("open", p.Dest).Start() }

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

// updateInfoPlist gives the clone its own bundle identifier. CFBundleName is
// deliberately left alone: Electron apps find their helpers as
// "<CFBundleName> Helper.app" and crash at launch if it changes. Finder and
// the Dock show the renamed bundle's file name anyway.
func updateInfoPlist(p Plan) (string, error) {
	info := filepath.Join(p.Dest, "Contents", "Info.plist")
	return "", run("plutil", "-replace", "CFBundleIdentifier", "-string", p.ID, info)
}

func injectLauncher(p Plan) (string, error) {
	if err := os.MkdirAll(p.Profile, 0o755); err != nil {
		return "", err
	}
	macos := filepath.Join(p.Dest, "Contents", "MacOS")
	exe := filepath.Join(macos, p.Src.Exec)
	if err := os.Rename(exe, exe+".real"); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(macos, "libtwinisolate.dylib"), isolateLib, 0o755); err != nil {
		return "", err
	}
	return "", os.WriteFile(exe, []byte(launcher(p)), 0o755)
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
