package main

// Linux app menus are built from .desktop files (the freedesktop.org Desktop
// Entry Specification). Reading and writing them is plain text work, so it
// lives here without a build tag and is tested on every platform.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type desktopEntry struct {
	Name, Exec, Icon, Categories string
	TwinSource                   string // set on twins: the entry they came from
	TwinProfile                  string // set on twins: their profile folder
	IsApp, Terminal, Hidden      bool   // Hidden covers NoDisplay and Hidden
}

// parseDesktop reads the [Desktop Entry] group. Localized keys are skipped,
// so Name is the untranslated one.
func parseDesktop(data string) desktopEntry {
	var e desktopEntry
	inMain := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || line[0] == '#':
			continue
		case line[0] == '[':
			inMain = line == "[Desktop Entry]"
			continue
		case !inMain:
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = unescapeDesktop(strings.TrimSpace(v))
		switch strings.TrimSpace(k) {
		case "Type":
			e.IsApp = v == "Application"
		case "Name":
			e.Name = v
		case "Exec":
			e.Exec = v
		case "Icon":
			e.Icon = v
		case "Categories":
			e.Categories = v
		case "Terminal":
			e.Terminal = v == "true"
		case "NoDisplay", "Hidden":
			e.Hidden = e.Hidden || v == "true"
		case "X-Twin-Source":
			e.TwinSource = v
		case "X-Twin-Profile":
			e.TwinProfile = v
		}
	}
	return e
}

