package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDataItemsDarwin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	lib := filepath.Join(home, "Library")
	app := filepath.Join(home, "Applications", "Visual Studio Code.app")
	write(t, filepath.Join(app, "Contents", "Info.plist"), `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleName</key><string>Code</string></dict></plist>`)
	for _, f := range []string{
		"Application Support/Code/User/settings.json",
		"Application Support/Microsoft/Code/x", // publisher folder
		"Application Support/Slack/x",          // another app
		"Preferences/com.microsoft.VSCode/config.toml",
		"Preferences/com.microsoft.VSCode.plist", // via cfprefsd instead
		"HTTPStorages/com.microsoft.VSCode/x",
		"HTTPStorages/com.microsoft.VSCode.binarycookies",
		"Containers/com.microsoft.VSCode/Data/Library/Application Support/Sandboxed/x",
	} {
		write(t, filepath.Join(lib, f), "x")
	}
	write(t, filepath.Join(home, ".vscode", "extensions", "x"), "x")
	write(t, filepath.Join(home, ".config", "code", "x"), "x")
	os.Symlink(filepath.Join(home, "Desktop"), filepath.Join(lib, "Containers/com.microsoft.VSCode/Data/Library/Application Support/Desktop"))

	profile := filepath.Join(home, ".app_profiles", "Visual Studio Code-Work")
	p := Plan{Src: App{Name: "Visual Studio Code", ID: "com.microsoft.VSCode", Path: app}, ID: "com.microsoft.VSCode.Work", Profile: profile}
	var got []string
	for _, it := range dataItems(p) {
		got = append(got, strings.TrimPrefix(it.From, home+"/")+" → "+strings.TrimPrefix(it.To, profile+"/"))
	}
	want := []string{
		"Library/Application Support/Code → Library/Application Support/Code",
		"Library/Application Support/Microsoft/Code → Library/Application Support/Microsoft/Code",
		"Library/Preferences/com.microsoft.VSCode → Library/Preferences/com.microsoft.VSCode",
		".config/code → .config/code",
		".vscode → .vscode",
		"Library/HTTPStorages/com.microsoft.VSCode → Library/HTTPStorages/com.microsoft.VSCode.Work",
		"Library/HTTPStorages/com.microsoft.VSCode.binarycookies → Library/HTTPStorages/com.microsoft.VSCode.Work.binarycookies",
		"Library/Containers/com.microsoft.VSCode/Data/Library/Application Support/Sandboxed → Library/Application Support/Sandboxed",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("dataItems:\n got %q\nwant %q", got, want)
	}
}

func TestBringDataKeepsTwinData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(home, "Library/Application Support/Fooapp/state"), "original")
	profile := filepath.Join(home, ".app_profiles", "Fooapp-Work")
	p := Plan{Src: App{Name: "Fooapp", Path: filepath.Join(home, "Fooapp.app")}, Profile: profile}

	if note, _ := bringData(p); note != "" {
		t.Errorf("first copy warned: %s", note)
	}
	twinState := filepath.Join(profile, "Library/Application Support/Fooapp/state")
	if b, _ := os.ReadFile(twinState); string(b) != "original" {
		t.Fatalf("not copied: %q", b)
	}
	os.WriteFile(twinState, []byte("twin's own"), 0o644)
	bringData(p) // re-creating the twin
	if b, _ := os.ReadFile(twinState); string(b) != "twin's own" {
		t.Errorf("re-copy overwrote the twin's data: %q", b)
	}

	empty := Plan{Src: App{Name: "Nothing", Path: filepath.Join(home, "Nothing.app")}, Profile: filepath.Join(home, ".app_profiles", "Nothing-Work")}
	if note, _ := bringData(empty); !strings.Contains(note, "found no data") {
		t.Errorf("no data, note = %q", note)
	}
}
