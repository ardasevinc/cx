package picker

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ardasevinc/cx/internal/sessions"
)

type Action string

const (
	ActionNone   Action = ""
	ActionResume Action = "resume"
	ActionFork   Action = "fork"
	ActionQuit   Action = "quit"
)

type Result struct {
	Action  Action
	Session sessions.Session
}

type Model struct {
	all      []sessions.Session
	filtered []sessions.Session
	query    string
	cursor   int
	offset   int
	width    int
	height   int
	preview  bool
	detail   bool
	comfy    bool
	help     bool
	command  bool
	cmdText  string
	notice   string
	result   Result
}

type copyMsg struct {
	label string
	err   error
}

func New(items []sessions.Session) Model {
	return Model{
		all:      items,
		filtered: items,
		preview:  true,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.command {
			return m.updateCommand(msg)
		}
		if m.help {
			return m.updateHelp(msg)
		}
		return m.updateKeys(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case copyMsg:
		if msg.err != nil {
			m.notice = "copy failed: " + msg.err.Error()
		} else {
			m.notice = "copied " + msg.label
		}
	}
	return m, nil
}

func (m Model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		m.result = m.selection(ActionResume)
		return m, tea.Quit
	case tea.KeyCtrlF:
		m.result = m.selection(ActionFork)
		return m, tea.Quit
	case tea.KeyCtrlE:
		m.detail = !m.detail
	case tea.KeyCtrlV:
		m.comfy = !m.comfy
		m.clamp()
	case tea.KeyTab:
		m.preview = !m.preview
	case tea.KeyCtrlL:
		m.notice = ""
	case tea.KeyUp, tea.KeyCtrlK:
		m.move(-1)
	case tea.KeyDown, tea.KeyCtrlJ:
		m.move(1)
	case tea.KeyPgUp:
		m.move(-m.listHeight())
	case tea.KeyPgDown:
		m.move(m.listHeight())
	case tea.KeyHome:
		m.cursor = 0
		m.clamp()
	case tea.KeyEnd:
		m.cursor = len(m.filtered) - 1
		m.clamp()
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.query != "" {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.refreshFilter()
		}
	case tea.KeyCtrlU:
		m.query = ""
		m.refreshFilter()
	case tea.KeyRunes:
		key := msg.String()
		switch key {
		case ":":
			m.command = true
			m.cmdText = ""
			return m, nil
		case "?":
			m.help = true
			return m, nil
		case "y":
			return m, m.copySelection("id")
		}
		m.query += key
		m.refreshFilter()
	}
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.move(-3)
	case tea.MouseButtonWheelDown:
		m.move(3)
	}
	return m, nil
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.help = false
	case tea.KeyRunes:
		if msg.String() == "q" || msg.String() == "?" {
			m.help = false
		}
	}
	return m, nil
}

func (m Model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.command = false
		m.cmdText = ""
	case tea.KeyEnter:
		cmd := strings.TrimSpace(m.cmdText)
		m.command = false
		m.cmdText = ""
		return m.executeCommand(cmd)
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.cmdText != "" {
			runes := []rune(m.cmdText)
			m.cmdText = string(runes[:len(runes)-1])
		}
	case tea.KeyCtrlU:
		m.cmdText = ""
	case tea.KeyRunes:
		m.cmdText += msg.String()
	}
	return m, nil
}

func (m Model) executeCommand(input string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(strings.ToLower(input))
	if len(fields) == 0 {
		return m, nil
	}
	switch fields[0] {
	case "q", "quit", "exit":
		m.result = Result{Action: ActionQuit}
		return m, tea.Quit
	case "r", "resume":
		m.result = m.selection(ActionResume)
		return m, tea.Quit
	case "f", "fork":
		m.result = m.selection(ActionFork)
		return m, tea.Quit
	case "y", "copy", "cp":
		target := "id"
		if len(fields) > 1 {
			target = fields[1]
		}
		return m, m.copySelection(target)
	case "h", "help", "?":
		m.help = true
	case "p", "preview":
		m.preview = !m.preview
	case "e", "detail", "explain":
		m.detail = !m.detail
	case "v", "view":
		if len(fields) > 1 {
			m.setView(fields[1])
		} else {
			m.comfy = !m.comfy
		}
	case "compact":
		m.comfy = false
	case "comfy", "comfortable":
		m.comfy = true
	case "clear":
		m.query = ""
		m.refreshFilter()
	default:
		m.notice = "unknown command: " + input
	}
	m.clamp()
	return m, nil
}

