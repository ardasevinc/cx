package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ardasevinc/cx/internal/indexer"
	"github.com/ardasevinc/cx/internal/sessions"
)

func TestRunesSearchInsteadOfMoving(t *testing.T) {
	model := New([]sessions.Session{
		{ID: "one", Title: "journal", SearchText: "journal"},
		{ID: "two", Title: "notes", SearchText: "notes"},
	})
	model.width = 80
	model.height = 20

	updated, _ := model.updateKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next := updated.(Model)

	if next.query != "j" {
		t.Fatalf("expected j to enter search query, got %q", next.query)
	}
	if next.cursor != 0 {
		t.Fatalf("expected cursor to remain on first search result, got %d", next.cursor)
	}
	if len(next.filtered) != 1 || next.filtered[0].ID != "one" {
		t.Fatalf("unexpected filtered sessions: %#v", next.filtered)
	}
}

func TestMouseWheelMovesSelection(t *testing.T) {
	model := New([]sessions.Session{
		{ID: "one", Title: "one"},
		{ID: "two", Title: "two"},
		{ID: "three", Title: "three"},
		{ID: "four", Title: "four"},
	})
	model.width = 80
	model.height = 20

	updated, _ := model.updateMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	next := updated.(Model)

	if next.cursor != 2 {
		t.Fatalf("expected wheel down to move one row, got cursor %d", next.cursor)
	}
}

func TestNewChatRowRendersAboveSessions(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20

	view := model.View()

	if !strings.Contains(view, "+ new chat") || !strings.Contains(view, "today · Documents/Codex") {
		t.Fatalf("expected pinned new chat row, got %q", view)
	}
	if model.cursor != 1 {
		t.Fatalf("expected default cursor on first real session, got %d", model.cursor)
	}
}

func TestEnterOnNewChatRowReturnsNewChatAction(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20
	model.cursor = 0

	updated, _ := model.updateKeys(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(Model)

	if next.result.Action != ActionNew || !next.result.Chat {
		t.Fatalf("expected new chat action, got %#v", next.result)
	}
}

func TestProjectViewStartsNewThreadInProject(t *testing.T) {
	model := New([]sessions.Session{
		{ID: "chat", Title: "chat", Project: "chats", CWD: "/home/alice/Documents/Codex/2026-05-20/chat-001"},
		{ID: "project", Title: "project", Project: "cx", CWD: "/home/alice/src/cx"},
	})
	model.width = 80
	model.height = 20

	updated, _ := model.executeCommand("view projects")
	next := updated.(Model)
	if next.view != viewProjects {
		t.Fatalf("expected projects view, got %q", next.view)
	}
	if !strings.Contains(next.View(), "chats") || !strings.Contains(next.View(), "cx") {
		t.Fatalf("expected projects and chats rows, got %q", next.View())
	}

	for i, row := range next.rows {
		if row.Kind == rowProject && row.Title == "cx" {
			next.cursor = i
			break
		}
	}
	updated, _ = next.updateKeys(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(Model).result

	if result.Action != ActionNew || result.Dir != "/home/alice/src/cx" {
		t.Fatalf("expected project new action, got %#v", result)
	}
}

func TestGroupedViewTogglesProjectGroup(t *testing.T) {
	model := New([]sessions.Session{
		{ID: "one", Title: "one", Project: "cx", CWD: "/home/alice/src/cx"},
		{ID: "two", Title: "two", Project: "cx", CWD: "/home/alice/src/cx"},
	})
	model.width = 80
	model.height = 20

	updated, _ := model.executeCommand("group projects")
	next := updated.(Model)
	if !strings.Contains(next.View(), "▾ cx") || !strings.Contains(next.View(), "one") {
		t.Fatalf("expected expanded project group, got %q", next.View())
	}

	next.cursor = 1
	updated, _ = next.updateKeys(tea.KeyMsg{Type: tea.KeyEnter})
	next = updated.(Model)

	if !strings.Contains(next.View(), "▸ cx") || strings.Contains(next.View(), "one") {
		t.Fatalf("expected collapsed project group, got %q", next.View())
	}
}

func TestGroupedViewLeftRightCloseAndOpenProjectGroup(t *testing.T) {
	model := New([]sessions.Session{
		{ID: "one", Title: "one", Project: "cx", CWD: "/home/alice/src/cx"},
	})
	model.width = 80
	model.height = 20

	updated, _ := model.executeCommand("group projects")
	next := updated.(Model)
	next.cursor = 1

	updated, _ = next.updateKeys(tea.KeyMsg{Type: tea.KeyLeft})
	next = updated.(Model)
	if !strings.Contains(next.View(), "▸ cx") || strings.Contains(next.View(), "one") {
		t.Fatalf("expected left arrow to close group, got %q", next.View())
	}

	updated, _ = next.updateKeys(tea.KeyMsg{Type: tea.KeyRight})
	next = updated.(Model)
	if !strings.Contains(next.View(), "▾ cx") || !strings.Contains(next.View(), "one") {
		t.Fatalf("expected right arrow to open group, got %q", next.View())
	}
}

func TestCommandModeAcceptsSpaces(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20

	var updated tea.Model = model
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("v")},
		{Type: tea.KeyRunes, Runes: []rune("i")},
		{Type: tea.KeyRunes, Runes: []rune("e")},
		{Type: tea.KeyRunes, Runes: []rune("w")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("a")},
		{Type: tea.KeyRunes, Runes: []rune("l")},
		{Type: tea.KeyRunes, Runes: []rune("l")},
	} {
		updated, _ = updated.(Model).updateCommand(msg)
	}
	next := updated.(Model)

	if next.cmdText != "view all" {
		t.Fatalf("expected command text with space, got %q", next.cmdText)
	}
}

