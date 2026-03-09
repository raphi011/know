package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/parser"
	knowhowdav "github.com/raphi011/knowhow/internal/webdav"
)

// setupWebDAV creates a vault, document service, and httptest.Server
// with WebDAV handler (noAuth mode). Returns the server and vault cleanup func.
func setupWebDAV(t *testing.T, suffix string) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()

	vaultID, vaultSvc := setupVault(t, ctx, "webdav-"+suffix+"-"+fmt.Sprint(time.Now().UnixNano()))
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil)

	handler := knowhowdav.NewHandler("/dav/", testDB, docSvc, vaultSvc, true, 1024*1024)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv, vaultID
}

// davURL builds a full URL for a WebDAV request.
func davURL(srv *httptest.Server, vaultName, path string) string {
	return srv.URL + "/dav/" + vaultName + path
}

func TestWebDAV_PutGetRoundtrip(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "roundtrip")
	vaultName := vaultNameFromID(vaultID)

	content := "# Hello\n\nThis is a test document.\n"
	url := davURL(srv, vaultName, "/test.md")

	// PUT
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(content))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 201 or 204", resp.StatusCode)
	}

	// GET
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET body: %v", err)
	}
	if string(body) != content {
		t.Errorf("GET body = %q, want %q", string(body), content)
	}
}

func TestWebDAV_PropfindDepth1(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "propfind")
	vaultName := vaultNameFromID(vaultID)

	// PUT two files
	for _, name := range []string{"/a.md", "/b.md"} {
		req, _ := http.NewRequest(http.MethodPut, davURL(srv, vaultName, name), strings.NewReader("# "+name))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", name, err)
		}
		resp.Body.Close()
	}

	// PROPFIND Depth:1 on root
	req, _ := http.NewRequest("PROPFIND", davURL(srv, vaultName, "/"), nil)
	req.Header.Set("Depth", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PROPFIND: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("PROPFIND status = %d, want 207", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "a.md") || !strings.Contains(bodyStr, "b.md") {
		t.Errorf("PROPFIND response missing file entries:\n%s", bodyStr)
	}
}

