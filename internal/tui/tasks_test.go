package tui

import (
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestShortDocPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/02 Notes/Programming/Next.js.md", "Programming/Next.js.md"},
		{"/toplevel.md", "toplevel.md"},
		{"", ""},
		{"/a/b/c.md", "b/c.md"},
		{"/single/file.md", "single/file.md"},
	}

	for _, tt := range tests {
		got := shortDocPath(tt.input)
		if got != tt.want {
			t.Errorf("shortDocPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsOverdue(t *testing.T) {
	tests := []struct {
		date string
		want bool
	}{
		{"2020-01-01", true},  // past
		{"2099-12-31", false}, // future
		{"invalid", false},    // parse error
		{"", false},           // empty
	}

	for _, tt := range tests {
		got := isOverdue(tt.date)
		if got != tt.want {
			t.Errorf("isOverdue(%q) = %v, want %v", tt.date, got, tt.want)
		}
	}
}

func TestHighlightRunes(t *testing.T) {
	normal := lipgloss.NewStyle()
	highlight := lipgloss.NewStyle().Bold(true)

	t.Run("no matches returns full text styled", func(t *testing.T) {
		got := highlightRunes("hello", nil, normal, highlight)
		if got == "" {
			t.Error("expected non-empty output")
		}
	})

	t.Run("with matches highlights specific positions", func(t *testing.T) {
		// Match positions 0 and 2 in "hello" → 'h' and 'l'
		got := highlightRunes("hello", []int{0, 2}, normal, highlight)
		if got == "" {
			t.Error("expected non-empty output")
		}
		// The output contains styled characters — we can't easily assert on exact
		// ANSI codes, but the function shouldn't panic and should produce output
		// that's different from the no-match case.
		noMatch := highlightRunes("hello", nil, normal, highlight)
		if got == noMatch {
			t.Error("highlighted output should differ from non-highlighted")
		}
	})
}
