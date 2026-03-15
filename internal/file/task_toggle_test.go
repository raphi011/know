package file

import (
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestToggleLine(t *testing.T) {
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
			got := toggleLine(tt.input)
			if got != tt.expected {
				t.Errorf("toggleLine(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToggleLine_DoubleToggleRoundtrip(t *testing.T) {
	lines := []string{
		"- [ ] open task",
		"- [x] done task",
		"- [X] done uppercase",
		"  - [ ] nested open",
		"- [ ] with labels #work #urgent",
	}

	for _, line := range lines {
		toggled := toggleLine(line)
		restored := toggleLine(toggled)

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

func TestIsMatchingCheckbox(t *testing.T) {
	// Compute the content hash for "fix the bug" (same as CleanTaskText output).
	hash := models.ContentHash("fix the bug")

	tests := []struct {
		name        string
		line        string
		contentHash string
		expected    bool
	}{
		{
			name:        "open checkbox matches",
			line:        "- [ ] fix the bug",
			contentHash: hash,
			expected:    true,
		},
		{
			name:        "done checkbox matches",
			line:        "- [x] fix the bug",
			contentHash: hash,
			expected:    true,
		},
		{
			name:        "done uppercase matches",
			line:        "- [X] fix the bug",
			contentHash: hash,
			expected:    true,
		},
		{
			name:        "checkbox with labels matches (labels stripped by CleanTaskText)",
			line:        "- [ ] fix the bug #work #urgent",
			contentHash: hash,
			expected:    true,
		},
		{
			name:        "checkbox with due date matches",
			line:        "- [ ] fix the bug due:2026-03-20",
			contentHash: hash,
			expected:    true,
		},
		{
			name:        "different text does not match",
			line:        "- [ ] different task",
			contentHash: hash,
			expected:    false,
		},
		{
			name:        "non-checkbox line does not match",
			line:        "- regular list item",
			contentHash: hash,
			expected:    false,
		},
		{
			name:        "empty line does not match",
			line:        "",
			contentHash: hash,
			expected:    false,
		},
		{
			name:        "indented checkbox matches",
			line:        "  - [ ] fix the bug",
			contentHash: hash,
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMatchingCheckbox(tt.line, tt.contentHash)
			if got != tt.expected {
				t.Errorf("isMatchingCheckbox(%q, %q) = %v, want %v", tt.line, tt.contentHash, got, tt.expected)
			}
		})
	}
}
