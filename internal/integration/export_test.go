package integration

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"testing"
)

func TestExport_DocumentsAndAssets(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "export")

	// Seed documents and an asset via bulk upload.
	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/docs/readme.md":   []byte("# Hello\n\nWorld"),
		"/docs/nested/a.md": []byte("# Nested"),
		"/images/logo.png":  {0x89, 0x50, 0x4E, 0x47},
	})

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export")
	if err != nil {
		t.Fatalf("GET export: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type = %q, want application/gzip", ct)
	}

	// Extract tar.gz and verify entries.
	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	entries := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar entry %s: %v", hdr.Name, err)
		}
		entries[hdr.Name] = data
	}

	// Verify documents (paths should have leading slash stripped).
	wantDocs := map[string]string{
		"docs/readme.md":   "# Hello\n\nWorld",
		"docs/nested/a.md": "# Nested",
	}
	for path, wantContent := range wantDocs {
		got, ok := entries[path]
		if !ok {
			t.Errorf("missing tar entry %q", path)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("entry %q: got %q, want %q", path, string(got), wantContent)
		}
	}

	// Verify asset.
	assetData, ok := entries["images/logo.png"]
	if !ok {
		t.Fatal("missing tar entry images/logo.png")
	}
	if len(assetData) != 4 || assetData[0] != 0x89 {
		t.Errorf("asset data mismatch: got %v", assetData)
	}

	// Should have exactly 3 entries.
	if len(entries) != 3 {
		t.Errorf("expected 3 tar entries, got %d: %v", len(entries), keys(entries))
	}
}

func TestExport_EmptyVault(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "export-empty")

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export")
	if err != nil {
		t.Fatalf("GET export: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	count := 0
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 tar entries for empty vault, got %d", count)
	}
}

func TestExport_NonexistentVault(t *testing.T) {
	srv, _, _ := setupBulkServer(t, "export-no-vault")

	resp, err := http.Get(srv.URL + "/api/v1/vaults/nonexistent/export")
	if err != nil {
		t.Fatalf("GET export: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func keys[V any](m map[string]V) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
