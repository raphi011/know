package document

import (
	"testing"
	"time"
)

func TestApplyTemplateVars(t *testing.T) {
	tests := []struct {
		name    string
		content string
		vars    map[string]string
		want    string
	}{
		{
			name:    "all built-in vars",
			content: "# {{date}}\nCreated: {{datetime}}\nTitle: {{title}}\nVault: {{vault}}",
			vars: map[string]string{
				"date":     "2026-03-14",
				"datetime": "2026-03-14 10:30",
				"title":    "My Note",
				"vault":    "default",
			},
			want: "# 2026-03-14\nCreated: 2026-03-14 10:30\nTitle: My Note\nVault: default",
		},
		{
			name:    "no vars in content",
			content: "# Just plain markdown",
			vars:    map[string]string{"date": "2026-03-14"},
			want:    "# Just plain markdown",
		},
		{
			name:    "empty vars",
			content: "# {{date}}",
			vars:    map[string]string{},
			want:    "# {{date}}",
		},
		{
			name:    "missing var left as-is",
			content: "# {{date}} {{unknown}}",
			vars:    map[string]string{"date": "2026-03-14"},
			want:    "# 2026-03-14 {{unknown}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyTemplateVars(tt.content, tt.vars)
			if got != tt.want {
				t.Errorf("ApplyTemplateVars() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultTemplateVars(t *testing.T) {
	ts := time.Date(2026, 3, 14, 10, 30, 0, 0, time.UTC)
	vars := DefaultTemplateVars(ts, "My Title", "default")

	if vars["date"] != "2026-03-14" {
		t.Errorf("date = %q, want %q", vars["date"], "2026-03-14")
	}
	if vars["datetime"] != "2026-03-14 10:30" {
		t.Errorf("datetime = %q, want %q", vars["datetime"], "2026-03-14 10:30")
	}
	if vars["title"] != "My Title" {
		t.Errorf("title = %q, want %q", vars["title"], "My Title")
	}
	if vars["vault"] != "default" {
		t.Errorf("vault = %q, want %q", vars["vault"], "default")
	}
}
