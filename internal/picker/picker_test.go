package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

	if next.cursor != 1 {
		t.Fatalf("expected wheel down to move one row, got cursor %d", next.cursor)
	}
}

func TestForkedSessionRowShowsMarker(t *testing.T) {
	model := New([]sessions.Session{{ID: "fork", Title: "Production billing audit", ParentID: "parent"}})
	row := model.renderRow(model.all[0], 80, false)

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
	model.cursor = 2
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
