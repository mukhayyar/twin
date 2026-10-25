package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// App is something twin can clone: a macOS .app bundle, a Linux .desktop
// entry, or a Windows Start Menu shortcut.
type App struct {
	Name    string // display name; twin names derive from it
	Path    string // the bundle, .desktop file or shortcut
	ID      string // bundle identifier, desktop file ID, or target executable
	Version string
	Exec    string // executable name (macOS), Exec= line (Linux), target (Windows)
	Args    string // shortcut arguments (Windows)
	Icon    string // Icon= key (Linux), icon location (Windows)
	IsClone bool   // made by twin
}

func (a App) Title() string { return a.Name }

func (a App) Description() string {
	if a.Version == "" {
		return a.ID
	}
	return a.ID + " · " + a.Version
}

func (a App) FilterValue() string { return a.Name }

func sortApps(apps []App) {
	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
}

// findApp resolves a command-line argument: a path, or the name of an
// installed app (case-insensitive).
func findApp(arg string, apps []App) (App, error) {
	if looksLikePath(arg) || isAppFile(arg) {
		return appFromPath(arg)
	}
	for _, a := range apps {
		if strings.EqualFold(a.Name, arg) {
			return a, nil
		}
	}
	return App{}, fmt.Errorf("no app named %q (pass a path instead)", arg)
}

// looksLikePath reports whether pasted text is probably a dropped file:
// a Unix path, ~-path, or Windows drive or UNC path, optionally quoted.
func looksLikePath(s string) bool {
	s = strings.TrimLeft(strings.TrimSpace(s), `'"`)
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `\\`) {
		return true
	}
	return len(s) > 2 && unicode.IsLetter(rune(s[0])) && s[1] == ':' && (s[2] == '\\' || s[2] == '/')
}

// shellWords splits s like a shell would: whitespace separates words unless
// quoted or backslash-escaped. Backslashes are literal when windowsPaths is
// set, since they're path separators there.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'' && !windowsPaths:
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// firstPath returns the first path in pasted or typed text, with ~ expanded.
func firstPath(raw string) (string, error) {
	words := shellWords(raw)
	if len(words) == 0 {
		return "", fmt.Errorf("empty path")
	}
	return expandHome(strings.TrimRight(words[0], `/\`)), nil
}
