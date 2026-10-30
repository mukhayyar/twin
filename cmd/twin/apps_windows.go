package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// hardwareOption: whether twins can get their own hardware IDs here.
	hardwareOption = false
	windowsPaths   = true
	dropHint       = "Pick from the list, or drag a shortcut or .exe into this window."
	dirsHelp       = "(default: the Start Menu folders)"
	pathExample    = `C:\Tools\Foo.lnk`
	sharedHardware = ""
	// sessionOptions: whether --display, --env and --service exist here.
	sessionOptions = false
	platformHelp   = ""
)

// commands are the subcommands this platform has, by name.
var commands map[string]func(args []string, yes bool) error

func startMenuDirs() []string {
	return []string{
		filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`),
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`),
	}
}

// twinFolder is the Start Menu folder that holds twins.
func twinFolder() string {
	return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Twin`)
}

func isUnder(path, dir string) bool {
	return strings.HasPrefix(strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(dir))+`\`)
}

func searchRoots() []string {
	if v := os.Getenv("TWIN_DIRS"); v != "" {
		return filepath.SplitList(v)
	}
	return startMenuDirs()
}

type shortcut struct{ Path, Name, Target, Args, Icon string }

func readShortcuts(paths []string) []shortcut {
	out, err := powershell(winScanScript(paths))
	if err != nil {
		return nil
	}
	var list []shortcut
	json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(out, "\ufeff"))), &list)
	return list
}

func powershell(script string) (string, error) {
	return output("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

func shortcutApp(s shortcut) App {
	return App{Name: s.Name, Path: s.Path, ID: s.Target, Exec: s.Target, Args: s.Args, Icon: s.Icon,
		IsClone: isUnder(s.Path, twinFolder())}
}

// scanApps lists Start Menu shortcuts to programs, skipping uninstallers and
// duplicates.
func scanApps(roots []string) []App {
	seen := map[string]bool{}
	var apps []App
	for _, s := range readShortcuts(roots) {
		key := strings.ToLower(s.Name)
		if !strings.EqualFold(filepath.Ext(s.Target), ".exe") || strings.Contains(key, "uninstall") || seen[key] {
			continue
		}
		if a := shortcutApp(s); !a.IsClone {
			seen[key] = true
			apps = append(apps, a)
		}
	}
	sortApps(apps)
	return apps
}

func isAppFile(arg string) bool {
	ext := strings.ToLower(filepath.Ext(arg))
	return ext == ".lnk" || ext == ".exe"
}

func appFromPath(raw string) (App, error) {
	path, err := firstPath(raw)
	if err != nil {
		return App{}, err
	}
	base := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe":
		if _, err := os.Stat(path); err != nil {
			return App{}, fmt.Errorf("couldn't find %s", base)
		}
		return App{Name: strings.TrimSuffix(base, filepath.Ext(base)), Path: path, ID: path, Exec: path}, nil
	case ".lnk":
		list := readShortcuts([]string{path})
		if len(list) == 0 {
			return App{}, fmt.Errorf("couldn't read %s", base)
		}
		a := shortcutApp(list[0])
		switch {
		case a.IsClone:
			return App{}, fmt.Errorf("%s is already a twin", a.Name)
		case !strings.EqualFold(filepath.Ext(a.Exec), ".exe"):
			return App{}, fmt.Errorf("%s doesn't point to a program", base)
		}
		return a, nil
	}
	return App{}, fmt.Errorf("%s isn't a shortcut or .exe", base)
}

// twinsOf lists the names of existing twins of the app called name.
func twinsOf(name string) []string {
	entries, _ := os.ReadDir(twinFolder())
	var out []string
	for _, e := range entries {
		if n := e.Name(); strings.HasPrefix(n, name+"-") && strings.EqualFold(filepath.Ext(n), ".lnk") {
			out = append(out, strings.TrimSuffix(n, filepath.Ext(n)))
		}
	}
	return out
}
