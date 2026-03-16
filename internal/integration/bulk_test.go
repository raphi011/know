package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raphi011/know/internal/api"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/server"
)

// bulkResponse mirrors the server response structure.
type bulkResponse struct {
	Results []bulkResult `json:"results"`
	Error   string       `json:"error,omitempty"`
}

type bulkResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// setupBulkServer creates a vault and httptest.Server with the bulk upload endpoint.
func setupBulkServer(t *testing.T, suffix string) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()

	vaultID, vaultSvc := setupVault(t, ctx, "bulk-"+suffix+"-"+fmt.Sprint(time.Now().UnixNano()))
	fileSvc := file.NewService(testDB, nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	app := server.NewForTest(testDB, blob.NewFS(t.TempDir()), fileSvc, vaultSvc)
	apiSrv := api.NewServer(app)

	mux := http.NewServeMux()
	apiSrv.Register(mux, auth.NoAuthMiddleware)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, vaultID
}

// bulkUploadRequest builds and sends a multipart bulk upload request.
func bulkUploadRequest(t *testing.T, srv *httptest.Server, meta map[string]any, files map[string][]byte) bulkResponse {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Write meta part
	metaPart, err := writer.CreateFormField("meta")
	if err != nil {
		t.Fatalf("create meta part: %v", err)
	}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		t.Fatalf("encode meta: %v", err)
	}

	// Write file parts
	for path, data := range files {
		part, err := writer.CreateFormFile(path, path)
		if err != nil {
			t.Fatalf("create file part %s: %v", path, err)
		}
		if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
			t.Fatalf("copy file data %s: %v", path, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/bulk", &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result bulkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func findResult(results []bulkResult, path string) *bulkResult {
	for _, r := range results {
		if r.Path == path {
			return &r
		}
	}
	return nil
}

func TestBulkUpload_CreateDocuments(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "create")

	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/a.md": []byte("# Doc A\n\nContent A"),
		"/docs/b.md": []byte("# Doc B\n\nContent B"),
	})

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	for _, r := range resp.Results {
		if r.Status != "created" {
			t.Errorf("file %s: expected status 'created', got %q", r.Path, r.Status)
		}
	}

	// Verify documents exist in DB
	ctx := context.Background()
	for _, path := range []string{"/docs/a.md", "/docs/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc == nil {
			t.Errorf("doc %s should exist after bulk upload", path)
		}
	}
}

func TestBulkUpload_HashSkip(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "hash-skip")
	content := []byte("# Same Content\n\nThis won't change.")

	// First upload — creates the document
	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/stable.md": content,
	})

	if resp.Results[0].Status != "created" {
		t.Fatalf("first upload: expected 'created', got %q", resp.Results[0].Status)
	}

	// Second upload with same content — should skip with hash_match
	resp = bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/stable.md": content,
	})

	r := resp.Results[0]
	if r.Status != "skipped" || r.Reason != "hash_match" {
		t.Errorf("second upload: expected skipped/hash_match, got %s/%s", r.Status, r.Reason)
	}
}

func TestBulkUpload_ForceUpdate(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "force-update")

	// Create initial document
	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/changing.md": []byte("# Version 1"),
	})

	// Upload with different content and force=true — should update
	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/changing.md": []byte("# Version 2"),
	})

	r := resp.Results[0]
	if r.Status != "updated" {
		t.Errorf("force update: expected 'updated', got %q (reason: %s)", r.Status, r.Reason)
	}
}

func TestBulkUpload_NoForceSkipsExisting(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "no-force")

	// Create initial document
	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/existing.md": []byte("# Original"),
	})

	// Upload with different content but force=false — should skip with "exists"
	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   false,
	}, map[string][]byte{
		"/docs/existing.md": []byte("# Changed"),
	})

	r := resp.Results[0]
	if r.Status != "skipped" || r.Reason != "exists" {
		t.Errorf("no-force: expected skipped/exists, got %s/%s", r.Status, r.Reason)
	}

	// But hash_match should still work with force=false
	resp = bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   false,
	}, map[string][]byte{
		"/docs/existing.md": []byte("# Original"),
	})

	r = resp.Results[0]
	if r.Status != "skipped" || r.Reason != "hash_match" {
		t.Errorf("no-force same hash: expected skipped/hash_match, got %s/%s", r.Status, r.Reason)
	}
}

