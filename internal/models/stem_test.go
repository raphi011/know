package models

import "testing"

func TestFilenameStem(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/notes/vector-embeddings.md", "vector-embeddings"},
		{"/daily/2026-03-16.md", "2026-03-16"},
		{"/README.md", "readme"},
		{"/notes/My Notes.txt", "my notes"},
		{"/a/b/c.md", "c"},
		{"/folder/", ""},
		{"file.md", "file"},
	}
	for _, tt := range tests {
		got := FilenameStem(tt.input)
		if got != tt.want {
			t.Errorf("FilenameStem(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
