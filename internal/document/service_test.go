package document

import (
	"testing"

	"github.com/raphi011/knowhow/internal/models"
)

func TestExtractInlineLabels(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		fmLabels   []string
		wantLabels []string
	}{
		{
			name:       "standalone tags",
			content:    "Some text #golang and #kubernetes here",
			fmLabels:   nil,
			wantLabels: []string{"golang", "kubernetes"},
		},
		{
			name:       "merge with frontmatter",
			content:    "Text with #newtag here",
			fmLabels:   []string{"existing"},
			wantLabels: []string{"existing", "newtag"},
		},
		{
			name:       "no duplicates",
			content:    "Has #golang twice #golang and #Golang",
			fmLabels:   []string{"golang"},
			wantLabels: []string{"golang"},
		},
		{
			name:       "start of line",
			content:    "#firsttag is at start",
			fmLabels:   nil,
			wantLabels: []string{"firsttag"},
		},
		{
			name:       "ignore markdown headings",
			content:    "## Not a tag\n#valid tag",
			fmLabels:   nil,
			wantLabels: []string{"valid"},
		},
		{
			name:       "no tags",
			content:    "No tags here",
			fmLabels:   nil,
			wantLabels: nil,
		},
		{
			name:       "ignore numeric-only tags",
			content:    "Item #123 is numeric",
			fmLabels:   nil,
			wantLabels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractInlineLabels(tt.content, tt.fmLabels)
			if len(got) != len(tt.wantLabels) {
				t.Errorf("ExtractInlineLabels() = %v, want %v", got, tt.wantLabels)
				return
			}
			for i, label := range got {
				if label != tt.wantLabels[i] {
					t.Errorf("label[%d] = %q, want %q", i, label, tt.wantLabels[i])
				}
			}
		})
	}
}

func TestBuildEmbeddingContext(t *testing.T) {
	hp := func(s string) *string { return &s }

	tests := []struct {
		name     string
		chunk    models.Chunk
		docTitle string
		want     string
	}{
		{
			name:     "full context",
			chunk:    models.Chunk{Content: "Install with pip.", HeadingPath: hp("## Setup > ### Install")},
			docTitle: "Getting Started",
			want:     "Document: Getting Started\nSection: Setup > Install\n\nInstall with pip.",
		},
		{
			name:     "no title",
			chunk:    models.Chunk{Content: "Some content.", HeadingPath: hp("## Section")},
			docTitle: "",
			want:     "Section: Section\n\nSome content.",
		},
		{
			name:     "no heading path",
			chunk:    models.Chunk{Content: "Preamble text."},
			docTitle: "My Doc",
			want:     "Document: My Doc\n\nPreamble text.",
		},
		{
			name:     "no context at all",
			chunk:    models.Chunk{Content: "Raw content."},
			docTitle: "",
			want:     "Raw content.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEmbeddingContext(tt.chunk, tt.docTitle)
			if got != tt.want {
				t.Errorf("buildEmbeddingContext() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestStripMarkdownHeadingPrefixes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"## Setup > ### Install", "Setup > Install"},
		{"# Title", "Title"},
		{"## Single", "Single"},
		{"# A > ## B > ### C", "A > B > C"},
	}
	for _, tt := range tests {
		got := stripMarkdownHeadingPrefixes(tt.input)
		if got != tt.want {
			t.Errorf("stripMarkdownHeadingPrefixes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFilenameTitle(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/docs/my-doc.md", "My Doc"},
		{"/notes/some_note.md", "Some Note"},
		{"/readme.md", "Readme"},
	}
	for _, tt := range tests {
		got := filenameTitle(tt.path)
		if got != tt.want {
			t.Errorf("filenameTitle(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
