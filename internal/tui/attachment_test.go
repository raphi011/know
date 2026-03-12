package tui

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLooksLikeFilePath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/Users/me/file.md", true},
		{"~/Documents/notes.md", true},
		{"./relative/path.md", true},
		{"../parent/path.md", true},
		{"/", true},
		{"~/", true},
		{"some random text", false},
		{"main.go", false},
		{"", false},
		{"/path/one\n/path/two", false},
		{"http://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := looksLikeFilePath(tt.input)
			if got != tt.want {
				t.Errorf("looksLikeFilePath(%q) = %v, want %v", tt.input, got, tt.want)
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
		{".webp", FileTypeImage},
		{".svg", FileTypeBinary},
		{".bmp", FileTypeBinary},
		{".ico", FileTypeBinary},
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
		imgData := []byte("\x89PNG\r\n\x1a\nfakedata")
		if err := os.WriteFile(path, imgData, 0o644); err != nil {
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
		if att.Content == "" {
			t.Fatal("expected base64 content for image file")
		}
		// Verify base64 round-trip
		decoded, err := base64.StdEncoding.DecodeString(att.Content)
		if err != nil {
			t.Fatalf("Content is not valid base64: %v", err)
		}
		if !bytes.Equal(decoded, imgData) {
			t.Error("base64 decoded content does not match original image data")
		}
		if att.MimeType != "image/png" {
			t.Errorf("MimeType = %q, want %q", att.MimeType, "image/png")
		}
		if att.Type != FileTypeImage {
			t.Errorf("type = %v, want %v", att.Type, FileTypeImage)
		}
		if att.Size != int64(len(imgData)) {
			t.Errorf("Size = %d, want %d", att.Size, len(imgData))
		}
	})

	t.Run("oversized image file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "huge.png")
		data := make([]byte, maxImageAttachmentSize+1)
		for i := range data {
			data[i] = 0xFF
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

	t.Run("empty image file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.png")
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

func TestMimeForExt_Images(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".png", "image/png"},
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".gif", "image/gif"},
		{".webp", "image/webp"},
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

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{10485760, "10.0 MB"},
		{1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
