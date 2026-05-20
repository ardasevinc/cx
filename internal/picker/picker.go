package picker

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
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
	ActionNew    Action = "new"
	ActionQuit   Action = "quit"

	sideBySideMinWidth = 100
	floatingMinWidth   = 42
	floatingMinHeight  = 4
)

type Result struct {
	Action  Action
	Session sessions.Session
	Dir     string
	Chat    bool
	Name    string
}

type viewMode string

const (
	viewAll      viewMode = "all"
	viewChats    viewMode = "chats"
	viewProjects viewMode = "projects"
	viewGrouped  viewMode = "grouped"
)

type groupMode string

const (
	groupProjects groupMode = "projects"
	groupChats    groupMode = "chats"
)

type rowKind int

const (
	rowNewChat rowKind = iota
	rowSession
	rowProject
	rowGroup
)

type row struct {
	Kind     rowKind
	ID       string
	Title    string
	Meta     string
	Dir      string
	Depth    int
	Count    int
	Latest   time.Time
	Chat     bool
	Expanded bool
	Session  sessions.Session
}

type Model struct {
	all                 []sessions.Session
	filtered            []sessions.Session
	rows                []row
	query               string
	cursor              int
	offset              int
	width               int
	height              int
	view                viewMode
	group               groupMode
	collapsed           map[string]bool
	preview             bool
	previewInitialized  bool
	detail              bool
	previewBeforeDetail bool
	comfy               bool
	help                bool
	command             bool
	cmdText             string
	notice              string
	result              Result
}

type copyMsg struct {
	label string
	err   error
}

func New(items []sessions.Session) Model {
	model := Model{
		all:       items,
		filtered:  items,
		view:      viewAll,
		group:     groupProjects,
		collapsed: make(map[string]bool),
	}
	model.refreshRows()
	return model
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.initPreviewForWidth()
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
	m.initPreviewForWidth()
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		result, toggled := m.enterSelection()
		if toggled {
			return m, nil
		}
		m.result = result
		return m, tea.Quit
	case tea.KeyCtrlF:
		m.result = m.selection(ActionFork)
		return m, tea.Quit
	case tea.KeyCtrlN:
		m.result = m.newSelection()
		return m, tea.Quit
	case tea.KeyCtrlP:
		m.view = viewProjects
		m.refreshRows()
	case tea.KeyCtrlG:
		m.view = viewGrouped
		m.group = groupProjects
		m.refreshRows()
	case tea.KeyCtrlE:
		m.toggleDetail()
	case tea.KeyCtrlV:
		m.comfy = !m.comfy
		m.clamp()
	case tea.KeyTab:
		m.preview = !m.preview
		m.previewInitialized = true
		if !m.preview {
			m.detail = false
		}
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
		m.cursor = len(m.rows) - 1
		m.clamp()
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.query != "" {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.refreshRows()
		}
	case tea.KeyCtrlU:
		m.query = ""
		m.refreshRows()
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
		m.refreshRows()
	}
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.move(-1)
	case tea.MouseButtonWheelDown:
		m.move(1)
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
	m.initPreviewForWidth()
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m, nil
	}
	command := strings.ToLower(fields[0])
	switch command {
	case "q", "quit", "exit":
		m.result = Result{Action: ActionQuit}
		return m, tea.Quit
	case "r", "resume":
		m.result = m.selection(ActionResume)
		return m, tea.Quit
	case "n", "new":
		m.result = Result{Action: ActionNew, Chat: true, Name: strings.Join(fields[1:], " ")}
		return m, tea.Quit
	case "f", "fork":
		m.result = m.selection(ActionFork)
		return m, tea.Quit
	case "y", "copy", "cp":
		target := "id"
		if len(fields) > 1 {
			target = strings.ToLower(fields[1])
		}
		return m, m.copySelection(target)
	case "h", "help", "?":
		m.help = true
	case "p", "preview":
		m.preview = !m.preview
		m.previewInitialized = true
		if !m.preview {
			m.detail = false
		}
	case "e", "detail", "explain":
		m.toggleDetail()
	case "v", "view":
		if len(fields) > 1 {
			m.setView(strings.ToLower(fields[1]))
		} else {
			m.comfy = !m.comfy
		}
	case "g", "group":
		m.view = viewGrouped
		if len(fields) > 1 {
			m.setGroup(strings.ToLower(fields[1]))
		}
		m.refreshRows()
	case "open":
		m.setSelectedGroupCollapsed(false)
	case "close":
		m.setSelectedGroupCollapsed(true)
	case "toggle":
		m.toggleSelectedGroup()
	case "open-all":
		m.setAllGroupsCollapsed(false)
	case "close-all":
		m.setAllGroupsCollapsed(true)
	case "compact":
		m.comfy = false
	case "comfy", "comfortable":
		m.comfy = true
	case "clear":
		m.query = ""
		m.refreshRows()
	default:
		m.notice = "unknown command: " + input
	}
	m.clamp()
	return m, nil
}

