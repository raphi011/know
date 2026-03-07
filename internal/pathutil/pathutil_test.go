package pathutil

import "testing"

func TestNormalizeFolderPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"guides", "/guides/"},
		{"/guides", "/guides/"},
		{"guides/", "/guides/"},
		{"/guides/", "/guides/"},
		{"/", "/"},
	}
	for _, tt := range tests {
		if got := NormalizeFolderPath(tt.input); got != tt.want {
			t.Errorf("NormalizeFolderPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsImmediateChild(t *testing.T) {
	tests := []struct {
		folder string
		child  string
		want   bool
	}{
		{"/guides/", "/guides/intro.md", true},
		{"/guides/", "/guides/advanced/deep.md", false},
		{"/guides/", "/other/intro.md", false},
		{"/guides/", "/guides/", false},
		{"/", "/doc.md", true},
		{"/", "/sub/doc.md", false},
	}
	for _, tt := range tests {
		if got := IsImmediateChild(tt.folder, tt.child); got != tt.want {
			t.Errorf("IsImmediateChild(%q, %q) = %v, want %v", tt.folder, tt.child, got, tt.want)
		}
	}
}

func TestIsImmediateChildFolder(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"/guides/", "/guides/advanced/", true},
		{"/guides/", "/guides/advanced/deep/", false},
		{"/guides/", "/other/stuff/", false},
		{"/guides/", "/guides/", false},
		{"/", "/guides/", true},
		{"/", "/guides/sub/", false},
	}
	for _, tt := range tests {
		if got := IsImmediateChildFolder(tt.parent, tt.child); got != tt.want {
			t.Errorf("IsImmediateChildFolder(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}
