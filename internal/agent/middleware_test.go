package agent

import (
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
