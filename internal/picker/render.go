package picker

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ardasevinc/cx/internal/projects"
	"github.com/ardasevinc/cx/internal/sessions"
)

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

func (m Model) bodyHeight() int {
	reserved := 3
	if m.height <= reserved {
		return 1
	}
	return max(1, m.height-reserved)
}

func (m Model) header() string {
	query := m.query
	if query == "" {
		query = "type to search"
	}
	count := fmt.Sprintf("%d/%d", len(m.filtered), len(m.all))
	mode := string(m.view)
	if m.view == viewGrouped {
		mode += ":" + string(m.group)
	}
	text := "cx  " + mode + "  " + query + "  " + count
	return headerStyle.Render(truncate(text, paddedWidth(m.width)))
}

func (m Model) footer() string {
	if m.command {
		return commandStyle.Render(truncate(":"+m.cmdText, paddedWidth(m.width)))
	}
	if m.view == viewGrouped {
		text := "enter toggle  ← close  → open  ^n new here  :open-all/:close-all  ^p projects  :n chat"
		if m.notice != "" {
			text = m.notice + "  " + text
		}
		return footerStyle.Render(truncate(text, paddedWidth(m.width)))
	}
	text := "enter resume/new/toggle  ^n new here  ^p projects  ^g grouped  :n chat  ^f fork  y copy  ? help"
	if m.notice != "" {
		text = m.notice + "  " + text
	}
	return footerStyle.Render(truncate(text, paddedWidth(m.width)))
}