func TestWebDAV_Mkcol(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "mkcol")
	vaultName := vaultNameFromID(vaultID)

	// MKCOL
	req, _ := http.NewRequest("MKCOL", davURL(srv, vaultName, "/notes"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("MKCOL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("MKCOL status = %d, want 201", resp.StatusCode)
	}

	// PUT file inside
	req, _ = http.NewRequest(http.MethodPut, davURL(srv, vaultName, "/notes/doc.md"), strings.NewReader("# Doc"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// PROPFIND on folder
	req, _ = http.NewRequest("PROPFIND", davURL(srv, vaultName, "/notes"), nil)
	req.Header.Set("Depth", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PROPFIND /notes: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	if !strings.Contains(string(body), "doc.md") {
		t.Errorf("PROPFIND /notes missing doc.md:\n%s", string(body))
	}
}

func TestWebDAV_Delete(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "delete")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/deleteme.md")

	// PUT
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader("# Delete me"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// DELETE
	req, _ = http.NewRequest(http.MethodDelete, url, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}

	// GET should 404
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET after DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after DELETE status = %d, want 404", resp.StatusCode)
	}
}

func TestWebDAV_Move(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "move")
	vaultName := vaultNameFromID(vaultID)
	oldURL := davURL(srv, vaultName, "/old.md")
	newURL := davURL(srv, vaultName, "/new.md")

	// PUT at old path
	req, _ := http.NewRequest(http.MethodPut, oldURL, strings.NewReader("# Move me"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// MOVE
	req, _ = http.NewRequest("MOVE", oldURL, nil)
	req.Header.Set("Destination", newURL)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("MOVE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("MOVE status = %d, want 201 or 204", resp.StatusCode)
	}

	// Old path should 404
	resp, err = http.Get(oldURL)
	if err != nil {
		t.Fatalf("GET old: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET old status = %d, want 404", resp.StatusCode)
	}

	// New path should 200
	resp, err = http.Get(newURL)
	if err != nil {
		t.Fatalf("GET new: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET new status = %d, want 200", resp.StatusCode)
	}
}

func TestWebDAV_NonMarkdownRejected(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "nonmd")
	vaultName := vaultNameFromID(vaultID)

	req, _ := http.NewRequest(http.MethodPut, davURL(srv, vaultName, "/file.txt"), strings.NewReader("hello"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT .txt: %v", err)
	}
	resp.Body.Close()
	// The webdav library translates os.ErrPermission from OpenFile into a 404 on PUT.
	if resp.StatusCode < 400 {
		t.Errorf("PUT .txt status = %d, want error status", resp.StatusCode)
	}
}

func TestWebDAV_MacOSMetadata(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "macos")
	vaultName := vaultNameFromID(vaultID)

	// PUT ._foo (accepted but discarded)
	req, _ := http.NewRequest(http.MethodPut, davURL(srv, vaultName, "/._foo"), strings.NewReader("\x00\x05\x16"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT ._foo: %v", err)
	}
	resp.Body.Close()
	// Should succeed (silently accept)
	if resp.StatusCode >= 400 {
		t.Errorf("PUT ._foo status = %d, want success", resp.StatusCode)
	}

	// GET ._foo should 404 (not stored)
	resp, err = http.Get(davURL(srv, vaultName, "/._foo"))
	if err != nil {
		t.Fatalf("GET ._foo: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET ._foo status = %d, want 404", resp.StatusCode)
	}
}

func TestWebDAV_DepthInfinityRejected(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "depth-inf")
	vaultName := vaultNameFromID(vaultID)

	req, _ := http.NewRequest("PROPFIND", davURL(srv, vaultName, "/"), nil)
	req.Header.Set("Depth", "infinity")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PROPFIND: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("PROPFIND Depth:infinity status = %d, want 403", resp.StatusCode)
	}
}

func TestWebDAV_BodySizeLimit(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "sizelimit")
	vaultName := vaultNameFromID(vaultID)

	// setupWebDAV uses 1MB limit. Send 2MB.
	oversized := strings.Repeat("x", 2*1024*1024)
	req, _ := http.NewRequest(http.MethodPut, davURL(srv, vaultName, "/big.md"), strings.NewReader(oversized))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT oversized: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("PUT oversized status = %d, want 413", resp.StatusCode)
	}
}

func TestWebDAV_LockUnlock(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "lock")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/lockme.md")

	// PUT first so the resource exists
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader("# Lock test"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// LOCK
	lockBody := `<?xml version="1.0" encoding="utf-8"?>
<D:lockinfo xmlns:D="DAV:">
  <D:lockscope><D:exclusive/></D:lockscope>
  <D:locktype><D:write/></D:locktype>
  <D:owner><D:href>test</D:href></D:owner>
</D:lockinfo>`
	req, _ = http.NewRequest("LOCK", url, strings.NewReader(lockBody))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Timeout", "Second-60")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("LOCK: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("LOCK status = %d, want 200, body: %s", resp.StatusCode, body)
	}

	// Extract lock token from Lock-Token header
	lockToken := resp.Header.Get("Lock-Token")
	if lockToken == "" {
		t.Fatal("LOCK response missing Lock-Token header")
	}

	// UNLOCK
	req, _ = http.NewRequest("UNLOCK", url, nil)
	req.Header.Set("Lock-Token", lockToken)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("UNLOCK: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNoContent {
		t.Errorf("UNLOCK status = %d, want 204", resp2.StatusCode)
	}
}

// vaultNameFromID looks up the vault name from its record ID.
// The WebDAV handler resolves vaults by name, so we fetch the actual name from DB.
func vaultNameFromID(vaultID string) string {
	ctx := context.Background()
	v, err := testDB.GetVault(ctx, vaultID)
	if err != nil || v == nil {
		panic(fmt.Sprintf("vaultNameFromID: vault %s not found: %v", vaultID, err))
	}
	return v.Name
}
