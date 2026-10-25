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

// launcherScript runs cmd with HOME and the XDG directories pointed at the
// profile, and libtwinisolate preloaded for /tmp, runtime sockets and
// hardware IDs. XAUTHORITY is pinned first: X11 looks for ~/.Xauthority
// otherwise.
func launcherScript(appName, profile string, cmd []string, iso isolation) string {
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
	return fmt.Sprintf(`#!/bin/sh
# Made by twin: runs %s with its own profile. Delete it along with the twin.
P=%s
export XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"
export HOME="$P" %s
mkdir -p %s
%sexec %s "$@"
`, appName, shellQuote(profile), strings.Join(env, " "), strings.Join(dirs, " "), preload, strings.Join(quoted, " "))
}

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