func (m *Model) toggleDetail() {
	if m.detail {
		m.detail = false
		m.preview = m.previewBeforeDetail
		m.previewInitialized = true
		return
	}
	m.previewBeforeDetail = m.preview
	m.preview = true
	m.previewInitialized = true
	m.detail = true
}

func (m *Model) initPreviewForWidth() {
	if m.previewInitialized {
		return
	}
	m.preview = m.width >= sideBySideMinWidth
	m.previewInitialized = true
}

func (m *Model) setView(view string) {
	switch view {
	case "all", "sessions":
		m.view = viewAll
		m.refreshRows()
	case "chats":
		m.view = viewChats
		m.refreshRows()
	case "projects", "project":
		m.view = viewProjects
		m.refreshRows()
	case "grouped", "tree":
		m.view = viewGrouped
		m.refreshRows()
	case "compact", "dense":
		m.comfy = false
	case "comfy", "comfortable":
		m.comfy = true
	}
}

func (m *Model) setGroup(group string) {
	switch group {
	case "chats", "chat":
		m.group = groupChats
	case "projects", "project":
		m.group = groupProjects
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading cx..."
	}
	m.initPreviewForWidth()
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")

	if len(m.rows) == 0 {
		b.WriteString(emptyStyle.Render("no sessions match"))
		b.WriteString("\n")
		b.WriteString(m.footer())
		return m.overlay(b.String())
	}

	listWidth := m.width
	if m.preview && m.width >= sideBySideMinWidth {
		listWidth = (m.width * 58) / 100
	}

	list := m.renderList(listWidth)
	side := ""
	if m.preview && m.width >= sideBySideMinWidth {
		side = m.renderSide(m.width-listWidth-2, m.listHeight())
	}

	if side == "" {
		if m.preview && m.width >= floatingMinWidth {
			panelWidth := min(84, max(floatingMinWidth, m.width-6))
			panelHeight := m.floatingPanelHeight()
			list = m.placeFloatingPanel(list, m.renderSide(panelWidth, panelHeight), m.width)
		}
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

func (m Model) firstSessionCursor() int {
	for i, row := range m.rows {
		if row.Kind == rowSession {
			return i
		}
	}
	return 0
}

func (m Model) selectedRow() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

func (m *Model) toggleSelectedGroup() bool {
	selected, ok := m.selectedRow()
	if !ok || selected.Kind != rowGroup {
		return false
	}
	m.collapsed[selected.ID] = !m.collapsed[selected.ID]
	m.refreshRowsKeepingCursor(selected.ID)
	return true
}

func (m *Model) setSelectedGroupCollapsed(collapsed bool) {
	selected, ok := m.selectedRow()
	if !ok || selected.Kind != rowGroup {
		return
	}
	m.collapsed[selected.ID] = collapsed
	m.refreshRowsKeepingCursor(selected.ID)
}

func (m *Model) setAllGroupsCollapsed(collapsed bool) {
	for _, row := range m.rows {
		if row.Kind == rowGroup {
			m.collapsed[row.ID] = collapsed
		}
	}
	m.refreshRows()
}

func (m *Model) refreshRowsKeepingCursor(id string) {
	m.filtered = sessions.Filter(m.all, m.query)
	m.rows = m.buildRows()
	for i, row := range m.rows {
		if row.ID == id {
			m.cursor = i
			if m.height > 0 {
				m.clamp()
			}
			return
		}
	}
	if m.height > 0 {
		m.clamp()
	}
}

func (m *Model) refreshRows() {
	m.filtered = sessions.Filter(m.all, m.query)
	m.rows = m.buildRows()
	m.cursor = m.firstSessionCursor()
	m.offset = 0
	if m.height > 0 {
		m.clamp()
	}
}

func (m Model) buildRows() []row {
	rows := make([]row, 0, len(m.filtered)+1)
	if m.query == "" {
		rows = append(rows, row{
			Kind:  rowNewChat,
			ID:    "action:new-chat",
			Title: "+ new chat",
			Meta:  "today · Documents/Codex",
			Chat:  true,
		})
	}

	switch m.view {
	case viewChats:
		for _, session := range m.filtered {
			if isChatSession(session) {
				rows = append(rows, sessionRow(session, 0))
			}
		}
	case viewProjects:
		rows = append(rows, projectRows(m.filtered)...)
	case viewGrouped:
		if m.group == groupChats {
			rows = append(rows, m.groupedChatRows()...)
		} else {
			rows = append(rows, m.groupedProjectRows()...)
		}
	default:
		for _, session := range m.filtered {
			rows = append(rows, sessionRow(session, 0))
		}
	}
	return rows
}

func sessionRow(session sessions.Session, depth int) row {
	return row{
		Kind:    rowSession,
		ID:      "session:" + session.ID,
		Title:   session.Title,
		Meta:    compactMeta(session),
		Depth:   depth,
		Dir:     session.CWD,
		Chat:    isChatSession(session),
		Session: session,
		Latest:  session.UpdatedAt,
	}
}

func projectRows(items []sessions.Session) []row {
	projects := make(map[string]row)
	var chats row
	for _, session := range items {
		if isChatSession(session) {
			chats.Kind = rowProject
			chats.ID = "project:chats"
			chats.Title = "chats"
			chats.Chat = true
			chats.Count++
			if session.UpdatedAt.After(chats.Latest) {
				chats.Latest = session.UpdatedAt
			}
			continue
		}
		if strings.TrimSpace(session.CWD) == "" {
			continue
		}
		key := session.CWD
		project := projects[key]
		project.Kind = rowProject
		project.ID = "project:" + key
		project.Title = firstNonEmpty(session.Project, session.CWD)
		project.Dir = session.CWD
		project.Count++
		if session.UpdatedAt.After(project.Latest) {
			project.Latest = session.UpdatedAt
		}
		projects[key] = project
	}

	rows := make([]row, 0, len(projects)+1)
	if chats.Count > 0 {
		chats.Meta = countMeta(chats.Count, chats.Latest)
		rows = append(rows, chats)
	}
	for _, project := range projects {
		project.Meta = countMeta(project.Count, project.Latest)
		rows = append(rows, project)
	}
	sortRows(rows)
	if len(rows) > 0 && rows[0].Chat {
		return rows
	}
	for i, item := range rows {
		if item.Chat {
			rows = append([]row{item}, append(rows[:i], rows[i+1:]...)...)
			break
		}
	}
	return rows
}

func (m Model) groupedProjectRows() []row {
	rows := make([]row, 0, len(m.filtered))
	for _, group := range projectRows(m.filtered) {
		key := "group:" + group.ID
		collapsed := m.query == "" && m.collapsed[key]
		group.Kind = rowGroup
		group.ID = key
		group.Expanded = !collapsed
		group.Meta = countMeta(group.Count, group.Latest)
		rows = append(rows, group)
		if collapsed {
			continue
		}
		for _, session := range m.filtered {
			if group.Chat != isChatSession(session) {
				continue
			}
			if !group.Chat && session.CWD != group.Dir {
				continue
			}
			rows = append(rows, sessionRow(session, 1))
		}
	}
	return rows
}

func (m Model) groupedChatRows() []row {
	groups := make(map[string]row)
	for _, session := range m.filtered {
		if !isChatSession(session) {
			continue
		}
		key := chatDateKey(session)
		group := groups[key]
		group.Kind = rowGroup
		group.ID = "group:chats:" + key
		group.Title = chatDateTitle(key)
		group.Chat = true
		group.Count++
		if session.UpdatedAt.After(group.Latest) {
			group.Latest = session.UpdatedAt
		}
		groups[key] = group
	}
	groupRows := make([]row, 0, len(groups))
	for _, group := range groups {
		group.Meta = countMeta(group.Count, group.Latest)
		groupRows = append(groupRows, group)
	}
	sortRows(groupRows)

	rows := make([]row, 0, len(m.filtered)+len(groupRows))
	for _, group := range groupRows {
		collapsed := m.query == "" && m.collapsed[group.ID]
		group.Expanded = !collapsed
		rows = append(rows, group)
		if collapsed {
			continue
		}
		for _, session := range m.filtered {
			if isChatSession(session) && chatDateKey(session) == strings.TrimPrefix(group.ID, "group:chats:") {
				rows = append(rows, sessionRow(session, 1))
			}
		}
	}
	return rows
}

func (m *Model) move(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor += delta
	m.clamp()
}

func (m *Model) clamp() {
	length := len(m.rows)
	if length == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= length {
		m.cursor = length - 1
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
	selected, ok := m.selectedRow()
	if !ok || selected.Kind != rowSession {
		return Result{}
	}
	return Result{Action: action, Session: selected.Session}
}

func (m *Model) enterSelection() (Result, bool) {
	selected, ok := m.selectedRow()
	if !ok {
		return Result{}, false
	}
	switch selected.Kind {
	case rowNewChat:
		return Result{Action: ActionNew, Chat: true}, false
	case rowSession:
		return Result{Action: ActionResume, Session: selected.Session}, false
	case rowProject:
		if selected.Chat {
			return Result{Action: ActionNew, Chat: true}, false
		}
		return Result{Action: ActionNew, Dir: selected.Dir}, false
	case rowGroup:
		m.collapsed[selected.ID] = !m.collapsed[selected.ID]
		m.refreshRowsKeepingCursor(selected.ID)
		return Result{}, true
	default:
		return Result{}, false
	}
}

func (m Model) newSelection() Result {
	selected, ok := m.selectedRow()
	if !ok {
		return Result{Action: ActionNew, Chat: true}
	}
	switch selected.Kind {
	case rowSession:
		if selected.Session.CWD != "" && selected.Session.Project != "chats" {
			return Result{Action: ActionNew, Dir: selected.Session.CWD}
		}
	case rowProject, rowGroup:
		if selected.Chat {
			return Result{Action: ActionNew, Chat: true}
		}
		if selected.Dir != "" {
			return Result{Action: ActionNew, Dir: selected.Dir}
		}
	}
	return Result{Action: ActionNew, Chat: true}
}

func (m Model) copySelection(target string) tea.Cmd {
	selected, ok := m.selectedRow()
	if !ok || selected.Kind != rowSession {
		return nil
	}
	label, value := copyValue(selected.Session, target)
	return func() tea.Msg {
		return copyMsg{label: label, err: writeClipboard(value)}
	}
}

func isChatSession(session sessions.Session) bool {
	return session.Project == "chats"
}

func countMeta(count int, latest time.Time) string {
	label := "thread"
	if count != 1 {
		label = "threads"
	}
	return fmt.Sprintf("%d %s · %s", count, label, shortTime(latest))
}

func sortRows(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Latest.Equal(rows[j].Latest) {
			return rows[i].Title < rows[j].Title
		}
		return rows[i].Latest.After(rows[j].Latest)
	})
}

func chatDateKey(session sessions.Session) string {
	parts := strings.Split(session.CWD, string(osPathSeparator()))
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "Documents" && parts[i+1] == "Codex" && looksLikeDate(parts[i+2]) {
			return parts[i+2]
		}
	}
	if !session.UpdatedAt.IsZero() {
		return session.UpdatedAt.Local().Format("2006-01-02")
	}
	return "unknown"
}

