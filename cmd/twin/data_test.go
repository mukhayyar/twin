package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestPlanSteps(t *testing.T) {
	titles := func(p Plan) []string {
		var out []string
		for _, s := range planSteps(p) {
			out = append(out, s.Title)
		}
		return out
	}
	if slices.Contains(titles(Plan{}), dataStep) {
		t.Error("data step runs although the twin starts fresh")
	}
	got := titles(Plan{BringData: true})
	if !slices.Contains(got, dataStep) || len(got) != len(steps) {
		t.Errorf("with BringData: %q", got)
	}
}

func TestDataNames(t *testing.T) {
	n := newDataNames("Visual Studio Code", "com.microsoft.VSCode", "Code")
	for _, name := range []string{"Code", "code", ".vscode", "com.microsoft.VSCode", "Visual-Studio-Code"} {
		if !n.has(name) {
			t.Errorf("has(%q) = false", name)
		}
	}
	for _, name := range []string{"Slack", "Co", "com.microsoft.Word"} {
		if n.has(name) {
			t.Errorf("has(%q) = true", name)
		}
	}
	chrome := newDataNames("Google Chrome", "com.google.Chrome", "Chrome")
	if !chrome.publisher("Google") || chrome.publisher("Mozilla") || chrome.publisher("Go") {
		t.Error("publisher folders misread")
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTree(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "Slack"), filepath.Join(dir, "twin", "Slack")
	write(t, filepath.Join(src, "settings.json"), `{"theme":"dark"}`)
	write(t, filepath.Join(src, "Local Storage", "leveldb", "000003.log"), "data")
	write(t, filepath.Join(src, "Cache", "data_0"), "cache")
	write(t, filepath.Join(src, "Default", "Code Cache", "js"), "cache")
	symlinks := runtime.GOOS != "windows"
	if symlinks {
		os.Symlink("host-1234", filepath.Join(src, "SingletonLock"))
		os.Symlink(filepath.Join(src, "settings.json"), filepath.Join(src, "current"))
		os.Symlink("settings.json", filepath.Join(src, "relative"))
	}

	if skipped, err := copyTree(src, dst); err != nil || skipped != 0 {
		t.Fatalf("copyTree = %d, %v", skipped, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "Local Storage", "leveldb", "000003.log")); string(b) != "data" {
		t.Errorf("nested file not copied: %q", b)
	}
	for _, gone := range []string{"Cache", filepath.Join("Default", "Code Cache"), "SingletonLock"} {
		if _, err := os.Lstat(filepath.Join(dst, gone)); err == nil {
			t.Errorf("%s was copied", gone)
		}
	}
	if _, err := os.Stat(dst + ".twin-partial"); err == nil {
		t.Error("temporary copy left behind")
	}
	if symlinks {
		if link, _ := os.Readlink(filepath.Join(dst, "current")); link != filepath.Join(dst, "settings.json") {
			t.Errorf("absolute symlink into the source points at %q, want the copy", link)
		}
		if link, _ := os.Readlink(filepath.Join(dst, "relative")); link != "settings.json" {
			t.Errorf("relative symlink changed to %q", link)
		}
	}
}

func TestWithoutApp(t *testing.T) {
	items := []dataItem{
		{"/home/a/.local/share/Foo", "/p/.local/share/Foo"},
		{"/home/a/.config/Foo", "/p/.config/Foo"},
	}
	got := withoutApp(items, "/home/a/.local/share/Foo/bin/foo")
	if len(got) != 1 || got[0].From != "/home/a/.config/Foo" {
		t.Errorf("install folder kept: %v", got)
	}
}
