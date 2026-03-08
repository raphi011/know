package sshd

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/auth"
	"golang.org/x/crypto/ssh"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		input     string
		wantVault string
		wantDoc   string
	}{
		{"/", "", ""},
		{"", "", ""},
		{".", "", ""},
		{"/myvault", "myvault", "/"},
		{"/myvault/", "myvault", "/"},
		{"/myvault/notes/foo.md", "myvault", "/notes/foo.md"},
		{"/myvault/foo.md", "myvault", "/foo.md"},
		{"myvault/foo.md", "myvault", "/foo.md"},
		{"/myvault/a/../b/foo.md", "myvault", "/b/foo.md"},
		{"/myvault/./foo.md", "myvault", "/foo.md"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			vault, doc := parsePath(tt.input)
			if vault != tt.wantVault {
				t.Errorf("parsePath(%q) vault = %q, want %q", tt.input, vault, tt.wantVault)
			}
			if doc != tt.wantDoc {
				t.Errorf("parsePath(%q) doc = %q, want %q", tt.input, doc, tt.wantDoc)
			}
		})
	}
}

func TestListAt(t *testing.T) {
	entries := []os.FileInfo{
		&fileInfo{name: "a"},
		&fileInfo{name: "b"},
		&fileInfo{name: "c"},
		&fileInfo{name: "d"},
	}
	la := &listAt{entries: entries}

	t.Run("from start", func(t *testing.T) {
		buf := make([]os.FileInfo, 2)
		n, err := la.ListAt(buf, 0)
		if n != 2 {
			t.Errorf("n = %d, want 2", n)
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if buf[0].Name() != "a" || buf[1].Name() != "b" {
			t.Errorf("got %q %q, want a b", buf[0].Name(), buf[1].Name())
		}
	})

	t.Run("from middle", func(t *testing.T) {
		buf := make([]os.FileInfo, 2)
		n, err := la.ListAt(buf, 2)
		if n != 2 {
			t.Errorf("n = %d, want 2", n)
		}
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
		if buf[0].Name() != "c" || buf[1].Name() != "d" {
			t.Errorf("got %q %q, want c d", buf[0].Name(), buf[1].Name())
		}
	})

	t.Run("past end", func(t *testing.T) {
		buf := make([]os.FileInfo, 2)
		n, err := la.ListAt(buf, 10)
		if n != 0 {
			t.Errorf("n = %d, want 0", n)
		}
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
	})

	t.Run("partial at end", func(t *testing.T) {
		buf := make([]os.FileInfo, 3)
		n, err := la.ListAt(buf, 3)
		if n != 1 {
			t.Errorf("n = %d, want 1", n)
		}
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
		if buf[0].Name() != "d" {
			t.Errorf("got %q, want d", buf[0].Name())
		}
	})

	t.Run("empty list", func(t *testing.T) {
		la := &listAt{entries: nil}
		buf := make([]os.FileInfo, 2)
		n, err := la.ListAt(buf, 0)
		if n != 0 || err != io.EOF {
			t.Errorf("n = %d, err = %v, want 0, io.EOF", n, err)
		}
	})
}

func TestFileInfoMode(t *testing.T) {
	dir := &fileInfo{name: "docs", isDir: true}
	file := &fileInfo{name: "readme.md"}

	if dir.Mode() != fs.ModeDir|0755 {
		t.Errorf("dir mode = %v, want ModeDir|0755", dir.Mode())
	}
	if file.Mode() != fs.FileMode(0644) {
		t.Errorf("file mode = %v, want 0644", file.Mode())
	}
	if !dir.IsDir() {
		t.Error("dir.IsDir() should be true")
	}
	if file.IsDir() {
		t.Error("file.IsDir() should be false")
	}
}

func TestFileInfoFields(t *testing.T) {
	now := time.Now()
	fi := &fileInfo{
		name:    "test.md",
		size:    1234,
		modTime: now,
		isDir:   false,
	}

	if fi.Name() != "test.md" {
		t.Errorf("Name() = %q, want test.md", fi.Name())
	}
	if fi.Size() != 1234 {
		t.Errorf("Size() = %d, want 1234", fi.Size())
	}
	if !fi.ModTime().Equal(now) {
		t.Errorf("ModTime() = %v, want %v", fi.ModTime(), now)
	}
	if fi.Sys() != nil {
		t.Error("Sys() should be nil")
	}
}

func TestIsMarkdownFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"foo.md", true},
		{"foo.MD", true},
		{"foo.Md", true},
		{"/path/to/notes.md", true},
		{"foo.txt", false},
		{"foo", false},
		{"foo.markdown", false},
		{".md", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMarkdownFile(tt.name); got != tt.want {
				t.Errorf("isMarkdownFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestWriteBufferWriteAt(t *testing.T) {
	t.Run("sequential writes", func(t *testing.T) {
		wb := &writeBuffer{}
		n, err := wb.WriteAt([]byte("hello"), 0)
		if err != nil || n != 5 {
			t.Fatalf("WriteAt(0) = %d, %v", n, err)
		}
		n, err = wb.WriteAt([]byte(" world"), 5)
		if err != nil || n != 6 {
			t.Fatalf("WriteAt(5) = %d, %v", n, err)
		}
		if string(wb.buf) != "hello world" {
			t.Errorf("buf = %q, want %q", string(wb.buf), "hello world")
		}
	})

	t.Run("write with gap", func(t *testing.T) {
		wb := &writeBuffer{}
		wb.WriteAt([]byte("abc"), 0)
		wb.WriteAt([]byte("xyz"), 10)
		if len(wb.buf) != 13 {
			t.Errorf("buf len = %d, want 13", len(wb.buf))
		}
		if string(wb.buf[10:]) != "xyz" {
			t.Errorf("buf[10:] = %q, want xyz", string(wb.buf[10:]))
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		wb := &writeBuffer{}
		wb.WriteAt([]byte("hello"), 0)
		wb.WriteAt([]byte("world"), 0)
		if string(wb.buf) != "world" {
			t.Errorf("buf = %q, want %q", string(wb.buf), "world")
		}
	})

	t.Run("negative offset", func(t *testing.T) {
		wb := &writeBuffer{}
		_, err := wb.WriteAt([]byte("data"), -1)
		if err == nil {
			t.Fatal("expected error for negative offset")
		}
		if !errors.Is(err, os.ErrInvalid) {
			t.Errorf("expected os.ErrInvalid, got %v", err)
		}
	})

	t.Run("exceeds max size", func(t *testing.T) {
		wb := &writeBuffer{}
		_, err := wb.WriteAt([]byte("x"), maxDocSize)
		if err == nil {
			t.Fatal("expected error when exceeding max doc size")
		}
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("expected os.ErrPermission, got %v", err)
		}
	})
}

func TestAuthContextFromPermissions(t *testing.T) {
	t.Run("nil permissions", func(t *testing.T) {
		ac := authContextFromPermissions(nil)
		if ac.UserID != "" {
			t.Errorf("UserID = %q, want empty", ac.UserID)
		}
		if len(ac.VaultAccess) != 0 {
			t.Errorf("VaultAccess = %v, want empty", ac.VaultAccess)
		}
	})

	t.Run("empty extensions", func(t *testing.T) {
		ac := authContextFromPermissions(&ssh.Permissions{
			Extensions: map[string]string{},
		})
		if ac.UserID != "" {
			t.Errorf("UserID = %q, want empty", ac.UserID)
		}
		if len(ac.VaultAccess) != 0 {
			t.Errorf("VaultAccess = %v, want empty", ac.VaultAccess)
		}
	})

	t.Run("single vault", func(t *testing.T) {
		ac := authContextFromPermissions(&ssh.Permissions{
			Extensions: map[string]string{
				"user_id":      "user1",
				"vault_access": "default",
			},
		})
		if ac.UserID != "user1" {
			t.Errorf("UserID = %q, want user1", ac.UserID)
		}
		if len(ac.VaultAccess) != 1 || ac.VaultAccess[0] != "default" {
			t.Errorf("VaultAccess = %v, want [default]", ac.VaultAccess)
		}
	})

	t.Run("multiple vaults", func(t *testing.T) {
		ac := authContextFromPermissions(&ssh.Permissions{
			Extensions: map[string]string{
				"user_id":      "user2",
				"vault_access": "vault-a,vault-b,vault-c",
			},
		})
		if len(ac.VaultAccess) != 3 {
			t.Fatalf("VaultAccess len = %d, want 3", len(ac.VaultAccess))
		}
		if ac.VaultAccess[0] != "vault-a" || ac.VaultAccess[1] != "vault-b" || ac.VaultAccess[2] != "vault-c" {
			t.Errorf("VaultAccess = %v, want [vault-a vault-b vault-c]", ac.VaultAccess)
		}
	})

	t.Run("wildcard access", func(t *testing.T) {
		ac := authContextFromPermissions(&ssh.Permissions{
			Extensions: map[string]string{
				"user_id":      "admin",
				"vault_access": auth.WildcardVaultAccess,
			},
		})
		if len(ac.VaultAccess) != 1 || ac.VaultAccess[0] != "*" {
			t.Errorf("VaultAccess = %v, want [*]", ac.VaultAccess)
		}
	})
}

func TestErrNotMarkdownWrapsErrPermission(t *testing.T) {
	if !errors.Is(errNotMarkdown, os.ErrPermission) {
		t.Errorf("errNotMarkdown does not wrap os.ErrPermission")
	}
}
