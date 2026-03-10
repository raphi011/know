# WebDAV Performance Optimization — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reduce WebDAV per-request latency to speed up macOS Finder bulk uploads by ~30-50%.

**Architecture:** Two changes: (1) short-circuit OS metadata file requests before auth/vault resolution in the handler, (2) cache folder existence in `db.Client` to skip redundant `EnsureFolders` DB roundtrips.

**Tech Stack:** Go, `x/net/webdav`, `sync.Map`

---

### Task 1: Fast-path OS metadata files in handler

54% of all WebDAV requests are for `._*` / `.DS_Store` files. Currently these traverse the full middleware: auth DB lookup → vault resolution → role check → filesystem `Stat()` / `OpenFile()`. Short-circuit them right after vault name extraction, before auth.

**Files:**
- Modify: `internal/webdav/handler.go:43-136`
- Modify: `internal/webdav/fs.go:320-323` (move `isOSMetadataFile` or export it)
- Test: `internal/webdav/handler_test.go` (new file)

**Step 1: Write the failing test**

Create `internal/webdav/handler_test.go`:

```go
package webdav

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_OSMetadataFastPath(t *testing.T) {
	// Handler with nil dependencies — if the fast path works,
	// these are never touched and we don't panic.
	handler := NewHandler("/dav/", nil, nil, nil, true, 0)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		// PROPFIND on ._ files should return 404 without hitting auth
		{"PROPFIND", "/dav/somevault/._file.md", http.StatusNotFound},
		{"PROPFIND", "/dav/somevault/._.", http.StatusNotFound},
		{"PROPFIND", "/dav/somevault/.DS_Store", http.StatusNotFound},
		// PUT on ._ files should return 201 (accepted, discarded)
		{"PUT", "/dav/somevault/._file.md", http.StatusCreated},
		{"PUT", "/dav/somevault/.DS_Store", http.StatusCreated},
		// LOCK on ._ files should return 200
		{"LOCK", "/dav/somevault/._file.md", http.StatusOK},
		// UNLOCK on ._ files should return 204
		{"UNLOCK", "/dav/somevault/._file.md", http.StatusNoContent},
		// DELETE on ._ files should return 204
		{"DELETE", "/dav/somevault/._file.md", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestHandler_RealFilesNotShortCircuited(t *testing.T) {
	// A request for a real .md file with nil deps should panic or error,
	// proving the fast path did NOT intercept it.
	handler := NewHandler("/dav/", nil, nil, nil, true, 0)
	req := httptest.NewRequest("PROPFIND", "/dav/somevault/readme.md", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r == nil && rec.Code == http.StatusNotFound {
			t.Error("real .md file was short-circuited — fast path is too broad")
		}
	}()

	handler.ServeHTTP(rec, req)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false -run TestHandler_OSMetadata ./internal/webdav/ -v`
Expected: FAIL — handler passes nil deps through to auth, panics.

**Step 3: Implement the fast path**

In `handler.go`, after extracting `vaultName` (line 55) and before auth (line 57), add the metadata short-circuit. Extract the file path from the URL, check `isOSMetadataFile`, and return immediately with appropriate status codes:

```go
		// Fast-path: short-circuit OS metadata files (._*, .DS_Store) before auth.
		// These are 54% of Finder's requests and never touch real data.
		filePath := ""
		if len(parts) > 1 {
			filePath = "/" + parts[1]
		}
		if filePath != "" && isOSMetadataFile(filePath) {
			switch r.Method {
			case "PROPFIND", http.MethodGet, http.MethodHead:
				http.Error(w, "not found", http.StatusNotFound)
			case http.MethodPut:
				// Drain body so connection stays clean for keep-alive
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusCreated)
			case "LOCK":
				w.WriteHeader(http.StatusOK)
			case "UNLOCK", http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
```

Add `"io"` to the import list.

**Step 4: Run tests to verify they pass**

Run: `go test -buildvcs=false -run TestHandler_ ./internal/webdav/ -v`
Expected: All PASS.

**Step 5: Run full webdav package tests**

Run: `go test -buildvcs=false ./internal/webdav/ -v`
Expected: All PASS.

**Step 6: Commit**

```bash
git add internal/webdav/handler.go internal/webdav/handler_test.go
git commit -m "perf: fast-path OS metadata files in WebDAV handler before auth"
```

---

### Task 2: Add folder existence cache to db.Client

`EnsureFolders` does a DB roundtrip per `document.Service.Create()`. When uploading 51 files to the same directory, all 51 calls are identical. Add an in-memory cache to skip the DB call when the folder was recently ensured.