func (m *Model) setView(view string) {
	switch view {
	case "compact", "dense":
		m.comfy = false
	case "comfy", "comfortable":
		m.comfy = true
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading cx..."
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		b.WriteString(emptyStyle.Render("no sessions match"))
		b.WriteString("\n")
		b.WriteString(m.footer())
		return m.overlay(b.String())
	}

	listWidth := m.width
	if m.preview && m.width >= 100 {
		listWidth = (m.width * 58) / 100
	}

	list := m.renderList(listWidth)
	side := ""
	if m.preview && m.width >= 100 {
		side = m.renderSide(m.width - listWidth - 2)
	}

	if side == "" {
		b.WriteString(list)
	} else {
		b.WriteString(joinColumns(list, side, "  "))
	}
	b.WriteString("\n")
	b.WriteString(m.footer())
	return m.overlay(b.String())
}

func (m Model) Result() Result {
	return m.result
}

func (m *Model) refreshFilter() {
	m.filtered = sessions.Filter(m.all, m.query)
	m.cursor = 0
	m.offset = 0
	m.clamp()
}

func (m *Model) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	m.clamp()
}

func (m *Model) clamp() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	height := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) selection(action Action) Result {
	if len(m.filtered) == 0 {
		return Result{}
	}
	return Result{Action: action, Session: m.filtered[m.cursor]}
}

func (m Model) copySelection(target string) tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}
	session := m.filtered[m.cursor]
	label, value := copyValue(session, target)
	return func() tea.Msg {
		return copyMsg{label: label, err: writeClipboard(value)}
	}
}

func copyValue(session sessions.Session, target string) (string, string) {
	switch target {
	case "path", "file", "jsonl":
		return "path", session.Path
	case "cwd", "dir":
		return "cwd", session.CWD
	case "title", "name":
		return "title", session.Title
	case "resume":
		return "resume command", "codex resume " + session.ID
	case "fork":
		return "fork command", "codex fork " + session.ID
	default:
		return "id", session.ID
	}
}

func writeClipboard(value string) error {
	if value == "" {
		return fmt.Errorf("empty value")
	}
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(value)
		return cmd.Run()
	}
	return fmt.Errorf("clipboard unsupported on %s", runtime.GOOS)
}

func (m Model) listHeight() int {
	reserved := 3
	if m.height <= reserved {
		return 1
	}
	rowHeight := 1
	if m.comfy {
		rowHeight = 2
	}
	return max(1, (m.height-reserved)/rowHeight)
}

func (m Model) header() string {
	query := m.query
	if query == "" {
		query = "type to search"
	}
	count := fmt.Sprintf("%d/%d", len(m.filtered), len(m.all))
	text := "cx  " + query + "  " + count
	return headerStyle.Render(truncate(text, paddedWidth(m.width)))
}

func (m Model) footer() string {
	if m.command {
		return commandStyle.Render(truncate(":"+m.cmdText, paddedWidth(m.width)))
	}
	text := "enter resume  ^f fork  y copy  : cmd  ? help  ^j/^k move  tab preview  ^e detail  ^v view"
	if m.notice != "" {
		text = m.notice + "  " + text
	}
	return footerStyle.Render(truncate(text, paddedWidth(m.width)))
}

