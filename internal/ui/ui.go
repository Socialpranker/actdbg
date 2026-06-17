// Package ui is actdbg's lazygit-style terminal UI: jobs and steps on the
// left, the selected step's live log on the right, the command log at the
// bottom. Run a workflow, watch it stop at the failed step, drop into a
// shell — without leaving the screen.
package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sirupsen/logrus"

	"github.com/Socialpranker/actdbg/internal/cmdlog"
	"github.com/Socialpranker/actdbg/internal/doctor"
	"github.com/Socialpranker/actdbg/internal/enginerun"
	"github.com/Socialpranker/actdbg/internal/fidelity"
	"github.com/Socialpranker/actdbg/internal/state"
	"github.com/Socialpranker/actdbg/internal/timeline"
)

// ---------- messages from the run goroutine ----------

type (
	stepStartMsg  struct{ job, step string }
	stepResultMsg struct{ job, step, result string }
	logLineMsg    struct{ job, step, line string }
	runDoneMsg    struct {
		res *enginerun.RunResult
		err error
	}
	tickMsg struct{}
)

// ---------- model ----------

type mode int

const (
	modeLog mode = iota
	modeCheck
	modeDoctor
	modeHelp
)

// row is one line of the left panel: a job header (step == "") or a step.
type row struct {
	job    string
	step   string
	name   string
	number int
}

const maxLogLines = 2000

type Model struct {
	opts enginerun.Options

	jobs   []enginerun.JobMeta
	rows   []row
	cursor int
	follow bool // cursor tracks the running step until the user moves it

	status map[string]string   // job\x00step -> running/success/failure/skipped
	logs   map[string][]string // job\x00step -> full output lines

	running    bool
	notice     string
	shellState *state.State

	mode     mode
	paneText []string // content of the check/doctor pane

	pick       bool // matrix picker active
	pickJob    string
	pickCombos []map[string]interface{}
	pickCursor int

	cmds          []string
	width, height int

	prog      *tea.Program
	wantShell bool
}

func sk(job, step string) string { return job + "\x00" + step }

// buildRows flattens jobs into left-panel rows (job header, then its steps).
func buildRows(jobs []enginerun.JobMeta) []row {
	var out []row
	for _, j := range jobs {
		out = append(out, row{job: j.ID})
		for _, s := range j.Steps {
			out = append(out, row{job: j.ID, step: s.ID, name: s.Name, number: s.Number})
		}
	}
	return out
}

// firstStepRow returns the index of the first selectable (step) row.
func firstStepRow(rows []row) int {
	for i, r := range rows {
		if r.step != "" {
			return i
		}
	}
	return 0
}

// moveCursor advances over step rows only, skipping job headers.
func moveCursor(rows []row, cur, dir int) int {
	for i := cur + dir; i >= 0 && i < len(rows); i += dir {
		if rows[i].step != "" {
			return i
		}
	}
	return cur
}

func (m *Model) rowIndex(job, step string) int {
	for i, r := range m.rows {
		if r.job == job && r.step == step {
			return i
		}
	}
	return -1
}

// statusIcon maps a step status to its timeline glyph.
func statusIcon(s string) string {
	switch s {
	case "running":
		return "▶"
	case "success":
		return "✅"
	case "failure":
		return "❌"
	case "skipped":
		return "⏭"
	}
	return "·"
}

// truncate cuts s to a display width (ANSI-aware), appending … when cut.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// lastLines returns at most n trailing lines.
func lastLines(lines []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// window slices lines to height h keeping the focus line visible.
func window(lines []string, focus, h int) []string {
	if h <= 0 {
		return nil
	}
	if len(lines) <= h {
		return lines
	}
	start := focus - h/2
	if start < 0 {
		start = 0
	}
	if start > len(lines)-h {
		start = len(lines) - h
	}
	return lines[start : start+h]
}

// comboLabel renders a matrix combination as "k:v k:v" with sorted keys.
func comboLabel(c map[string]interface{}) string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%v", k, c[k]))
	}
	return strings.Join(parts, " ")
}

// matrixFilter converts one chosen combination into act's filter shape.
func matrixFilter(c map[string]interface{}) map[string]map[string]bool {
	if len(c) == 0 {
		return nil
	}
	m := map[string]map[string]bool{}
	for k, v := range c {
		m[k] = map[string]bool{fmt.Sprintf("%v", v): true}
	}
	return m
}

