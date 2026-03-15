package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raphi011/know/internal/api"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/server"
)

// moveResponse mirrors the server's moveResponse.
type moveResponse struct {
	Type   string   `json:"type"`
	Moved  []string `json:"moved"`
	Count  int      `json:"count"`
	DryRun bool     `json:"dryRun"`
}

// errorResponse mirrors the server's error JSON.
type errorResponse struct {
	Error string `json:"error"`
}

// setupMoveServer creates a vault and httptest.Server with the move endpoint.
func setupMoveServer(t *testing.T, suffix string) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()

	vaultID, vaultSvc := setupVault(t, ctx, "move-"+suffix+"-"+fmt.Sprint(time.Now().UnixNano()))
	fileSvc := file.NewService(testDB, nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	app := server.NewForTest(testDB, fileSvc, vaultSvc)
	apiSrv := api.NewServer(app)

	mux := http.NewServeMux()
	apiSrv.Register(mux, auth.NoAuthMiddleware)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, vaultID
}

// moveRequest sends a POST /api/move request and returns the HTTP response.
func moveRequest(t *testing.T, srv *httptest.Server, vaultID, source, destination string, dryRun bool) (*http.Response, []byte) {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"vaultId":     vaultID,
		"source":      source,
		"destination": destination,
		"dryRun":      dryRun,
	})
	if err != nil {
		t.Fatalf("marshal move request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/move", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return resp, respBody
}

// createDoc creates a document via the bulk upload endpoint.
func createDoc(t *testing.T, srv *httptest.Server, vaultID, path, content string) {
	t.Helper()

	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		path: []byte(content),
	})
}

func TestMove_DocumentToExistingDocument_Conflict(t *testing.T) {
	srv, vaultID := setupMoveServer(t, "doc-to-doc")

	// Create two documents
	createDoc(t, srv, vaultID, "/doc-a.md", "# Doc A")
	createDoc(t, srv, vaultID, "/doc-b.md", "# Doc B")

	// Try to move doc-a to doc-b's path — should 409
	resp, body := moveRequest(t, srv, vaultID, "/doc-a.md", "/doc-b.md", false)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestMove_DocumentToExistingFolder_Conflict(t *testing.T) {
	srv, vaultID := setupMoveServer(t, "doc-to-folder")

	// Create a document and a folder (by creating a doc inside a folder)
	createDoc(t, srv, vaultID, "/doc.md", "# Document")
	createDoc(t, srv, vaultID, "/myfolder/nested.md", "# Nested")

	// Try to move /doc.md to /myfolder — should 409 (cross-type: document → folder)
	resp, body := moveRequest(t, srv, vaultID, "/doc.md", "/myfolder", false)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error != "cannot move document to existing folder path: /myfolder" {
		t.Errorf("unexpected error message: %s", errResp.Error)
	}
}

func TestMove_FolderToExistingDocument_Conflict(t *testing.T) {
	srv, vaultID := setupMoveServer(t, "folder-to-doc")

	// Create a folder (by creating a doc inside it) and a standalone document
	createDoc(t, srv, vaultID, "/myfolder/nested.md", "# Nested")
	createDoc(t, srv, vaultID, "/target.md", "# Target")

	// Try to move /myfolder to /target.md — should 409 (cross-type: folder → document)
	resp, body := moveRequest(t, srv, vaultID, "/myfolder", "/target.md", false)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error != "cannot move folder to existing document path: /target.md" {
		t.Errorf("unexpected error message: %s", errResp.Error)
	}
}

func TestMove_FolderToNewPath_Success(t *testing.T) {
	srv, vaultID := setupMoveServer(t, "folder-success")

	createDoc(t, srv, vaultID, "/src/a.md", "# A")
	createDoc(t, srv, vaultID, "/src/b.md", "# B")

	resp, body := moveRequest(t, srv, vaultID, "/src", "/dst", false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result moveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Type != "folder" {
		t.Errorf("expected type=folder, got %s", result.Type)
	}
	if result.Count != 2 {
		t.Errorf("expected count=2, got %d", result.Count)
	}

	// Verify documents moved
	ctx := context.Background()
	for _, p := range []string{"/dst/a.md", "/dst/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get %s: %v", p, err)
		}
		if doc == nil {
			t.Errorf("expected document at %s", p)
		}
	}
	for _, p := range []string{"/src/a.md", "/src/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get %s: %v", p, err)
		}
		if doc != nil {
			t.Errorf("document should no longer exist at %s", p)
		}
	}
}

func TestMove_DryRunReportsConflict(t *testing.T) {
	srv, vaultID := setupMoveServer(t, "dryrun-conflict")

	createDoc(t, srv, vaultID, "/doc.md", "# Document")
	createDoc(t, srv, vaultID, "/myfolder/nested.md", "# Nested")

	// Dry-run move of doc to existing folder path — should still 409
	resp, body := moveRequest(t, srv, vaultID, "/doc.md", "/myfolder", true)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 even with dry-run, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestMove_DocumentToNewPath_Success(t *testing.T) {
	srv, vaultID := setupMoveServer(t, "doc-success")

	createDoc(t, srv, vaultID, "/original.md", "# Original")

	resp, body := moveRequest(t, srv, vaultID, "/original.md", "/renamed.md", false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result moveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Type != "document" {
		t.Errorf("expected type=document, got %s", result.Type)
	}
	if result.Count != 1 {
		t.Errorf("expected count=1, got %d", result.Count)
	}

	// Verify the document moved
	ctx := context.Background()
	doc, err := testDB.GetFileByPath(ctx, vaultID, "/renamed.md")
	if err != nil {
		t.Fatalf("get renamed doc: %v", err)
	}
	if doc == nil {
		t.Error("expected document at /renamed.md")
	}

	old, err := testDB.GetFileByPath(ctx, vaultID, "/original.md")
	if err != nil {
		t.Fatalf("get original doc: %v", err)
	}
	if old != nil {
		t.Error("document should no longer exist at /original.md")
	}
}
