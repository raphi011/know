package render

import (
	"strings"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestFindFencedBlockEnd(t *testing.T) {
	tests := []struct {
		name    string
		content string
		offset  int
		want    int
	}{
		{
			name:    "simple block",
			content: "before\n```know\nFROM /daily\n```\nafter",
			offset:  7,
			want:    31, // past "```\n"
		},
		{
			name:    "block at start",
			content: "```know\nFROM /daily\n```\n",
			offset:  0,
			want:    24, // past "```\n" (includes trailing newline)
		},
		{
			name:    "no closing fence",
			content: "```know\nFROM /daily\n",
			offset:  0,
			want:    -1,
		},
		{
			name:    "indented closing fence",
			content: "```know\nFROM /daily\n  ```  \nafter",
			offset:  0,
			want:    28, // past "  ```  \n"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findFencedBlockEnd(tt.content, tt.offset)
			if got != tt.want {
				t.Errorf("findFencedBlockEnd() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWikiLinkPattern(t *testing.T) {
	tests := []struct {
		input string
		want  []string // expected full matches
	}{
		{"See [[Target Page]]", []string{"[[Target Page]]"}},
		{"[[A]] and [[B]]", []string{"[[A]]", "[[B]]"}},
		{"[[Target|alias]]", []string{"[[Target|alias]]"}},
		{"no links here", nil},
		{"[[]]", nil}, // empty target
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := wikiLinkPattern.FindAllString(tt.input, -1)
			if len(matches) != len(tt.want) {
				t.Errorf("got %d matches, want %d: %v", len(matches), len(tt.want), matches)
				return
			}
			for i, m := range matches {
				if m != tt.want[i] {
					t.Errorf("match[%d] = %q, want %q", i, m, tt.want[i])
				}
			}
		})
	}
}

func TestRenderList(t *testing.T) {
	files := []struct {
		title string
		path  string
	}{
		{"My Note", "/notes/my-note.md"},
		{"Other", "/notes/other.md"},
	}

	// Construct models.File slice
	var modelFiles []models.File
	for _, f := range files {
		modelFiles = append(modelFiles, models.File{Title: f.title, Path: f.path})
	}

	result := renderList(modelFiles, []string{"title", "path"})
	if !strings.Contains(result, "[My Note](/notes/my-note.md)") {
		t.Errorf("expected link to My Note, got: %s", result)
	}
	if !strings.Contains(result, "[Other](/notes/other.md)") {
		t.Errorf("expected link to Other, got: %s", result)
	}
}

func TestRenderTable(t *testing.T) {
	modelFiles := []models.File{
		{Title: "A", Path: "/a.md", MimeType: "text/markdown"},
	}

	result := renderTable(modelFiles, []string{"title", "path", "mime_type"})
	if !strings.Contains(result, "| title |") {
		t.Errorf("expected header row, got: %s", result)
	}
	if !strings.Contains(result, "| A |") {
		t.Errorf("expected data row, got: %s", result)
	}
}
