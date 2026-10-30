package main

// Integration test of a --display launcher against a real X server. It runs
// with TWIN_INTEGRATION=1 where Xvfb and xauth are installed, such as a
// Debian container: apt-get install xvfb xauth.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLauncherDisplay(t *testing.T) {
	integration(t)
	for _, tool := range []string{"Xvfb", "xauth", "sleep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("needs %s", tool)
		}
	}
	home := t.TempDir()
	auth, serverAuth := filepath.Join(home, ".Xauthority"), filepath.Join(t.TempDir(), "server-auth")
	const display, cookie = ":57", "0123456789abcdef0123456789abcdef"
	for _, file := range []string{auth, serverAuth} {
		if out, err := exec.Command("xauth", "-f", file, "add", display, ".", cookie).CombinedOutput(); err != nil {
			t.Fatalf("xauth: %v %s", err, out)
		}
	}
	start := func(name string, args []string, env ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Env = env
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	}
	start("Xvfb", []string{display, "-auth", serverAuth, "-noreset", "-nolisten", "tcp"}, "PATH="+os.Getenv("PATH"))

	// A process on that display that started first, with another desktop's
	// bus (say, a terminal command), and the desktop's session manager.
	start("sleep", []string{"60"}, "DISPLAY="+display, "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus", "XDG_CURRENT_DESKTOP=GNOME")
	manager := filepath.Join(t.TempDir(), "xfce4-session")
	if data, err := os.ReadFile(mustLookPath(t, "sleep")); err != nil || os.WriteFile(manager, data, 0o755) != nil {
		t.Fatal("couldn't copy sleep")
	}
	start(manager, []string{"60"}, "DISPLAY="+display, "DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/dbus-xfce", "XDG_CURRENT_DESKTOP=XFCE")
	for i := 0; i < 50 && !fileExists("/tmp/.X11-unix/X57"); i++ {
		time.Sleep(100 * time.Millisecond)
	}

	lib := filepath.Join(t.TempDir(), "libtwinisolate.so")
	if err := os.WriteFile(lib, isolateSO, 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := func(display string) string {
		path := filepath.Join(t.TempDir(), "launcher.sh")
		iso := isolation{Lib: lib, Name: "Fake-Display", Hash: twinHash("Fake-Display"), Hardware: "separate"}
		os.WriteFile(path, []byte(launcherScript("Fake", filepath.Join(home, "profile"), []string{"true"}, iso, session{Display: display})), 0o755)
		return path
	}
	check := func(path string) (env string, stderr string, code int) {
		cmd := exec.Command(path)
		// What a systemd user service inherits from a GNOME login.
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TWIN_CHECK_ONLY=1",
			"XAUTHORITY=/run/user/1000/.mutter-Xwaylandauth.ABC123", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"}
		var errOut strings.Builder
		cmd.Stderr = &errOut
		out, err := cmd.Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), errOut.String(), code
	}
	t.Cleanup(func() { os.RemoveAll(twinRoot(os.Getuid(), "Fake-Display")) })

	path := launcher(display)
	env, stderr, code := check(path)
	if code != 0 {
		t.Fatalf("check failed (%d): %s", code, stderr)
	}
	for _, want := range []string{"XAUTHORITY=" + auth, "DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/dbus-xfce", "XDG_CURRENT_DESKTOP=XFCE", "DISPLAY=" + display} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf("launcher environment lacks %s", want)
		}
	}

	// A cookie the display doesn't know: the server's own reason, exit 4.
	exec.Command("xauth", "-f", auth, "add", display, ".", "ffffffffffffffffffffffffffffffff").Run()
	_, stderr, code = check(path)
	if code != 4 || !strings.Contains(stderr, "can't open "+display+" with "+auth+": ") || !strings.Contains(stderr, "MIT-MAGIC-COOKIE-1") {
		t.Errorf("bad cookie: exit %d, %q", code, stderr)
	}

	// No X server there: exit 3.
	if _, stderr, code = check(launcher(":58")); code != 3 || !strings.Contains(stderr, "no X server") {
		t.Errorf("no display: exit %d, %q", code, stderr)
	}
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(path)
	return real
}
