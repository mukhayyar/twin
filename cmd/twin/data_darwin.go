package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"howett.net/plist"
)

// dataItems finds the original's data. The twin's HOME is its profile, so
// each item goes to the same place under the profile. Preferences plists are
// handled by copyPrefs instead.
func dataItems(p Plan) []dataItem {
	home, _ := os.UserHomeDir()
	lib, twinLib := filepath.Join(home, "Library"), filepath.Join(p.Profile, "Library")
	names := newDataNames(p.Src.Name, p.Src.ID, bundleName(p.Src.Path))

	items := names.match(filepath.Join(lib, "Application Support"), filepath.Join(twinLib, "Application Support"), false)
	for _, it := range names.match(filepath.Join(lib, "Preferences"), filepath.Join(twinLib, "Preferences"), false) {
		if !strings.HasSuffix(it.From, ".plist") { // folders, like RustDesk's config
			items = append(items, it)
		}
	}
	items = append(items, names.match(filepath.Join(home, ".config"), filepath.Join(p.Profile, ".config"), false)...)
	items = append(items, names.match(home, p.Profile, true)...)
	items = append(items, byBundleID(lib, twinLib, p.Src.ID, p.ID)...)

	// A sandboxed app keeps everything in its container. Re-signing drops the
	// twin's sandbox, so the twin looks in its profile instead.
	if c := filepath.Join(lib, "Containers", p.Src.ID, "Data"); p.Src.ID != "" && isDir(c) {
		cLib := filepath.Join(c, "Library")
		items = append(items, everything(filepath.Join(cLib, "Application Support"), filepath.Join(twinLib, "Application Support"))...)
		for _, it := range everything(filepath.Join(cLib, "Preferences"), filepath.Join(twinLib, "Preferences")) {
			if !strings.HasSuffix(it.From, ".plist") {
				items = append(items, it)
			}
		}
		items = append(items, everything(filepath.Join(c, "Documents"), filepath.Join(p.Profile, "Documents"))...)
		items = append(items, byBundleID(cLib, twinLib, p.Src.ID, p.ID)...)
	}
	return withoutApp(items, p.Src.Path)
}

// byBundleID finds web data named after the bundle ID (cookies, website
// storage) and renames the copy after the twin's ID, which is what the twin
// looks for.
func byBundleID(lib, twinLib, id, twinID string) []dataItem {
	if id == "" {
		return nil
	}
	var out []dataItem
	for _, dir := range []string{"HTTPStorages", "WebKit", "Cookies"} {
		for _, suffix := range []string{"", ".binarycookies"} {
			from := filepath.Join(lib, dir, id+suffix)
			if _, err := os.Lstat(from); err == nil {
				out = append(out, dataItem{from, filepath.Join(twinLib, dir, twinID+suffix)})
			}
		}
	}
	return out
}

// copyPrefs gives the twin's preferences domains a copy of the original's.
// cfprefsd keeps them all in ~/Library/Preferences, so this goes through
// defaults rather than the files, and nothing cfprefsd caches goes stale.
// Domains the twin already has are left alone.
func copyPrefs(p Plan) (bool, error) {
	if p.Src.ID == "" {
		return false, nil
	}
	home, _ := os.UserHomeDir()
	container := filepath.Join(home, "Library", "Containers", p.Src.ID, "Data", "Library", "Preferences")
	main := filepath.Join(container, p.Src.ID+".plist")
	if _, err := os.Stat(main); err != nil {
		main = "" // not sandboxed: the domain itself
	}
	copied, err := importDomain(p.Src.ID, main, p.ID)
	if err != nil {
		return copied, err
	}
	// A container holds only the app's own domains. libtwinisolate gives the
	// twin "<domain>.twin-<label>" for each one besides the main domain.
	entries, _ := os.ReadDir(container)
	for _, e := range entries {
		d, isPlist := strings.CutSuffix(e.Name(), ".plist")
		if !isPlist || strings.EqualFold(d, p.Src.ID) || strings.HasPrefix(d, "com.apple.") || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		ok, err := importDomain(d, filepath.Join(container, e.Name()), d+".twin-"+p.Label)
		copied = copied || ok
		if err != nil {
			return copied, err
		}
	}
	return copied, nil
}

// importDomain copies the preferences domain from (or the plist file, if
// given) into the domain to, unless to already exists.
func importDomain(from, file, to string) (bool, error) {
	if run("defaults", "read", to) == nil {
		return false, nil
	}
	if file == "" {
		if run("defaults", "read", from) != nil {
			return false, nil // nothing to copy
		}
		f, err := os.CreateTemp("", "twin-prefs-*.plist")
		if err != nil {
			return false, err
		}
		f.Close()
		defer os.Remove(f.Name())
		if err := run("defaults", "export", from, f.Name()); err != nil {
			return false, err
		}
		file = f.Name()
	}
	if err := run("defaults", "import", to, file); err != nil {
		return false, err
	}
	return true, nil
}

// originalRunning reports whether the original app is open.
func originalRunning(p Plan) bool {
	exe := filepath.Join(p.Src.Path, "Contents", "MacOS", p.Src.Exec)
	return run("pgrep", "-f", "^"+regexp.QuoteMeta(exe)) == nil
}

// bundleName is the app's CFBundleName, which Electron apps and others name
// their data folder after ("Code" for Visual Studio Code).
func bundleName(app string) string {
	f, err := os.Open(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	defer f.Close()
	var info struct {
		Name string `plist:"CFBundleName"`
	}
	plist.NewDecoder(f).Decode(&info)
	return info.Name
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
