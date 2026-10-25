package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"
)

const (
	// hardwareOption: whether twins can get their own hardware IDs here.
	hardwareOption = true
	windowsPaths   = false
	dropHint       = "Pick from the list, or drag an app from Finder into this window."
	dirsHelp       = "(default: /Applications:~/Applications)"
	pathExample    = "~/Downloads/Foo.app"
)

// searchRoots returns the folders scanned for apps. TWIN_DIRS
// (colon-separated) overrides the defaults.
func searchRoots() []string {
	if v := os.Getenv("TWIN_DIRS"); v != "" {
		return filepath.SplitList(v)
	}
	home, _ := os.UserHomeDir()
	return []string{"/Applications", filepath.Join(home, "Applications")}
}

// scanApps finds bundles up to two levels deep (X.app, or Folder/X.app),
// sorted by name. Clones are excluded: re-wrapping one would overwrite its
// .real binary.
func scanApps(roots []string) []App {
	var apps []App
	add := func(path string) {
		if a, ok := readApp(path); ok && !a.IsClone {
			apps = append(apps, a)
		}
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(root, e.Name())
			if strings.HasSuffix(e.Name(), ".app") {
				add(path)
				continue
			}
			if !e.IsDir() {
				continue
			}
			sub, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, s := range sub {
				if strings.HasSuffix(s.Name(), ".app") {
					add(filepath.Join(path, s.Name()))
				}
			}
		}
	}
	sortApps(apps)
	return apps
}

func isAppFile(arg string) bool { return strings.HasSuffix(arg, ".app") }

// appFromPath accepts a bundle path as typed, pasted, or dropped from Finder
// into a terminal (quoted, ~-prefixed, or with backslash-escaped spaces).
// Only the first path counts when several are dropped at once.
func appFromPath(raw string) (App, error) {
	path, err := firstPath(raw)
	if err != nil {
		return App{}, err
	}
	if !strings.HasSuffix(path, ".app") {
		return App{}, fmt.Errorf("%s isn't an app", filepath.Base(path))
	}
	a, ok := readApp(path)
	switch {
	case !ok:
		return App{}, fmt.Errorf("couldn't read %s", filepath.Base(path))
	case a.IsClone:
		return App{}, fmt.Errorf("%s is already a twin", a.Name)
	}
	return a, nil
}

// twinsOf lists the names of existing clones of the app called name.
func twinsOf(name string) []string {
	entries, _ := os.ReadDir(destRoot)
	var out []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, name+"-") || !strings.HasSuffix(n, ".app") {
			continue
		}
		if a, ok := readApp(filepath.Join(destRoot, n)); ok && a.IsClone {
			out = append(out, a.Name)
		}
	}
	return out
}

func readApp(path string) (App, bool) {
	f, err := os.Open(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		return App{}, false
	}
	defer f.Close()

	var info map[string]any
	if plist.NewDecoder(f).Decode(&info) != nil {
		return App{}, false
	}
	str := func(k string) string { s, _ := info[k].(string); return s }

	exec := str("CFBundleExecutable")
	if exec == "" {
		return App{}, false
	}
	_, err = os.Stat(filepath.Join(path, "Contents", "MacOS", exec+".real"))
	return App{
		Name:    strings.TrimSuffix(filepath.Base(path), ".app"),
		Path:    path,
		ID:      str("CFBundleIdentifier"),
		Version: str("CFBundleShortVersionString"),
		Exec:    exec,
		IsClone: err == nil,
	}, true
}
