package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// dataStep copies the original app's data into the twin's profile. It only
// runs when asked for (Options.BringData); by default a twin starts fresh.
const dataStep = "Copy original's data"

// planSteps is the platform's step list for p, with its app's adapter steps,
// and without the data, display and service steps unless p asks for them.
func planSteps(p Plan) []step {
	var out []step
	for _, s := range withAdapter(p, steps) {
		if s.Title == dataStep && !p.BringData || s.Title == serviceStep && !p.Service ||
			s.Title == displayStep && p.Session.Display == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// A dataItem is one file or folder of the original's data, and where the
// twin finds its copy.
type dataItem struct{ From, To string }

// bringData copies the original's data into the profile. Anything the twin
// already has is left alone, so re-creating a twin never overwrites what it
// has saved since. Problems only warn: the twin works either way.
func bringData(p Plan) (string, error) {
	var notes []string
	if originalRunning(p) {
		notes = append(notes, p.Src.Name+" is open, so files it was writing may be incomplete; quit it for a clean copy")
	}
	found, skipped := false, 0
	for _, it := range dataItems(p) {
		found = true
		if _, err := os.Lstat(it.To); err == nil {
			continue
		}
		n, err := copyTree(it.From, it.To)
		skipped += n
		if err != nil {
			notes = append(notes, "couldn't copy "+tildify(it.From)+": "+err.Error())
		}
	}
	prefs, err := copyPrefs(p)
	if err != nil {
		notes = append(notes, "preferences: "+err.Error())
	}
	if !found && !prefs && err == nil {
		notes = append(notes, "found no data of "+p.Src.Name+" to copy")
	}
	if skipped > 0 {
		notes = append(notes, fmt.Sprintf("skipped %d files it couldn't read", skipped))
	}
	return strings.Join(notes, "; "), nil
}

// skipEntry leaves out what a copy shouldn't carry: caches the twin rebuilds,
// crash-report state, and the Singleton* files Chromium uses to hand a launch
// to its running instance. Copied, those would send the twin's windows to the
// original.
func skipEntry(name string, dir bool) bool {
	return strings.HasPrefix(name, "Singleton") || dir && cacheDirs[name]
}

var cacheDirs = map[string]bool{
	"Cache": true, "Code Cache": true, "GPUCache": true, "ShaderCache": true, "GrShaderCache": true,
	"GraphiteDawnCache": true, "DawnCache": true, "DawnGraphiteCache": true, "DawnWebGPUCache": true,
	"CachedData": true, "Crashpad": true,
}

// copyTree copies src (a file or a folder) to dst, keeping permissions and
// modification times. It copies into a temporary sibling and renames it at
// the end, so an interrupted copy never looks finished. Symlinks into src are
// re-pointed into dst; sockets and other special files are left out. It
// returns how many files it couldn't read.
func copyTree(src, dst string) (skipped int, err error) {
	given := src
	if src, err = filepath.EvalSymlinks(src); err != nil {
		return 0, err
	}
	tmp := dst + ".twin-partial"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == src {
				return err
			}
			skipped++
			return nil // WalkDir skips a folder it couldn't read
		}
		if path != src && skipEntry(d.Name(), d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(tmp, rel)
		switch t := d.Type(); {
		case t.IsDir():
			mode := fs.FileMode(0o755)
			if info, err := d.Info(); err == nil {
				mode = info.Mode().Perm()
			}
			return os.MkdirAll(target, mode|0o700)
		case t&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				skipped++
				return nil
			}
			for _, root := range []string{src, given} { // links may spell the path either way
				if filepath.IsAbs(link) && within(link, root) {
					r, _ := filepath.Rel(root, link)
					link = filepath.Join(dst, r)
					break
				}
			}
			if os.Symlink(link, target) != nil {
				skipped++
			}
			return nil
		case t.IsRegular():
			unreadable, err := copyFile(path, target)
			if unreadable {
				skipped++
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		os.RemoveAll(tmp)
		return skipped, err
	}
	return skipped, os.Rename(tmp, dst)
}

// copyFile copies one regular file. unreadable means src couldn't be opened;
// err is a failure to write the copy.
func copyFile(src, dst string) (unreadable bool, err error) {
	if cloneFile(src, dst) == nil {
		return false, nil
	}
	in, err := os.Open(src)
	if err != nil {
		return true, nil
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return true, nil
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()|0o200)
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return false, err
	}
	if err := out.Close(); err != nil {
		return false, err
	}
	os.Chtimes(dst, info.ModTime(), info.ModTime())
	return false, nil
}

// within reports whether path is inside dir.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// dataNames are the names an app's data folders go by, compared ignoring
// case and punctuation: its display name, its bundle or desktop file ID and
// that ID's last part ("com.carriez.RustDesk" → RustDesk), and so on. No app
// is listed by name; every app is matched the same way.
type dataNames map[string]bool

func newDataNames(names ...string) dataNames {
	n := dataNames{}
	add := func(s string) {
		if k := nameKey(s); len(k) >= 3 {
			n[k] = true
		}
	}
	for _, s := range names {
		add(s)
		if i := strings.LastIndexByte(s, '.'); i > 0 {
			add(s[i+1:])
		}
	}
	return n
}

func nameKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// has reports whether a file or folder name is one of the app's names.
// Leading dots are ignored, so ~/.vscode matches VSCode.
func (n dataNames) has(name string) bool { return n[nameKey(strings.TrimLeft(name, "."))] }

// publisher reports whether a folder could be the publisher's folder an app
// keeps its data in, like "Google" in Google/Chrome: its name is part of one
// of the app's names, but not all of it.
func (n dataNames) publisher(name string) bool {
	k := nameKey(name)
	if len(k) < 3 {
		return false
	}
	for key := range n {
		if key != k && strings.Contains(key, k) {
			return true
		}
	}
	return false
}

// match lists the entries of from that belong to the app, directly or one
// publisher folder down, each with its place under to. dotOnly limits it to
// hidden entries (for the home folder).
func (n dataNames) match(from, to string, dotOnly bool) []dataItem {
	entries, _ := os.ReadDir(from)
	var out []dataItem
	for _, e := range entries {
		name := e.Name()
		if dotOnly && !strings.HasPrefix(name, ".") {
			continue
		}
		switch {
		case n.has(name):
			out = append(out, dataItem{filepath.Join(from, name), filepath.Join(to, name)})
		case e.IsDir() && !dotOnly && n.publisher(name):
			subs, _ := os.ReadDir(filepath.Join(from, name))
			for _, s := range subs {
				if n.has(s.Name()) {
					out = append(out, dataItem{filepath.Join(from, name, s.Name()), filepath.Join(to, name, s.Name())})
				}
			}
		}
	}
	return out
}

// everything lists all entries of from, for folders that belong to the app
// as a whole (a sandbox container, a flatpak's data folder). Symlinks are
// left out: in a container they point back at shared folders.
func everything(from, to string) []dataItem {
	entries, _ := os.ReadDir(from)
	var out []dataItem
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 {
			out = append(out, dataItem{filepath.Join(from, e.Name()), filepath.Join(to, e.Name())})
		}
	}
	return out
}

// withoutApp drops items that contain the app itself, such as an install
// folder in ~/.local/share or %LOCALAPPDATA%: that's the program, not its
// data. It also drops items whose copy would land inside the source.
func withoutApp(items []dataItem, app string) []dataItem {
	var out []dataItem
	for _, it := range items {
		if app != "" && (it.From == app || within(app, it.From)) || within(it.To, it.From) {
			continue
		}
		out = append(out, it)
	}
	return out
}
