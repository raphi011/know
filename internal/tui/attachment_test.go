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
		{"no match for email", "email admin@config.yaml", nil},
		{"path with space stops at space", "check @./my file.go", []string{"./my"}},
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
		{"only refs", "@./file.go", []string{"./file.go"}, ""},
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
				t.Errorf("classifyFile(%q) = %v, want %v", tt.ext, got, tt.want)
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

func TestMimeForExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".json", "application/json"},
		{".xml", "application/xml"},
		{".html", "text/html"},
		{".css", "text/css"},
		{".js", "text/javascript"},
		{".md", "text/markdown"},
		{".go", "text/plain"},
		{".py", "text/plain"},
		{"", "text/plain"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := mimeForExt(tt.ext)
			if got != tt.want {
				t.Errorf("mimeForExt(%q) = %q, want %q", tt.ext, got, tt.want)
			}
		})
	}
}

func TestFileTypeString(t *testing.T) {
	tests := []struct {
		ft   FileType
		want string
	}{
		{FileTypeText, "text"},
		{FileTypeImage, "image"},
		{FileTypeBinary, "binary"},
		{FileTypeUnknown, "unknown"},
		{FileType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.ft.String()
			if got != tt.want {
				t.Errorf("FileType(%d).String() = %q, want %q", tt.ft, got, tt.want)
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
		if att.Name() != "test.go" {
			t.Errorf("Name() = %q, want %q", att.Name(), "test.go")
		}
		if att.MimeType != "text/plain" {
			t.Errorf("MimeType = %q, want %q", att.MimeType, "text/plain")
		}
		if att.Type != FileTypeText {
			t.Errorf("type = %v, want %v", att.Type, FileTypeText)
		}
		if att.AbsPath != path {
			t.Errorf("AbsPath = %q, want %q", att.AbsPath, path)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		atts := resolveAttachments([]string{"/nonexistent/file.go"})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if !strings.Contains(atts[0].Error, "file not found") {
			t.Errorf("expected 'file not found' error, got: %s", atts[0].Error)
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
		for i := range data {
			data[i] = 'x' // non-zero to avoid empty check
		}
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

	t.Run("image file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "photo.png")
		if err := os.WriteFile(path, []byte("\x89PNG"), 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if !strings.Contains(atts[0].Error, "image attachments not yet supported") {
			t.Errorf("expected image error, got: %s", atts[0].Error)
		}
		if atts[0].Content != "" {
			t.Error("expected empty content for image file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.go")
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		if !strings.Contains(atts[0].Error, "empty") {
			t.Errorf("expected empty file error, got: %s", atts[0].Error)
		}
	})

	t.Run("line count with trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lines.go")
		content := "line1\nline2\nline3\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if atts[0].LineCount() != 3 {
			t.Errorf("LineCount() = %d, want 3", atts[0].LineCount())
		}
	})

	t.Run("line count without trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "notrail.go")
		content := "line1\nline2\nline3"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		atts := resolveAttachments([]string{path})
		if atts[0].LineCount() != 3 {
			t.Errorf("LineCount() = %d, want 3", atts[0].LineCount())
		}
	})
}
