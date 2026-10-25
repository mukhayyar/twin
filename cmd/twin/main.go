// Command twin clones an app into a twin with its own isolated profile, so
// you can run e.g. Slack-Work and Slack-Personal side by side.
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

var usage = `twin — clone any app with its own isolated profile

Usage:
  twin [flags] [app] [label]

Examples:
  twin                        pick an app interactively
  twin Slack Work             clone Slack as Slack-Work
  twin ` + fmt.Sprintf("%-23s", pathExample) + `clone an app from anywhere
  twin Slack Hobby -c pink    clone with a pink icon and badge
  twin -y Slack Work          clone without the interactive UI
  twin -n                     try the whole flow without touching the disk

Flags:
  -y, --yes           clone straight away, without the interactive UI
  -n, --dry-run       walk through the whole flow without touching the disk
  -c, --color COLOR   icon color: auto (default), none, a name, or #RRGGBB
                      names: blue green orange purple pink teal indigo
                             yellow red mint brown cyan
      --no-badge      don't draw the label on the twin's icon
  -v, --version       print the version

Environment:
  TWIN_DIRS       folders to scan, as a path list
                  ` + dirsHelp + `

Tip: ` + strings.ToLower(dropHint[:1]) + dropHint[1:] + `
`

func main() {
	var yes, dryRun, showVersion, noBadge bool
	var color string
	flag.BoolVar(&yes, "yes", false, "")
	flag.BoolVar(&yes, "y", false, "")
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
		fail(2, err)
	}
	if yes && len(args) == 0 {
		fail(2, fmt.Errorf("--yes needs an app, e.g. twin -y Slack Work"))
	}

	roots := searchRoots()
	apps := scanApps(roots)
	if len(apps) == 0 && len(args) == 0 {
		fail(1, fmt.Errorf("no apps found in %s", strings.Join(roots, ", ")))
	}

	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	st = newStyles(isDark)

	var app App
	if len(args) > 0 {
		if app, err = findApp(args[0], apps); err != nil {
			fail(1, err)
		}
	}
	label := strings.Join(args[min(len(args), 1):], " ")
	if yes {
		if err := runHeadless(newPlan(app, label, style), dryRun); err != nil {
			fail(1, err)
		}
		return
	}

	m := newModel(apps, isDark, dryRun, style)
	if len(args) > 0 {
		m = m.preselect(app, label)
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fail(1, err)
	}
}

func fail(code int, err error) {
	fmt.Fprintln(os.Stderr, "twin:", err)
	os.Exit(code)
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
