package main

import (
	_ "embed"
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

// libtwinisolate closes the channels HOME redirection leaves open (/tmp,
// temp and cache folders, preferences, keychain items); see isolate/isolate.m.
//
//go:embed isolate/libtwinisolate.dylib
var isolateLib []byte

// twinEnv is what macOS starts a twin with (its Info.plist LSEnvironment).
// The app's own executable stays the main executable, so privacy
// permissions (Screen Recording, Accessibility…) attach to the twin itself;
// libtwinisolate does the rest at launch. Every twin gets the same
// isolation; no app is special-cased.
func twinEnv(p Plan) [][2]string {
	root := fmt.Sprintf("/private/tmp/twin.%d.%s", os.Getuid(), twinHash(p.Name))
	return [][2]string{
		{"TWIN_HOME", p.Profile},
		{"CFFIXED_USER_HOME", p.Profile},
		{"TWIN_ROOT", root},
		{"TMPDIR", root + "/T/"},
		{"TWIN_CACHE", p.Profile + "/Library/Caches/TwinDarwin"},
		{"TWIN_NAME", p.Name},
		{"TWIN_LABEL", p.Label},
		{"TWIN_ID", p.ID},
		{"TWIN_ORIG_ID", p.Src.ID},
		{"TWIN_HARDWARE", hardwareMode(p)},
		{"DYLD_INSERT_LIBRARIES", "@executable_path/libtwinisolate.dylib"},
	}
}

// twinExec is the twin's executable name. It differs from the original's so
// the two never look like the same process: some apps (RustDesk) kill other
// processes that share their name.
func twinExec(p Plan) string { return p.Src.Exec + "-" + p.Label }

func hardwareMode(p Plan) string {
	if p.OwnHardware {
		return "separate"
	}
	return "shared"
}

var steps = []step{
	{"Copy app bundle", copyBundle},
	{"Update bundle identifier", updateInfoPlist},
	{paintStep, paintIcon},
	{"Set up isolated profile", setupProfile},
	{dataStep, bringData},
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

func setupProfile(p Plan) (string, error) {
	// The profile, with the login keychain linked in: macOS looks for it under
	// $HOME, which libtwinisolate moves here.
	if err := os.MkdirAll(filepath.Join(p.Profile, "Library"), 0o755); err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	keychains := filepath.Join(p.Profile, "Library", "Keychains")
	if _, err := os.Lstat(keychains); err != nil {
		if err := os.Symlink(filepath.Join(home, "Library", "Keychains"), keychains); err != nil {
			return "", err
		}
	}

	macos := filepath.Join(p.Dest, "Contents", "MacOS")
	if err := os.Rename(filepath.Join(macos, p.Src.Exec), filepath.Join(macos, twinExec(p))); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(macos, "libtwinisolate.dylib"), isolateLib, 0o755); err != nil {
		return "", err
	}

	info := filepath.Join(p.Dest, "Contents", "Info.plist")
	_ = run("plutil", "-insert", "LSEnvironment", "-dictionary", info) // fails if the app already has one
	edits := [][]string{
		{"CFBundleExecutable", twinExec(p)},
		{"TwinOriginalBundleID", p.Src.ID},
		{"TwinProfile", p.Profile},
	}
	for _, kv := range twinEnv(p) {
		edits = append(edits, []string{"LSEnvironment." + kv[0], kv[1]})
	}
	for _, e := range edits {
		if err := run("plutil", "-replace", e[0], "-string", e[1], info); err != nil {
			return "", err
		}
	}
	return "", nil
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
