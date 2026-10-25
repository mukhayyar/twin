package main

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

// A Windows twin is a Start Menu shortcut that starts the app with the user
// folders pointed at the profile, plus a repainted icon.
var steps = []step{
	{"Create profile folder", makeWinProfile},
	{dataStep, bringData},
	{paintStep, paintWinIcon},
	{"Create Start Menu shortcut", writeShortcut},
}

func iconFile(name string) string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "twin", "icons", name+".ico")
}

func twinTarget(src App, name, label string) (dest, id string) {
	return filepath.Join(twinFolder(), name+".lnk"), name
}

func isTwin(dest string) bool {
	_, err := os.Stat(dest)
	return err == nil && isUnder(dest, twinFolder())
}

func previewRows(p Plan) []string {
	return []string{"Shortcut", tildify(p.Dest), "Profile", tildify(p.Profile)}
}

// cleanup removes a half-built twin after a failed step. The profile stays.
func cleanup(p Plan) {
	os.Remove(p.Dest)
	os.Remove(iconFile(p.Name))
}

func openTwin(p Plan) error { return exec.Command("cmd", "/c", "start", "", p.Dest).Start() }

func makeWinProfile(p Plan) (string, error) {
	for _, d := range []string{`AppData\Roaming`, `AppData\Local\Temp`} {
		if err := os.MkdirAll(filepath.Join(p.Profile, d), 0o755); err != nil {
			return "", err
		}
	}
	return "", nil
}

func paintWinIcon(p Plan) (string, error) {
	if !p.Tint && !p.Badge {
		return "", nil
	}
	src := p.Src.Exec
	if loc := iconLocation(os.ExpandEnv(p.Src.Icon)); loc != "" {
		src = loc
	}
	tmp, err := os.MkdirTemp("", "twin-icon-")
	if err != nil {
		return err.Error(), nil
	}
	defer os.RemoveAll(tmp)
	extracted := filepath.Join(tmp, "icon.png")
	if _, err := powershell(winIconScript(src, extracted)); err != nil {
		return "couldn't read the app's icon, kept the original", nil
	}
	f, err := os.Open(extracted)
	if err != nil {
		return err.Error(), nil
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err.Error(), nil
	}

	painted := paint(img, p.Label, parseHex(p.Color), p.Tint, p.Badge)
	var sizes []image.Image
	for _, s := range []int{256, 64, 48, 32, 16} {
		dst := image.NewNRGBA(image.Rect(0, 0, s, s))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), painted, painted.Bounds(), xdraw.Src, nil)
		sizes = append(sizes, dst)
	}
	out := iconFile(p.Name)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err.Error(), nil
	}
	w, err := os.Create(out)
	if err != nil {
		return err.Error(), nil
	}
	defer w.Close()
	if err := encodeICO(w, sizes); err != nil {
		return err.Error(), nil
	}
	return "", nil
}

func writeShortcut(p Plan) (string, error) {
	icon := p.Src.Icon
	if _, err := os.Stat(iconFile(p.Name)); err == nil {
		icon = iconFile(p.Name) + ",0"
	}
	if err := os.MkdirAll(twinFolder(), 0o755); err != nil {
		return "", err
	}
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = `C:\Windows\System32\cmd.exe`
	}
	_, err := powershell(winShortcutScript(p.Dest, comspec, winLauncherArgs(p.Profile, p.Src.Exec, p.Src.Args),
		filepath.Dir(p.Src.Exec), icon, "Made by twin: "+p.Src.Name+" with its own "+p.Label+" profile"))
	return "", err
}
