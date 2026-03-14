package nfs

import (
	"errors"
	"os"
	"testing"
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
		{"/vault/../vault/doc.md", "vault", "/doc.md"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			vault, doc := parsePath(tt.input)
			if vault != tt.wantVault || doc != tt.wantDoc {
				t.Errorf("parsePath(%q) = (%q, %q), want (%q, %q)",
					tt.input, vault, doc, tt.wantVault, tt.wantDoc)
			}
		})
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
		{"foo.txt", false},
		{"foo.pdf", false},
		{"foo", false},
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

func TestChrootResolve(t *testing.T) {
	c := &chrootFS{root: "/vault"}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"simple file", "doc.md", "/vault/doc.md", false},
		{"nested path", "notes/foo.md", "/vault/notes/foo.md", false},
		{"root", ".", "/vault", false},
		{"escape attempt", "../../etc/passwd", "", true},
		{"escape one level", "../other/doc.md", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.resolve(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolve(%q) = %q, want error", tt.path, got)
				}
				if !errors.Is(err, os.ErrPermission) {
					t.Errorf("resolve(%q) error = %v, want permission error", tt.path, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve(%q) unexpected error: %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("resolve(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestWriteFileRejectsWriteAfterClose(t *testing.T) {
	wf := &writeFile{
		name:   "test.md",
		closed: true,
	}

	_, err := wf.Write([]byte("data"))
	if err == nil {
		t.Fatal("Write after Close should return an error")
	}
	if !errors.Is(err, os.ErrClosed) {
		t.Errorf("Write after Close error = %v, want os.ErrClosed", err)
	}
}