// matrixJob picks which job's matrix to offer: the one under the cursor if it
// has one, else the first job that does. nil when no job has a matrix.
func (m *Model) matrixJob() *enginerun.JobMeta {
	curJob := ""
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		curJob = m.rows[m.cursor].job
	}
	var first *enginerun.JobMeta
	for i := range m.jobs {
		if len(m.jobs[i].Matrixes) == 0 {
			continue
		}
		if m.jobs[i].ID == curJob {
			return &m.jobs[i]
		}
		if first == nil {
			first = &m.jobs[i]
		}
	}
	return first
}

// ---------- bubbletea plumbing ----------

func tickCmd() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) Init() tea.Cmd { return tickCmd() }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.cmds = cmdlog.Tail(3)
		return m, tickCmd()
	case stepStartMsg:
		m.status[sk(msg.job, msg.step)] = "running"
		if m.follow {
			if i := m.rowIndex(msg.job, msg.step); i >= 0 {
				m.cursor = i
			}
		}
	case stepResultMsg:
		m.status[sk(msg.job, msg.step)] = msg.result
	case logLineMsg:
		k := sk(msg.job, msg.step)
		m.logs[k] = append(m.logs[k], ansi.Strip(msg.line))
		if len(m.logs[k]) > maxLogLines {
			m.logs[k] = m.logs[k][len(m.logs[k])-maxLogLines:]
		}
	case runDoneMsg:
		m.running = false
		switch {
		case msg.err != nil:
			m.notice = "✖ run error: " + msg.err.Error()
		case msg.res.Failure == nil:
			m.notice = "✔ all steps passed locally — green here ≠ green on GitHub (c explains)"
		case msg.res.Container == "":
			m.notice = fmt.Sprintf("❌ step %d/%d %q failed — container not found, no shell available",
				msg.res.StepNumber, msg.res.TotalSteps, msg.res.StepName)
		default:
			m.shellState = msg.res.State
			m.notice = fmt.Sprintf("❌ step %d/%d %q failed — s = shell into %s",
				msg.res.StepNumber, msg.res.TotalSteps, msg.res.StepName, msg.res.Container)
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.mode == modeHelp {
		m.mode = modeLog // any key closes help
		return m, nil
	}
	if m.pick {
		switch k.String() {
		case "up", "k":
			if m.pickCursor > 0 {
				m.pickCursor--
			}
		case "down", "j":
			if m.pickCursor < len(m.pickCombos)-1 {
				m.pickCursor++
			}
		case "enter":
			m.pick = false
			m.startRun(matrixFilter(m.pickCombos[m.pickCursor]))
		case "esc", "q":
			m.pick = false
		}
		return m, nil
	}
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		m.cursor = moveCursor(m.rows, m.cursor, -1)
		m.follow = false
		m.mode = modeLog
	case "down", "j":
		m.cursor = moveCursor(m.rows, m.cursor, +1)
		m.follow = false
		m.mode = modeLog
	case "r":
		if m.running {
			return m, nil
		}
		if jm := m.matrixJob(); jm != nil {
			m.pick, m.pickJob, m.pickCombos, m.pickCursor = true, jm.ID, jm.Matrixes, 0
			m.mode = modeLog
			return m, nil
		}
		m.startRun(nil)
	case "s":
		if m.shellState != nil && !m.running {
			m.wantShell = true
			return m, tea.Quit
		}
		m.notice = "s — no failed step with a live container yet (run something first)"
	case "c":
		m.showCheck()
	case "d":
		m.showDoctor()
	case "?":
		m.mode = modeHelp
	}
	return m, nil
}

