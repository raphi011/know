package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestLs_NonRecursiveRoot(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "ls-root")

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/readme.md":        []byte("# Readme"),
		"/docs/guide.md":    []byte("# Guide"),
		"/docs/sub/deep.md": []byte("# Deep"),
		"/notes/todo.md":    []byte("# Todo"),
	})

	entries := lsRequest(t, srv, vaultName, "/", false)

	// Should list direct children only: readme.md + folders docs/ and notes/
	names := entryNames(entries)
	sort.Strings(names)

	wantNames := []string{"docs", "notes", "readme.md"}
	if len(names) != len(wantNames) {
		t.Fatalf("expected %d entries, got %d: %v", len(wantNames), len(names), names)
	}
	for i, want := range wantNames {
		if names[i] != want {
			t.Errorf("entry[%d]: got %q, want %q", i, names[i], want)
		}
	}

	// Verify isDir flags
	dirMap := make(map[string]bool)
	for _, e := range entries {
		dirMap[e.Name] = e.IsDir
	}
	if !dirMap["docs"] {
		t.Error("docs should be a directory")
	}
	if !dirMap["notes"] {
		t.Error("notes should be a directory")
	}
	if dirMap["readme.md"] {
		t.Error("readme.md should not be a directory")
	}
}

func TestLs_NonRecursiveSubfolder(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "ls-subfolder")

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/docs/guide.md":    []byte("# Guide"),
		"/docs/setup.md":    []byte("# Setup"),
		"/docs/sub/deep.md": []byte("# Deep"),
	})

	entries := lsRequest(t, srv, vaultName, "/docs", false)

	names := entryNames(entries)
	sort.Strings(names)

	// Should list guide.md, setup.md, and sub/ folder — NOT deep.md
	wantNames := []string{"guide.md", "setup.md", "sub"}
	if len(names) != len(wantNames) {
		t.Fatalf("expected %d entries, got %d: %v", len(wantNames), len(names), names)
	}
	for i, want := range wantNames {
		if names[i] != want {
			t.Errorf("entry[%d]: got %q, want %q", i, names[i], want)
		}
	}
}

func TestLs_Recursive(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "ls-recursive")

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/docs/guide.md":    []byte("# Guide"),
		"/docs/sub/deep.md": []byte("# Deep"),
	})

	entries := lsRequest(t, srv, vaultName, "/docs", true)

	paths := entryPaths(entries)
	sort.Strings(paths)

	// Should include the sub folder and both documents
	wantPaths := []string{"/docs/guide.md", "/docs/sub", "/docs/sub/deep.md"}
	if len(paths) != len(wantPaths) {
		t.Fatalf("expected %d entries, got %d: %v", len(wantPaths), len(paths), paths)
	}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Errorf("entry[%d]: got %q, want %q", i, paths[i], want)
		}
	}
}

func TestLs_PrefixIsolation(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "ls-prefix")

	// Create docs under /docs and /docs-other — they share the "/docs" prefix
	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/docs/a.md":       []byte("# A"),
		"/docs-other/b.md": []byte("# B"),
	})

	// Non-recursive ls of /docs should NOT include /docs-other/b.md
	entries := lsRequest(t, srv, vaultName, "/docs", false)
	for _, e := range entries {
		if e.Name == "b.md" || e.Path == "/docs-other/b.md" {
			t.Errorf("ls /docs should not include /docs-other entries: got %+v", e)
		}
	}
	if len(entries) != 1 || entries[0].Name != "a.md" {
		t.Errorf("expected [a.md], got %v", entryNames(entries))
	}

	// Recursive ls of /docs should also not include /docs-other
	entries = lsRequest(t, srv, vaultName, "/docs", true)
	for _, e := range entries {
		if e.Name == "b.md" || e.Path == "/docs-other/b.md" {
			t.Errorf("recursive ls /docs should not include /docs-other entries: got %+v", e)
		}
	}
}

func TestLs_EmptyVault(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "ls-empty")

	entries := lsRequest(t, srv, vaultName, "/", false)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty vault, got %d", len(entries))
	}
}

func TestLs_NonexistentVault(t *testing.T) {
	srv, _, _ := setupBulkServer(t, "ls-no-vault")

	resp, err := http.Get(srv.URL + "/api/v1/vaults/nonexistent/documents/ls?path=/")
	if err != nil {
		t.Fatalf("GET ls: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// lsRequest performs a GET /api/v1/vaults/{vault}/documents/ls and returns the parsed entries.
func lsRequest(t *testing.T, srv *httptest.Server, vaultName, path string, recursive bool) []models.FileEntry {
	t.Helper()

	u := srv.URL + "/api/v1/vaults/" + vaultName + "/documents/ls?path=" + path
	if recursive {
		u += "&recursive=true"
	}

	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET ls: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var entries []models.FileEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return entries
}

func entryNames(entries []models.FileEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

func entryPaths(entries []models.FileEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths
}
