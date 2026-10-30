package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// rustdeskAdapter makes RustDesk twins work. RustDesk encrypts its settings
// with the machine-id and makes its ID from the MAC address, both of which a
// twin sees changed, and its system service SIGKILLs every process whose
// command line matches "rustdesk +--server".
var rustdeskAdapter = adapter{
	Name:    "RustDesk",
	Matches: isRustDesk,
	Steps: []adapterStep{
		{After: dataStep, step: step{"Move RustDesk's secrets to the twin", rekeyRustDesk},
			When: func(p Plan) bool { return p.BringData }},
		{After: displayStep, step: step{"Check the twin's RustDesk ID", checkRustDeskID}},
	},
	Env: func(p Plan) []string {
		if p.Session.Display == "" {
			return nil
		}
		// RustDesk asks loginctl what seat0 runs, not the display it's on.
		return []string{"RUSTDESK_FORCED_DISPLAY_SERVER=x11"}
	},
	Server: []string{"--server"},
	Rename: true,
	Rows:   rustdeskRows,
}

// isRustDesk matches RustDesk by its name, desktop file or program.
func isRustDesk(a App) bool {
	if nameKey(a.Name) == "rustdesk" || strings.Contains(strings.ToLower(a.ID), "rustdesk") {
		return true
	}
	args := execArgs(a.Exec)
	if isFlatpak(args) {
		return slices.ContainsFunc(args[2:], func(s string) bool { return strings.Contains(strings.ToLower(s), "rustdesk") })
	}
	cmd, _ := splitFieldCodes(args)
	return len(cmd) > 0 && strings.HasPrefix(strings.ToLower(filepath.Base(cmd[0])), "rustdesk")
}

// rustdeskConfigDir is where a twin's RustDesk keeps its config: its
// launcher points XDG_CONFIG_HOME at the profile's .config.
func rustdeskConfigDir(profile string) string { return filepath.Join(profile, ".config", "rustdesk") }

// rustdeskOriginalDir is where the original RustDesk keeps its config.
func rustdeskOriginalDir() string { return filepath.Join(configHome(), "rustdesk") }

// errRustDeskFlatpak: libtwinisolate can't reach into a flatpak.
var errRustDeskFlatpak = errors.New("RustDesk's flatpak sees this computer's real machine-id and MAC address, so its twin would answer to RustDesk's own ID; twin RustDesk's .deb, .rpm or AppImage instead")

// rustdeskMachineID reads the machine-id the way RustDesk does (the
// machine-uid crate): /var/lib/dbus/machine-id first, trimmed.
func rustdeskMachineID() (string, error) {
	data, err := os.ReadFile("/var/lib/dbus/machine-id")
	if err != nil {
		data, err = os.ReadFile("/etc/machine-id")
	}
	return strings.TrimSpace(string(data)), err
}

// rustdeskTwinKey is the key the twin's RustDesk encrypts with: the
// machine-id libtwinisolate shows it, which it derives from /etc/machine-id
// first, or the real one when the twin shares the hardware.
func rustdeskTwinKey(p Plan) ([32]byte, error) {
	if !p.OwnHardware {
		id, err := rustdeskMachineID()
		return rustdeskKey([]byte(id)), err
	}
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		if data, err = os.ReadFile("/var/lib/dbus/machine-id"); err != nil {
			return [32]byte{}, fmt.Errorf("couldn't read the machine-id: %w", err)
		}
	}
	return rustdeskKey([]byte(twinMachineID(string(data), p.Name))), nil
}

// rekeyRustDesk moves the copied config's secrets to the twin's key. A
// config that already answers to an ID of its own (the twin made again
// with --copy-data) keeps that ID.
func rekeyRustDesk(p Plan) (string, error) {
	if isFlatpak(execArgs(p.Src.Exec)) {
		return "", errRustDeskFlatpak
	}
	dir := rustdeskConfigDir(p.Profile)
	if !fileExists(dir) {
		return "", nil
	}
	real, err := rustdeskMachineID()
	if err != nil {
		return "", fmt.Errorf("couldn't read the machine-id: %w", err)
	}
	to, err := rustdeskTwinKey(p)
	if err != nil {
		return "", err
	}
	orig := rustdeskOriginalDir()
	origID := rustdeskID(orig, rustdeskKeys{From: rustdeskOriginalKeys(orig, real)})
	own := rustdeskID(dir, rustdeskKeys{To: to})
	keys := rustdeskKeys{From: rustdeskOriginalKeys(dir, real), To: to}
	strip := own == "" || own == origID
	moved, notes, err := rekeyRustDeskConfig(dir, keys, strip)
	if err != nil {
		return "", fmt.Errorf("%w; left the twin's copy as it was", err)
	}
	s := rustdeskSummary(p.Name)
	s.moved, s.newID = moved, strip
	return strings.Join(notes, "; "), nil
}