**Files:**
- Modify: `internal/db/queries_folder.go:42-48` (`EnsureFolders` method)
- Modify: `internal/db/client.go` (add cache field)
- Test: `internal/db/queries_folder_test.go`

**Step 1: Write the failing test**

Add to `internal/db/queries_folder_test.go`:

```go
func TestEnsureFolders_CachesRecentCalls(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	vaultID := createTestVault(t, ctx, user)

	docPath := "/" + t.Name() + "/sub/doc.md"

	// First call should succeed (hits DB)
	if err := testDB.EnsureFolders(ctx, vaultID, docPath); err != nil {
		t.Fatalf("first EnsureFolders: %v", err)
	}

	// Verify folders were created
	folders, err := testDB.ListFolders(ctx, vaultID)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) == 0 {
		t.Fatal("expected folders after EnsureFolders")
	}

	// Second call for same path should succeed (from cache, no DB error even if DB fails)
	if err := testDB.EnsureFolders(ctx, vaultID, docPath); err != nil {
		t.Fatalf("second EnsureFolders (cached): %v", err)
	}

	// Call for different file in same dir should also be cached
	if err := testDB.EnsureFolders(ctx, vaultID, "/"+t.Name()+"/sub/other.md"); err != nil {
		t.Fatalf("third EnsureFolders (same dir, cached): %v", err)
	}
}
```

**Step 2: Run test to verify it passes (baseline)**

Run: `go test -buildvcs=false -run TestEnsureFolders_Caches ./internal/db/ -v`
Expected: PASS (the test itself passes even without cache — it just exercises the code path. The cache makes it faster, not different in behavior.)

**Step 3: Add folder cache to Client**

In `internal/db/client.go`, add a `folderCache` field:

```go
type Client struct {
	db         *surrealdb.DB
	folderCache sync.Map // key: "vaultID:folderPath" → value: time.Time (expiry)
}
```

Add `"sync"` and `"time"` to imports.

**Step 4: Modify EnsureFolders to check cache**

In `internal/db/queries_folder.go`, update `EnsureFolders`:

```go
func (c *Client) EnsureFolders(ctx context.Context, vaultID, docPath string) error {
	docPath = models.NormalizePath(docPath)
	dir := path.Dir(docPath)
	if dir == "/" || dir == "." {
		return nil // root-level document, no folders to create
	}

	// Check cache — skip DB call if this folder was recently ensured
	cacheKey := vaultID + ":" + dir
	if expiry, ok := c.folderCache.Load(cacheKey); ok {
		if expiry.(time.Time).After(time.Now()) {
			return nil
		}
		c.folderCache.Delete(cacheKey)
	}

	if err := c.EnsureFolderPath(ctx, vaultID, dir); err != nil {
		return err
	}

	// Cache for 60 seconds
	c.folderCache.Store(cacheKey, time.Now().Add(60*time.Second))
	return nil
}
```

Add `"time"` to imports if not already present.

**Step 5: Add cache invalidation on folder delete**

In `internal/db/queries_folder.go`, find the `DeleteFolder` method and add cache invalidation. Also add a helper method:

```go
// InvalidateFolderCache removes cached folder entries for a vault+path prefix.
func (c *Client) InvalidateFolderCache(vaultID, folderPath string) {
	c.folderCache.Range(func(key, _ any) bool {
		k := key.(string)
		prefix := vaultID + ":" + folderPath
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			c.folderCache.Delete(key)
		}
		return true
	})
}
```

Call `c.InvalidateFolderCache(vaultID, folderPath)` at the start of `DeleteFolder` and `MoveFoldersByPrefix`.

**Step 6: Run all folder tests**

Run: `go test -buildvcs=false -run TestEnsureFolder ./internal/db/ -v`
Expected: All PASS.

**Step 7: Run full test suite**

Run: `go test -buildvcs=false ./... -v 2>&1 | tail -30`
Expected: All PASS.

**Step 8: Commit**

```bash
git add internal/db/queries_folder.go internal/db/queries_folder_test.go internal/db/client.go
git commit -m "perf: cache folder existence to skip redundant EnsureFolders DB calls"
```

---

### Task 3: Manual verification with Finder

**Step 1: Rebuild and restart**

Run: `just dev`

**Step 2: Clear old debug log and copy files**

```bash
: > /tmp/dav-debug.log
```

Copy the same ~50 files via Finder.

**Step 3: Compare timing**

Read `/tmp/dav-debug.log` and compare:
- Total wall-clock time (was 9m 15s)
- Average per-request latency (was 52ms)
- Number of requests hitting auth (should be ~50% fewer)

**Step 4: Commit debug log analysis as comment in design doc (optional)**

Update `docs/plans/2026-03-09-webdav-performance-design.md` with results.
