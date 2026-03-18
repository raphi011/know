package webdav

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestReadFile(t *testing.T) {
	content := []byte("# Hello\nWorld")
	now := time.Now()

	f := newReadFile("/test.md", content, now, nil)

	// Read
	buf := make([]byte, 100)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error: %v", err)
	}
	if string(buf[:n]) != "# Hello\nWorld" {
		t.Errorf("Read() = %q, want %q", string(buf[:n]), "# Hello\nWorld")
	}

	// Seek back
	pos, err := f.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek() error: %v", err)
	}
	if pos != 0 {
		t.Errorf("Seek() = %d, want 0", pos)
	}

	// Write should fail
	_, err = f.Write([]byte("nope"))
	if err != os.ErrPermission {
		t.Errorf("Write() error = %v, want ErrPermission", err)
	}

	// Readdir should fail
	_, err = f.Readdir(0)
	if err != os.ErrInvalid {
		t.Errorf("Readdir() error = %v, want ErrInvalid", err)
	}

	// Stat
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if fi.Name() != "test.md" {
		t.Errorf("Stat().Name() = %q, want %q", fi.Name(), "test.md")
	}
	if fi.Size() != int64(len("# Hello\nWorld")) {
		t.Errorf("Stat().Size() = %d, want %d", fi.Size(), len("# Hello\nWorld"))
	}

	// Close
	if err := f.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestDirFile(t *testing.T) {
	entries := []os.FileInfo{
		&fileInfo{name: "a.md", size: 10},
		&fileInfo{name: "b.md", size: 20},
		&fileInfo{name: "subdir", isDir: true},
	}

	d := newDirFile("/docs", time.Now(), entries)

	// Stat
	fi, err := d.Stat()
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if !fi.IsDir() {
		t.Error("Stat().IsDir() = false, want true")
	}

	// Readdir all at once
	got, err := d.Readdir(0)
	if err != nil {
		t.Fatalf("Readdir(0) error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Readdir(0) returned %d entries, want 3", len(got))
	}

	// Reset position and read one at a time
	d.pos = 0
	got, err = d.Readdir(1)
	if err != nil {
		t.Fatalf("Readdir(1) error: %v", err)
	}
	if len(got) != 1 || got[0].Name() != "a.md" {
		t.Errorf("Readdir(1) = %v, want [a.md]", got)
	}

	got, err = d.Readdir(1)
	if err != nil {
		t.Fatalf("Readdir(1) error: %v", err)
	}
	if len(got) != 1 || got[0].Name() != "b.md" {
		t.Errorf("Readdir(1) = %v, want [b.md]", got)
	}

	got, err = d.Readdir(1)
	if err != io.EOF {
		t.Fatalf("Readdir(1) error = %v, want io.EOF", err)
	}
	if len(got) != 1 || got[0].Name() != "subdir" {
		t.Errorf("Readdir(1) = %v, want [subdir]", got)
	}

	// Past end
	got, err = d.Readdir(1)
	if err != io.EOF {
		t.Fatalf("Readdir(1) at end error = %v, want io.EOF", err)
	}
	if len(got) != 0 {
		t.Errorf("Readdir(1) at end returned %d entries, want 0", len(got))
	}

	// Write/Read should fail
	_, err = d.Write([]byte("x"))
	if err != os.ErrPermission {
		t.Errorf("Write() error = %v, want ErrPermission", err)
	}
	_, err = d.Read([]byte{0})
	if err != os.ErrInvalid {
		t.Errorf("Read() error = %v, want ErrInvalid", err)
	}
}

// writeFile.Close() empty-PUT behavior (Finder error -43 fix) is tested via
// integration tests — writeFile depends on *file.Service which requires DB.
