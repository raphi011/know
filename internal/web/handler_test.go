package web

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/web/templates/components"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func TestBuildFolderTree(t *testing.T) {
	folders := []models.Folder{
		{ID: surrealmodels.RecordID{Table: "folder"}, Path: "/docs", Name: "docs"},
		{ID: surrealmodels.RecordID{Table: "folder"}, Path: "/docs/guides", Name: "guides"},
		{ID: surrealmodels.RecordID{Table: "folder"}, Path: "/docs/guides/setup", Name: "setup"},
		{ID: surrealmodels.RecordID{Table: "folder"}, Path: "/notes", Name: "notes"},
	}

	tree := buildFolderTree(folders)

	if len(tree) != 2 {
		t.Fatalf("expected 2 root folders, got %d", len(tree))
	}

	// Find docs folder
	var docs *components.FolderNode
	for i := range tree {
		if tree[i].Name == "docs" {
			docs = &tree[i]
			break
		}
	}
	if docs == nil {
		t.Fatal("docs folder not found")
	}
	if len(docs.Children) != 1 {
		t.Fatalf("expected 1 child of docs, got %d", len(docs.Children))
	}
	if docs.Children[0].Name != "guides" {
		t.Errorf("expected guides, got %q", docs.Children[0].Name)
	}
	if len(docs.Children[0].Children) != 1 {
		t.Fatalf("expected 1 child of guides, got %d", len(docs.Children[0].Children))
	}
	if docs.Children[0].Children[0].Name != "setup" {
		t.Errorf("expected setup, got %q", docs.Children[0].Children[0].Name)
	}
}

func TestBuildFolderTree_Empty(t *testing.T) {
	tree := buildFolderTree(nil)
	if tree != nil {
		t.Errorf("expected nil, got %v", tree)
	}
}

func TestWriteSSE_SingleLine(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSSE(rec, "test-event", "<p>hello</p>")

	want := "event: test-event\ndata: <p>hello</p>\n\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("writeSSE() = %q, want %q", got, want)
	}
}

func TestWriteSSE_MultiLine(t *testing.T) {
	var buf bytes.Buffer
	rec := httptest.NewRecorder()
	rec.Body = &buf
	writeSSE(rec, "doc-updated", "<h1>Title</h1>\n<p>Body</p>")

	want := "event: doc-updated\ndata: <h1>Title</h1>\ndata: <p>Body</p>\n\n"
	if got := buf.String(); got != want {
		t.Errorf("writeSSE() = %q, want %q", got, want)
	}
}

func TestDecodePathSegment(t *testing.T) {
	got, err := decodePathSegment("hello%20world")
	if err != nil {
		t.Fatalf("decodePathSegment() error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}
