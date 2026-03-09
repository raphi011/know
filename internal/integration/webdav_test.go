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

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/vault"
	knowhowdav "github.com/raphi011/knowhow/internal/webdav"
)

// mustNewRequest creates an HTTP request, failing the test on error.
func mustNewRequest(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	return req
}

// mustReadAll reads all bytes from r, failing the test on error.
func mustReadAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}

// requireStatus fails the test if resp status doesn't match expected.
func requireStatus(t *testing.T, resp *http.Response, label string, expected ...int) {
	t.Helper()
	for _, code := range expected {
		if resp.StatusCode == code {
			return
		}
	}
	t.Fatalf("%s status = %d, want one of %v", label, resp.StatusCode, expected)
}

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
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader(content))
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
		req := mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, name), strings.NewReader("# "+name))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", name, err)
		}
		resp.Body.Close()
	}

	// PROPFIND Depth:1 on root
	req := mustNewRequest(t, "PROPFIND", davURL(srv, vaultName, "/"), nil)
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
	req := mustNewRequest(t, "MKCOL", davURL(srv, vaultName, "/notes"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("MKCOL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("MKCOL status = %d, want 201", resp.StatusCode)
	}

	// PUT file inside
	req = mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, "/notes/doc.md"), strings.NewReader("# Doc"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// PROPFIND on folder
	req = mustNewRequest(t, "PROPFIND", davURL(srv, vaultName, "/notes"), nil)
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
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Delete me"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// DELETE
	req = mustNewRequest(t, http.MethodDelete, url, nil)
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
	req := mustNewRequest(t, http.MethodPut, oldURL, strings.NewReader("# Move me"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()

	// MOVE
	req = mustNewRequest(t, "MOVE", oldURL, nil)
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

	req := mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, "/file.txt"), strings.NewReader("hello"))
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
	req := mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, "/._foo"), strings.NewReader("\x00\x05\x16"))
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

	req := mustNewRequest(t, "PROPFIND", davURL(srv, vaultName, "/"), nil)
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
	req := mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, "/big.md"), strings.NewReader(oversized))
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
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Lock test"))
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
	req = mustNewRequest(t, "LOCK", url, strings.NewReader(lockBody))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Timeout", "Second-60")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("LOCK: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body := mustReadAll(t, resp.Body)
		t.Fatalf("LOCK status = %d, want 200, body: %s", resp.StatusCode, body)
	}

	// Extract lock token from Lock-Token header
	lockToken := resp.Header.Get("Lock-Token")
	if lockToken == "" {
		t.Fatal("LOCK response missing Lock-Token header")
	}

	// UNLOCK
	req = mustNewRequest(t, "UNLOCK", url, nil)
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

// setupWebDAVWithAuth creates a vault with real token-based auth.
// Returns server, vault name, and raw token for Basic Auth.
func setupWebDAVWithAuth(t *testing.T, suffix string) (*httptest.Server, string, string) {
	t.Helper()
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "webdav-auth-user-" + suffix + "-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "webdav-auth-" + suffix + "-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	// Create vault membership (write role)
	_, err = testDB.CreateVaultMember(ctx, userID, vaultID, models.RoleWrite)
	if err != nil {
		t.Fatalf("create vault member: %v", err)
	}

	// Create API token
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	_, err = testDB.CreateToken(ctx, userID, tokenHash, "webdav-test-token")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil)
	handler := knowhowdav.NewHandler("/dav/", testDB, docSvc, vaultSvc, false, 1024*1024)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv, v.Name, rawToken
}

func TestWebDAV_PutOverwrite(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "overwrite")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/overwrite.md")

	// PUT v1
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Version 1"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT v1: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT v1", http.StatusCreated, http.StatusNoContent)

	// PUT v2 (overwrite)
	req = mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Version 2"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT v2: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT v2 status = %d, want 201 or 204", resp.StatusCode)
	}

	// GET — should contain only v2 content (PUT is a full replacement)
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body := mustReadAll(t, resp.Body)
	if !strings.Contains(string(body), "# Version 2") {
		t.Errorf("GET body should contain v2 content, got %q", string(body))
	}
	if strings.Contains(string(body), "# Version 1") {
		t.Errorf("GET body should NOT contain v1 content after overwrite, got %q", string(body))
	}
}