func TestCommandFooterStaysAtBottomInComfyMode(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20
	model.comfy = true
	model.command = true
	model.cmdText = "view all"

	lines := strings.Split(model.View(), "\n")

	if len(lines) < 19 {
		t.Fatalf("expected footer near terminal bottom, got %d lines: %q", len(lines), model.View())
	}
	if !strings.Contains(lines[len(lines)-1], ":view all") {
		t.Fatalf("expected command footer on last line, got %q", lines[len(lines)-1])
	}
}

func TestForkedSessionRowShowsMarker(t *testing.T) {
	model := New([]sessions.Session{{ID: "fork", Title: "Production billing audit", ParentID: "parent"}})
	row := model.renderRow(model.sessionRow(model.all[0], 0), 80, false)

	if len(row) != 1 {
		t.Fatalf("expected compact row, got %#v", row)
	}
	if !strings.Contains(row[0], "Production billing audit") || !strings.Contains(row[0], "⎇ unknown") {
		t.Fatalf("expected fork marker in row, got %q", row[0])
	}
}

func TestNarrowPreviewRendersFloatingPanel(t *testing.T) {
	model := New([]sessions.Session{
		{
			ID:         "one",
			Title:      "Production billing audit",
			Project:    "billing-api",
			SearchText: "billing",
		},
	})
	model.width = 80
	model.height = 20
	model.preview = true
	model.previewInitialized = true

	view := model.View()

	if !strings.Contains(view, "Production billing audit") {
		t.Fatalf("expected narrow preview popup to render selected title, got %q", view)
	}
	if !strings.Contains(view, "no transcript preview") {
		t.Fatalf("expected narrow preview popup content, got %q", view)
	}
}

func TestFloatingPreviewDoesNotCoverSelectedRow(t *testing.T) {
	items := []sessions.Session{
		{ID: "one", Title: "one"},
		{ID: "two", Title: "two"},
		{ID: "three", Title: "three"},
		{ID: "four", Title: "four"},
		{ID: "five", Title: "five"},
	}
	model := New(items)
	model.width = 80
	model.height = 12
	model.preview = true
	model.previewInitialized = true
	model.cursor = 3
	model.clamp()

	view := model.View()

	if !strings.Contains(view, "▌ three") {
		t.Fatalf("expected selected row to remain visible, got %q", view)
	}
}

