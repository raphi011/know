package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

// deleteResponse mirrors the server's deleteDocumentsResponse.
type deleteResponse struct {
	Deleted []string `json:"deleted"`
	Count   int      `json:"count"`
	DryRun  bool     `json:"dryRun"`
}

// deleteRequest performs a DELETE /api/documents request and returns the parsed response.
func deleteRequest(t *testing.T, srv *httptest.Server, vaultID, path string, recursive, dryRun bool) (*http.Response, deleteResponse) {
	t.Helper()

	u := srv.URL + "/api/documents?vault=" + vaultID + "&path=" + path
	if recursive {
		u += "&recursive=true"
	}
	if dryRun {
		u += "&dry-run=true"
	}

	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}

	var result deleteResponse
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode response: %v (body: %s)", err, string(body))
		}
	}

	return resp, result
}

func TestDelete_SingleDocument(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "del-single")

	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/readme.md": []byte("# Readme"),
	})

	resp, result := deleteRequest(t, srv, vaultID, "/docs/readme.md", false, false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result.Count != 1 {
		t.Errorf("expected count=1, got %d", result.Count)
	}
	if result.DryRun {
		t.Error("expected dryRun=false")
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "/docs/readme.md" {
		t.Errorf("expected deleted=[/docs/readme.md], got %v", result.Deleted)
	}

	// Verify document is actually gone
	ctx := context.Background()
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/docs/readme.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc != nil {
		t.Error("document should be deleted")
	}
}

func TestDelete_RecursiveFolder(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "del-recursive")

	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/a.md":      []byte("# A"),
		"/docs/sub/b.md":  []byte("# B"),
		"/other/c.md":     []byte("# C"),
	})

	resp, result := deleteRequest(t, srv, vaultID, "/docs", true, false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result.Count != 2 {
		t.Errorf("expected count=2, got %d", result.Count)
	}

	deleted := make([]string, len(result.Deleted))
	copy(deleted, result.Deleted)
	sort.Strings(deleted)
	want := []string{"/docs/a.md", "/docs/sub/b.md"}
	if len(deleted) != len(want) {
		t.Fatalf("expected deleted=%v, got %v", want, deleted)
	}
	for i, w := range want {
		if deleted[i] != w {
			t.Errorf("deleted[%d]: got %q, want %q", i, deleted[i], w)
		}
	}

	// Verify docs under /docs are gone
	ctx := context.Background()
	for _, p := range []string{"/docs/a.md", "/docs/sub/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted", p)
		}
	}

	// Verify /other/c.md still exists
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/other/c.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("/other/c.md should still exist")
	}
}

func TestDelete_DryRunSingleDocument(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "del-dryrun-single")

	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/readme.md": []byte("# Readme"),
	})

	resp, result := deleteRequest(t, srv, vaultID, "/docs/readme.md", false, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !result.DryRun {
		t.Error("expected dryRun=true")
	}
	if result.Count != 1 {
		t.Errorf("expected count=1, got %d", result.Count)
	}

	// Verify document still exists
	ctx := context.Background()
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/docs/readme.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("dry run should not delete document")
	}
}

func TestDelete_DryRunRecursiveFolder(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "del-dryrun-recursive")

	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/a.md":     []byte("# A"),
		"/docs/sub/b.md": []byte("# B"),
	})

	resp, result := deleteRequest(t, srv, vaultID, "/docs", true, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !result.DryRun {
		t.Error("expected dryRun=true")
	}
	if result.Count != 2 {
		t.Errorf("expected count=2, got %d", result.Count)
	}

	// Verify documents still exist
	ctx := context.Background()
	for _, p := range []string{"/docs/a.md", "/docs/sub/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc == nil {
			t.Errorf("dry run should not delete %s", p)
		}
	}
}

func TestDelete_NonRecursiveOnFolder(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "del-nonrecursive-folder")

	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/a.md": []byte("# A"),
	})

	resp, _ := deleteRequest(t, srv, vaultID, "/docs", false, false)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDelete_NotFound(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "del-notfound")

	resp, _ := deleteRequest(t, srv, vaultID, "/nonexistent.md", false, false)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDelete_MissingParams(t *testing.T) {
	srv, _ := setupBulkServer(t, "del-missing-params")

	// Missing vault
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/documents?path=/docs/a.md", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing vault: expected 400, got %d", resp.StatusCode)
	}

	// Missing path
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/documents?vault=somevault", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing path: expected 400, got %d", resp.StatusCode)
	}
}
