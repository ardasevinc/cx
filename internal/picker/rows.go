package picker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ardasevinc/cx/internal/projects"
	"github.com/ardasevinc/cx/internal/sessions"
)

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
	m.filtered = m.filteredSessions()
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
	m.filtered = m.filteredSessions()
	m.rows = m.buildRows()
	m.cursor = m.firstSessionCursor()
	m.offset = 0
	if m.height > 0 {
		m.clamp()
	}
}

func (m Model) filteredSessions() []sessions.Session {
	filtered := sessions.Filter(m.all, m.query)
	if strings.TrimSpace(m.query) == "" || len(m.transcriptHits) == 0 {
		return filtered
	}
	seen := make(map[string]bool, len(filtered))
	for _, session := range filtered {
		seen[session.ID] = true
	}
	for _, session := range m.all {
		if seen[session.ID] {
			continue
		}
		if _, ok := m.transcriptHits[session.ID]; ok {
			filtered = append(filtered, session)
			seen[session.ID] = true
		}
	}
	return filtered
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
			if m.isChatSession(session) {
				rows = append(rows, m.sessionRow(session, 0))
			}
		}
	case viewProjects:
		rows = append(rows, m.projectRows()...)
	case viewGrouped:
		if m.group == groupChats {
			rows = append(rows, m.groupedChatRows()...)
		} else {
			rows = append(rows, m.groupedProjectRows()...)
		}
	default:
		for _, session := range m.filtered {
			rows = append(rows, m.sessionRow(session, 0))
		}
	}
	return rows
}

func (m Model) sessionRow(session sessions.Session, depth int) row {
	root := m.rootFor(session)
	meta := compactMeta(session, root)
	if snippet := m.hitSnippet(session.ID); snippet != "" {
		meta = "hit · " + meta
	}
	return row{
		Kind:    rowSession,
		ID:      "session:" + session.ID,
		Title:   session.Title,
		Meta:    meta,
		Depth:   depth,
		Dir:     root.Dir,
		Chat:    root.Kind == projects.KindChat,
		Session: session,
		Latest:  session.UpdatedAt,
	}
}

func (m Model) projectRows() []row {
	projectMap := make(map[string]row)
	var chats row
	for _, session := range m.filtered {
		root := m.rootFor(session)
		if root.Kind == projects.KindChat {
			chats.Kind = rowProject
			chats.ID = "project:chats"
			chats.Title = "chats"
			chats.Chat = true
			chats.Dir = root.Dir
			chats.Count++
			if session.UpdatedAt.After(chats.Latest) {
				chats.Latest = session.UpdatedAt
			}
			continue
		}
		if root.Key == "" || root.Kind == projects.KindUnknown {
			continue
		}
		key := root.Key
		project := projectMap[key]
		project.Kind = rowProject
		project.ID = "project:" + key
		project.Title = root.DisplayName
		project.Dir = root.Dir
		project.Count++
		if session.UpdatedAt.After(project.Latest) {
			project.Latest = session.UpdatedAt
		}
		projectMap[key] = project
	}

	rows := make([]row, 0, len(projectMap)+1)
	if chats.Count > 0 {
		chats.Meta = countMeta(chats.Count, chats.Latest)
		rows = append(rows, chats)
	}
	for _, project := range projectMap {
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
	for _, group := range m.projectRows() {
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
			root := m.rootFor(session)
			if group.Chat != (root.Kind == projects.KindChat) {
				continue
			}
			if !group.Chat && root.Key != strings.TrimPrefix(group.ID, "group:project:") {
				continue
			}
			rows = append(rows, m.sessionRow(session, 1))
		}
	}
	return rows
}

func (m Model) groupedChatRows() []row {
	groups := make(map[string]row)
	for _, session := range m.filtered {
		if !m.isChatSession(session) {
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
			if m.isChatSession(session) && chatDateKey(session) == strings.TrimPrefix(group.ID, "group:chats:") {
				rows = append(rows, m.sessionRow(session, 1))
			}
		}
	}
	return rows
}

func (m Model) isChatSession(session sessions.Session) bool {
	return m.rootFor(session).Kind == projects.KindChat
}

func (m Model) rootFor(session sessions.Session) projects.Root {
	if session.Project == "chats" {
		return projects.Root{Key: "chats", DisplayName: "chats", Dir: session.CWD, Kind: projects.KindChat}
	}
	if root, ok := m.roots[session.CWD]; ok {
		return root
	}
	if session.Project != "" {
		return projects.Root{
			Key:         "session:" + session.Project,
			DisplayName: session.Project,
			Dir:         session.CWD,
			Kind:        projects.KindCWD,
		}
	}
	return projects.Root{
		Key:         "session:" + session.CWD,
		DisplayName: firstNonEmpty(session.Project, session.CWD, "unknown"),
		Dir:         session.CWD,
		Kind:        projects.KindUnknown,
	}
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
