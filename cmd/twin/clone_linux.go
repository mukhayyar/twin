package main

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	xdraw "golang.org/x/image/draw"
)

// A Linux twin is a .desktop entry whose launcher points HOME and the XDG
// directories at the profile, plus a repainted icon. The app itself is
// shared, so nothing large is copied (unless the twin brings the original's
// data along).
var steps = []step{
	{"Create profile folder", makeProfile},
	{dataStep, bringData},
	{paintStep, paintDesktopIcon},
	{launcherStep, writeLauncher},
	{displayStep, checkDisplay},
	{"Refresh app menu", refreshMenu},
	{serviceStep, keepServerRunning},
}

const launcherStep = "Write launcher"

// adapters cover apps that need more than the generic isolation.
var adapters = []adapter{rustdeskAdapter}

func launcherPath(id string) string { return filepath.Join(dataHome(), "twin", id+".sh") }

func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func serviceName(id string) string { return id + ".service" }

func servicePath(id string) string {
	return filepath.Join(configHome(), "systemd", "user", serviceName(id))
}

// isolateLibPath is where the embedded libtwinisolate.so is written; every
// twin preloads the same copy.
func isolateLibPath() string { return filepath.Join(dataHome(), "twin", "libtwinisolate.so") }

func iconPath(id string) string {
	return filepath.Join(dataHome(), "icons", "hicolor", "512x512", "apps", id+".png")
}

func twinTarget(src App, name, label string) (dest, id string) {
	id = "twin-" + slugify(name)
	return filepath.Join(dataHome(), "applications", id+".desktop"), id
}

func isTwin(dest string) bool {
	a, _, ok := readDesktopApp(dest)
	return ok && a.IsClone
}

func previewRows(p Plan) []string {
	rows := []string{"Launcher", tildify(p.Dest), "Profile", tildify(p.Profile)}
	if p.Session.Display != "" {
		rows = append(rows, "Display", p.Session.Display)
	}
	if p.Service {
		rows = append(rows, "Service", serviceName(p.ID)+" (systemd, user)")
	}
	return rows
}

// cleanup removes a half-built twin after a failed step, including a service
// left from an earlier version of it, which would run a missing launcher.
// The profile stays.
func cleanup(p Plan) {
	os.Remove(p.Dest)
	os.Remove(launcherPath(p.ID))
	os.Remove(renamedPath(p.ID))
	os.Remove(iconPath(p.ID))
	if _, err := os.Stat(servicePath(p.ID)); err == nil {
		run("systemctl", "--user", "disable", "--now", serviceName(p.ID))
		os.Remove(servicePath(p.ID))
		run("systemctl", "--user", "daemon-reload")
	}
}

func openTwin(p Plan) error {
	if path, err := exec.LookPath("gtk-launch"); err == nil {
		return exec.Command(path, p.ID).Start()
	}
	if path, err := exec.LookPath("gio"); err == nil {
		return exec.Command(path, "launch", p.Dest).Start()
	}
	return exec.Command(launcherPath(p.ID)).Start()
}

func makeProfile(p Plan) (string, error) {
	if err := planError(p); err != nil {
		return "", err
	}
	if isSnap(p.Src.Exec) {
		return "", fmt.Errorf("%s is a snap: snapd decides where snaps keep their data, so it can't get a separate profile", p.Src.Name)
	}
	for _, d := range xdgDirs {
		if err := os.MkdirAll(filepath.Join(p.Profile, d[1]), 0o755); err != nil {
			return "", err
		}
	}
	home, _ := os.UserHomeDir()
	for _, rel := range sharedLook {
		src, dst := filepath.Join(home, rel), filepath.Join(p.Profile, rel)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		os.MkdirAll(filepath.Dir(dst), 0o755)
		os.Symlink(src, dst)
	}
	return "", nil
}

func paintDesktopIcon(p Plan) (string, error) {
	if !p.Tint && !p.Badge {
		return "", nil
	}
	file := findIcon(p.Src.Icon)
	if file == "" {
		return "no icon file found for " + p.Src.Icon + ", kept the original", nil
	}
	img, err := loadIcon(file)
	if err != nil {
		return "couldn't read " + filepath.Base(file) + ", kept the original icon", nil
	}
	painted := paint(img, p.Label, parseHex(p.Color), p.Tint, p.Badge)
	half := image.NewNRGBA(image.Rect(0, 0, paintSize/2, paintSize/2))
	xdraw.CatmullRom.Scale(half, half.Bounds(), painted, painted.Bounds(), xdraw.Src, nil)

	out := iconPath(p.ID)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err.Error(), nil
	}
	f, err := os.Create(out)
	if err != nil {
		return err.Error(), nil
	}
	defer f.Close()
	if err := png.Encode(f, half); err != nil {
		return err.Error(), nil
	}
	return "", nil
}

