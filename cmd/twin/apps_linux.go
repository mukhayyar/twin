package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// hardwareOption: whether twins can get their own hardware IDs here.
	hardwareOption = true
	windowsPaths   = false
	dropHint       = "Pick from the list, or drag a .desktop file into this window."
	dirsHelp       = "(default: the XDG applications folders)"
	pathExample    = "~/Desktop/foo.desktop"
	sharedHardware = "this computer's hardware ID"
	// sessionOptions: whether --display, --env and --service exist here.
	sessionOptions = true
	platformHelp   = `      --display :N    run the twin on X display :N (say, an Xvnc desktop), with
                      that desktop's session; it checks the display opens first
      --env KEY=VALUE set a variable in the twin's launcher; repeatable
      --service       keep the twin's background server running with a
                      systemd user service (for apps twin knows, like RustDesk)

Commands:
  twin remove TWIN    stop a twin and delete it: its service, launcher, menu
                      entry and profile (asks first; -y doesn't)
  twin rustdesk-password TWIN
                      set a RustDesk twin's permanent password, read from
                      stdin or a prompt (never from the command line)
`
)

// commands are the subcommands this platform has, by name.
var commands = map[string]func(args []string, yes bool) error{
	"remove":            removeCommand,
	"rustdesk-password": rustdeskPasswordCommand,
}

func dataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func dataDirs() []string {
	dirs := filepath.SplitList(os.Getenv("XDG_DATA_DIRS"))
	if len(dirs) == 0 {
		dirs = []string{"/usr/local/share", "/usr/share"}
	}
	home, _ := os.UserHomeDir()
	for _, extra := range []string{"/var/lib/flatpak/exports/share", filepath.Join(home, ".local/share/flatpak/exports/share")} {
		if !slices.Contains(dirs, extra) {
			dirs = append(dirs, extra)
		}
	}
	return dirs
}

// searchRoots returns the folders scanned for .desktop files, most specific
// first. TWIN_DIRS overrides them.
func searchRoots() []string {
	if v := os.Getenv("TWIN_DIRS"); v != "" {
		return filepath.SplitList(v)
	}
	roots := []string{filepath.Join(dataHome(), "applications")}
	for _, d := range dataDirs() {
		roots = append(roots, filepath.Join(d, "applications"))
	}
	return append(roots, "/var/lib/snapd/desktop/applications")
}

// scanApps lists launchable apps. An entry in an earlier root hides one with
// the same desktop file ID in a later root, as the spec says, even when the
// earlier one is hidden.
func scanApps(roots []string) []App {
	seen := map[string]bool{}
	var apps []App
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			id := strings.ReplaceAll(rel, string(filepath.Separator), "-")
			if seen[id] {
				return nil
			}
			seen[id] = true
			if a, e, ok := readDesktopApp(path); ok && !a.IsClone && !e.Hidden {
				apps = append(apps, a)
			}
			return nil
		})
	}
	sortApps(apps)
	return apps
}

func readDesktopApp(path string) (App, desktopEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return App{}, desktopEntry{}, false
	}
	e := parseDesktop(string(data))
	if !e.IsApp || e.Name == "" || e.Exec == "" {
		return App{}, e, false
	}
	return App{
		Name:    strings.ReplaceAll(e.Name, "/", "-"),
		Path:    path,
		ID:      filepath.Base(path),
		Exec:    e.Exec,
		Icon:    e.Icon,
		IsClone: e.TwinSource != "",
	}, e, true
}

func isAppFile(arg string) bool { return strings.HasSuffix(arg, ".desktop") }

func appFromPath(raw string) (App, error) {
	path, err := firstPath(raw)
	if err != nil {
		return App{}, err
	}
	if !strings.HasSuffix(path, ".desktop") {
		return App{}, fmt.Errorf("%s isn't a .desktop file", filepath.Base(path))
	}
	a, _, ok := readDesktopApp(path)
	switch {
	case !ok:
		return App{}, fmt.Errorf("couldn't read an app from %s", filepath.Base(path))
	case a.IsClone:
		return App{}, fmt.Errorf("%s is already a twin", a.Name)
	}
	return a, nil
}

// findTwin finds an existing twin by name ("Slack-Work") from its .desktop
// file.
func findTwin(name string) (Plan, desktopEntry, error) {
	id := "twin-" + slugify(name)
	dest := filepath.Join(dataHome(), "applications", id+".desktop")
	data, err := os.ReadFile(dest)
	if err != nil {
		return Plan{}, desktopEntry{}, fmt.Errorf("no twin named %s (looked for %s)", name, tildify(dest))
	}
	e := parseDesktop(string(data))
	if e.TwinSource == "" {
		return Plan{}, e, fmt.Errorf("%s wasn't made by twin", tildify(dest))
	}
	return Plan{Name: e.Name, Dest: dest, ID: id, Profile: e.TwinProfile, Exists: true, ExistingIsClone: true,
		Service: fileExists(servicePath(id))}, e, nil
}

// twinPlan rebuilds the plan of an existing twin from its .desktop file and
// launcher, for commands that change a twin after it was made.
func twinPlan(name string) (Plan, error) {
	p, e, err := findTwin(name)
	if err != nil {
		return Plan{}, err
	}
	if e.TwinProfile == "" {
		return Plan{}, fmt.Errorf("%s has no profile folder; make it again with twin", p.Name)
	}
	src, _, ok := readDesktopApp(e.TwinSource)
	if !ok {
		return Plan{}, fmt.Errorf("%s was made from %s, which is gone", p.Name, tildify(e.TwinSource))
	}
	launcher, err := os.ReadFile(launcherPath(p.ID))
	if err != nil {
		return Plan{}, fmt.Errorf("%s has no launcher; make it again with twin", p.Name)
	}
	p.Src, p.Label = src, strings.TrimPrefix(p.Name, src.Name+"-")
	p.OwnHardware = !strings.Contains(string(launcher), "TWIN_HARDWARE=shared")
	return p, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// twinsOf lists the names of existing twins of the app called name.
func twinsOf(name string) []string {
	matches, _ := filepath.Glob(filepath.Join(dataHome(), "applications", "twin-*.desktop"))
	var out []string
	for _, m := range matches {
		if a, _, ok := readDesktopApp(m); ok && a.IsClone && strings.HasPrefix(a.Name, name+"-") {
			out = append(out, a.Name)
		}
	}
	return out
}
