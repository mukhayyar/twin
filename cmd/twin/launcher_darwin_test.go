package main

import (
	"strings"
	"testing"
)

func TestLauncher(t *testing.T) {
	p := Plan{Name: `Odd "$name"`, Label: "Work", ID: "com.example.app.Work", Src: App{ID: "com.example.app", Exec: "Odd"}}
	s := launcher(p)
	for _, want := range []string{
		`PROFILE_DIR="$HOME/.app_profiles/Odd \"\$name\""`,
		`[ -e "$PROFILE_DIR/Library/Keychains" ] || ln -s "$HOME/Library/Keychains" "$PROFILE_DIR/Library/Keychains"`,
		`TWIN_ID="com.example.app.Work" TWIN_ORIG_ID="com.example.app"`,
		`export TWIN_ROOT="/private/tmp/twin.$UID.` + twinHash(p.Name) + `"`,
		`export DYLD_INSERT_LIBRARIES="$(dirname "$0")/libtwinisolate.dylib"`,
		`exec "$(dirname "$0")/Odd.real" "$@"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("launcher missing %q:\n%s", want, s)
		}
	}
	// The keychain link must be made while $HOME is still the real home.
	if strings.Index(s, "ln -s") > strings.Index(s, `export HOME=`) {
		t.Error("keychain link comes after HOME moves")
	}
	if len(isolateLib) < 10000 {
		t.Error("libtwinisolate.dylib isn't embedded")
	}
}
