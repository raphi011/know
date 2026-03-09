package webdav

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"golang.org/x/net/webdav"
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
		{"/notes/.md", true},         // hidden file with .md extension
		{"/notes/._readme.md", true}, // has .md extension (OS filtering is separate)
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

func TestIsOSMetadataFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"/notes/._readme.md", true},
		{"/notes/._claude-code-permissions-guide.md", true},
		{"/.DS_Store", true},
		{"/notes/readme.md", false},
		{"/notes/.hidden.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOSMetadataFile(tt.name)
			if got != tt.want {
				t.Errorf("isOSMetadataFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestNopFile(t *testing.T) {
	f := newNopFile("/notes/._foo.md")

	// Write accepts and reports correct length
	n, err := f.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Errorf("Write() = (%d, %v), want (5, nil)", n, err)
	}

	// Read returns EOF immediately
	buf := make([]byte, 10)
	n, err = f.Read(buf)
	if n != 0 || err != io.EOF {
		t.Errorf("Read() = (%d, %v), want (0, EOF)", n, err)
	}

	// Stat returns base name and stable modTime
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if fi.Name() != "._foo.md" {
		t.Errorf("Stat().Name() = %q, want %q", fi.Name(), "._foo.md")
	}
	if fi.Size() != 0 {
		t.Errorf("Stat().Size() = %d, want 0", fi.Size())
	}

	// Close is no-op
	if err := f.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestErrNotMarkdownWrapsErrPermission(t *testing.T) {
	if !errors.Is(errNotMarkdown, os.ErrPermission) {
		t.Error("errNotMarkdown should wrap os.ErrPermission")
	}
}

func TestFileInfoContentType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns content type when set", func(t *testing.T) {
		fi := &fileInfo{name: "test.md", contentType: "text/markdown; charset=utf-8"}
		ct, err := fi.ContentType(ctx)
		if err != nil {
			t.Fatalf("ContentType() error: %v", err)
		}
		if ct != "text/markdown; charset=utf-8" {
			t.Errorf("ContentType() = %q, want %q", ct, "text/markdown; charset=utf-8")
		}
	})

	t.Run("returns ErrNotImplemented when empty", func(t *testing.T) {
		fi := &fileInfo{name: "test.md"}
		_, err := fi.ContentType(ctx)
		if err != webdav.ErrNotImplemented {
			t.Errorf("ContentType() error = %v, want ErrNotImplemented", err)
		}
	})
}

func TestFileInfoETag(t *testing.T) {
	ctx := context.Background()

	t.Run("returns etag when set", func(t *testing.T) {
		fi := &fileInfo{name: "test.md", etag: `"abc123"`}
		etag, err := fi.ETag(ctx)
		if err != nil {
			t.Fatalf("ETag() error: %v", err)
		}
		if etag != `"abc123"` {
			t.Errorf("ETag() = %q, want %q", etag, `"abc123"`)
		}
	})

	t.Run("returns ErrNotImplemented when empty", func(t *testing.T) {
		fi := &fileInfo{name: "test.md"}
		_, err := fi.ETag(ctx)
		if err != webdav.ErrNotImplemented {
			t.Errorf("ETag() error = %v, want ErrNotImplemented", err)
		}
	})
}
