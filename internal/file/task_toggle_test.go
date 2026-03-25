package file

import (
	"testing"

	"github.com/raphi011/know/internal/parser"
)

func TestFindToggleLine(t *testing.T) {
	tests := []struct {
		name         string
		tasks        []parser.ExtractedTask
		contentHash  string
		expectedLine int
		want         int
	}{
		{
			name: "exact match at expected line",
			tasks: []parser.ExtractedTask{
				{ContentHash: "abc", LineNumber: 5},
			},
			contentHash:  "abc",
			expectedLine: 5,
			want:         5,
		},
		{
			name: "single match at drifted line",
			tasks: []parser.ExtractedTask{
				{ContentHash: "abc", LineNumber: 8},
			},
			contentHash:  "abc",
			expectedLine: 5,
			want:         8,
		},
		{
			name: "multiple matches picks nearest",
			tasks: []parser.ExtractedTask{
				{ContentHash: "abc", LineNumber: 2},
				{ContentHash: "abc", LineNumber: 10},
				{ContentHash: "abc", LineNumber: 6},
			},
			contentHash:  "abc",
			expectedLine: 5,
			want:         6,
		},
		{
			name: "no matches returns -1",
			tasks: []parser.ExtractedTask{
				{ContentHash: "xyz", LineNumber: 5},
			},
			contentHash:  "abc",
			expectedLine: 5,
			want:         -1,
		},
		{
			name:         "empty tasks returns -1",
			tasks:        nil,
			contentHash:  "abc",
			expectedLine: 1,
			want:         -1,
		},
		{
			name: "equidistant picks first encountered",
			tasks: []parser.ExtractedTask{
				{ContentHash: "abc", LineNumber: 3},
				{ContentHash: "abc", LineNumber: 7},
			},
			contentHash:  "abc",
			expectedLine: 5,
			want:         3, // dist=2 for both, first wins
		},
		{
			name: "ignores non-matching hashes",
			tasks: []parser.ExtractedTask{
				{ContentHash: "other", LineNumber: 5},
				{ContentHash: "abc", LineNumber: 10},
			},
			contentHash:  "abc",
			expectedLine: 5,
			want:         10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findToggleLine(tt.tasks, tt.contentHash, tt.expectedLine)
			if got != tt.want {
				t.Errorf("findToggleLine() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFlipCheckbox(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "open to done",
			input:    "- [ ] fix the bug",
			expected: "- [x] fix the bug",
		},
		{
			name:     "done lowercase to open",
			input:    "- [x] fix the bug",
			expected: "- [ ] fix the bug",
		},
		{
			name:     "done uppercase to open",
			input:    "- [X] fix the bug",
			expected: "- [ ] fix the bug",
		},
		{
			name:     "preserves indentation",
			input:    "  - [ ] nested task",
			expected: "  - [x] nested task",
		},
		{
			name:     "preserves labels and due date",
			input:    "- [ ] fix bug #work due:2026-03-20",
			expected: "- [x] fix bug #work due:2026-03-20",
		},
		{
			name:     "non-checkbox line unchanged",
			input:    "- regular list item",
			expected: "- regular list item",
		},
		{
			name:     "empty line unchanged",
			input:    "",
			expected: "",
		},
		{
			name:     "heading unchanged",
			input:    "# Tasks",
			expected: "# Tasks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flipCheckbox(tt.input)
			if got != tt.expected {
				t.Errorf("flipCheckbox(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFlipCheckbox_DoubleToggleRoundtrip(t *testing.T) {
	lines := []string{
		"- [ ] open task",
		"- [x] done task",
		"- [X] done uppercase",
		"  - [ ] nested open",
		"- [ ] with labels #work #urgent",
	}

	for _, line := range lines {
		toggled := flipCheckbox(line)
		restored := flipCheckbox(toggled)

		// After double toggle, the text should match except [X] normalizes to [x].
		expected := line
		if line == "- [X] done uppercase" {
			expected = "- [x] done uppercase"
		}
		if restored != expected {
			t.Errorf("double toggle of %q produced %q, want %q", line, restored, expected)
		}
	}
}