func (m Model) renderList(width int) string {
	height := m.listHeight()
	end := min(len(m.rows), m.offset+height)
	bodyHeight := m.bodyHeight()
	lines := make([]string, 0, bodyHeight)

	for i := m.offset; i < end; i++ {
		selected := i == m.cursor
		lines = append(lines, m.renderRow(m.rows[i], width, selected)...)
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	if len(lines) > bodyHeight {
		lines = lines[:bodyHeight]
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderRow(row row, width int, selected bool) []string {
	prefix := "  "
	style := rowStyle
	if selected {
		prefix = "▌ "
		style = selectedStyle
	}
	if row.Kind == rowNewChat {
		if selected {
			style = selectedActionStyle
		} else {
			style = actionRowStyle
		}
	}
	title := row.Title
	if title == "" {
		title = row.ID
	}
	if row.Kind == rowGroup {
		if row.Expanded {
			title = "▾ " + title
		} else {
			title = "▸ " + title
		}
	}
	if row.Depth > 0 {
		title = strings.Repeat("  ", row.Depth) + title
	}
	if row.Kind == rowNewChat {
		title = actionTitleStyle.Render(title)
	}
	first := lineWithMeta(prefix, title, row.Meta, width)

	if !m.comfy || row.Kind != rowSession {
		return []string{style.Width(width).Render(first)}
	}

	context := row.Session.Preview
	if context == "" {
		context = row.Session.CWD
	}
	second := "  " + previewStyle.Render(truncate(context, max(10, width-2)))
	return []string{
		style.Width(width).Render(first),
		style.Width(width).Render(second),
	}
}

func (m Model) renderSide(width, height int) string {
	selected, ok := m.selectedRow()
	if !ok {
		return ""
	}
	if selected.Kind != rowSession {
		return m.renderContextSide(selected, width, height)
	}
	if m.detail {
		return m.renderDetail(selected.Session, width, height)
	}
	return m.renderPreview(selected.Session, width, height)
}

func (m Model) renderContextSide(row row, width, height int) string {
	innerWidth := max(8, width-4)
	lines := []string{sideTitleStyle.Render(row.Title), mutedStyle.Render(row.Meta), ""}
	switch row.Kind {
	case rowNewChat:
		lines = append(lines, "creates a fresh chat folder for today", "then runs:", "", mutedStyle.Render("codex --yolo -C <created-dir>"))
	case rowProject:
		if row.Chat {
			lines = append(lines, "press enter to start a fresh general chat")
		} else {
			lines = append(lines, "press enter to start Codex in:", "", row.Dir)
		}
	case rowGroup:
		lines = append(lines, "press enter to expand or collapse")
		if row.Dir != "" {
			lines = append(lines, "", "ctrl+n starts Codex in:", "", row.Dir)
		}
	}
	return sideBoxStyle.Width(width).Render(strings.Join(fitPanelLines(fitTextLines(lines, innerWidth), height), "\n"))
}

func (m Model) renderPreview(session sessions.Session, width, height int) string {
	innerWidth := max(8, width-4)
	lines := []string{
		sideTitleStyle.Render(truncate(session.Title, innerWidth)),
		mutedStyle.Render(truncate(session.Project+"  "+shortTime(session.UpdatedAt), innerWidth)),
		mutedStyle.Render(truncate(session.ID, innerWidth)),
		"",
	}
	if len(session.Transcript) == 0 {
		lines = append(lines, mutedStyle.Render("no transcript preview"))
		return sideBoxStyle.Width(width).Render(strings.Join(fitPanelLines(lines, height), "\n"))
	}
	for _, line := range session.Transcript {
		role := roleStyle.Render(line.Role + ":")
		lines = append(lines, wrap(role+" "+line.Text, innerWidth)...)
		lines = append(lines, "")
	}
	return sideBoxStyle.Width(width).Render(strings.Join(fitPanelLines(lines, height), "\n"))
}

func (m Model) renderDetail(session sessions.Session, width, height int) string {
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
	if session.ParentID != "" {
		fields = append(fields[:2], append([]string{"forked from: " + session.ParentID}, fields[2:]...)...)
	}
	innerWidth := max(8, width-4)
	lines := []string{sideTitleStyle.Render("details"), ""}
	for _, field := range fields {
		lines = append(lines, wrap(field, innerWidth)...)
	}
	return sideBoxStyle.Width(width).Render(strings.Join(fitPanelLines(lines, height), "\n"))
}

func compactMeta(session sessions.Session, root projects.Root) string {
	project := root.DisplayName
	if project == "" {
		project = firstNonEmpty(session.Project, "unknown")
	}
	project = truncate(project, 22)
	if session.ParentID != "" {
		project = "⎇ " + project
	}
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

	metaWidth := min(40, lipgloss.Width(meta))
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
		"  grouped: left closes selected group, right opens selected group",
		"",
		"actions",
		"  enter          resume selected thread or start selected new chat",
		"  :new           start a fresh general chat",
		"  ctrl+n         start fresh Codex in selected context",
		"  ctrl+f         fork selected thread",
		"  y              copy selected session id",
		"  :copy path     copy rollout jsonl path",
		"  :copy resume   copy codex resume command",
		"",
		"views",
		"  ctrl+p         project launcher",
		"  ctrl+g         grouped projects",
		"  tab            preview side/popup",
		"  ctrl+e         detail/explain side/popup",
		"  ctrl+v         compact/comfy rows",
		"",
		"commands",
		"  :new [name]  :resume  :fork  :copy [id|path|cwd|title|resume|fork]",
		"  :view [all|chats|projects|grouped|compact|comfy]",
		"  :group [projects|chats]  :open  :close  :open-all  :close-all",
		"  :preview  :detail  :clear  :quit",
		"",
		"press ? or esc to close",
	}
	panelWidth := min(74, max(40, m.width-8))
	panel := helpBoxStyle.Width(panelWidth).Render(strings.Join(help, "\n"))
	return base + "\n" + panel
}

func (m Model) placeFloatingPanel(base, panel string, width int) string {
	baseLines := strings.Split(base, "\n")
	panelLines := strings.Split(panel, "\n")
	if len(panelLines) == 0 {
		return base
	}
	left := max(0, (width-lipgloss.Width(panelLines[0]))/2)
	top := floatingPanelTop(len(baseLines), len(panelLines), m.selectedRowInViewport())
	for i, panelLine := range panelLines {
		row := top + i
		line := strings.Repeat(" ", left) + panelLine
		if row >= len(baseLines) {
			baseLines = append(baseLines, line)
			continue
		}
		baseLines[row] = line
	}
	return strings.Join(baseLines, "\n")
}

func (m Model) selectedRowInViewport() int {
	rowHeight := 1
	if m.comfy {
		rowHeight = 2
	}
	return max(0, (m.cursor-m.offset)*rowHeight)
}

func (m Model) floatingPanelHeight() int {
	baseHeight := m.bodyHeight()
	desired := min(18, max(floatingMinHeight, m.height-5))
	if baseHeight <= desired {
		return min(desired, baseHeight)
	}
	selectedRow := m.selectedRowInViewport()
	aboveSpace := selectedRow
	belowSpace := baseHeight - selectedRow - 1
	if belowSpace >= floatingMinHeight {
		return min(desired, belowSpace)
	}
	if aboveSpace >= floatingMinHeight {
		return min(desired, aboveSpace)
	}
	return min(desired, baseHeight)
}

func floatingPanelTop(baseHeight, panelHeight, avoidRow int) int {
	if baseHeight <= panelHeight {
		return 0
	}
	aboveSpace := avoidRow
	belowSpace := baseHeight - avoidRow - 1
	switch {
	case belowSpace >= panelHeight:
		return avoidRow + 1
	case aboveSpace >= panelHeight:
		return avoidRow - panelHeight
	case belowSpace >= aboveSpace:
		return max(0, baseHeight-panelHeight)
	default:
		return 0
	}
}

func fitPanelLines(lines []string, height int) []string {
	maxLines := max(1, height-2)
	if len(lines) <= maxLines {
		return lines
	}
	fitted := append([]string{}, lines[:maxLines-1]...)
	fitted = append(fitted, mutedStyle.Render("…"))
	return fitted
}

func fitTextLines(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, truncate(line, width))
	}
	return out
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
	headerStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115")).Padding(0, 1)
	footerStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1)
	commandStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236")).Padding(0, 1)
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	projectStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
	previewStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	rowStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("236"))
	actionRowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("115"))
	selectedActionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("29"))
	actionTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115"))
	emptyStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(1, 2)
	sideBoxStyle        = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	helpBoxStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("109")).Padding(1, 2)
	helpTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115"))
	sideTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("115"))
	roleStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("109")).Bold(true)
)