func TestWebDAV_ETagReturned(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "etag")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/etag.md")

	// PUT
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# ETag test"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT", http.StatusCreated, http.StatusNoContent)

	// GET and check ETag
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Error("GET response missing ETag header")
	}
}

func TestWebDAV_IfMatchSuccess(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "ifmatch-ok")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/ifmatch.md")

	// PUT initial
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# If-Match"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT initial", http.StatusCreated, http.StatusNoContent)

	// GET to grab ETag
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag returned")
	}

	// PUT with correct If-Match
	req = mustNewRequest(t, http.MethodPut, url, strings.NewReader("# If-Match Updated"))
	req.Header.Set("If-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT with If-Match: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Errorf("PUT with correct If-Match status = %d, want success", resp.StatusCode)
	}
}

func TestWebDAV_IfMatchStale(t *testing.T) {
	// NOTE: The x/net/webdav library does not enforce If-Match preconditions
	// on PUT requests. The ETag returned via the ETager interface is used for
	// PROPFIND/GET responses only. If-Match enforcement would require custom
	// middleware in the handler. This test documents the current behavior:
	// stale If-Match headers are silently ignored on PUT.
	srv, vaultID := setupWebDAV(t, "ifmatch-stale")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/stale.md")

	// PUT v1
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# V1"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT v1: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT v1", http.StatusCreated, http.StatusNoContent)

	// GET to grab ETag
	resp, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	oldEtag := resp.Header.Get("ETag")
	if oldEtag == "" {
		t.Fatal("no ETag returned")
	}

	// PUT v2 (changes content, invalidates old ETag)
	req = mustNewRequest(t, http.MethodPut, url, strings.NewReader("# V2"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT v2: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT v2", http.StatusCreated, http.StatusNoContent)

	// PUT v3 with stale ETag — currently succeeds because x/net/webdav
	// doesn't enforce If-Match on PUT
	req = mustNewRequest(t, http.MethodPut, url, strings.NewReader("# V3"))
	req.Header.Set("If-Match", oldEtag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT with stale If-Match: %v", err)
	}
	resp.Body.Close()
	// x/net/webdav ignores If-Match on PUT, so it succeeds
	if resp.StatusCode >= 400 {
		t.Errorf("PUT with stale If-Match status = %d, expected success (library doesn't enforce If-Match on PUT)", resp.StatusCode)
	}
}

func TestWebDAV_FolderDeleteCascade(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "folder-cascade")
	vaultName := vaultNameFromID(vaultID)

	// MKCOL /cascade/
	req := mustNewRequest(t, "MKCOL", davURL(srv, vaultName, "/cascade"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("MKCOL: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "MKCOL", http.StatusCreated)

	// PUT files inside
	for _, name := range []string{"/cascade/a.md", "/cascade/b.md"} {
		req = mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, name), strings.NewReader("# "+name))
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", name, err)
		}
		resp.Body.Close()
		requireStatus(t, resp, "PUT "+name, http.StatusCreated, http.StatusNoContent)
	}

	// DELETE /cascade/
	req = mustNewRequest(t, http.MethodDelete, davURL(srv, vaultName, "/cascade/"), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /cascade/: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 204 or 200", resp.StatusCode)
	}

	// Verify files are gone
	for _, name := range []string{"/cascade/a.md", "/cascade/b.md"} {
		resp, err = http.Get(davURL(srv, vaultName, name))
		if err != nil {
			t.Fatalf("GET %s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s after folder delete status = %d, want 404", name, resp.StatusCode)
		}
	}
}

func TestWebDAV_CopyFile(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "copy")
	vaultName := vaultNameFromID(vaultID)
	srcURL := davURL(srv, vaultName, "/original.md")
	dstURL := davURL(srv, vaultName, "/copied.md")

	// PUT original
	req := mustNewRequest(t, http.MethodPut, srcURL, strings.NewReader("# Original"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT original", http.StatusCreated, http.StatusNoContent)

	// COPY
	req = mustNewRequest(t, "COPY", srcURL, nil)
	req.Header.Set("Destination", dstURL)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("COPY: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("COPY status = %d, want 201 or 204", resp.StatusCode)
	}

	// Both paths should return content
	for _, u := range []string{srcURL, dstURL} {
		resp, err = http.Get(u)
		if err != nil {
			t.Fatalf("GET %s: %v", u, err)
		}
		body := mustReadAll(t, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", u, resp.StatusCode)
		}
		if !strings.Contains(string(body), "Original") {
			t.Errorf("GET %s body missing content: %q", u, string(body))
		}
	}
}

func TestWebDAV_Auth_ValidToken(t *testing.T) {
	srv, vaultName, rawToken := setupWebDAVWithAuth(t, "valid")
	url := davURL(srv, vaultName, "/authed.md")

	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Authenticated"))
	req.SetBasicAuth("", rawToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Errorf("PUT with valid token status = %d, want success", resp.StatusCode)
	}
}

func TestWebDAV_Auth_InvalidToken(t *testing.T) {
	srv, vaultName, _ := setupWebDAVWithAuth(t, "invalid")
	url := davURL(srv, vaultName, "/rejected.md")

	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Rejected"))
	req.SetBasicAuth("", "kh_invalid_token_000000000000000000000000000000000000000000000000000000000000")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("PUT with invalid token status = %d, want 401", resp.StatusCode)
	}
}

func TestWebDAV_Auth_MissingAuth(t *testing.T) {
	srv, vaultName, _ := setupWebDAVWithAuth(t, "missing")
	url := davURL(srv, vaultName, "/noauth.md")

	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# No Auth"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("PUT without auth status = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("missing WWW-Authenticate header on 401 response")
	}
}