func chatDateTitle(key string) string {
	if key == time.Now().Local().Format("2006-01-02") {
		return "today"
	}
	return key
}

func looksLikeDate(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func osPathSeparator() rune {
	return '/'
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
		return "resume command", codexSessionCommand("resume", session)
	case "fork":
		return "fork command", codexSessionCommand("fork", session)
	default:
		return "id", session.ID
	}
}

func codexSessionCommand(action string, session sessions.Session) string {
	if session.CWD == "" {
		return "codex " + action + " " + session.ID
	}
	return "codex --yolo -C " + shellQuote(session.CWD) + " " + action + " " + session.ID
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`!*?[]{}()&;<>|") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
	text := "enter resume/new/toggle  ^n new here  ^p projects  ^g grouped  :n chat  ^f fork  y copy  ? help"
	if m.notice != "" {
		text = m.notice + "  " + text
	}
	return footerStyle.Render(truncate(text, paddedWidth(m.width)))
}

func (m Model) renderList(width int) string {
	height := m.listHeight()
	end := min(len(m.rows), m.offset+height)
	lines := make([]string, 0, height)

	for i := m.offset; i < end; i++ {
		selected := i == m.cursor
		lines = append(lines, m.renderRow(m.rows[i], width, selected)...)
	}
	for len(lines) < height {
		lines = append(lines, "")
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

func compactMeta(session sessions.Session) string {
	project := session.Project
	if project == "" {
		project = "unknown"
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
	baseHeight := m.listHeight()
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
