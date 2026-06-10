package picker

import (
	"fmt"
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

type indexRefreshMsg struct {
	result indexer.RebuildResult
	err    error
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
		preview, err := indexer.LoadPreview(m.indexOptions, sessionID, query, 16)
		return previewLoadedMsg{sessionID: sessionID, preview: preview, err: err}
	}
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
}

func (m *Model) applyIndexRefresh(msg indexRefreshMsg) {
	if msg.err != nil {
		m.notice = "index refresh failed: " + msg.err.Error()
		return
	}
	if msg.result.IndexedCount > 0 || msg.result.Status.TruncatedSessions > 0 {
		m.notice = fmt.Sprintf("index refreshed: %d indexed, %d truncated", msg.result.IndexedCount, msg.result.Status.TruncatedSessions)
	}
}

func (m Model) hitSnippet(sessionID string) string {
	hits := m.transcriptHits[sessionID]
	if len(hits) == 0 {
		return ""
	}
	return hits[0].Role + ": " + hits[0].Snippet
}