func (m Model) renderList(width int) string {
	height := m.listHeight()
	end := min(len(m.filtered), m.offset+height)
	lines := make([]string, 0, height)

	for i := m.offset; i < end; i++ {
		session := m.filtered[i]
		selected := i == m.cursor
		lines = append(lines, m.renderRow(session, width, selected)...)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderRow(session sessions.Session, width int, selected bool) []string {
	prefix := "  "
	style := rowStyle
	if selected {
		prefix = "▌ "
		style = selectedStyle
	}

	title := session.Title
	if title == "" {
		title = session.ID
	}
	meta := compactMeta(session)
	first := lineWithMeta(prefix, title, meta, width)

	if !m.comfy {
		return []string{style.Width(width).Render(first)}
	}

	context := session.Preview
	if context == "" {
		context = session.CWD
	}
	second := "  " + previewStyle.Render(truncate(context, max(10, width-2)))
	return []string{
		style.Width(width).Render(first),
		style.Width(width).Render(second),
	}
}

func (m Model) renderSide(width int) string {
	session := m.filtered[m.cursor]
	if m.detail {
		return m.renderDetail(session, width)
	}
	return m.renderPreview(session, width)
}

func (m Model) renderPreview(session sessions.Session, width int) string {
	innerWidth := max(8, width-4)
	lines := []string{
		sideTitleStyle.Render(truncate(session.Title, innerWidth)),
		mutedStyle.Render(truncate(session.Project+"  "+shortTime(session.UpdatedAt), innerWidth)),
		mutedStyle.Render(truncate(session.ID, innerWidth)),
		"",
	}
	if len(session.Transcript) == 0 {
		lines = append(lines, mutedStyle.Render("no transcript preview"))
		return sideBoxStyle.Width(width).Render(strings.Join(lines, "\n"))
	}
	for _, line := range session.Transcript {
		role := roleStyle.Render(line.Role + ":")
		lines = append(lines, wrap(role+" "+line.Text, innerWidth)...)
		lines = append(lines, "")
	}
	return sideBoxStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) renderDetail(session sessions.Session, width int) string {
	fields := []string{
		"title: " + session.Title,
		"id: " + session.ID,
		"project: " + session.Project,
		"source: " + session.Source,
		"turns: " + fmt.Sprint(session.Turns),
		"tokens: " + fmt.Sprint(session.TokensUsed),
		"created: " + fullTime(session.CreatedAt),
		"updated: " + fullTime(session.UpdatedAt),
		"cwd: " + session.CWD,
		"path: " + session.Path,
	}
	innerWidth := max(8, width-4)
	lines := []string{sideTitleStyle.Render("details"), ""}
	for _, field := range fields {
		lines = append(lines, wrap(field, innerWidth)...)
	}
	return sideBoxStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func compactMeta(session sessions.Session) string {
	project := session.Project
	if project == "" {
		project = "unknown"
	}
	project = truncate(project, 12)
	return project + " · " + shortTime(session.UpdatedAt)
}

func paddedWidth(width int) int {
	return max(1, width-2)
}

func lineWithMeta(prefix, title, meta string, width int) string {
	if width <= 0 {
		return ""
	}
	prefixWidth := lipgloss.Width(prefix)
	available := width - prefixWidth
	if available <= 0 {
		return truncate(prefix, width)
	}
	if width < 48 || meta == "" {
		return truncate(prefix+title, width)
	}

	metaWidth := min(28, lipgloss.Width(meta))
	meta = truncate(meta, metaWidth)
	gapWidth := 2
	titleWidth := available - metaWidth - gapWidth
	if titleWidth < 12 {
		return truncate(prefix+title, width)
	}

	title = truncate(title, titleWidth)
	padding := width - prefixWidth - lipgloss.Width(title) - metaWidth
	if padding < gapWidth {
		padding = gapWidth
	}
	return prefix + title + strings.Repeat(" ", padding) + projectStyle.Render(meta)
}

func (m Model) overlay(base string) string {
	if !m.help {
		return base
	}
	help := []string{
		helpTitleStyle.Render("cx help"),
		"",
		"navigation",
		"  arrows, ctrl+j/ctrl+k, mouse wheel, pgup/pgdn, home/end",
		"  plain typing always searches, including j and k",
		"",
		"actions",
		"  enter          resume selected thread",
		"  ctrl+f         fork selected thread",
		"  y              copy selected session id",
		"  :copy path     copy rollout jsonl path",
		"  :copy resume   copy codex resume command",
		"",
		"views",
		"  tab            preview panel",
		"  ctrl+e         detail/explain panel",
		"  ctrl+v         compact/comfy rows",
		"",
		"commands",
		"  :resume  :fork  :copy [id|path|cwd|title|resume|fork]",
		"  :view [compact|comfy]  :preview  :detail  :clear  :quit",
		"",
		"press ? or esc to close",
	}
	panelWidth := min(74, max(40, m.width-8))
	panel := helpBoxStyle.Width(panelWidth).Render(strings.Join(help, "\n"))
	return base + "\n" + panel
}

func joinColumns(left, right, gap string) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	rows := max(len(leftLines), len(rightLines))
	out := make([]string, 0, rows)
	for i := range rows {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		out = append(out, l+gap+r)
	}
	return strings.Join(out, "\n")
}

func wrap(text string, width int) []string {
	if width <= 4 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) > width {
			lines = append(lines, truncate(current, width))
			current = word
			continue
		}
		current += " " + word
	}
	lines = append(lines, truncate(current, width))
	return lines
}

func truncate(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func shortTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("Jan 02 15:04")
}

func fullTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format(time.RFC3339)
}

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115")).Padding(0, 1)
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1)
	commandStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236")).Padding(0, 1)
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	projectStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
	previewStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	rowStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236"))
	emptyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(1, 2)
	sideBoxStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	helpBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("109")).Padding(1, 2)
	helpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115"))
	sideTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115"))
	roleStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("109")).Bold(true)
)
