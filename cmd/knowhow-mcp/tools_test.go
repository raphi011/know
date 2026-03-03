package main

import (
	"testing"

	"github.com/raphi011/knowhow/internal/graphqlclient"
)

func TestFilterInstances(t *testing.T) {
	mt := &mcpTools{instances: []*connectedInstance{
		{name: "private", client: graphqlclient.New("http://a/query", "t1")},
		{name: "work", client: graphqlclient.New("http://b/query", "t2")},
	}}

	t.Run("nil name returns all", func(t *testing.T) {
		got := mt.filterInstances(nil)
		if len(got) != 2 {
			t.Errorf("len = %d, want 2", len(got))
		}
	})

	t.Run("matching name returns single", func(t *testing.T) {
		name := "work"
		got := mt.filterInstances(&name)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].name != "work" {
			t.Errorf("name = %q, want %q", got[0].name, "work")
		}
	})

	t.Run("unknown name returns nil", func(t *testing.T) {
		name := "unknown"
		got := mt.filterInstances(&name)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestNewGQLClient(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"appends /query", "http://localhost:8484"},
		{"strips trailing slash", "http://localhost:8484/"},
		{"preserves existing /query", "http://localhost:8484/query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := Instance{Name: "test", URL: tt.url, Token: "tok"}
			client := newGQLClient(inst)
			if client == nil {
				t.Fatal("client is nil")
			}
		})
	}
}

func TestSearchInputValidation(t *testing.T) {
	mt := &mcpTools{instances: []*connectedInstance{
		{name: "test", client: graphqlclient.New("http://a/query", "t")},
	}}

	t.Run("empty query", func(t *testing.T) {
		_, _, err := mt.searchDocuments(t.Context(), nil, SearchInput{Query: ""})
		if err == nil {
			t.Fatal("expected error for empty query")
		}
	})

	t.Run("whitespace query", func(t *testing.T) {
		_, _, err := mt.searchDocuments(t.Context(), nil, SearchInput{Query: "   "})
		if err == nil {
			t.Fatal("expected error for whitespace query")
		}
	})

	t.Run("negative limit", func(t *testing.T) {
		neg := -1
		_, _, err := mt.searchDocuments(t.Context(), nil, SearchInput{Query: "test", Limit: &neg})
		if err == nil {
			t.Fatal("expected error for negative limit")
		}
	})

	t.Run("unknown instance", func(t *testing.T) {
		name := "nonexistent"
		_, _, err := mt.searchDocuments(t.Context(), nil, SearchInput{Query: "test", Instance: &name})
		if err == nil {
			t.Fatal("expected error for unknown instance")
		}
	})
}

func TestGetDocumentInputValidation(t *testing.T) {
	mt := &mcpTools{instances: []*connectedInstance{
		{name: "test", client: graphqlclient.New("http://a/query", "t")},
	}}

	t.Run("empty path", func(t *testing.T) {
		_, _, err := mt.getDocument(t.Context(), nil, GetDocumentInput{Path: ""})
		if err == nil {
			t.Fatal("expected error for empty path")
		}
	})

	t.Run("unknown instance", func(t *testing.T) {
		name := "nonexistent"
		_, _, err := mt.getDocument(t.Context(), nil, GetDocumentInput{Path: "/doc.md", Instance: &name})
		if err == nil {
			t.Fatal("expected error for unknown instance")
		}
	})
}

func TestCreateMemoryInputValidation(t *testing.T) {
	mt := &mcpTools{instances: []*connectedInstance{
		{name: "test", client: graphqlclient.New("http://a/query", "t")},
	}}

	t.Run("empty title", func(t *testing.T) {
		_, _, err := mt.createMemory(t.Context(), nil, CreateMemoryInput{
			Title: "", Content: "body", Instance: "test",
		})
		if err == nil {
			t.Fatal("expected error for empty title")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		_, _, err := mt.createMemory(t.Context(), nil, CreateMemoryInput{
			Title: "title", Content: "", Instance: "test",
		})
		if err == nil {
			t.Fatal("expected error for empty content")
		}
	})

	t.Run("unknown instance", func(t *testing.T) {
		_, _, err := mt.createMemory(t.Context(), nil, CreateMemoryInput{
			Title: "title", Content: "body", Instance: "nonexistent",
		})
		if err == nil {
			t.Fatal("expected error for unknown instance")
		}
	})
}

func TestListLabelsUnknownInstance(t *testing.T) {
	mt := &mcpTools{instances: []*connectedInstance{
		{name: "test", client: graphqlclient.New("http://a/query", "t")},
	}}

	name := "nonexistent"
	_, _, err := mt.listLabels(t.Context(), nil, ListLabelsInput{Instance: &name})
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
}
