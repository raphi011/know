package tools

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"Meeting Notes 2026-01-15", "meeting-notes-2026-01-15"},
		{"  spaces  around  ", "spaces-around"},
		{"Special!@#$%Characters", "special-characters"},
		{"already-a-slug", "already-a-slug"},
		{"UPPER CASE", "upper-case"},
		{"", "untitled"},
		{"---", "untitled"},
		{"café résumé", "caf-r-sum"},
		{"one", "one"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
