package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
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
	{"Write launcher", writeLauncher},
	{"Refresh app menu", refreshMenu},
}

func launcherPath(id string) string { return filepath.Join(dataHome(), "twin", id+".sh") }

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
	return []string{"Launcher", tildify(p.Dest), "Profile", tildify(p.Profile)}
}

// cleanup removes a half-built twin after a failed step. The profile stays.
func cleanup(p Plan) {
	os.Remove(p.Dest)
	os.Remove(launcherPath(p.ID))
	os.Remove(iconPath(p.ID))
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

func writeLauncher(p Plan) (string, error) {
	data, err := os.ReadFile(p.Src.Path)
	if err != nil {
		return "", err
	}
	src := parseDesktop(string(data))
	args := execArgs(src.Exec)
	if len(args) == 0 {
		return "", fmt.Errorf("%s has an empty Exec line", filepath.Base(p.Src.Path))
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
		if err := os.WriteFile(script, []byte(launcherScript(p.Src.Name, p.Profile, cmd, iso)), 0o755); err != nil {
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
	return "", os.WriteFile(p.Dest, []byte(entry), 0o644)
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