func unescapeDesktop(v string) string {
	r := strings.NewReplacer(`\s`, " ", `\n`, "\n", `\t`, "\t", `\r`, "\r", `\\`, `\`)
	return r.Replace(v)
}

func escapeDesktop(v string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	return r.Replace(v)
}

// execArgs splits an Exec value into arguments: spaces separate them, double
// quotes group them, and inside quotes a backslash escapes the next rune.
// Field codes such as %U are kept as their own arguments.
func execArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inQuote, escaped, started := false, false, false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case inQuote && r == '\\':
			escaped = true
		case r == '"':
			inQuote, started = !inQuote, true
		case !inQuote && (r == ' ' || r == '\t'):
			if started || cur.Len() > 0 {
				args = append(args, strings.ReplaceAll(cur.String(), "%%", "%"))
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if started || cur.Len() > 0 {
		args = append(args, strings.ReplaceAll(cur.String(), "%%", "%"))
	}
	return args
}

func isFieldCode(a string) bool {
	return len(a) == 2 && a[0] == '%' && strings.ContainsRune("fFuUdDnNickvm", rune(a[1]))
}

// splitFieldCodes separates the command from its field codes, keeping only
// the file and URL codes (%f %F %u %U) that a launcher should forward.
func splitFieldCodes(args []string) (cmd, codes []string) {
	for _, a := range args {
		switch {
		case a == "%f" || a == "%F" || a == "%u" || a == "%U":
			codes = append(codes, a)
		case !isFieldCode(a):
			cmd = append(cmd, a)
		}
	}
	return cmd, codes
}

// joinExec is the inverse of execArgs: it quotes arguments with reserved
// characters and escapes literal percent signs.
func joinExec(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if isFieldCode(a) {
			out[i] = a
			continue
		}
		a = strings.ReplaceAll(a, "%", "%%")
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`") {
			a = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(a) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

func isFlatpak(args []string) bool {
	return len(args) > 1 && path.Base(args[0]) == "flatpak" && args[1] == "run"
}

// isSnap spots snap launchers. Snaps get their data folders from snapd and
// can't be pointed elsewhere.
func isSnap(exec string) bool {
	return strings.Contains(exec, "/snap/bin/") || strings.HasPrefix(exec, "snap run ")
}

// xdgDirs are where a twin's profile keeps each XDG base directory.
var xdgDirs = [][2]string{
	{"XDG_CONFIG_HOME", ".config"},
	{"XDG_DATA_HOME", ".local/share"},
	{"XDG_CACHE_HOME", ".cache"},
	{"XDG_STATE_HOME", ".local/state"},
}

// sharedLook lists settings a twin borrows from the real home through
// symlinks, so it keeps the desktop's theme, fonts and cursor while its own
// data stays separate.
var sharedLook = []string{
	".config/dconf", ".config/fontconfig", ".config/gtk-3.0", ".config/gtk-4.0", ".config/kdeglobals",
	".local/share/fonts", ".local/share/icons", ".local/share/themes", ".fonts", ".icons", ".themes",
}

// flatpakExec rewrites `flatpak run …` so the sandboxed app keeps its data in
// the profile: flatpak sets the XDG directories itself, so they're overridden
// with --env, and the profile is shared into the sandbox. These are Linux
// paths whatever platform builds them, hence path rather than filepath.
func flatpakExec(args []string, profile string) []string {
	out := []string{args[0], "run", "--filesystem=" + profile}
	for _, d := range xdgDirs {
		out = append(out, "--env="+d[0]+"="+path.Join(profile, d[1]))
	}
	return append(out, args[2:]...)
}

// isolation is what a Linux twin's launcher sets up beyond HOME and XDG:
// libtwinisolate (empty Lib: none) and its settings.
type isolation struct {
	Lib      string // path to libtwinisolate.so
	Name     string // twin name, salts the hardware IDs
	Hash     string // short per-twin ID for TWIN_ROOT
	Hardware string // "separate" or "shared"
}

// session is where a twin's launcher runs it: on the desktop it was started
// from, or on a given X display, with any extra environment variables.
type session struct {
	Display string   // X display such as ":3"; empty: wherever it's started
	Env     []string // KEY=value, set last so they win
}

// launcherScript runs cmd with HOME and the XDG directories pointed at the
// profile, and libtwinisolate preloaded for /tmp, runtime sockets and
// hardware IDs. XAUTHORITY is pinned first, while HOME is still the user's:
// X11 looks for ~/.Xauthority otherwise. With TWIN_CHECK_ONLY set, it only
// checks the display (see libtwinisolate) and prints the environment it
// would run the app with.
func launcherScript(appName, profile string, cmd []string, iso isolation, ses session) string {
	quoted := make([]string, len(cmd))
	for i, a := range cmd {
		quoted[i] = shellQuote(a)
	}
	var env, dirs []string
	for _, d := range xdgDirs {
		env = append(env, fmt.Sprintf(`%s="$P/%s"`, d[0], d[1]))
		dirs = append(dirs, `"$`+d[0]+`"`)
	}
	preload := ""
	if iso.Lib != "" {
		preload = fmt.Sprintf(`export TWIN_NAME=%s TWIN_ROOT="/tmp/twin.$(id -u).%s" TWIN_HARDWARE=%s
export LD_PRELOAD=%s"${LD_PRELOAD:+:$LD_PRELOAD}"
`, shellQuote(iso.Name), iso.Hash, iso.Hardware, shellQuote(iso.Lib))
	}
	auth := `export XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"` + "\n"
	if ses.Display != "" {
		auth = "" // the display's own session decides
	}
	return fmt.Sprintf(`#!/bin/sh
# Made by twin: runs %s with its own profile. Delete it along with the twin.
P=%s
%s%sexport HOME="$P" %s
mkdir -p %s
%s[ -z "${TWIN_CHECK_ONLY:-}" ] || exec env
exec %s "$@"
`, appName, shellQuote(profile), auth, sessionScript(ses), strings.Join(env, " "), strings.Join(dirs, " "), preload, strings.Join(quoted, " "))
}

// sessionScript points the launcher at an X display and the desktop
// session running there: it takes D-Bus, X authority and desktop name from
// the session's environment, so menus, tray icons and notifications reach
// that desktop. Without its own X authority, the session uses the user's
// ~/.Xauthority, never one inherited from another desktop (GNOME's Xwayland
// cookie, say, which the display refuses). libtwinisolate then checks that
// the display opens before the app starts, unless the caller passes
// TWIN_CHECK_DISPLAY=0 for a command that doesn't need it.
//
// The session is its session manager if one runs there, else a panel or
// window manager, else its oldest process. Twins never count, and for the
// last, neither do processes started over SSH or by systemd.
func sessionScript(ses session) string {
	var b strings.Builder
	if ses.Display != "" {
		fmt.Fprintf(&b, `export DISPLAY=%s XDG_SESSION_TYPE=x11 XAUTHORITY="$HOME/.Xauthority" TWIN_CHECK_DISPLAY="${TWIN_CHECK_DISPLAY:-1}"
unset WAYLAND_DISPLAY
session= rank=9 low=
for e in /proc/[0-9]*/environ; do
	grep -qzxF "DISPLAY=$DISPLAY" "$e" 2>/dev/null && grep -qz '^DBUS_SESSION_BUS_ADDRESS=' "$e" && ! grep -qz '^TWIN_NAME=' "$e" || continue
	pid=${e#/proc/}; pid=${pid%%/environ}
	case $(cat "/proc/$pid/comm" 2>/dev/null) in
	%s) r=1 ;;
	%s) r=2 ;;
	*) ! grep -qzE '^(SSH_CONNECTION|INVOCATION_ID)=' "$e" || continue; r=3 ;;
	esac
	if [ "$r" -lt "$rank" ] || { [ "$r" -eq "$rank" ] && [ "$pid" -lt "$low" ]; }; then
		session=$e rank=$r low=$pid
	fi
done
for v in DBUS_SESSION_BUS_ADDRESS XAUTHORITY XDG_CURRENT_DESKTOP; do
	kv=$([ -n "$session" ] && grep -z "^$v=" "$session" | tr -d '\0')
	[ -z "$kv" ] || export "$kv"
done
`, shellQuote(ses.Display), strings.Join(sessionManagers, "|"), strings.Join(desktopShells, "|"))
	}
	for _, kv := range ses.Env {
		k, v, _ := strings.Cut(kv, "=")
		fmt.Fprintf(&b, "export %s=%s\n", k, shellQuote(v))
	}
	return b.String()
}

// sessionManagers and desktopShells are process names (as /proc/PID/comm
// shows them, cut to 15 characters) that run a desktop session.
var (
	sessionManagers = []string{"xfce4-session", "gnome-session*", "ksmserver", "plasma_session", "startplasma-x11",
		"mate-session", "cinnamon-sessio*", "lxsession", "lxqt-session", "budgie-desktop", "openbox-session", "startxfce4"}
	desktopShells = []string{"xfce4-panel", "xfwm4", "xfdesktop", "plasmashell", "kwin_x11", "mate-panel", "marco",
		"cinnamon", "lxpanel", "lxqt-panel", "openbox", "i3", "fluxbox", "icewm*", "awesome", "xmonad*", "gnome-shell"}
)

// serviceUnit is a systemd user service that runs a twin's launcher with
// args and starts it again whenever it stops, without ever giving up.
func serviceUnit(name, appName, launcher string, args []string) string {
	return fmt.Sprintf(`# Made by twin: keeps %s's server running. Delete it along with the twin.
[Unit]
Description=%s, a twin of %s
StartLimitIntervalSec=0

[Service]
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, name, systemdEscape(name), systemdEscape(appName), systemdExec(append([]string{launcher}, args...)))
}

// systemdExec quotes a command for ExecStart=, each argument in double quotes.
func systemdExec(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(systemdEscape(a)) + `"`
	}
	return strings.Join(out, " ")
}

// systemdEscape keeps systemd from expanding specifiers (%) and variables ($).
func systemdEscape(s string) string { return strings.NewReplacer("%", "%%", "$", "$$").Replace(s) }

// chromiumMarkers are files only Chromium-based apps (Chrome, Electron apps
// like Slack, Discord, VS Code) ship next to their executable.
var chromiumMarkers = []string{"chrome-sandbox", "chrome_crashpad_handler", "resources/app.asar", "resources/electron.asar"}

// isChromiumDir reports whether dir holds a Chromium-based app. Those keep
// their encryption key in the desktop keyring, shared by every copy, unless
// told --password-store=basic, which keeps it in the profile instead.
func isChromiumDir(dir string) bool {
	for _, m := range chromiumMarkers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// desktopTwin renders a twin's .desktop file.
func desktopTwin(name, comment, exec, icon, categories, source, profile string, terminal bool) string {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s=%s\n", k, escapeDesktop(v))
		}
	}
	b.WriteString("[Desktop Entry]\nType=Application\n")
	line("Name", name)
	line("Comment", comment)
	b.WriteString("Exec=" + strings.ReplaceAll(exec, `\`, `\\`) + "\n")
	line("Icon", icon)
	fmt.Fprintf(&b, "Terminal=%t\n", terminal)
	line("Categories", categories)
	b.WriteString("StartupNotify=true\n")
	line("X-Twin-Source", source)
	line("X-Twin-Profile", profile)
	return b.String()
}

// withDesktopKey sets key=value in a twin's .desktop entry, which only has
// the [Desktop Entry] group.
func withDesktopKey(entry, key, value string) string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(entry, "\n"), "\n") {
		if !strings.HasPrefix(line, key+"=") {
			out = append(out, line)
		}
	}
	return strings.Join(append(out, key+"="+escapeDesktop(value)), "\n") + "\n"
}

// slugify makes a file-name-safe ID: lowercase letters, digits and dashes.
func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
