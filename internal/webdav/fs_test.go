package webdav

import (
	"errors"
	"os"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{".", "/"},
		{"/", "/"},
		{"/docs/readme.md", "/docs/readme.md"},
		{"docs/readme.md", "/docs/readme.md"},
		{"/docs/../readme.md", "/readme.md"},
		{"/docs/./readme.md", "/docs/readme.md"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFileInfo(t *testing.T) {
	fi := &fileInfo{name: "test.md", size: 42, isDir: false}

	if fi.Name() != "test.md" {
		t.Errorf("Name() = %q, want %q", fi.Name(), "test.md")
	}
	if fi.Size() != 42 {
		t.Errorf("Size() = %d, want %d", fi.Size(), 42)
	}
	if fi.IsDir() {
		t.Error("IsDir() = true, want false")
	}
	if fi.Mode() != 0644 {
		t.Errorf("Mode() = %v, want 0644", fi.Mode())
	}

	dirFi := &fileInfo{name: "docs", isDir: true}
	if !dirFi.IsDir() {
		t.Error("IsDir() = false, want true")
	}
	if dirFi.Mode()&os.ModeDir == 0 {
		t.Error("Mode() should include ModeDir")
	}
}

func TestIsMarkdownFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"/notes/readme.md", true},
		{"/notes/README.MD", true},
		{"/notes/doc.Md", true},
		{"/notes/file.txt", false},
		{"/notes/binary.exe", false},
		{"/notes/noext", false},
		{"/notes/.md", true}, // hidden file with .md extension
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMarkdownFile(tt.name)
			if got != tt.want {
				t.Errorf("isMarkdownFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestErrNotMarkdownWrapsErrPermission(t *testing.T) {
	if !errors.Is(errNotMarkdown, os.ErrPermission) {
		t.Error("errNotMarkdown should wrap os.ErrPermission")
	}
}
