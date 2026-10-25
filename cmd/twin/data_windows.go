package main

import (
	"os"
	"path/filepath"
	"strings"
)

// dataItems finds the original's data in AppData. The twin's shortcut points
// APPDATA and LOCALAPPDATA at the profile, so each item goes to the matching
// place there. Folders that hold the program itself (Squirrel installs in
// %LOCALAPPDATA%) are left out.
func dataItems(p Plan) []dataItem {
	exe := p.Src.Exec
	base := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	names := newDataNames(p.Src.Name, base, filepath.Base(filepath.Dir(exe)))

	items := names.match(os.Getenv("APPDATA"), filepath.Join(p.Profile, `AppData\Roaming`), false)
	for _, it := range names.match(os.Getenv("LOCALAPPDATA"), filepath.Join(p.Profile, `AppData\Local`), false) {
		switch strings.ToLower(filepath.Base(it.From)) {
		case "programs", "temp", "twin":
		default:
			items = append(items, it)
		}
	}
	return withoutApp(items, filepath.Dir(exe))
}

// copyPrefs has nothing to do on Windows: twins share the registry.
func copyPrefs(Plan) (bool, error) { return false, nil }

// originalRunning can't tell the original from its twins on Windows, where
// they run the same program, so it doesn't try.
func originalRunning(Plan) bool { return false }
