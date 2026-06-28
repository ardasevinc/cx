package picker

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/indexer"
)

type transcriptSearchMsg struct {
	seq     int
	query   string
	results []indexer.SearchResult
	err     error
}

type previewLoadedMsg struct {
	sessionID string
	preview   indexer.Preview
	err       error
}

type indexStatusMsg struct {
	status indexer.Status
	err    error
}

type indexRefreshMsg struct {
	result indexer.RebuildResult
	err    error
}

func (m Model) checkIndexStatusCmd() tea.Cmd {
	if !m.indexEnabled {
		return nil
	}
	return func() tea.Msg {
		status, err := indexer.CurrentStatus(m.indexOptions)
		return indexStatusMsg{status: status, err: err}
	}
}

func (m *Model) queueTranscriptSearch() tea.Cmd {
	if !m.indexEnabled {
		return nil
	}
	query := strings.TrimSpace(m.query)
	m.searchSeq++
	seq := m.searchSeq
	if len([]rune(query)) < 3 {
		m.searchPending = false
		m.transcriptHits = make(map[string][]indexer.SearchResult)
		return nil
	}
	m.searchPending = true
	return func() tea.Msg {
		results, err := indexer.Search(m.indexOptions, query, 80)
		return transcriptSearchMsg{seq: seq, query: query, results: results, err: err}
	}
}

func (m Model) refreshIndexCmd() tea.Cmd {
	if !m.indexEnabled {
		return nil
	}
	return func() tea.Msg {
		result, err := indexer.Refresh(m.indexOptions)
		return indexRefreshMsg{result: result, err: err}
	}
}

func (m *Model) loadSelectedPreviewCmd() tea.Cmd {
	if !m.indexEnabled {
		return nil
	}
	selected, ok := m.selectedRow()
	if !ok || selected.Kind != rowSession {
		return nil
	}
	sessionID := selected.Session.ID
	if sessionID == "" || m.previewPending[sessionID] {
		return nil
	}
	query := m.query
	m.previewPending[sessionID] = true
	return func() tea.Msg {
		preview, err := indexer.LoadPreview(m.indexOptions, sessionID, query, 2048)
		return previewLoadedMsg{sessionID: sessionID, preview: preview, err: err}
	}
}

func (m *Model) applyIndexStatus(msg indexStatusMsg) {
	m.indexChecking = false
	if msg.err != nil {
		m.notice = "index status failed: " + msg.err.Error()
		return
	}
	m.indexStatus = &msg.status
}

func (m *Model) applyIndexRefresh(msg indexRefreshMsg) {
	m.indexChecking = false
	m.indexRefreshing = false
	if msg.err != nil {
		m.notice = "index refresh failed: " + msg.err.Error()
		return
	}
	m.indexStatus = &msg.result.Status
	if stale := indexStaleCount(msg.result.Status); stale > 0 {
		m.notice = "index refreshed; still stale: " + indexSessionCount(stale)
		return
	}
	m.notice = "index fresh: " + indexSessionCount(msg.result.Status.FreshSessions)
}

func (m *Model) applyTranscriptSearch(msg transcriptSearchMsg) {
	if msg.seq != m.searchSeq || msg.query != strings.TrimSpace(m.query) {
		return
	}
	m.searchPending = false
	if msg.err != nil {
		m.notice = "transcript search failed: " + msg.err.Error()
		return
	}
	hits := make(map[string][]indexer.SearchResult)
	for _, result := range msg.results {
		hits[result.SessionID] = append(hits[result.SessionID], result)
	}
	m.transcriptHits = hits
	selectedID := ""
	if selected, ok := m.selectedRow(); ok {
		selectedID = selected.ID
	}
	m.filtered = m.filteredSessions()
	m.rows = m.buildRows()
	if selectedID != "" {
		for i, row := range m.rows {
			if row.ID == selectedID {
				m.cursor = i
				m.clamp()
				return
			}
		}
	}
	m.cursor = m.firstSessionCursor()
	m.clamp()
}

func (m *Model) applyPreviewLoaded(msg previewLoadedMsg) {
	delete(m.previewPending, msg.sessionID)
	if msg.err != nil {
		m.notice = "preview failed: " + msg.err.Error()
		return
	}
	m.previewCache[msg.sessionID] = msg.preview
	m.previewScroll = min(m.previewScroll, m.maxPreviewScroll())
}

func (m Model) hitSnippet(sessionID string) string {
	hits := m.transcriptHits[sessionID]
	if len(hits) == 0 {
		return ""
	}
	return hits[0].Role + ": " + hits[0].Snippet
}

func indexStaleCount(status indexer.Status) int {
	return status.StaleSessions + status.UncachedSessions
}

func indexSessionCount(count int) string {
	if count == 1 {
		return "1 session"
	}
	return strconv.Itoa(count) + " sessions"
}
