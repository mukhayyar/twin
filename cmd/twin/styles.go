package main

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	violet = lipgloss.Color("#7D56F4")
	pink   = lipgloss.Color("#FF5FAF")
	green  = lipgloss.Color("#04B575")
	amber  = lipgloss.Color("#FFB454")
	red    = lipgloss.Color("#FF4F6D")
	white  = lipgloss.Color("#FFFFFF")
)

// styles depend on whether the terminal background is dark, so they are
// built once at startup.
type styles struct {
	textC, mutedC, accentC color.Color

	Text, Muted, Faint, Accent, Accent2 lipgloss.Style
	Success, Warn, Fail                 lipgloss.Style
	Key, KeyDesc, Sep                   lipgloss.Style
	Label, RowKey, CardTitle            lipgloss.Style
	Chip, ChipActive, Badge, WarnBadge  lipgloss.Style
	Card, DoneBox, FailBox              lipgloss.Style
}

var st styles

func newStyles(isDark bool) styles {
	ld := lipgloss.LightDark(isDark)
	text := ld(lipgloss.Color("#1C1B22"), lipgloss.Color("#ECEAF4"))
	muted := ld(lipgloss.Color("#6F6B84"), lipgloss.Color("#9490AA"))
	faint := ld(lipgloss.Color("#B5B2C4"), lipgloss.Color("#4E4A5E"))
	border := ld(lipgloss.Color("#DCD8EA"), lipgloss.Color("#3A3748"))
	chip := ld(lipgloss.Color("#EFEDF7"), lipgloss.Color("#2A2835"))
	accent := ld(lipgloss.Color("#5A3FD1"), lipgloss.Color("#B4A2FF"))
	accent2 := ld(lipgloss.Color("#D6338A"), pink)

	s := lipgloss.NewStyle()
	return styles{
		textC:   text,
		mutedC:  muted,
		accentC: accent,

		Text:    s.Foreground(text),
		Muted:   s.Foreground(muted),
		Faint:   s.Foreground(faint),
		Accent:  s.Foreground(accent).Bold(true),
		Accent2: s.Foreground(accent2).Bold(true),
		Success: s.Foreground(green),
		Warn:    s.Foreground(amber),
		Fail:    s.Foreground(red),

		Key:     s.Foreground(accent).Bold(true),
		KeyDesc: s.Foreground(muted),
		Sep:     s.Foreground(faint),

		Label:     s.Foreground(text).Bold(true),
		RowKey:    s.Foreground(muted).Width(10),
		CardTitle: s.Foreground(muted).Bold(true),

		Chip:       s.Foreground(muted).Background(chip).Padding(0, 1),
		ChipActive: s.Foreground(white).Background(violet).Bold(true).Padding(0, 1),
		Badge:      s.Foreground(white).Background(violet).Bold(true).Padding(0, 1),
		WarnBadge:  s.Foreground(lipgloss.Color("#1C1B22")).Background(amber).Bold(true).Padding(0, 1),

		Card:    s.Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 2),
		DoneBox: s.Border(lipgloss.RoundedBorder()).BorderForeground(green).Padding(1, 3),
		FailBox: s.Border(lipgloss.RoundedBorder()).BorderForeground(red).Padding(1, 3),
	}
}

// gradientBadge paints each rune of s on a violet→pink background.
func gradientBadge(s string) string {
	runes := []rune(s)
	colors := lipgloss.Blend1D(len(runes), violet, pink)
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(lipgloss.NewStyle().Foreground(white).Background(colors[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

// badge renders a label pill in its color, mirroring the badge drawn on the
// clone's icon.
func badge(label, hex string) string {
	return lipgloss.NewStyle().Foreground(white).Background(lipgloss.Color(hex)).Bold(true).Padding(0, 1).
		Render(strings.ReplaceAll(label, "-", " "))
}

// tile renders a 7×3 gradient "app icon" with the app's initial.
func tile(name string, from, to color.Color) string {
	initial := "?"
	if r := []rune(strings.ToUpper(name)); len(r) > 0 {
		initial = string(r[0])
	}
	colors := lipgloss.Blend1D(3, from, to)
	rows := []string{"       ", "   " + initial + "   ", "       "}
	for i := range rows {
		rows[i] = lipgloss.NewStyle().Foreground(white).Background(colors[i]).Bold(true).Render(rows[i])
	}
	return strings.Join(rows, "\n")
}

// help renders "key desc • key desc" pairs.
func help(pairs ...string) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, st.Key.Render(pairs[i])+" "+st.KeyDesc.Render(pairs[i+1]))
	}
	return strings.Join(out, st.Sep.Render("  •  "))
}

// rows renders aligned "key value" lines.
func rows(pairs ...string) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, st.RowKey.Render(pairs[i])+st.Text.Render(pairs[i+1]))
	}
	return strings.Join(out, "\n")
}

func tildify(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
