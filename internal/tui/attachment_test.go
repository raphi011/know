package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAtRefs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"no refs", "hello world", nil},
		{"single relative", "explain @./main.go", []string{"./main.go"}},
		{"single absolute", "read @/etc/hosts please", []string{"/etc/hosts"}},
		{"tilde path", "check @~/Documents/notes.md", []string{"~/Documents/notes.md"}},
		{"multiple refs", "@./a.go @./b.go compare", []string{"./a.go", "./b.go"}},
		{"duplicate refs", "@./a.go @./a.go", []string{"./a.go"}},
		{"bare file with dot", "review @main.go", []string{"main.go"}},
		{"path with hyphens", "@./my-file.ts ok", []string{"./my-file.ts"}},
		{"path with dots", "@./dir.name/file.ext", []string{"./dir.name/file.ext"}},
		{"nested path", "@./internal/tui/app.go", []string{"./internal/tui/app.go"}},
		{"parent path", "@../other/file.py", []string{"../other/file.py"}},
		{"no match for @mention", "hey @john how are you", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAtRefs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseAtRefs(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i, g := range got {
				if g != tt.want[i] {
					t.Errorf("parseAtRefs(%q)[%d] = %q, want %q", tt.input, i, g, tt.want[i])
				}
			}
		})
	}
}

func TestStripAtRefs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		refs  []string
		want  string
	}{
		{"single ref", "explain @./main.go please", []string{"./main.go"}, "explain please"},
		{"multiple refs", "@./a.go @./b.go compare these", []string{"./a.go", "./b.go"}, "compare these"},
		{"no refs", "hello world", nil, "hello world"},
		{"ref at end", "review @./file.go", []string{"./file.go"}, "review"},
		{"ref at start", "@./file.go explain", []string{"./file.go"}, "explain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAtRefs(tt.input, tt.refs)
			if got != tt.want {
				t.Errorf("stripAtRefs(%q, %v) = %q, want %q", tt.input, tt.refs, got, tt.want)
			}
		})
	}
}

func TestClassifyFile(t *testing.T) {
	tests := []struct {
		ext  string
		want FileType
	}{
		{".go", FileTypeText},
		{".py", FileTypeText},
		{".ts", FileTypeText},
		{".md", FileTypeText},
		{".json", FileTypeText},
		{".yaml", FileTypeText},
		{".toml", FileTypeText},
		{".sql", FileTypeText},
		{".sh", FileTypeText},
		{".png", FileTypeImage},
		{".jpg", FileTypeImage},
		{".jpeg", FileTypeImage},
		{".gif", FileTypeImage},
		{".svg", FileTypeImage},
		{".exe", FileTypeBinary},
		{".zip", FileTypeBinary},
		{".pdf", FileTypeBinary},
		{"", FileTypeBinary},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := classifyFile(tt.ext)
			if got != tt.want {
				t.Errorf("classifyFile(%q) = %d, want %d", tt.ext, got, tt.want)
			}
		})
	}
}

func TestLangForExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".py", "python"},
		{".ts", "typescript"},
		{".tsx", "tsx"},
		{".rs", "rust"},
		{".java", "java"},
		{".md", "markdown"},
		{".json", "json"},
		{".yaml", "yaml"},
		{".yml", "yaml"},
		{".sql", "sql"},
		{".unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := langForExt(tt.ext)
			if got != tt.want {
				t.Errorf("langForExt(%q) = %q, want %q", tt.ext, got, tt.want)
			}
		})
	}
}

func TestResolveAttachments(t *testing.T) {
	t.Run("reads real file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		content := "package main\n\nfunc main() {}\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		att := atts[0]
		if att.Error != "" {
			t.Fatalf("unexpected error: %s", att.Error)
		}
		if att.Content != content {
			t.Errorf("content = %q, want %q", att.Content, content)
		}
		if att.Language != "go" {
			t.Errorf("language = %q, want %q", att.Language, "go")
		}
		if att.Name != "test.go" {
			t.Errorf("name = %q, want %q", att.Name, "test.go")
		}
		if att.Type != FileTypeText {
			t.Errorf("type = %d, want %d", att.Type, FileTypeText)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		atts := resolveAttachments([]string{"/nonexistent/file.go"})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if atts[0].Error == "" {
			t.Error("expected error for missing file")
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		atts := resolveAttachments([]string{dir})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if !strings.Contains(atts[0].Error, "directory") {
			t.Errorf("expected directory error, got: %s", atts[0].Error)
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "big.txt")
		// Create file just over 1MB
		data := make([]byte, maxAttachmentSize+1)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if !strings.Contains(atts[0].Error, "too large") {
			t.Errorf("expected size error, got: %s", atts[0].Error)
		}
	})

	t.Run("binary file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.zip")
		if err := os.WriteFile(path, []byte("PK\x03\x04"), 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if !strings.Contains(atts[0].Error, "binary") {
			t.Errorf("expected binary error, got: %s", atts[0].Error)
		}
	})

	t.Run("line count", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lines.go")
		content := "line1\nline2\nline3\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if atts[0].LineCount() != 4 {
			t.Errorf("LineCount() = %d, want 4", atts[0].LineCount())
		}
	})
}
