package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type state int

const (
	statePick state = iota
	stateLabel
	stateConfirm
	stateRun
	stateDone
	stateFailed
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepOK
	stepWarn
	stepFailed
)

var (
	presets = []string{"Work", "Personal", "Hobby", "School"}
	crumbs  = []string{"Choose app", "Name profile", "Clone"}
	// dryRunDelays paces the simulated steps so --dry-run reads like a real run.
	dryRunDelays = []time.Duration{900, 450, 700, 550, 1100, 400}
)

type stepDoneMsg struct {
	index   int
	warning string
	err     error
	took    time.Duration
}

// finishMsg lets the full progress bar linger a moment before the summary.
type finishMsg struct{}

type model struct {
	state    state
	dryRun   bool
	style    IconStyle
	width    int
	quitting bool

	list    list.Model
	input   textinput.Model
	spinner spinner.Model
	bar     progress.Model
	preset  int

	plan    Plan
	twins   []string // existing clones of plan.Src
	status  []stepState
	notes   []string
	took    []time.Duration
	started time.Time
	elapsed time.Duration
	err     error
	failed  int
}

func newModel(apps []App, isDark, dryRun bool, style IconStyle) model {
	items := make([]list.Item, len(apps))
	for i, a := range apps {
		items[i] = a
	}

	d := list.NewDefaultDelegate()
	d.Styles = list.NewDefaultItemStyles(isDark)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(st.textC)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(st.mutedC)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(st.accentC).BorderForeground(pink).Bold(true)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(st.mutedC).BorderForeground(pink)
	d.Styles.FilterMatch = lipgloss.NewStyle().Foreground(pink).Underline(true)

	l := list.New(items, d, 0, 0)
	l.Title = "Choose an app to clone"
	l.Styles = list.DefaultStyles(isDark)
	l.Styles.Title = st.Badge
	l.Styles.TitleBar = l.Styles.TitleBar.PaddingLeft(0)
	l.Styles.StatusBar = l.Styles.StatusBar.PaddingLeft(0)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.PaddingLeft(0)
	l.Styles.HelpStyle = l.Styles.HelpStyle.PaddingLeft(0)
	l.Styles.ActivePaginationDot = l.Styles.ActivePaginationDot.Foreground(pink)
	l.Styles.Filter.Focused.Prompt = l.Styles.Filter.Focused.Prompt.Foreground(pink)
	l.FilterInput.Prompt = "Search: "
	l.SetStatusBarItemName("app", "apps")
	// bubbles binds quit to "v" (labelled "select"); use the usual q instead.
	l.KeyMap.Quit = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose"))}
	}
	l.Help.Styles.ShortKey, l.Help.Styles.FullKey = st.Key, st.Key
	l.Help.Styles.ShortDesc, l.Help.Styles.FullDesc = st.KeyDesc, st.KeyDesc
	l.Help.Styles.ShortSeparator, l.Help.Styles.FullSeparator = st.Sep, st.Sep

	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "Work, Personal, Hobby…"
	in.CharLimit = 32
	is := textinput.DefaultStyles(isDark)
	is.Focused.Prompt = is.Focused.Prompt.Foreground(pink).Bold(true)
	is.Focused.Text = is.Focused.Text.Foreground(st.textC)
	in.SetStyles(is)

	return model{
		dryRun:  dryRun,
		style:   style,
		list:    l,
		input:   in,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(lipgloss.NewStyle().Foreground(pink))),
		bar:     progress.New(progress.WithColors(violet, pink), progress.WithWidth(48)),
		preset:  -1,
	}
}

// preselect skips the picker, e.g. for `twin Slack Work`.
func (m model) preselect(app App, label string) model {
	m, _ = m.choose(app)
	m.input.SetValue(label)
	m.input.CursorEnd()
	return m
}

func (m model) choose(app App) (model, tea.Cmd) {
	m.plan = newPlan(app, "", m.style)
	m.twins = twinsOf(app.Name)
	m.state = stateLabel
	m.preset = -1
	m.input.Reset()
	return m, m.input.Focus()
}

