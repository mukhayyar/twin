package main

// Integration tests against the real libtwinisolate and, when installed, the
// real RustDesk. They only run with TWIN_INTEGRATION=1. They work in a
// temporary home folder, never start a RustDesk server, and leave the
// user's own RustDesk alone: the twin's RustDesk only runs --get-id and
// --version.

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func integration(t *testing.T) {
	t.Helper()
	if os.Getenv("TWIN_INTEGRATION") != "1" {
		t.Skip("set TWIN_INTEGRATION=1 to run")
	}
	if len(isolateSO) == 0 {
		t.Skip("no libtwinisolate for this architecture")
	}
}

// TestIsolateMachineIDMatchesGo checks that libtwinisolate shows a twin the
// machine-id twinMachineID computes, at both paths RustDesk reads.
func TestIsolateMachineIDMatchesGo(t *testing.T) {
	integration(t)
	lib := filepath.Join(t.TempDir(), "libtwinisolate.so")
	if err := os.WriteFile(lib, isolateSO, 0o755); err != nil {
		t.Fatal(err)
	}
	real, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		t.Skip("no /etc/machine-id")
	}
	const name = "RustDesk-ITest"
	// libtwinisolate names the twin's runtime folder after TWIN_ROOT's suffix.
	root := filepath.Join(t.TempDir(), "twin.itest")
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		t.Cleanup(func() { os.RemoveAll(filepath.Join(run, "twin.itest")) })
	}
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		cmd := exec.Command("cat", path)
		cmd.Env = append(os.Environ(), "LD_PRELOAD="+lib, "TWIN_NAME="+name, "TWIN_ROOT="+root, "TWIN_HARDWARE=separate")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		served := strings.TrimSpace(string(out))
		if served != twinMachineID(string(real), name) {
			t.Errorf("%s: libtwinisolate and twinMachineID disagree", path) // the IDs aren't printed: they're key material
		}
		if served == strings.TrimSpace(string(real)) {
			t.Errorf("%s: the twin sees the real machine-id", path)
		}
	}
}

// TestRustDeskTwinEndToEnd makes a RustDesk twin with copied data from a
// made-up RustDesk config in a temporary home, and lets the real RustDesk
// load it.
func TestRustDeskTwinEndToEnd(t *testing.T) {
	integration(t)
	desktop := "/usr/share/applications/rustdesk.desktop"
	src, _, ok := readDesktopApp(desktop)
	if !ok {
		t.Skip("RustDesk isn't installed")
	}
	realMachine, err := rustdeskMachineID()
	if err != nil {
		t.Skip("no machine-id")
	}
	realHome, _ := os.UserHomeDir()
	mainID := rustdeskID(rustdeskOriginalDir(), rustdeskKeys{From: rustdeskOriginalKeys(rustdeskOriginalDir(), realMachine)})

	// Not under /tmp, which libtwinisolate moves out of the twin's sight.
	home, err := os.MkdirTemp(realHome, ".twin-integration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("HOME", home)
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "DISPLAY", "WAYLAND_DISPLAY"} {
		t.Setenv(k, "")
	}
	// The "original": RustDesk 1.5's layout, encrypted for this machine.
	key := rustdeskKey([]byte(realMachine))
	const salt, password, fakeID = "k7mz2p9qrs3tuvw4xy5abc6defgh8ijn", "it-Pass-123", "123456789"
	orig := rustdeskOriginalDir()
	write(t, filepath.Join(orig, "RustDesk.toml"), "enc_id = '"+enc(fakeID, key, true)+"'\npassword = '"+
		rustdeskPasswordStorage(password, salt, key, true)+"'\nsalt = '"+salt+"'\nkey_confirmed = true\n\n[keys_confirmed]\nrs-ny = true\n")
	write(t, filepath.Join(orig, "RustDesk2.toml"), "unlock_pin = '"+enc("4321", key, true)+"'\n\n[options]\nverification-method = 'use-permanent-password'\n")

	p := newPlan(src, "ITest", Options{OwnHardware: true, BringData: true})
	t.Cleanup(func() {
		os.RemoveAll(twinRoot(os.Getuid(), p.Name))
		if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
			os.RemoveAll(filepath.Join(run, "twin."+twinHash(p.Name)))
		}
	})
	for _, s := range planSteps(p) {
		if s.Title == "Refresh app menu" {
			continue
		}
		warning, err := s.Run(p)
		if err != nil {
			t.Fatalf("%s: %v", s.Title, err)
		}
		if warning != "" {
			t.Logf("%s: %s", s.Title, warning)
		}
	}

	twinKey, _ := rustdeskTwinKey(p)
	dir := rustdeskConfigDir(p.Profile)
	id := rustdeskID(dir, rustdeskKeys{To: twinKey})
	if id == "" || id == fakeID || id == mainID {
		t.Fatalf("the twin's ID is empty or someone else's")
	}
	t.Logf("the twin's RustDesk ID: %s", id)
	if !fileExists(filepath.Join(twinRoot(os.Getuid(), p.Name), "hw", "machine-id")) {
		t.Error("libtwinisolate didn't run inside the twin's RustDesk")
	}
	assertPassword := func(want string) {
		t.Helper()
		doc, _ := readTOML(filepath.Join(dir, "RustDesk.toml"))
		stored, _ := doc.get("", "password")
		gotSalt, _ := doc.get("", "salt")
		inner, ok := decWith(t, stored, twinKey)
		h1 := sha256.Sum256([]byte(want + gotSalt))
		if !ok || inner != "00"+base64.StdEncoding.EncodeToString(h1[:]) {
			t.Errorf("the twin's stored password isn't %q under its own key", want)
		}
	}
	assertPassword(password) // RustDesk loaded and stored it without throwing it away

	// rustdesk-password, with the password on stdin.
	r, w, _ := os.Pipe()
	w.WriteString("new-Pass-456\n")
	w.Close()
	stdin := os.Stdin
	os.Stdin = r
	err = rustdeskPasswordCommand([]string{p.Name}, false)
	os.Stdin = stdin
	if err != nil {
		t.Fatal(err)
	}
	if _, err := getRustDeskID(launcherPath(p.ID)); err != nil { // RustDesk loads and stores its config again
		t.Fatal(err)
	}
	assertPassword("new-Pass-456")

	// The renamed program runs, and the main service's kill pattern misses it.
	link := renamedPath(p.ID)
	if out, err := exec.Command(link, "--version").Output(); err != nil || strings.TrimSpace(string(out)) == "" {
		t.Errorf("%s --version: %v", link, err)
	}
	if regexp.MustCompile(`rustdesk +--server`).MatchString(link + " --server") {
		t.Errorf("%s --server matches the service's kill pattern", link)
	}
	if launcher, _ := os.ReadFile(launcherPath(p.ID)); !strings.Contains(string(launcher), shellQuote(link)) {
		t.Error("the launcher doesn't run the renamed program")
	}
}
