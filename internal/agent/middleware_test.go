package agent

import (
	"strings"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestFormatTemplates(t *testing.T) {
	tests := []struct {
		name string
		docs []models.File
		want string
	}{
		{
			name: "empty list",
			docs: nil,
			want: "",
		},
		{
			name: "single doc with title",
			docs: []models.File{{Path: "/templates/meeting.md", Title: "Meeting Notes"}},
			want: "\n\nAvailable templates (read with read_document to use):\n- /templates/meeting.md — Meeting Notes\n",
		},
		{
			name: "single doc without title",
			docs: []models.File{{Path: "/templates/scratch.md"}},
			want: "\n\nAvailable templates (read with read_document to use):\n- /templates/scratch.md\n",
		},
		{
			name: "curly braces in path and title are escaped",
			docs: []models.File{{Path: "/templates/{daily}.md", Title: "Daily {note}"}},
			want: "\n\nAvailable templates (read with read_document to use):\n- /templates/{{daily}}.md — Daily {{note}}\n",
		},
		{
			name: "multiple docs",
			docs: []models.File{
				{Path: "/templates/a.md", Title: "Alpha"},
				{Path: "/templates/b.md"},
			},
			want: "\n\nAvailable templates (read with read_document to use):\n- /templates/a.md — Alpha\n- /templates/b.md\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTemplates(tt.docs)
			if got != tt.want {
				t.Errorf("formatTemplates() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatVaultInstructions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "normal content",
			in:   "Always respond in German",
			want: "\n\n## Vault Instructions (/VAULT.md)\nYou can update this file using edit_document when the user asks you to remember preferences or conventions.\n\n<vault-instructions>\nAlways respond in German\n</vault-instructions>",
		},
		{
			name: "curly braces escaped",
			in:   "Use {name} template",
			want: "\n\n## Vault Instructions (/VAULT.md)\nYou can update this file using edit_document when the user asks you to remember preferences or conventions.\n\n<vault-instructions>\nUse {{name}} template\n</vault-instructions>",
		},
		{
			name: "XML-like tags passed through",
			in:   "<foo>bar</foo>",
			want: "\n\n## Vault Instructions (/VAULT.md)\nYou can update this file using edit_document when the user asks you to remember preferences or conventions.\n\n<vault-instructions>\n<foo>bar</foo>\n</vault-instructions>",
		},
		{
			name: "exact boundary not truncated",
			in:   strings.Repeat("x", maxVaultInstructionsBytes),
			want: "\n\n## Vault Instructions (/VAULT.md)\nYou can update this file using edit_document when the user asks you to remember preferences or conventions.\n\n<vault-instructions>\n" + strings.Repeat("x", maxVaultInstructionsBytes) + "\n</vault-instructions>",
		},
		{
			name: "oversized content truncated",
			in:   strings.Repeat("x", maxVaultInstructionsBytes+100),
			want: "\n\n## Vault Instructions (/VAULT.md)\nYou can update this file using edit_document when the user asks you to remember preferences or conventions.\n\n<vault-instructions>\n" + strings.Repeat("x", maxVaultInstructionsBytes) + "\n\n(truncated — VAULT.md exceeds 32 KB limit)\n</vault-instructions>",
		},
		{
			name: "multi-byte char at boundary truncated cleanly",
			// 'ä' is 2 bytes in UTF-8; place it so second byte falls past the limit
			in:   strings.Repeat("a", maxVaultInstructionsBytes-1) + "ä",
			want: "\n\n## Vault Instructions (/VAULT.md)\nYou can update this file using edit_document when the user asks you to remember preferences or conventions.\n\n<vault-instructions>\n" + strings.Repeat("a", maxVaultInstructionsBytes-1) + "\n\n(truncated — VAULT.md exceeds 32 KB limit)\n</vault-instructions>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatVaultInstructions(tt.in)
			if got != tt.want {
				t.Errorf("formatVaultInstructions() = %q, want %q", got, tt.want)
			}
		})
	}
}
