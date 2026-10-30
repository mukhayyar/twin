package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
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
