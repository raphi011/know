package pick

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/models"
	"github.com/sahilm/fuzzy"
)

var testFiles = []models.FileEntry{
	{Path: "/docs/readme.md", Name: "readme.md", Title: "Welcome"},
	{Path: "/docs/api.md", Name: "api.md", Title: "API Reference"},
	{Path: "/notes/todo.md", Name: "todo.md"},
	{Path: "/notes/ideas.txt", Name: "ideas.txt", Title: "Ideas"},
}

func TestFileSource_String(t *testing.T) {
	src := FileSource(testFiles)

	tests := []struct {
		name string
		idx  int
		want string
	}{
		{"with title", 0, "/docs/readme.md Welcome"},
		{"without title", 2, "/notes/todo.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := src.String(tt.idx)
			if got != tt.want {
				t.Errorf("String(%d) = %q, want %q", tt.idx, got, tt.want)
			}
		})
	}
}

func TestFileSource_Len(t *testing.T) {
	src := FileSource(testFiles)
	if got := src.Len(); got != 4 {
		t.Errorf("Len() = %d, want 4", got)
	}
}

func TestRefilter_EmptyQuery(t *testing.T) {
	m := NewModel(testFiles)
	m.Input.SetValue("")
	m.Refilter()

	if got := len(m.Matches); got != len(testFiles) {
		t.Errorf("empty query: got %d matches, want %d", got, len(testFiles))
	}

	// Matches should be in original order.
	for i, match := range m.Matches {
		if match.Index != i {
			t.Errorf("match[%d].Index = %d, want %d", i, match.Index, i)
		}
	}
}

func TestRefilter_WithQuery(t *testing.T) {
	m := NewModel(testFiles)
	m.Input.SetValue("api")
	m.Refilter()

	if len(m.Matches) == 0 {
		t.Fatal("expected matches for 'api'")
	}

	// First match should be the API doc.
	idx := m.Matches[0].Index
	if testFiles[idx].Path != "/docs/api.md" {
		t.Errorf("first match = %q, want /docs/api.md", testFiles[idx].Path)
	}
}

func TestRefilter_CursorClamp(t *testing.T) {
	m := NewModel(testFiles)
	m.Cursor = 3 // last file

	m.Input.SetValue("readme")
	m.Refilter()

	if m.Cursor >= len(m.Matches) {
		t.Errorf("cursor %d out of range (matches=%d)", m.Cursor, len(m.Matches))
	}
}

func TestUpdate_Enter_SelectsFile(t *testing.T) {
	m := NewModel(testFiles)

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model := result.(Model)

	if model.Selected != "/docs/readme.md" {
		t.Errorf("Selected = %q, want /docs/readme.md", model.Selected)
	}
	if cmd == nil {
		t.Error("expected quit cmd on Enter")
	}
}

func TestUpdate_Enter_NoMatches(t *testing.T) {
	m := NewModel(nil)

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model := result.(Model)

	if model.Selected != "" {
		t.Errorf("Selected = %q, want empty", model.Selected)
	}
	if cmd == nil {
		t.Error("expected quit cmd on Enter")
	}
}

func TestUpdate_Escape_Cancels(t *testing.T) {
	m := NewModel(testFiles)

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model := result.(Model)

	if model.Selected != "" {
		t.Errorf("Selected = %q, want empty on Escape", model.Selected)
	}
	if cmd == nil {
		t.Error("expected quit cmd on Escape")
	}
}

func TestUpdate_DownUp_MovesCursor(t *testing.T) {
	m := NewModel(testFiles)

	// Move down.
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = result.(Model)
	if m.Cursor != 1 {
		t.Errorf("after down: cursor = %d, want 1", m.Cursor)
	}

	// Move down again.
	result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = result.(Model)
	if m.Cursor != 2 {
		t.Errorf("after second down: cursor = %d, want 2", m.Cursor)
	}

	// Move up.
	result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = result.(Model)
	if m.Cursor != 1 {
		t.Errorf("after up: cursor = %d, want 1", m.Cursor)
	}
}

func TestUpdate_Down_ClampsAtEnd(t *testing.T) {
	m := NewModel(testFiles)
	m.Cursor = len(testFiles) - 1

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = result.(Model)
	if m.Cursor != len(testFiles)-1 {
		t.Errorf("cursor moved past end: %d", m.Cursor)
	}
}

func TestUpdate_Up_ClampsAtStart(t *testing.T) {
	m := NewModel(testFiles)
	m.Cursor = 0

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = result.(Model)
	if m.Cursor != 0 {
		t.Errorf("cursor moved before start: %d", m.Cursor)
	}
}

