package webdav

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// stubVirtualFile is a test double for VirtualFile.
type stubVirtualFile struct {
	name    string
	content string
	err     error
}

func (s *stubVirtualFile) Name() string { return s.name }
func (s *stubVirtualFile) Generate(_ context.Context, _ string) (string, error) {
	return s.content, s.err
}

func TestFindVirtualFile(t *testing.T) {
	stub := &stubVirtualFile{name: ".labels.md", content: "# Labels\n"}
	fs := &FS{virtualFiles: []VirtualFile{stub}}

	t.Run("found", func(t *testing.T) {
		vf := fs.findVirtualFile("/.labels.md")
		if vf == nil {
			t.Fatal("expected virtual file, got nil")
		}
		if vf.Name() != ".labels.md" {
			t.Errorf("Name() = %q, want %q", vf.Name(), ".labels.md")
		}
	})

	t.Run("not found", func(t *testing.T) {
		vf := fs.findVirtualFile("/readme.md")
		if vf != nil {
			t.Errorf("expected nil, got %v", vf)
		}
	})
}

func TestIsVirtualFilePath(t *testing.T) {
	stub := &stubVirtualFile{name: ".labels.md"}
	fs := &FS{virtualFiles: []VirtualFile{stub}}

	tests := []struct {
		path string
		want bool
	}{
		{"/.labels.md", true},
		{"/subfolder/.labels.md", false}, // not root level
		{"/readme.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := fs.isVirtualFilePath(tt.path)
			if got != tt.want {
				t.Errorf("isVirtualFilePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestVirtualReadFile(t *testing.T) {
	content := "# Hello\n\nWorld\n"

	t.Run("read", func(t *testing.T) {
		f := newVirtualReadFile("/.test.md", content)
		buf := make([]byte, 100)
		n, err := f.Read(buf)
		if err != nil {
			t.Fatalf("Read() error: %v", err)
		}
		if string(buf[:n]) != content {
			t.Errorf("Read() = %q, want %q", string(buf[:n]), content)
		}
	})

	t.Run("write rejected", func(t *testing.T) {
		f := newVirtualReadFile("/.test.md", content)
		_, err := f.Write([]byte("nope"))
		if err != os.ErrPermission {
			t.Errorf("Write() error = %v, want ErrPermission", err)
		}
	})

	t.Run("seek", func(t *testing.T) {
		f := newVirtualReadFile("/.test.md", content)
		pos, err := f.Seek(0, 0)
		if err != nil {
			t.Fatalf("Seek() error: %v", err)
		}
		if pos != 0 {
			t.Errorf("Seek() = %d, want 0", pos)
		}
	})

	t.Run("stat", func(t *testing.T) {
		f := newVirtualReadFile("/.test.md", content)
		fi, err := f.Stat()
		if err != nil {
			t.Fatalf("Stat() error: %v", err)
		}
		if fi.Name() != ".test.md" {
			t.Errorf("Name() = %q, want %q", fi.Name(), ".test.md")
		}
		if fi.Size() != int64(len(content)) {
			t.Errorf("Size() = %d, want %d", fi.Size(), len(content))
		}
		if fi.IsDir() {
			t.Error("IsDir() = true, want false")
		}
	})

	t.Run("readdir invalid", func(t *testing.T) {
		f := newVirtualReadFile("/.test.md", content)
		_, err := f.Readdir(0)
		if err != os.ErrInvalid {
			t.Errorf("Readdir() error = %v, want ErrInvalid", err)
		}
	})

	t.Run("close", func(t *testing.T) {
		f := newVirtualReadFile("/.test.md", content)
		if err := f.Close(); err != nil {
			t.Errorf("Close() = %v, want nil", err)
		}
	})
}

func TestDefaultVirtualFiles(t *testing.T) {
	files := defaultVirtualFiles(nil)
	if len(files) != 2 {
		t.Fatalf("expected 2 virtual files, got %d", len(files))
	}

	names := make(map[string]bool)
	for _, vf := range files {
		names[vf.Name()] = true
	}

	if !names[".labels.md"] {
		t.Error("missing .labels.md")
	}
	if !names[".recent.md"] {
		t.Error("missing .recent.md")
	}
}

func TestVirtualReadFileContentType(t *testing.T) {
	f := newVirtualReadFile("/.test.md", "content")
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}

	ct, err := fi.(*fileInfo).ContentType(context.Background())
	if err != nil {
		t.Fatalf("ContentType() error: %v", err)
	}
	if !strings.Contains(ct, "markdown") {
		t.Errorf("ContentType() = %q, want markdown content type", ct)
	}
}

// Tests for virtual file guards in FS methods.
// These use nil service dependencies because virtual file checks
// happen before any service calls.

func TestOpenFileVirtualReadOnly(t *testing.T) {
	stub := &stubVirtualFile{name: ".labels.md", content: "# Labels\n"}
	fs := &FS{virtualFiles: []VirtualFile{stub}}

	t.Run("read succeeds", func(t *testing.T) {
		f, err := fs.OpenFile(context.Background(), "/.labels.md", os.O_RDONLY, 0)
		if err != nil {
			t.Fatalf("OpenFile() error: %v", err)
		}
		buf := make([]byte, 100)
		n, err := f.Read(buf)
		if err != nil {
			t.Fatalf("Read() error: %v", err)
		}
		if string(buf[:n]) != "# Labels\n" {
			t.Errorf("content = %q, want %q", string(buf[:n]), "# Labels\n")
		}
	})

	t.Run("write rejected", func(t *testing.T) {
		_, err := fs.OpenFile(context.Background(), "/.labels.md", os.O_WRONLY, 0)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("OpenFile(O_WRONLY) error = %v, want ErrPermission", err)
		}
	})

	t.Run("create rejected", func(t *testing.T) {
		_, err := fs.OpenFile(context.Background(), "/.labels.md", os.O_CREATE, 0)
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("OpenFile(O_CREATE) error = %v, want ErrPermission", err)
		}
	})

	t.Run("generate error propagated", func(t *testing.T) {
		errStub := &stubVirtualFile{name: ".broken.md", err: errors.New("db down")}
		fs := &FS{virtualFiles: []VirtualFile{errStub}}
		_, err := fs.OpenFile(context.Background(), "/.broken.md", os.O_RDONLY, 0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "db down") {
			t.Errorf("error = %v, want to contain 'db down'", err)
		}
	})
}

func TestStatVirtualFile(t *testing.T) {
	stub := &stubVirtualFile{name: ".labels.md", content: "# Labels\n"}
	fs := &FS{virtualFiles: []VirtualFile{stub}}

	fi, err := fs.Stat(context.Background(), "/.labels.md")
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if fi.Name() != ".labels.md" {
		t.Errorf("Name() = %q, want %q", fi.Name(), ".labels.md")
	}
	if fi.IsDir() {
		t.Error("IsDir() = true, want false")
	}
}

func TestRemoveAllVirtualFile(t *testing.T) {
	stub := &stubVirtualFile{name: ".labels.md"}
	fs := &FS{virtualFiles: []VirtualFile{stub}}

	err := fs.RemoveAll(context.Background(), "/.labels.md")
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("RemoveAll() error = %v, want ErrPermission", err)
	}
}

func TestRenameVirtualFile(t *testing.T) {
	stub := &stubVirtualFile{name: ".labels.md"}
	fs := &FS{virtualFiles: []VirtualFile{stub}}

	t.Run("rename from virtual", func(t *testing.T) {
		err := fs.Rename(context.Background(), "/.labels.md", "/other.md")
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("Rename(from virtual) error = %v, want ErrPermission", err)
		}
	})

	t.Run("rename to virtual", func(t *testing.T) {
		err := fs.Rename(context.Background(), "/other.md", "/.labels.md")
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("Rename(to virtual) error = %v, want ErrPermission", err)
		}
	})
}