func (m model) Init() tea.Cmd {
	if m.state == stateLabel {
		return textinput.Blink
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.list.SetSize(msg.Width-4, max(msg.Height-8, 8))
		m.bar.SetWidth(min(msg.Width-8, 52))
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case tea.PasteMsg:
		// Dragging an app from Finder onto the terminal pastes its path.
		if m.state == statePick && looksLikePath(msg.Content) {
			app, err := appFromPath(msg.Content)
			if err != nil {
				return m, m.list.NewStatusMessage(st.Fail.Render("✗ " + err.Error()))
			}
			m.list.ResetFilter()
			return m.choose(app)
		}
	case spinner.TickMsg:
		if m.state != stateRun {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		var cmd tea.Cmd
		m.bar, cmd = m.bar.Update(msg)
		return m, cmd
	case stepDoneMsg:
		return m.stepDone(msg)
	case finishMsg:
		m.state = stateDone
		return m, nil
	}

	switch m.state {
	case statePick:
		return m.updatePick(msg)
	case stateLabel:
		return m.updateLabel(msg)
	case stateConfirm:
		return m.updateConfirm(msg)
	case stateDone, stateFailed:
		return m.updateFinished(msg)
	}
	return m, nil
}

func (m model) updatePick(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "enter" && m.list.FilterState() != list.Filtering {
		if app, ok := m.list.SelectedItem().(App); ok {
			return m.choose(app)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateLabel(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			m.plan = newPlan(m.plan.Src, m.input.Value(), m.style)
			if m.plan.Exists {
				m.state = stateConfirm
				return m, nil
			}
			return m.start()
		case "esc":
			m.state = statePick
			return m, nil
		case "tab":
			m.preset = (m.preset + 1) % len(presets)
			m.input.SetValue(presets[m.preset])
			m.input.CursorEnd()
			return m, nil
		case "shift+tab":
			m.preset = (max(m.preset, 0) + len(presets) - 1) % len(presets)
			m.input.SetValue(presets[m.preset])
			m.input.CursorEnd()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "y", "Y":
			return m.start()
		case "n", "N", "esc", "enter":
			m.state = stateLabel
		}
	}
	return m, nil
}

func (m model) updateFinished(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "o":
			if m.state != stateDone {
				return m, nil
			}
			if !m.dryRun {
				_ = exec.Command("open", m.plan.Dest).Start()
			}
			m.quitting = true
			return m, tea.Quit
		case "q", "enter", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) start() (tea.Model, tea.Cmd) {
	m.state = stateRun
	m.status = make([]stepState, len(steps))
	m.notes = make([]string, len(steps))
	m.took = make([]time.Duration, len(steps))
	m.status[0] = stepRunning
	m.started = time.Now()
	m.input.Blur()
	return m, tea.Batch(m.spinner.Tick, m.bar.SetPercent(0), runStep(m.plan, 0, m.dryRun))
}

func runStep(p Plan, i int, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		if dryRun {
			time.Sleep(dryRunDelays[i] * time.Millisecond)
			return stepDoneMsg{index: i, took: time.Since(start)}
		}
		warning, err := steps[i].Run(p)
		if err != nil {
			os.RemoveAll(p.Dest) // don't leave a half-built clone behind
		}
		return stepDoneMsg{index: i, warning: warning, err: err, took: time.Since(start)}
	}
}

func (m model) stepDone(msg stepDoneMsg) (tea.Model, tea.Cmd) {
	i := msg.index
	m.took[i] = msg.took
	if msg.err != nil {
		m.status[i] = stepFailed
		m.err = msg.err
		m.failed = i
		m.elapsed = time.Since(m.started)
		m.state = stateFailed
		return m, nil
	}
	m.status[i] = stepOK
	if msg.warning != "" {
		m.status[i] = stepWarn
		m.notes[i] = msg.warning
	}
	bar := m.bar.SetPercent(float64(i+1) / float64(len(steps)))
	if i+1 == len(steps) {
		m.elapsed = time.Since(m.started)
		return m, tea.Batch(bar, tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg { return finishMsg{} }))
	}
	m.status[i+1] = stepRunning
	return m, tea.Batch(bar, runStep(m.plan, i+1, m.dryRun))
}