// startRun launches the workflow in a goroutine; progress arrives as messages.
func (m *Model) startRun(matrix map[string]map[string]bool) {
	if m.prog == nil {
		return
	}
	m.running = true
	m.follow = true
	m.shellState = nil
	m.mode = modeLog
	m.status = map[string]string{}
	m.logs = map[string][]string{}

	// honest pre-flight: where this workflow is known to diverge from GitHub
	m.notice = "running…"
	if fs, err := fidelity.CheckPath(m.opts.WorkflowPath, m.opts.Job); err == nil {
		if s := fidelity.Summary(fs); s != "" {
			m.notice = s
		}
	}
	if matrix != nil {
		m.notice = fmt.Sprintf("matrix %s · %s", m.pickJob, m.notice)
	}

	o := m.opts
	o.NoShell = true
	o.Matrix = matrix
	prog := m.prog
	go func() {
		tr := timeline.New(io.Discard, false)
		tr.OnStepStart = func(j, s string) { prog.Send(stepStartMsg{j, s}) }
		tr.OnStepResult = func(j, s, r string) string { prog.Send(stepResultMsg{j, s, r}); return "" }
		tr.OnLine = func(j, s, l string) { prog.Send(logLineMsg{j, s, l}) }
		res, err := enginerun.RunWithTracker(context.Background(), o, tr)
		prog.Send(runDoneMsg{res, err})
	}()
}

func (m *Model) showCheck() {
	m.mode = modeCheck
	fs, err := fidelity.CheckPath(m.opts.WorkflowPath, m.opts.Job)
	if err != nil {
		m.paneText = []string{"check failed: " + err.Error()}
		return
	}
	m.paneText = strings.Split(strings.TrimRight(fidelity.Render(fs), "\n"), "\n")
}

func (m *Model) showDoctor() {
	m.mode = modeDoctor
	var buf bytes.Buffer
	_ = doctor.Run(&buf)
	m.paneText = strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// ---------- view ----------

var (
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	jobStyle    = lipgloss.NewStyle().Bold(true)
	selStyle    = lipgloss.NewStyle().Reverse(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	noticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "starting…"
	}
	if m.mode == modeHelp {
		return m.helpView()
	}
	const cmdInnerH = 4 // title + 3 commands
	mainOuterH := m.height - 1 - (cmdInnerH + 2)
	if mainOuterH < 5 {
		mainOuterH = 5
	}
	mainInnerH := mainOuterH - 2
	leftOuterW := m.width * 3 / 10
	if leftOuterW < 26 {
		leftOuterW = 26
	}
	rightOuterW := m.width - leftOuterW
	if rightOuterW < 22 {
		rightOuterW = 22
	}
	leftW, rightW := leftOuterW-2, rightOuterW-2

	left := panelStyle.Width(leftW).Height(mainInnerH).Render(
		strings.Join(m.leftLines(leftW, mainInnerH), "\n"))
	right := panelStyle.Width(rightW).Height(mainInnerH).Render(
		strings.Join(m.rightLines(rightW, mainInnerH), "\n"))
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	cmdW := m.width - 2
	cmds := panelStyle.Width(cmdW).Height(cmdInnerH).Render(
		strings.Join(m.cmdLines(cmdW, cmdInnerH), "\n"))

	return lipgloss.JoinVertical(lipgloss.Left, main, cmds, m.footer())
}

func (m *Model) leftLines(w, h int) []string {
	title := truncate("steps · "+filepath.Base(m.opts.WorkflowPath), w)
	var lines []string
	for i, r := range m.rows {
		var s string
		switch {
		case r.step == "":
			s = truncate("job: "+r.job, w)
			if i != m.cursor {
				s = jobStyle.Render(s)
			}
		default:
			icon := statusIcon(m.status[sk(r.job, r.step)])
			s = truncate(fmt.Sprintf(" %s %d %s", icon, r.number, r.name), w)
		}
		if i == m.cursor {
			s = selStyle.Render(s)
		}
		lines = append(lines, s)
	}
	return append([]string{titleStyle.Render(title)}, window(lines, m.cursor, h-1)...)
}

func (m *Model) rightLines(w, h int) []string {
	if m.pick {
		return m.pickerLines(w, h)
	}
	switch m.mode {
	case modeCheck, modeDoctor:
		title := map[mode]string{modeCheck: "fidelity check", modeDoctor: "doctor"}[m.mode]
		out := []string{titleStyle.Render(truncate(title, w))}
		for _, l := range lastLines(m.paneText, h-1) {
			out = append(out, truncate(l, w))
		}
		return out
	}
	// log mode
	title := "log"
	key := ""
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		if r.step != "" {
			key = sk(r.job, r.step)
			title = fmt.Sprintf("log · %s %s", r.name, statusIcon(m.status[key]))
		}
	}
	out := []string{titleStyle.Render(truncate(title, w))}
	body := h - 1
	if m.notice != "" {
		out = append(out, noticeStyle.Render(truncate(m.notice, w)))
		body--
	}
	if key == "" || len(m.logs[key]) == 0 {
		if !m.running {
			out = append(out, dimStyle.Render(truncate("no output yet — press r to run the workflow", w)))
		}
		return out
	}
	for _, l := range lastLines(m.logs[key], body) {
		out = append(out, truncate(l, w))
	}
	return out
}