func TestFloatingPanelTopAvoidsSelectedRow(t *testing.T) {
	tests := []struct {
		name        string
		baseHeight  int
		panelHeight int
		avoidRow    int
		want        int
	}{
		{name: "below selected", baseHeight: 10, panelHeight: 3, avoidRow: 2, want: 3},
		{name: "above selected", baseHeight: 10, panelHeight: 3, avoidRow: 8, want: 5},
		{name: "fallback bottom", baseHeight: 8, panelHeight: 6, avoidRow: 3, want: 2},
	}

	for _, tt := range tests {
		if got := floatingPanelTop(tt.baseHeight, tt.panelHeight, tt.avoidRow); got != tt.want {
			t.Fatalf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestDetailShortcutOpensHiddenPreview(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20
	model.preview = false

	updated, _ := model.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlE})
	next := updated.(Model)

	if !next.preview || !next.detail {
		t.Fatalf("expected ctrl+e to open detail popup, preview=%v detail=%v", next.preview, next.detail)
	}
}

func TestDetailShortcutRestoresHiddenPreview(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20
	model.preview = false

	updated, _ := model.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlE})
	updated, _ = updated.(Model).updateKeys(tea.KeyMsg{Type: tea.KeyCtrlE})
	next := updated.(Model)

	if next.preview || next.detail {
		t.Fatalf("expected ctrl+e to restore hidden preview, preview=%v detail=%v", next.preview, next.detail)
	}
}

func TestPreviewToggleClosesDetail(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20
	model.detail = true
	model.preview = true
	model.previewInitialized = true

	updated, _ := model.updateKeys(tea.KeyMsg{Type: tea.KeyTab})
	next := updated.(Model)

	if next.preview || next.detail {
		t.Fatalf("expected tab to close preview and detail, preview=%v detail=%v", next.preview, next.detail)
	}
}

func TestNarrowLaunchDefaultsToListOnly(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 80
	model.height = 20

	view := model.View()

	if strings.Contains(view, "no transcript preview") {
		t.Fatalf("expected narrow launch to hide preview popup by default, got %q", view)
	}
}

func TestWideLaunchDefaultsToPreview(t *testing.T) {
	model := New([]sessions.Session{{ID: "one", Title: "one"}})
	model.width = 120
	model.height = 20

	view := model.View()

	if !strings.Contains(view, "no transcript preview") {
		t.Fatalf("expected wide launch to show preview by default, got %q", view)
	}
}

func TestTranscriptSearchMessageAddsTranscriptOnlyRows(t *testing.T) {
	model := NewWithIndex([]sessions.Session{
		{ID: "one", Title: "metadata", SearchText: "metadata"},
		{ID: "two", Title: "other", SearchText: "other"},
	}, indexer.Options{})
	model.query = "needle"
	model.searchSeq = 2
	model.refreshRows()

	model.applyTranscriptSearch(transcriptSearchMsg{
		seq:   2,
		query: "needle",
		results: []indexer.SearchResult{{
			SessionID: "two",
			Role:      "user",
			Snippet:   "needle only in transcript",
		}},
	})

	if len(model.filtered) != 1 || model.filtered[0].ID != "two" {
		t.Fatalf("expected transcript-only match, got %#v", model.filtered)
	}
	if snippet := model.hitSnippet("two"); !strings.Contains(snippet, "needle only") {
		t.Fatalf("expected hit snippet, got %q", snippet)
	}
}

func TestNormalizePreviewTextUnwrapsMarkdownTableFence(t *testing.T) {
	text := "```markdown\n| A | B |\n| --- | --- |\n| 1 | 2 |\n```"

	got := normalizePreviewText(text)

	if strings.Contains(got, "```") || !strings.Contains(got, "| A | B |") {
		t.Fatalf("unexpected normalized table: %q", got)
	}
}