func (m model) View() tea.View {
	if m.quitting && m.state < stateDone {
		return tea.NewView("")
	}

	var body string
	switch m.state {
	case statePick:
		body = m.list.View()
	case stateLabel:
		body = m.labelView()
	case stateConfirm:
		body = m.confirmView()
	case stateRun:
		body = m.runView()
	case stateDone:
		body = m.doneView()
	case stateFailed:
		body = m.failedView()
	}

	v := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(m.header() + "\n\n" + body))
	v.WindowTitle = "twin"
	v.AltScreen = m.state < stateDone
	if m.state == stateRun {
		v.ProgressBar = tea.NewProgressBar(tea.ProgressBarDefault, int(100*m.completed()/float64(len(steps))))
	}
	return v
}

func (m model) completed() float64 {
	n := 0
	for _, s := range m.status {
		if s == stepOK || s == stepWarn {
			n++
		}
	}
	return float64(n)
}

func (m model) header() string {
	line := gradientBadge(" ✦ twin ") + st.Muted.Render("  one app · many lives")
	if m.dryRun {
		line += "  " + st.WarnBadge.Render("DRY RUN")
	}
	if m.state >= stateDone {
		return line
	}

	active := map[state]int{statePick: 0, stateLabel: 1, stateConfirm: 1, stateRun: 2}[m.state]
	parts := make([]string, len(crumbs))
	for i, c := range crumbs {
		switch {
		case i < active:
			parts[i] = st.Success.Render("✓ " + c)
		case i == active:
			parts[i] = st.Accent.Render("● " + c)
		default:
			parts[i] = st.Faint.Render("○ " + c)
		}
	}
	line += "\n\n" + strings.Join(parts, st.Faint.Render("  ──  "))
	if m.state == statePick {
		line += "\n\n" + st.Muted.Render("Pick from the list, or drag an app from Finder into this window.")
	}
	return line
}

func (m model) labelView() string {
	p := newPlan(m.plan.Src, m.input.Value(), m.style)

	hero := lipgloss.JoinHorizontal(lipgloss.Center,
		tile(p.Src.Name, violet, pink), "  ",
		st.Label.Render(p.Src.Name)+"\n"+st.Muted.Render(p.Src.Description()),
	)
	if m.width >= 96 {
		c := lipgloss.Color(p.Color)
		hero = lipgloss.JoinHorizontal(lipgloss.Center, hero,
			st.Faint.Render("     →     "),
			tile(p.Src.Name, c, lipgloss.Lighten(c, 0.25)), "  ",
			st.Label.Render(p.Name)+"\n"+badge(p.Label, p.Color),
		)
	}

	chips := make([]string, len(presets))
	for i, name := range presets {
		if p.Label == name {
			chips[i] = badge(name, p.Color)
		} else {
			chips[i] = st.Chip.Render(name)
		}
	}

	card := st.Card.Render(st.CardTitle.Render("PREVIEW") + "\n" + rows(
		"App", tildify(p.Dest),
		"Bundle", p.BundleID,
		"Profile", tildify(p.Profile),
		"Icon", iconSummary(p),
	))

	out := []string{
		hero, "",
		st.Label.Render("Profile label"),
		m.input.View(),
		st.Muted.Render("quick pick  ") + strings.Join(chips, " "),
	}
	if len(m.twins) > 0 {
		have := make([]string, len(m.twins))
		for i, t := range m.twins {
			label := strings.TrimPrefix(t, p.Src.Name+"-")
			have[i] = badge(label, autoColor(label))
		}
		out = append(out, st.Muted.Render("you have    ")+strings.Join(have, " "))
	}
	out = append(out, "", card)
	if p.Exists {
		out = append(out, st.Warn.Render("▲ "+p.Name+".app already exists and will be replaced"))
	}
	out = append(out, "", help("enter", "create clone", "tab", "quick pick", "esc", "back", "ctrl+c", "quit"))
	return strings.Join(out, "\n")
}

