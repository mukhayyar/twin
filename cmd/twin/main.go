// Command twin clones a macOS app into a copy with its own isolated profile,
// so you can run e.g. Slack-Work and Slack-Personal side by side.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// version is set at release time with -ldflags "-X main.version=…".
var version = "dev"

const usage = `twin — clone any macOS app with its own isolated profile

Usage:
  twin [flags] [app] [label]

Examples:
  twin                        pick an app interactively
  twin Slack Work             clone Slack as Slack-Work
  twin ~/Downloads/Foo.app    clone an app from anywhere
  twin Slack Hobby -c pink    clone with a pink icon and badge
  twin -n                     try the whole flow without touching the disk

Flags:
  -n, --dry-run       walk through the whole flow without touching the disk
  -c, --color COLOR   icon color: auto (default), none, a name, or #RRGGBB
                      names: blue green orange purple pink teal indigo
                             yellow red mint brown cyan
      --no-badge      don't draw the label on the clone's icon
  -v, --version       print the version

Environment:
  TWIN_DIRS       colon-separated folders to scan
                  (default: /Applications:~/Applications)

Tip: drag an app from Finder into the terminal while twin is open.
`

func main() {
	var dryRun, showVersion, noBadge bool
	var color string
	flag.BoolVar(&dryRun, "dry-run", false, "")
	flag.BoolVar(&dryRun, "n", false, "")
	flag.StringVar(&color, "color", "auto", "")
	flag.StringVar(&color, "c", "auto", "")
	flag.BoolVar(&noBadge, "no-badge", false, "")
	flag.BoolVar(&showVersion, "version", false, "")
	flag.BoolVar(&showVersion, "v", false, "")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	// Accept flags anywhere, e.g. `twin Slack Hobby -c pink`; the flag
	// package alone stops at the first positional argument.
	var args []string
	for rest := os.Args[1:]; ; {
		flag.CommandLine.Parse(rest)
		if rest = flag.Args(); len(rest) == 0 {
			break
		}
		args, rest = append(args, rest[0]), rest[1:]
	}

	if showVersion {
		fmt.Println("twin", resolvedVersion())
		return
	}

	style, err := parseIconStyle(color, noBadge)
	if err != nil {
		fmt.Fprintln(os.Stderr, "twin:", err)
		os.Exit(2)
	}

	roots := searchRoots()
	apps := scanApps(roots)
	if len(apps) == 0 && len(args) == 0 {
		fmt.Fprintln(os.Stderr, "twin: no apps found in", strings.Join(roots, ", "))
		os.Exit(1)
	}

	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	st = newStyles(isDark)
	m := newModel(apps, isDark, dryRun, style)
	if len(args) > 0 {
		app, err := findApp(args[0], apps)
		if err != nil {
			fmt.Fprintln(os.Stderr, "twin:", err)
			os.Exit(1)
		}
		m = m.preselect(app, strings.Join(args[1:], " "))
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "twin:", err)
		os.Exit(1)
	}
}

// resolvedVersion falls back to the module version for `go install …@vX`.
func resolvedVersion() string {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return version
}