// rustdeskDone is what the RustDesk steps found, for the twin's summary.
type rustdeskDone struct {
	id    string
	moved []string
	newID bool
}

var rustdeskDoneByTwin = map[string]*rustdeskDone{}

func rustdeskSummary(name string) *rustdeskDone {
	if rustdeskDoneByTwin[name] == nil {
		rustdeskDoneByTwin[name] = &rustdeskDone{}
	}
	return rustdeskDoneByTwin[name]
}

// checkRustDeskID asks the twin for its RustDesk ID and refuses a twin that
// answers to the original's. It also checks that libtwinisolate shows the
// twin the machine-id its secrets were encrypted for, and that RustDesk
// accepted the copied permanent password.
func checkRustDeskID(p Plan) (string, error) {
	args := execArgs(p.Src.Exec)
	if isFlatpak(args) {
		return "", errRustDeskFlatpak
	}
	id, err := getRustDeskID(launcherPath(p.ID))
	if err != nil {
		return "", fmt.Errorf("couldn't read %s's RustDesk ID (%v); not finishing a twin whose ID isn't known", p.Name, err)
	}

	var notes []string
	orig := rustdeskOriginalDir()
	realID, _ := rustdeskMachineID()
	origID := rustdeskID(orig, rustdeskKeys{From: rustdeskOriginalKeys(orig, realID)})
	if cmd, _ := splitFieldCodes(args); origID == "" && len(cmd) > 0 {
		origID, _ = getRustDeskID(cmd[0])
	}
	if note, err := compareRustDeskIDs(p, id, origID); err != nil {
		return "", err
	} else if note != "" {
		notes = append(notes, note)
	}
	rustdeskSummary(p.Name).id = id

	if p.OwnHardware {
		want, _ := rustdeskTwinKey(p)
		served, err := os.ReadFile(filepath.Join(twinRoot(os.Getuid(), p.Name), "hw", "machine-id"))
		if err == nil && rustdeskKey([]byte(strings.TrimSpace(string(served)))) != want {
			return "", errors.New("libtwinisolate shows the twin a different machine-id than twin computed, so RustDesk can't read its secrets; update twin")
		}
	}
	if p.BringData {
		if err := rustdeskPasswordKept(p); err != nil {
			return "", err
		}
	}
	return strings.Join(notes, "; "), nil
}

// rustdeskPasswordKept checks that the twin's RustDesk didn't throw away
// the copied permanent password, which it does when it can't decrypt it.
func rustdeskPasswordKept(p Plan) error {
	orig, err := readTOML(filepath.Join(rustdeskOriginalDir(), "RustDesk.toml"))
	if err != nil || orig == nil {
		return nil
	}
	if had, _ := orig.get("", "password"); had == "" {
		return nil
	}
	twin, _ := readTOML(filepath.Join(rustdeskConfigDir(p.Profile), "RustDesk.toml"))
	if twin == nil {
		return nil // never written: nothing was thrown away
	}
	if kept, _ := twin.get("", "password"); kept == "" {
		return errors.New("RustDesk dropped the copied permanent password; set one with: twin rustdesk-password " + p.Name)
	}
	return nil
}

var rustdeskIDPattern = regexp.MustCompile(`^[0-9A-Za-z._-]{1,64}$`)

// getRustDeskID runs `program --get-id`. RustDesk asks its running server
// and falls back to its config, making an ID if it has none. It needs no
// display, so a twin's launcher doesn't check one.
func getRustDeskID(program string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, "--get-id")
	cmd.Env = append(os.Environ(), "TWIN_CHECK_DISPLAY=0")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if last := strings.TrimSpace(stderr.String()); last != "" {
			lines := strings.Split(last, "\n")
			return "", fmt.Errorf("%w: %s", err, lines[len(lines)-1])
		}
		return "", err
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 || !rustdeskIDPattern.MatchString(lines[len(lines)-1]) {
		return "", errors.New("it printed no ID")
	}
	return lines[len(lines)-1], nil
}

// rustdeskRows are the summary's RustDesk lines: the twin's ID as it
// reported it, and which secrets (field names only) moved to its key.
func rustdeskRows(p Plan) []string {
	s := rustdeskSummary(p.Name)
	var rows []string
	if s.id != "" {
		rows = append(rows, "RustDesk ID", s.id)
	}
	if p.BringData {
		moved := "no secrets to re-key"
		if len(s.moved) > 0 {
			moved = "re-keyed " + strings.Join(s.moved, ", ")
		}
		if s.newID {
			moved += "; left the original's ID, keys and trusted devices behind"
		}
		rows = append(rows, "RustDesk data", moved)
	}
	return rows
}

