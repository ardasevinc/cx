package picker

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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
	case transcriptSearchMsg:
		m.applyTranscriptSearch(msg)
		return m, m.loadSelectedPreviewCmd()
	case previewLoadedMsg:
		m.applyPreviewLoaded(msg)
	case indexRefreshMsg:
		m.applyIndexRefresh(msg)
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
		return m, m.loadSelectedPreviewCmd()
	case tea.KeyCtrlV:
		m.comfy = !m.comfy
		m.clamp()
	case tea.KeyTab:
		m.preview = !m.preview
		m.previewInitialized = true
		if !m.preview {
			m.detail = false
		}
		return m, m.loadSelectedPreviewCmd()
	case tea.KeyCtrlL:
		m.notice = ""
	case tea.KeyUp, tea.KeyCtrlK:
		m.move(-1)
		return m, m.loadSelectedPreviewCmd()
	case tea.KeyDown, tea.KeyCtrlJ:
		m.move(1)
		return m, m.loadSelectedPreviewCmd()
	case tea.KeyLeft:
		m.setSelectedGroupCollapsed(true)
	case tea.KeyRight:
		m.setSelectedGroupCollapsed(false)
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
			return m, tea.Batch(m.queueTranscriptSearch(), m.loadSelectedPreviewCmd())
		}
	case tea.KeyCtrlU:
		m.query = ""
		m.refreshRows()
		return m, tea.Batch(m.queueTranscriptSearch(), m.loadSelectedPreviewCmd())
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
		return m, tea.Batch(m.queueTranscriptSearch(), m.loadSelectedPreviewCmd())
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
	case tea.KeySpace:
		m.cmdText += " "
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
		return m, tea.Batch(m.queueTranscriptSearch(), m.loadSelectedPreviewCmd())
	default:
		m.notice = "unknown command: " + input
	}
	m.clamp()
	return m, m.loadSelectedPreviewCmd()
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
