package document

import (
	"testing"
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