func TestBulkUpload_DryRun(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "dryrun")

	// Dry run on new documents — should report "created" without writing
	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
		"dryRun":  true,
	}, map[string][]byte{
		"/docs/new.md": []byte("# New Doc"),
	})

	if resp.Results[0].Status != "created" {
		t.Fatalf("dry run new: expected 'created', got %q", resp.Results[0].Status)
	}

	// Verify document was NOT actually created
	ctx := context.Background()
	doc, err := testDB.GetFileByPath(ctx, vaultID, "/docs/new.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc != nil {
		t.Error("dry run should not create documents")
	}

	// Create a real document, then dry run with different content and force=true
	bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/exists.md": []byte("# Existing"),
	})

	resp = bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
		"dryRun":  true,
	}, map[string][]byte{
		"/docs/exists.md": []byte("# Changed"),
	})

	if resp.Results[0].Status != "updated" {
		t.Errorf("dry run existing: expected 'updated', got %q", resp.Results[0].Status)
	}
}

func TestBulkUpload_MixedDocumentsAndAssets(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "mixed")

	// Upload a markdown document and a PNG asset in the same request
	pngData := []byte{0x89, 0x50, 0x4E, 0x47} // minimal PNG magic bytes
	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/docs/readme.md":  []byte("# Readme"),
		"/images/logo.png": pngData,
	})

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	docResult := findResult(resp.Results, "/docs/readme.md")
	assetResult := findResult(resp.Results, "/images/logo.png")

	if docResult == nil {
		t.Fatal("missing result for /docs/readme.md")
	}
	if assetResult == nil {
		t.Fatal("missing result for /images/logo.png")
	}

	if docResult.Status != "created" {
		t.Errorf("document: expected 'created', got %q", docResult.Status)
	}
	if assetResult.Status != "created" {
		t.Errorf("asset: expected 'created', got %q", assetResult.Status)
	}

	// Verify both exist
	ctx := context.Background()
	doc, err := testDB.GetFileByPath(ctx, vaultID, "/docs/readme.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("document should exist")
	}

	assetMeta, err := testDB.GetFileMetaByPath(ctx, vaultID, "/images/logo.png")
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if assetMeta == nil {
		t.Error("asset should exist")
	}
}

func TestBulkUpload_EmptyRequest(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "empty")

	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
	}, map[string][]byte{})

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestBulkUpload_MissingVaultID(t *testing.T) {
	srv, _ := setupBulkServer(t, "no-vault")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	metaPart, err := writer.CreateFormField("meta")
	if err != nil {
		t.Fatalf("create meta part: %v", err)
	}
	if err := json.NewEncoder(metaPart).Encode(map[string]any{}); err != nil {
		t.Fatalf("encode meta: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/bulk", &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBulkUpload_AssetHashSkip(t *testing.T) {
	srv, vaultID := setupBulkServer(t, "asset-hash")
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	// First upload — creates
	resp := bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/images/icon.png": pngData,
	})

	if resp.Results[0].Status != "created" {
		t.Fatalf("first upload: expected 'created', got %q", resp.Results[0].Status)
	}

	// Same content — should skip with hash_match
	resp = bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/images/icon.png": pngData,
	})

	r := resp.Results[0]
	if r.Status != "skipped" || r.Reason != "hash_match" {
		t.Errorf("asset re-upload: expected skipped/hash_match, got %s/%s", r.Status, r.Reason)
	}

	// Different content with force — should update
	resp = bulkUploadRequest(t, srv, map[string]any{
		"vaultId": vaultID,
		"force":   true,
	}, map[string][]byte{
		"/images/icon.png": []byte{0x89, 0x50, 0x4E, 0x47, 0xFF, 0xFF},
	})

	r = resp.Results[0]
	if r.Status != "updated" {
		t.Errorf("asset force update: expected 'updated', got %q", r.Status)
	}
}
