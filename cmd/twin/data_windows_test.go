package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDataItemsWindows(t *testing.T) {
	home := t.TempDir()
	roaming, local := filepath.Join(home, `AppData\Roaming`), filepath.Join(home, `AppData\Local`)
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", local)
	exe := filepath.Join(local, `slack\slack.exe`) // a Squirrel install
	write(t, exe, "MZ")
	for _, f := range []string{`Roaming\Slack\storage\x`, `Roaming\Zoom\x`, `Local\Programs\slack\x`, `Local\SlackData\x`} {
		write(t, filepath.Join(home, "AppData", f), "x")
	}

	profile := filepath.Join(home, `.app_profiles\Slack-Work`)
	p := Plan{Src: App{Name: "Slack", Exec: exe}, Profile: profile}
	var got []string
	for _, it := range dataItems(p) {
		got = append(got, strings.TrimPrefix(it.From, home+`\`)+" → "+strings.TrimPrefix(it.To, profile+`\`))
	}
	want := []string{`AppData\Roaming\Slack → AppData\Roaming\Slack`}
	if !slices.Equal(got, want) {
		t.Errorf("dataItems:\n got %q\nwant %q", got, want)
	}
}
