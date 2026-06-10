package picker

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/indexer"
	"github.com/ardasevinc/cx/internal/projects"
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
	roots               map[string]projects.Root
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
	previewScroll       int
	detail              bool
	previewBeforeDetail bool
	comfy               bool
	help                bool
	command             bool
	cmdText             string
	notice              string
	result              Result
	indexEnabled        bool
	indexOptions        indexer.Options
	searchSeq           int
	searchPending       bool
	transcriptHits      map[string][]indexer.SearchResult
	previewCache        map[string]indexer.Preview
	previewPending      map[string]bool
}

type copyMsg struct {
	label string
	err   error
}

func New(items []sessions.Session) Model {
	return newModel(items, indexer.Options{}, false)
}

func NewWithIndex(items []sessions.Session, opts indexer.Options) Model {
	return newModel(items, opts, true)
}

func newModel(items []sessions.Session, opts indexer.Options, indexEnabled bool) Model {
	model := Model{
		all:            items,
		roots:          projects.ClassifySessions(items, projects.Options{}),
		filtered:       items,
		view:           viewAll,
		group:          groupProjects,
		collapsed:      make(map[string]bool),
		indexEnabled:   indexEnabled,
		indexOptions:   opts,
		transcriptHits: make(map[string][]indexer.SearchResult),
		previewCache:   make(map[string]indexer.Preview),
		previewPending: make(map[string]bool),
	}
	model.refreshRows()
	return model
}

func (m Model) Init() tea.Cmd {
	if !m.indexEnabled {
		return nil
	}
	return tea.Batch(m.refreshIndexCmd(), m.loadSelectedPreviewCmd())
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
		side = m.renderSide(m.width-listWidth-2, m.bodyHeight())
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

func (m *Model) move(delta int) {
	if len(m.rows) == 0 {
		return
	}
	before := m.cursor
	m.cursor += delta
	m.clamp()
	if m.cursor != before {
		m.previewScroll = 0
	}
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

func (m *Model) scrollPreview(delta int) {
	if !m.preview {
		return
	}
	m.previewScroll += delta
	m.previewScroll = min(max(0, m.previewScroll), m.maxPreviewScroll())
}

func (m Model) previewPageSize() int {
	return max(1, m.bodyHeight()-4)
}

func (m Model) maxPreviewScroll() int {
	selected, ok := m.selectedRow()
	if !ok || selected.Kind != rowSession {
		return 0
	}
	var width int
	if m.preview && m.width >= sideBySideMinWidth {
		listWidth := (m.width * 58) / 100
		width = m.width - listWidth - 2
	} else {
		width = min(84, max(floatingMinWidth, m.width-6))
	}
	innerWidth := max(8, width-4)
	maxLines := max(1, m.previewPanelHeight()-2)
	return max(0, len(m.previewLines(selected.Session, innerWidth))-maxLines+1)
}

func (m Model) previewPanelHeight() int {
	if m.preview && m.width >= sideBySideMinWidth {
		return m.bodyHeight()
	}
	return m.floatingPanelHeight()
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
		if selected.Dir != "" && !selected.Chat {
			return Result{Action: ActionNew, Dir: selected.Dir}
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
