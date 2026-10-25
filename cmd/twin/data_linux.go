package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// dataItems finds the original's data in the XDG folders and hidden folders
// of the home folder. The twin's launcher points HOME and the XDG folders at
// the profile, so each item goes to the matching place there.
func dataItems(p Plan) []dataItem {
	home, _ := os.UserHomeDir()
	args := execArgs(p.Src.Exec)
	if isFlatpak(args) {
		return flatpakData(args, home, p.Profile)
	}
	cmd, _ := splitFieldCodes(args)
	names := newDataNames(p.Src.Name, strings.TrimSuffix(p.Src.ID, ".desktop"))
	exe := ""
	if len(cmd) > 0 {
		if base := filepath.Base(cmd[0]); !launchers[base] {
			names = newDataNames(p.Src.Name, strings.TrimSuffix(p.Src.ID, ".desktop"), base)
		}
		if path, err := exec.LookPath(cmd[0]); err == nil {
			exe, _ = filepath.EvalSymlinks(path)
		}
	}

	var items []dataItem
	for _, d := range []struct{ env, def, twin string }{
		{"XDG_CONFIG_HOME", ".config", ".config"},
		{"XDG_DATA_HOME", ".local/share", ".local/share"},
		{"XDG_STATE_HOME", ".local/state", ".local/state"},
	} {
		from := os.Getenv(d.env)
		if from == "" {
			from = filepath.Join(home, d.def)
		}
		items = append(items, names.match(from, filepath.Join(p.Profile, d.twin), false)...)
	}
	items = append(items, names.match(home, p.Profile, true)...)
	return withoutApp(items, exe)
}

// launchers are programs an Exec line runs an app with; their names say
// nothing about where the app keeps its data.
var launchers = map[string]bool{
	"env": true, "sh": true, "bash": true, "electron": true, "java": true, "python": true,
	"python3": true, "node": true, "mono": true, "wine": true,
}

// flatpakData maps a flatpak's own data folders (~/.var/app/<id>) to the
// XDG folders the twin's sandbox is pointed at.
func flatpakData(args []string, home, profile string) []dataItem {
	for _, a := range args[2:] {
		if strings.HasPrefix(a, "-") {
			continue
		}
		app := filepath.Join(home, ".var", "app", a)
		items := everything(filepath.Join(app, "config"), filepath.Join(profile, ".config"))
		items = append(items, everything(filepath.Join(app, "data"), filepath.Join(profile, ".local", "share"))...)
		return append(items, everything(filepath.Join(app, ".local", "state"), filepath.Join(profile, ".local", "state"))...)
	}
	return nil
}

// copyPrefs has nothing to do on Linux: apps keep settings in files.
func copyPrefs(Plan) (bool, error) { return false, nil }

// originalRunning can't tell the original from its twins on Linux, where
// they run the same program, so it doesn't try.
func originalRunning(Plan) bool { return false }
