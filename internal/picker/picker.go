package picker

import (
	"fmt"
	"path/filepath"
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
	result   Result
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
		case tea.KeyUp:
			m.move(-1)
		case tea.KeyDown:
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
			case "j":
				if m.query == "" {
					m.move(1)
					return m, nil
				}
			case "k":
				if m.query == "" {
					m.move(-1)
					return m, nil
				}
			}
			m.query += key
			m.refreshFilter()
		}
	}
	return m, nil
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
		return b.String()
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
	return b.String()
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
	return headerStyle.Width(m.width).Render("cx  " + mutedStyle.Render(query) + "  " + count)
}

func (m Model) footer() string {
	mode := "compact"
	if m.comfy {
		mode = "comfy"
	}
	preview := "preview:on"
	if !m.preview {
		preview = "preview:off"
	}
	text := "enter resume  ctrl+f fork  tab preview  ctrl+e detail  ctrl+v " + mode + "  " + preview + "  esc quit"
	return footerStyle.Width(m.width).Render(truncate(text, m.width))
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
		prefix = "> "
		style = selectedStyle
	}

	title := session.Title
	if title == "" {
		title = session.ID
	}
	meta := fmt.Sprintf("%s  %s  %s", session.Project, shortTime(session.UpdatedAt), filepath.Base(session.CWD))
	first := prefix + title
	if width > 40 {
		padding := width - lipgloss.Width(prefix) - lipgloss.Width(title) - lipgloss.Width(meta) - 1
		if padding > 0 {
			first = prefix + title + strings.Repeat(" ", padding) + meta
		}
	}
	first = truncate(first, width)

	if !m.comfy {
		return []string{style.Width(width).Render(first)}
	}

	second := "  " + mutedStyle.Render(truncate(session.Preview, max(10, width-2)))
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
	lines := []string{
		sideTitleStyle.Render(truncate(session.Title, width)),
		mutedStyle.Render(truncate(session.ID, width)),
		"",
	}
	if len(session.Transcript) == 0 {
		lines = append(lines, mutedStyle.Render("no transcript preview"))
		return boxStyle.Width(width).Render(strings.Join(lines, "\n"))
	}
	for _, line := range session.Transcript {
		role := roleStyle.Render(line.Role + ":")
		lines = append(lines, wrap(role+" "+line.Text, width)...)
		lines = append(lines, "")
	}
	return boxStyle.Width(width).Render(strings.Join(lines, "\n"))
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
	lines := []string{sideTitleStyle.Render("details"), ""}
	for _, field := range fields {
		lines = append(lines, wrap(field, width)...)
	}
	return boxStyle.Width(width).Render(strings.Join(lines, "\n"))
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
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	rowStyle       = lipgloss.NewStyle()
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("235"))
	emptyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	boxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	sideTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	roleStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
)
