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
	model := New([]sessions.Session{{ID: "fork", Title: "GCP inpersona-staging pwned", ParentID: "parent"}})
	row := model.renderRow(model.all[0], 80, false)

	if len(row) != 1 {
		t.Fatalf("expected compact row, got %#v", row)
	}
	if !strings.Contains(row[0], "↳ GCP inpersona-staging pwned") {
		t.Fatalf("expected fork marker in row, got %q", row[0])
	}
}
