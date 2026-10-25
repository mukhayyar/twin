package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDataItemsLinux(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, "")
	}
	exe := filepath.Join(home, ".local/share/Fooapp/bin/fooapp") // installed in the data folder
	write(t, exe, "#!/bin/sh\n")
	os.Chmod(exe, 0o755)
	for _, f := range []string{".config/fooapp/settings.json", ".config/Other/x", ".local/state/fooapp/x", ".fooapp/x", "visible-fooapp/x"} {
		write(t, filepath.Join(home, f), "x")
	}
	write(t, filepath.Join(home, ".var/app/com.example.Foo/config/foo/x"), "x")
	write(t, filepath.Join(home, ".var/app/com.example.Foo/data/foo/x"), "x")

	profile := filepath.Join(home, ".app_profiles", "Fooapp-Work")
	list := func(p Plan) []string {
		var got []string
		for _, it := range dataItems(p) {
			got = append(got, strings.TrimPrefix(it.From, home+"/")+" → "+strings.TrimPrefix(it.To, profile+"/"))
		}
		slices.Sort(got)
		return got
	}

	native := Plan{Src: App{Name: "Fooapp", ID: "fooapp.desktop", Exec: exe + " %U"}, Profile: profile}
	want := []string{".config/fooapp → .config/fooapp", ".fooapp → .fooapp", ".local/state/fooapp → .local/state/fooapp"}
	if got := list(native); !slices.Equal(got, want) {
		t.Errorf("native app:\n got %q\nwant %q", got, want)
	}

	flatpak := Plan{Src: App{Name: "Foo", ID: "com.example.Foo.desktop", Exec: "/usr/bin/flatpak run --branch=stable com.example.Foo @@u %U @@"}, Profile: profile}
	want = []string{".var/app/com.example.Foo/config/foo → .config/foo", ".var/app/com.example.Foo/data/foo → .local/share/foo"}
	if got := list(flatpak); !slices.Equal(got, want) {
		t.Errorf("flatpak:\n got %q\nwant %q", got, want)
	}
}
