package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"howett.net/plist"
)

// App is an installed .app bundle.
type App struct {
	Name     string // bundle file name without .app; clone names derive from it
	Path     string
	BundleID string
	Version  string
	Exec     string
	IsClone  bool // has a <exec>.real binary, i.e. was made by twin
}

func (a App) Title() string { return a.Name }

func (a App) Description() string {
	if a.Version == "" {
		return a.BundleID
	}
	return a.BundleID + " · " + a.Version
}

func (a App) FilterValue() string { return a.Name }

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
	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	return apps
}

// findApp resolves a command-line argument: a path to a bundle, or the name
// of an installed app (case-insensitive).
func findApp(arg string, apps []App) (App, error) {
	if strings.ContainsRune(arg, '/') || strings.HasSuffix(arg, ".app") {
		return appFromPath(arg)
	}
	for _, a := range apps {
		if strings.EqualFold(a.Name, arg) {
			return a, nil
		}
	}
	return App{}, fmt.Errorf("no app named %q (pass a path to a .app instead)", arg)
}

// appFromPath accepts a bundle path as typed, pasted, or dropped from Finder
// into a terminal (quoted, ~-prefixed, or with backslash-escaped spaces).
// Only the first path counts when several are dropped at once.
func appFromPath(raw string) (App, error) {
	words := shellWords(raw)
	if len(words) == 0 {
		return App{}, fmt.Errorf("empty path")
	}
	path := strings.TrimSuffix(words[0], "/")
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, rest)
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

// looksLikePath reports whether pasted text is probably a dropped file.
func looksLikePath(s string) bool {
	s = strings.TrimLeft(strings.TrimSpace(s), `'"`)
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/")
}

// shellWords splits s like a shell would: whitespace separates words unless
// quoted or backslash-escaped.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
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
		Name:     strings.TrimSuffix(filepath.Base(path), ".app"),
		Path:     path,
		BundleID: str("CFBundleIdentifier"),
		Version:  str("CFBundleShortVersionString"),
		Exec:     exec,
		IsClone:  err == nil,
	}, true
}