// rustdeskPasswordCommand sets a RustDesk twin's permanent password without
// root: it writes the twin's own config, encrypted with the twin's key. The
// password comes from a prompt or stdin, never the command line, where
// anyone on the computer could read it in the process list.
func rustdeskPasswordCommand(args []string, _ bool) error {
	if len(args) != 1 {
		return errors.New("usage: twin rustdesk-password TWIN, e.g. twin rustdesk-password RustDesk-Work")
	}
	p, err := twinPlan(args[0])
	if err != nil {
		return err
	}
	if !isRustDesk(p.Src) {
		return fmt.Errorf("%s is a twin of %s, not RustDesk", p.Name, p.Src.Name)
	}
	key, err := rustdeskTwinKey(p)
	if err != nil {
		return err
	}
	password, err := readSecret("New permanent password for " + p.Name)
	if err != nil {
		return err
	}
	if n := utf8.RuneCountInString(password); n == 0 || n > 128 {
		return errors.New("the password must be 1 to 128 characters")
	}

	dir := rustdeskConfigDir(p.Profile)
	path := filepath.Join(dir, "RustDesk.toml")
	doc, err := readTOML(path)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = parseTOML("")
	}
	current, _ := doc.get("", "password")
	hashed := strings.HasPrefix(current, "01") || !strings.HasPrefix(current, "00") && rustdeskHashesPasswords(p.Src)
	salt, _ := doc.get("", "salt")
	if salt == "" {
		salt = newRustDeskSalt()
		doc.set("", "salt", salt)
	}
	doc.set("", "password", rustdeskPasswordStorage(password, salt, key, hashed))

	// A running RustDesk keeps its config in memory and would write the old
	// password back, so the twin stops first.
	restart, closed := stopTwin(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := writeSecretFile(path, []byte(doc.String())); err != nil {
		return err
	}
	fmt.Printf("Set %s's permanent password.\n", p.Name)
	if err := restart(); err != nil {
		fmt.Fprintf(os.Stderr, "twin: couldn't start %s's server again: %v\n", p.Name, err)
	}
	if closed {
		fmt.Printf("Closed %s's window to do it; open it again from the app menu.\n", p.Name)
	}
	if opts, _ := readTOML(filepath.Join(dir, "RustDesk2.toml")); opts != nil {
		if v, _ := opts.get("options", "verification-method"); v == "use-temporary-password" {
			fmt.Printf("Note: %s only accepts one-time passwords; set it to use permanent ones in its Security settings.\n", p.Name)
		}
		if v, _ := opts.get("options", "approve-mode"); v == "click" {
			fmt.Printf("Note: %s only accepts connections someone clicks to approve, not passwords.\n", p.Name)
		}
	}
	return nil
}

// rustdeskHashesPasswords reports whether the installed RustDesk stores
// hashed permanent passwords (1.4.7 and later). It assumes so when it can't
// tell.
func rustdeskHashesPasswords(src App) bool {
	cmd, _ := splitFieldCodes(execArgs(src.Exec))
	if len(cmd) == 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd[0], "--version").Output()
	if err != nil {
		return true
	}
	return !versionBefore(strings.TrimSpace(string(out)), 1, 4, 7)
}

// versionBefore reports whether a dotted version is older than major.minor.patch.
func versionBefore(v string, want ...int) bool {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	for i, w := range want {
		n := 0
		if i < len(parts) {
			n, _ = strconv.Atoi(strings.TrimFunc(parts[i], func(r rune) bool { return r < '0' || r > '9' }))
		}
		if n != w {
			return n < w
		}
	}
	return false
}

// stopTwin stops a twin's processes and returns how to start its server
// again; closed reports whether that closed its window.
func stopTwin(p Plan) (restart func() error, closed bool) {
	unit := serviceName(p.ID)
	active := p.Service && run("systemctl", "--user", "is-active", "--quiet", unit) == nil
	if active {
		run("systemctl", "--user", "stop", unit)
	}
	procs := twinProcesses(p.Name)
	server := false
	for _, proc := range procs {
		server = server || slices.Contains(proc.Args, "--server")
		closed = closed || !slices.Contains(proc.Args, "--server") && !slices.Contains(proc.Args, "--tray")
	}
	stopProcesses(procs)
	return func() error {
		switch {
		case active:
			return run("systemctl", "--user", "start", unit)
		case server:
			return startDetached(launcherPath(p.ID), "--server")
		}
		return nil
	}, closed
}

// readSecret reads a secret without echoing it: from a prompt, twice, or
// from stdin when that isn't a terminal (twin rustdesk-password Work < file).
func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(io.LimitReader(os.Stdin, 4096)).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, prompt+": ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Again: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("the passwords didn't match; nothing was changed")
	}
	return string(first), nil
}
