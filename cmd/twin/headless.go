package main

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
)

// runHeadless clones without the TUI, for scripts and `twin -y Slack Work`.
// It only replaces an existing twin, never something twin didn't make.
func runHeadless(p Plan, dryRun bool) error {
	if p.Exists && !p.ExistingIsClone {
		return fmt.Errorf("%s exists and wasn't made by twin; not replacing it", tildify(p.Dest))
	}
	head := gradientBadge(" ✦ twin ") + st.Muted.Render("  "+p.Src.Name+"  →  ") + st.Accent2.Render(p.Name)
	if dryRun {
		head += "  " + st.WarnBadge.Render("DRY RUN")
	}
	lipgloss.Println(head + "\n")

	iconKept := false
	for _, s := range steps {
		start := time.Now()
		var warning string
		var err error
		if !dryRun {
			warning, err = s.Run(p)
		}
		took := st.Faint.Render(fmt.Sprintf("  %.1fs", time.Since(start).Seconds()))
		switch {
		case err != nil:
			cleanup(p)
			lipgloss.Println(st.Fail.Render("✗ ") + st.Text.Render(s.Title) + took)
			return fmt.Errorf("%s: %w", s.Title, err)
		case warning != "":
			iconKept = iconKept || s.Title == paintStep
			lipgloss.Println(st.Warn.Render("▲ ") + st.Text.Render(s.Title) + took + "\n  " + st.Warn.Render(clip(warning, 100)))
		default:
			lipgloss.Println(st.Success.Render("✓ ") + st.Text.Render(s.Title) + took)
		}
	}
	lipgloss.Println("\n" + rows(append(append(previewRows(p), "Icon", iconRow(p, iconKept)), hardwareRow(p, false)...)...))
	return nil
}