func (m model) confirmView() string {
	p := m.plan
	out := []string{
		st.Warn.Bold(true).Render("▲ Replace " + p.Name + ".app?"),
		"",
		st.Text.Render("It will be deleted and rebuilt from " + p.Src.Name + "."),
		st.Muted.Render("Profile data in " + tildify(p.Profile) + " is kept."),
	}
	if !p.ExistingIsClone {
		out = append(out, "", st.Fail.Bold(true).Render("Heads up: this app was not made by twin."))
	}
	out = append(out, "", help("y", "replace", "n", "cancel"))
	return strings.Join(out, "\n")
}

func (m model) runView() string {
	title := st.Label.Render("Cloning ") + st.Accent.Render(m.plan.Src.Name) +
		st.Muted.Render("  →  ") + st.Accent2.Render(m.plan.Name)

	nameCol := lipgloss.NewStyle().Width(38)
	var lines []string
	for i, s := range steps {
		icon, name := st.Faint.Render("·"), st.Faint.Render(s.Title)
		switch m.status[i] {
		case stepRunning:
			icon, name = m.spinner.View(), st.Text.Bold(true).Render(s.Title)
		case stepOK:
			icon, name = st.Success.Render("✓"), st.Text.Render(s.Title)
		case stepWarn:
			icon, name = st.Warn.Render("▲"), st.Text.Render(s.Title)
		case stepFailed:
			icon, name = st.Fail.Render("✗"), st.Fail.Render(s.Title)
		}
		line := icon + "  " + nameCol.Render(name)
		if m.took[i] > 0 {
			line += st.Faint.Render(fmt.Sprintf("%.1fs", m.took[i].Seconds()))
		}
		lines = append(lines, line)
		if m.notes[i] != "" {
			lines = append(lines, "   "+st.Warn.Render(clip(m.notes[i], m.width-12)))
		}
	}
	return title + "\n\n" + strings.Join(lines, "\n") + "\n\n" + m.bar.View()
}

func (m model) doneView() string {
	title := st.Success.Bold(true).Render("✓ " + m.plan.Name + " is ready")
	if m.dryRun {
		title = st.Success.Bold(true).Render("✓ Dry run complete") + st.Muted.Render(" · nothing was changed")
	}
	body := title + "\n\n" + rows(
		"App", tildify(m.plan.Dest),
		"Bundle", m.plan.BundleID,
		"Profile", tildify(m.plan.Profile),
		"Icon", iconSummary(m.plan),
		"Time", fmt.Sprintf("%.1fs", m.elapsed.Seconds()),
	)
	for i, n := range m.notes {
		if n != "" {
			body += "\n\n" + st.Warn.Render("▲ "+steps[i].Title+": "+clip(n, 60))
		}
	}

	out := st.DoneBox.Render(body)
	if m.quitting {
		return out
	}
	if m.dryRun {
		return out + "\n" + help("q", "quit")
	}
	return out + "\n" + help("o", "open "+m.plan.Name, "q", "quit")
}

func (m model) failedView() string {
	body := st.Fail.Bold(true).Render("✗ Clone failed at “"+steps[m.failed].Title+"”") + "\n\n" +
		lipgloss.NewStyle().Width(min(max(m.width-16, 30), 72)).Render(st.Text.Render(m.err.Error())) + "\n\n" +
		st.Muted.Render("The partial clone was removed.")
	out := st.FailBox.Render(body)
	if m.quitting {
		return out
	}
	return out + "\n" + help("q", "quit")
}

func iconSummary(p Plan) string {
	dot := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color)).Render("●")
	switch {
	case p.Badge && p.Tint:
		return badge(p.Label, p.Color) + st.Muted.Render(" badge, recolored")
	case p.Badge:
		return badge(p.Label, p.Color) + st.Muted.Render(" badge")
	case p.Tint:
		return dot + st.Muted.Render(" recolored")
	}
	return st.Muted.Render("original")
}

// clip shortens s to its first line and at most n runes.
func clip(s string, n int) string {
	s, _, _ = strings.Cut(s, "\n")
	if r := []rune(s); n > 1 && len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
