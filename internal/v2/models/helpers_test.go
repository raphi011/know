package models

import (
	"testing"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/docs/hello.md", "/docs/hello.md"},
		{"docs/hello.md", "/docs/hello.md"},
		{"/docs/hello.md/", "/docs/hello.md"},
		{"//docs//hello.md", "/docs/hello.md"},
		{"/", "/"},
		{".", "/."},
		{"", "/."},
		{"hello.md", "/hello.md"},
		{"/a/b/c/d.md", "/a/b/c/d.md"},
		{"./relative/path.md", "/relative/path.md"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizePath(tt.input)
			if got != tt.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParentFolder(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/docs/hello.md", "/docs"},
		{"/hello.md", "/"},
		{"/a/b/c/d.md", "/a/b/c"},
		{"hello.md", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParentFolder(tt.input)
			if got != tt.want {
				t.Errorf("ParentFolder(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRecordIDString(t *testing.T) {
	// Happy path: string ID
	id := surrealmodels.RecordID{Table: "document", ID: "abc123"}
	got, err := RecordIDString(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc123" {
		t.Errorf("got %q, want %q", got, "abc123")
	}

	// Error: non-string ID
	badID := surrealmodels.RecordID{Table: "document", ID: 12345}
	_, err = RecordIDString(badID)
	if err == nil {
		t.Error("expected error for non-string ID")
	}
}

func TestMustRecordIDString(t *testing.T) {
	// Happy path
	id := surrealmodels.RecordID{Table: "user", ID: "user1"}
	got := MustRecordIDString(id)
	if got != "user1" {
		t.Errorf("got %q, want %q", got, "user1")
	}

	// Panic on bad ID
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-string ID")
		}
	}()
	MustRecordIDString(surrealmodels.RecordID{Table: "user", ID: 999})
}

func TestContentHash(t *testing.T) {
	hash1 := ContentHash("hello world")
	hash2 := ContentHash("hello world")
	hash3 := ContentHash("different")

	if hash1 != hash2 {
		t.Error("same content should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}
	if len(hash1) != 64 {
		t.Errorf("SHA256 hex should be 64 chars, got %d", len(hash1))
	}
}
