package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// twinProcess is a running process a twin's launcher started.
type twinProcess struct {
	PID  int
	Args []string
}

// twinProcesses lists this user's processes started by the twin called name:
// its launcher leaves TWIN_NAME in their environment, which only the user
// (and root) can read. The original's processes never have it.
func twinProcesses(name string) []twinProcess {
	want := []byte("TWIN_NAME=" + name)
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	var out []twinProcess
	for _, dir := range dirs {
		pid, _ := strconv.Atoi(filepath.Base(dir))
		if pid == os.Getpid() {
			continue
		}
		env, err := os.ReadFile(filepath.Join(dir, "environ"))
		if err != nil || !containsField(env, want) {
			continue
		}
		cmdline, _ := os.ReadFile(filepath.Join(dir, "cmdline"))
		var args []string
		for _, a := range bytes.Split(bytes.TrimRight(cmdline, "\x00"), []byte{0}) {
			args = append(args, string(a))
		}
		out = append(out, twinProcess{pid, args})
	}
	return out
}

func containsField(nulSeparated, field []byte) bool {
	for _, f := range bytes.Split(nulSeparated, []byte{0}) {
		if bytes.Equal(f, field) {
			return true
		}
	}
	return false
}

// stopProcesses asks each process to quit, and kills what's left after a
// few seconds.
func stopProcesses(procs []twinProcess) {
	for _, p := range procs {
		syscall.Kill(p.PID, syscall.SIGTERM)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		alive := false
		for _, p := range procs {
			alive = alive || syscall.Kill(p.PID, 0) == nil
		}
		if !alive {
			return
		}
	}
	for _, p := range procs {
		syscall.Kill(p.PID, syscall.SIGKILL)
	}
}

// startDetached starts a command in its own session, so it outlives twin and
// the terminal it ran in.
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// removeCommand removes a twin: it stops the twin and its service, then
// deletes the service, launcher, menu entry, icon and profile. The original
// app and its data aren't touched.
func removeCommand(args []string, yes bool) error {
	if len(args) != 1 {
		return errors.New("usage: twin remove TWIN, e.g. twin remove Slack-Work (-y skips the question)")
	}
	p, _, err := findTwin(args[0])
	if err != nil {
		return err
	}
	// Only a profile in twin's own folder is deleted, whatever the entry says.
	profile := p.Profile
	if profile != "" && !within(profile, profilesRoot()) {
		fmt.Printf("Keeping %s: it's outside %s.\n", tildify(profile), tildify(profilesRoot()))
		profile = ""
	}
	if !yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("add -y to remove a twin without being asked")
		}
		what := ""
		if profile != "" {
			what = " and its profile " + tildify(profile) + " (its logins, settings and data)"
		}
		fmt.Printf("Remove %s%s? [y/N] ", p.Name, what)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Println("Kept it.")
			return nil
		}
	}

	if p.Service {
		run("systemctl", "--user", "disable", "--now", serviceName(p.ID))
		os.Remove(servicePath(p.ID))
		run("systemctl", "--user", "daemon-reload")
	}
	stopProcesses(twinProcesses(p.Name))
	for _, path := range []string{launcherPath(p.ID), renamedPath(p.ID), p.Dest, iconPath(p.ID)} {
		os.Remove(path)
	}
	os.RemoveAll(twinRoot(os.Getuid(), p.Name))
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" {
		os.RemoveAll(filepath.Join(runtime, "twin."+twinHash(p.Name)))
	}
	if profile != "" {
		if err := os.RemoveAll(profile); err != nil {
			return err
		}
	}
	refreshMenu(p)
	fmt.Printf("Removed %s.\n", p.Name)
	return nil
}
