package file

import (
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestShortestUnambiguousTarget(t *testing.T) {
	tests := []struct {
		name            string
		filePath        string
		allWithSameStem []models.File
		want            string
	}{
		{
			name:     "unique stem returns stem only",
			filePath: "/notes/vector-embeddings.md",
			allWithSameStem: []models.File{
				{Path: "/notes/vector-embeddings.md"},
			},
			want: "vector-embeddings",
		},
		{
			name:            "empty slice returns stem",
			filePath:        "/docs/readme.md",
			allWithSameStem: nil,
			want:            "readme",
		},
		{
			name:     "two files same stem disambiguated by parent",
			filePath: "/a/notes.md",
			allWithSameStem: []models.File{
				{Path: "/a/notes.md"},
				{Path: "/b/notes.md"},
			},
			want: "a/notes",
		},
		{
			name:     "deeper disambiguation",
			filePath: "/x/a/notes.md",
			allWithSameStem: []models.File{
				{Path: "/x/a/notes.md"},
				{Path: "/y/a/notes.md"},
			},
			want: "x/a/notes",
		},
		{
			name:     "first depth sufficient",
			filePath: "/projects/alpha/readme.md",
			allWithSameStem: []models.File{
				{Path: "/projects/alpha/readme.md"},
				{Path: "/projects/beta/readme.md"},
			},
			want: "alpha/readme",
		},
		{
			name:     "full path fallback when all share same parent structure",
			filePath: "/a/b/notes.md",
			allWithSameStem: []models.File{
				{Path: "/a/b/notes.md"},
				{Path: "/a/b/notes.md"}, // duplicate path
			},
			want: "a/b/notes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortestUnambiguousTarget(tt.filePath, tt.allWithSameStem)
			if got != tt.want {
				t.Errorf("ShortestUnambiguousTarget(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}
