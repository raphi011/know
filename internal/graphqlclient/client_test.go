package graphqlclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("auth header = %q, want %q", got, "Bearer test-token")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q, want %q", got, "application/json")
		}

		var req gqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Query != "query { me { name } }" {
			t.Errorf("query = %q", req.Query)
		}

		w.Write([]byte(`{"data":{"me":{"name":"alice"}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	var resp struct {
		Me struct {
			Name string `json:"name"`
		} `json:"me"`
	}
	if err := c.Do(context.Background(), "query { me { name } }", nil, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Me.Name != "alice" {
		t.Errorf("name = %q, want %q", resp.Me.Name, "alice")
	}
}

func TestDo_WithVariables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Variables["id"] != "123" {
			t.Errorf("variables[id] = %v", req.Variables["id"])
		}
		w.Write([]byte(`{"data":{"doc":{"title":"Test"}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var resp struct {
		Doc struct {
			Title string `json:"title"`
		} `json:"doc"`
	}
	err := c.Do(context.Background(), "query($id: ID!) { doc(id: $id) { title } }", map[string]any{"id": "123"}, &resp)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Doc.Title != "Test" {
		t.Errorf("title = %q", resp.Doc.Title)
	}
}

func TestDo_NilTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"createDoc":{"id":"1"}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.Do(context.Background(), "mutation { createDoc { id } }", nil, nil); err != nil {
		t.Fatalf("unexpected error with nil target: %v", err)
	}
}

func TestDo_GraphQLSingleError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errors":[{"message":"not found"}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.Do(context.Background(), "query { x }", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err, "not found")
	}
}

func TestDo_GraphQLMultipleErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errors":[{"message":"field required"},{"message":"type mismatch"}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.Do(context.Background(), "query { x }", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "field required") {
		t.Errorf("error = %q, want to contain %q", err, "field required")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("error = %q, want to contain %q", err, "type mismatch")
	}
}

func TestDo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.Do(context.Background(), "query { x }", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, want to contain %q", err, "HTTP 500")
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := New(srv.URL, "tok")
	err := c.Do(ctx, "query { x }", nil, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDo_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.Do(context.Background(), "query { x }", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unmarshal response") {
		t.Errorf("error = %q, want to contain %q", err, "unmarshal response")
	}
}