func (m *Model) pickerLines(w, h int) []string {
	out := []string{
		titleStyle.Render(truncate(fmt.Sprintf("strategy.matrix · job %s", m.pickJob), w)),
		dimStyle.Render(truncate("pick a combination: enter = run · esc = cancel", w)),
	}
	var items []string
	for i, c := range m.pickCombos {
		s := truncate("  "+comboLabel(c), w)
		if i == m.pickCursor {
			s = selStyle.Render(truncate("> "+comboLabel(c), w))
		}
		items = append(items, s)
	}
	return append(out, window(items, m.pickCursor, h-2)...)
}

func (m *Model) cmdLines(w, h int) []string {
	out := []string{titleStyle.Render(truncate("commands actdbg ran · "+cmdlog.Path(), w))}
	if len(m.cmds) == 0 {
		return append(out, dimStyle.Render("none yet"))
	}
	for _, c := range lastLines(m.cmds, h-1) {
		out = append(out, dimStyle.Render(truncate("$ "+c, w)))
	}
	return out
}

func (m *Model) footer() string {
	parts := []string{"r run"}
	if m.running {
		parts = []string{"running…"}
	}
	if m.shellState != nil && !m.running {
		parts = append(parts, "s shell")
	}
	parts = append(parts, "j/k move", "c check", "d doctor", "? help", "q quit")
	return dimStyle.Render(truncate(" "+strings.Join(parts, " · "), m.width))
}

func (m *Model) helpView() string {
	help := strings.Join([]string{
		titleStyle.Render("actdbg ui — help"),
		"",
		"  r        run the workflow (matrix picker appears when a job has strategy.matrix)",
		"  j/k ↑↓   select a step — the right panel shows its live log",
		"  s        shell into the failed step's container (after a red run)",
		"  c        fidelity check: where this workflow diverges from GitHub",
		"  d        doctor: environment vs the usual act pitfalls",
		"  q        quit (quitting mid-run abandons the run; containers stay)",
		"",
		"  the bottom panel mirrors ~/.actdbg/commands.log — every docker",
		"  command actdbg executes, nothing hidden.",
		"",
		dimStyle.Render("  any key to close"),
	}, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, help)
}

// ---------- entry points ----------

// PickWorkflow resolves what the TUI should load: a file is used as-is, a
// directory means its first *.yml / *.yaml (sorted by name).
func PickWorkflow(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return path, nil
	}
	var files []string
	for _, pat := range []string{"*.yml", "*.yaml"} {
		matches, _ := filepath.Glob(filepath.Join(path, pat))
		files = append(files, matches...)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no workflows found in %s", path)
	}
	sort.Strings(files)
	return files[0], nil
}

// Start runs the TUI. When the user pressed s on a failed step it returns the
// stop state so the caller can exec the shell after the screen is restored.
func Start(o enginerun.Options) (*state.State, error) {
	jobs, err := enginerun.Inspect(o.WorkflowPath, o.Job, o.Event)
	if err != nil {
		return nil, err
	}
	// Keep stray global-logrus output (act internals) off the alt screen;
	// per-job loggers are created separately by the tracker.
	logrus.SetOutput(io.Discard)
	defer logrus.SetOutput(os.Stderr)

	m := &Model{
		opts:   o,
		jobs:   jobs,
		rows:   buildRows(jobs),
		status: map[string]string{},
		logs:   map[string][]string{},
		cmds:   cmdlog.Tail(3),
		mode:   modeLog,
		notice: "r to run · experimental TUI — file issues, not expectations",
	}
	m.cursor = firstStepRow(m.rows)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.prog = p
	if _, err := p.Run(); err != nil {
		return nil, err
	}
	if m.wantShell {
		return m.shellState, nil
	}
	return nil, nil
}