// findIcon resolves an Icon= value to a file, preferring large PNGs, then
// SVG, in the hicolor theme and pixmaps, as menus would.
func findIcon(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	home, _ := os.UserHomeDir()
	bases := []string{filepath.Join(dataHome(), "icons"), filepath.Join(home, ".icons")}
	for _, d := range dataDirs() {
		bases = append(bases, filepath.Join(d, "icons"))
	}
	var candidates []string
	for _, size := range []string{"512x512", "256x256", "scalable", "192x192", "128x128", "96x96", "64x64", "48x48"} {
		for _, b := range bases {
			candidates = append(candidates,
				filepath.Join(b, "hicolor", size, "apps", name+".png"),
				filepath.Join(b, "hicolor", size, "apps", name+".svg"))
		}
	}
	for _, d := range append([]string{"/usr/share/pixmaps"}, dataDirs()...) {
		candidates = append(candidates, filepath.Join(d, name+".png"), filepath.Join(d, name+".svg"),
			filepath.Join(d, "pixmaps", name+".png"), filepath.Join(d, "pixmaps", name+".svg"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func loadIcon(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if !strings.HasSuffix(path, ".svg") {
		img, _, err := image.Decode(f)
		return img, err
	}
	icon, err := oksvg.ReadIconStream(f, oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}
	icon.SetTarget(0, 0, paintSize, paintSize)
	img := image.NewRGBA(image.Rect(0, 0, paintSize, paintSize))
	icon.Draw(rasterx.NewDasher(paintSize, paintSize, rasterx.NewScannerGV(paintSize, paintSize, img, img.Bounds())), 1)
	return img, nil
}

func writeLauncher(p Plan) (warning string, err error) {
	data, err := os.ReadFile(p.Src.Path)
	if err != nil {
		return "", err
	}
	src := parseDesktop(string(data))
	args := execArgs(src.Exec)
	if len(args) == 0 {
		return "", fmt.Errorf("%s has an empty Exec line", filepath.Base(p.Src.Path))
	}

	ses := p.Session
	if a := adapterFor(p.Src); a != nil && a.Env != nil {
		ses.Env = append(a.Env(p), ses.Env...) // the user's --env comes last and wins
	}

	var command []string
	if isFlatpak(args) {
		// Flatpak sandboxes ignore LD_PRELOAD and give each app its own /tmp.
		command = flatpakExec(args, p.Profile)
		if flatpakIsChromium(args) {
			command = append(command, "--password-store=basic")
		}
	} else {
		cmd, codes := splitFieldCodes(args)
		if exe, err := exec.LookPath(cmd[0]); err == nil {
			if real, err := filepath.EvalSymlinks(exe); err == nil && isChromiumDir(filepath.Dir(real)) {
				cmd = append(cmd, "--password-store=basic")
			}
		}
		if a := adapterFor(p.Src); a != nil && a.Rename {
			link, err := renameProgram(p.ID, cmd[0])
			if err != nil {
				warning = "couldn't run it under the twin's own name (" + err.Error() + "); " + a.Name + "'s own service may stop it"
			} else {
				cmd[0] = link
			}
		}
		iso := isolation{Name: p.Name, Hash: twinHash(p.Name), Hardware: "separate"}
		if !p.OwnHardware {
			iso.Hardware = "shared"
		}
		if len(isolateSO) > 0 {
			iso.Lib = isolateLibPath()
			if err := os.MkdirAll(filepath.Dir(iso.Lib), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(iso.Lib, isolateSO, 0o755); err != nil {
				return "", err
			}
		}
		script := launcherPath(p.ID)
		if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(script, []byte(launcherScript(p.Src.Name, p.Profile, cmd, iso, ses)), 0o755); err != nil {
			return "", err
		}
		command = append([]string{script}, codes...)
	}

	icon := src.Icon
	if _, err := os.Stat(iconPath(p.ID)); err == nil {
		icon = iconPath(p.ID)
	}
	entry := desktopTwin(p.Name, p.Src.Name+" with its own "+p.Label+" profile", joinExec(command),
		icon, src.Categories, p.Src.Path, p.Profile, src.Terminal)
	if err := os.MkdirAll(filepath.Dir(p.Dest), 0o755); err != nil {
		return "", err
	}
	return warning, os.WriteFile(p.Dest, []byte(entry), 0o644)
}

// flatpakIsChromium reports whether a `flatpak run` app is built on the
// Electron or Chromium base apps.
func flatpakIsChromium(args []string) bool {
	for _, a := range args[2:] {
		if strings.HasPrefix(a, "-") {
			continue
		}
		meta, err := output("flatpak", "info", "--show-metadata", a)
		return err == nil && (strings.Contains(meta, "org.electronjs.Electron2.BaseApp") || strings.Contains(meta, "org.chromium.Chromium.BaseApp"))
	}
	return false
}

// renamedPath is the link a renamed twin's launcher runs the app through.
func renamedPath(id string) string { return filepath.Join(dataHome(), "twin", id+".run") }

// renameProgram makes the link that runs the program under the twin's own
// name, so tools that find the original's processes by their command line
// don't match the twin's. The kernel still reports the real program as
// /proc/self/exe, where the app finds its libraries.
func renameProgram(id, program string) (string, error) {
	path, err := exec.LookPath(program)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if f, err := os.Open(real); err == nil {
		head := make([]byte, 2)
		io.ReadFull(f, head)
		f.Close()
		if string(head) == "#!" {
			return "", fmt.Errorf("%s is a script", real)
		}
	}
	link := renamedPath(id)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return "", err
	}
	os.Remove(link)
	if err := os.Symlink(real, link); err != nil {
		return "", err
	}
	return link, nil
}

// keepServerRunning installs a systemd user service that runs the twin's
// background server and starts it again whenever it stops, such as when the
// original's own service restarts the servers it finds.
func keepServerRunning(p Plan) (string, error) {
	a := adapterFor(p.Src)
	if a == nil || a.Server == nil {
		return "", fmt.Errorf("twin doesn't know how to run %s in the background", p.Src.Name)
	}
	path := servicePath(p.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(serviceUnit(p.Name, p.Src.Name, launcherPath(p.ID), a.Server)), 0o644); err != nil {
		return "", err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", serviceName(p.ID)}, {"restart", serviceName(p.ID)}} {
		if err := run("systemctl", append([]string{"--user"}, args...)...); err != nil {
			return "", err
		}
	}
	// Without lingering, user services start when the user logs in, not at
	// boot. Turning it on needs root, so twin only says how.
	if u, err := user.Current(); err == nil {
		if linger, err := output("loginctl", "show-user", u.Username, "-p", "Linger", "--value"); err == nil && strings.TrimSpace(linger) == "no" {
			return "it starts when " + u.Username + " logs in; to start it at boot instead, run: sudo loginctl enable-linger " + u.Username, nil
		}
	}
	return "", nil
}

// checkDisplay runs the launcher in check-only mode: libtwinisolate opens
// the display with the X authority the launcher found, as the twin will,
// and the launcher prints the environment it would run the twin with. The
// twin's menu entry then only shows on that display's desktop, since it
// opens there.
func checkDisplay(p Plan) (string, error) {
	cmd := exec.Command(launcherPath(p.ID))
	cmd.Env = append(os.Environ(), "TWIN_CHECK_ONLY=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	reason := strings.TrimPrefix(lines[len(lines)-1], "twin: ")
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) && exit.ExitCode() == 3 {
		return reason + "; the twin starts once it is", setDesktopKey(p.Dest, "NoDisplay", "true")
	}
	if err != nil {
		if reason == "" {
			reason = err.Error()
		}
		return "", errors.New(reason)
	}
	desktop := ""
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "XDG_CURRENT_DESKTOP="); ok {
			desktop = v
		}
	}
	switch {
	case desktop == "":
		return "", setDesktopKey(p.Dest, "NoDisplay", "true")
	case desktop != os.Getenv("XDG_CURRENT_DESKTOP"):
		return "", setDesktopKey(p.Dest, "OnlyShowIn", strings.ReplaceAll(desktop, ":", ";")+";")
	}
	return "", nil
}

// setDesktopKey sets a key in a twin's .desktop file.
func setDesktopKey(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(withDesktopKey(string(data), key, value)), 0o644)
}

// refreshMenu nudges menus that cache entries; most watch the folder anyway.
func refreshMenu(p Plan) (string, error) {
	dir := filepath.Dir(p.Dest)
	now := time.Now()
	os.Chtimes(dir, now, now)
	if path, err := exec.LookPath("update-desktop-database"); err == nil {
		if err := run(path, dir); err != nil {
			return err.Error(), nil
		}
	}
	return "", nil
}