func TestWebDAV_PropfindDepth0(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "depth0")
	vaultName := vaultNameFromID(vaultID)
	url := davURL(srv, vaultName, "/depth0.md")

	// PUT
	req := mustNewRequest(t, http.MethodPut, url, strings.NewReader("# Depth 0"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "PUT", http.StatusCreated, http.StatusNoContent)

	// PROPFIND Depth:0 on the file
	req = mustNewRequest(t, "PROPFIND", url, nil)
	req.Header.Set("Depth", "0")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PROPFIND: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("PROPFIND Depth:0 status = %d, want 207", resp.StatusCode)
	}
	body := mustReadAll(t, resp.Body)
	if !strings.Contains(string(body), "depth0.md") {
		t.Errorf("PROPFIND Depth:0 response missing file: %s", string(body))
	}
}

func TestWebDAV_MoveFolder(t *testing.T) {
	srv, vaultID := setupWebDAV(t, "move-folder")
	vaultName := vaultNameFromID(vaultID)

	// Create folder with docs
	req := mustNewRequest(t, "MKCOL", davURL(srv, vaultName, "/moveme"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("MKCOL: %v", err)
	}
	resp.Body.Close()
	requireStatus(t, resp, "MKCOL", http.StatusCreated)

	for _, name := range []string{"/moveme/x.md", "/moveme/y.md"} {
		req = mustNewRequest(t, http.MethodPut, davURL(srv, vaultName, name), strings.NewReader("# "+name))
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", name, err)
		}
		resp.Body.Close()
		requireStatus(t, resp, "PUT "+name, http.StatusCreated, http.StatusNoContent)
	}

	// MOVE folder
	req = mustNewRequest(t, "MOVE", davURL(srv, vaultName, "/moveme"), nil)
	req.Header.Set("Destination", davURL(srv, vaultName, "/moved"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("MOVE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("MOVE status = %d, want 201 or 204", resp.StatusCode)
	}

	// Old paths should 404
	for _, name := range []string{"/moveme/x.md", "/moveme/y.md"} {
		resp, err = http.Get(davURL(srv, vaultName, name))
		if err != nil {
			t.Fatalf("GET %s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET old %s status = %d, want 404", name, resp.StatusCode)
		}
	}

	// New paths should exist
	for _, name := range []string{"/moved/x.md", "/moved/y.md"} {
		resp, err = http.Get(davURL(srv, vaultName, name))
		if err != nil {
			t.Fatalf("GET %s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET new %s status = %d, want 200", name, resp.StatusCode)
		}
	}
}