func TestEnsureCursorVisible(t *testing.T) {
	m := NewModel(testFiles)
	m.Height = 6 // visibleRows = max(6-4, 1) = 2

	// Cursor at 3 (past visible window).
	m.Cursor = 3
	m.Offset = 0
	m.EnsureCursorVisible()

	if m.Offset == 0 {
		t.Error("offset should have scrolled to keep cursor visible")
	}
	if m.Cursor < m.Offset || m.Cursor >= m.Offset+m.VisibleRows() {
		t.Errorf("cursor %d not visible (offset=%d, visible=%d)", m.Cursor, m.Offset, m.VisibleRows())
	}
}

func TestRenderHighlighted_NoMatches(t *testing.T) {
	result := RenderHighlighted("hello", nil, NormalStyle)
	// With no matches, the entire string is rendered with the base style.
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in output, got %q", result)
	}
}

func TestRenderHighlighted_WithMatches(t *testing.T) {
	result := RenderHighlighted("hello", []int{1, 3}, NormalStyle)
	// Each character should be present.
	for _, ch := range "hello" {
		if !strings.Contains(result, string(ch)) {
			t.Errorf("missing character %q in output %q", string(ch), result)
		}
	}
}

func TestVisibleRows(t *testing.T) {
	m := Model{Height: 10}
	if got := m.VisibleRows(); got != 6 {
		t.Errorf("VisibleRows() = %d, want 6", got)
	}
}

func TestVisibleRows_Minimum(t *testing.T) {
	m := Model{Height: 0}
	if got := m.VisibleRows(); got != 1 {
		t.Errorf("VisibleRows() = %d, want 1 (minimum)", got)
	}
}

func TestUpdate_WindowResize(t *testing.T) {
	m := NewModel(testFiles)

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)

	if m.Width != 120 || m.Height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m.Width, m.Height)
	}
}

func TestRefilter_NoFiles(t *testing.T) {
	m := NewModel(nil)

	if len(m.Matches) != 0 {
		t.Errorf("expected 0 matches for nil files, got %d", len(m.Matches))
	}
	if m.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.Cursor)
	}
}

func TestNewModel_InputFocused(t *testing.T) {
	m := NewModel(testFiles)
	if !m.Input.Focused() {
		t.Error("expected input to be focused after NewModel")
	}
}

func TestSetSize_PropagatesWidthToInput(t *testing.T) {
	m := NewModel(testFiles)
	m.SetSize(120, 40)

	if m.Width != 120 || m.Height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m.Width, m.Height)
	}

	wantInputWidth := 120 - len(m.Input.Prompt)
	if got := m.Input.Width(); got != wantInputWidth {
		t.Errorf("Input.Width() = %d, want %d", got, wantInputWidth)
	}
}

func TestUpdate_PageDown_MovesFullPage(t *testing.T) {
	m := NewModel(testFiles) // 4 files
	m.Height = 6             // visibleRows = max(6-4, 1) = 2

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = result.(Model)

	if m.Cursor != 2 {
		t.Errorf("after pgdown: cursor = %d, want 2", m.Cursor)
	}
}

func TestUpdate_PageDown_ClampsAtEnd(t *testing.T) {
	m := NewModel(testFiles) // 4 files
	m.Cursor = 3

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = result.(Model)

	if m.Cursor != 3 {
		t.Errorf("pgdown at end: cursor = %d, want 3", m.Cursor)
	}
}

func TestUpdate_PageUp_MovesFullPage(t *testing.T) {
	m := NewModel(testFiles) // 4 files
	m.Height = 6             // visibleRows = 2
	m.Cursor = 3

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = result.(Model)

	if m.Cursor != 1 {
		t.Errorf("after pgup: cursor = %d, want 1", m.Cursor)
	}
}

func TestUpdate_PageUp_ClampsAtStart(t *testing.T) {
	m := NewModel(testFiles)
	m.Cursor = 0

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = result.(Model)

	if m.Cursor != 0 {
		t.Errorf("pgup at start: cursor = %d, want 0", m.Cursor)
	}
}

func TestUpdate_Enter_WithCursorAtSecondItem(t *testing.T) {
	m := NewModel(testFiles)
	m.Cursor = 1
	// Ensure matches are set (they are from NewModel).
	m.Matches = []fuzzy.Match{
		{Index: 0},
		{Index: 1},
		{Index: 2},
		{Index: 3},
	}

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model := result.(Model)

	if model.Selected != "/docs/api.md" {
		t.Errorf("Selected = %q, want /docs/api.md", model.Selected)
	}
}
