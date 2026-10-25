package main

import (
	"os"
	"strings"
	"testing"
)

func TestTwinEnv(t *testing.T) {
	p := Plan{Name: "RustDesk-JXB", Label: "JXB", ID: "com.carriez.rustdesk.JXB", Profile: "/Users/a/.app_profiles/RustDesk-JXB",
		Src: App{ID: "com.carriez.rustdesk", Exec: "RustDesk"}, OwnHardware: true}
	env := map[string]string{}
	for _, kv := range twinEnv(p) {
		env[kv[0]] = kv[1]
	}
	want := map[string]string{
		"TWIN_HOME":             p.Profile,
		"CFFIXED_USER_HOME":     p.Profile,
		"TWIN_ID":               "com.carriez.rustdesk.JXB",
		"TWIN_ORIG_ID":          "com.carriez.rustdesk",
		"TWIN_HARDWARE":         "separate",
		"DYLD_INSERT_LIBRARIES": "@executable_path/libtwinisolate.dylib",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if !strings.HasPrefix(env["TWIN_ROOT"], "/private/tmp/twin.") || !strings.HasPrefix(env["TMPDIR"], env["TWIN_ROOT"]) {
		t.Errorf("TWIN_ROOT %q, TMPDIR %q", env["TWIN_ROOT"], env["TMPDIR"])
	}
	if _, ok := env["HOME"]; ok {
		t.Error("HOME can't be set through LSEnvironment; libtwinisolate moves it")
	}
	if got := twinExec(p); got != "RustDesk-JXB" {
		t.Errorf("twinExec = %q", got)
	}
	p.OwnHardware = false
	for _, kv := range twinEnv(p) {
		if kv[0] == "TWIN_HARDWARE" && kv[1] != "shared" {
			t.Errorf("--share-hardware gives %q", kv[1])
		}
	}
	if len(isolateLib) < 10000 {
		t.Error("libtwinisolate.dylib isn't embedded")
	}
}

// TWIN_SETUP_BUNDLE=<copy of an .app> runs setupProfile on it, for comparing
// with what Twin.app produces.
func TestSetupProfileOnBundle(t *testing.T) {
	dest := os.Getenv("TWIN_SETUP_BUNDLE")
	if dest == "" {
		t.Skip()
	}
	a, ok := readApp(dest)
	if !ok {
		t.Fatal("not an app")
	}
	p := Plan{Name: "Slack-Work", Label: "Work", ID: "com.tinyspeck.slackmacgap.Work", Dest: dest,
		Profile: os.Getenv("TWIN_SETUP_PROFILE"), Src: App{ID: "com.tinyspeck.slackmacgap", Exec: a.Exec}, OwnHardware: true}
	if _, err := setupProfile(p); err != nil {
		t.Fatal(err)
	}
}
