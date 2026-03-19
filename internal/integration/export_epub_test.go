package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestExportEPUB_SingleDocument(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "epub-single")

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/docs/hello.md": []byte("# Hello World\n\nSome content here."),
	})

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export/epub?path=/docs/hello.md")
	if err != nil {
		t.Fatalf("GET export/epub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/epub+zip" {
		t.Errorf("Content-Type = %q, want application/epub+zip", ct)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	assertValidEPUBZip(t, data)
}

func TestExportEPUB_FolderMultiChapter(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "epub-folder")

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/book/01-intro.md":      []byte("# Introduction\n\nWelcome."),
		"/book/02-chapter.md":    []byte("# Chapter 1\n\nContent."),
		"/book/03-conclusion.md": []byte("# Conclusion\n\nThe end."),
	})

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export/epub?path=/book/")
	if err != nil {
		t.Fatalf("GET export/epub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	assertValidEPUBZip(t, data)

	// Verify the EPUB contains multiple XHTML section files.
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	xhtmlCount := 0
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".xhtml") {
			xhtmlCount++
		}
	}
	// go-epub adds a title page + our 3 chapters = at least 3 section files.
	if xhtmlCount < 3 {
		t.Errorf("expected at least 3 xhtml files, got %d", xhtmlCount)
	}
}

func TestExportEPUB_WithImages(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "epub-images")

	// Minimal valid PNG (1x1 red pixel).
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
		0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00, 0x00, 0x00,
		0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/docs/with-image.md": []byte("# Doc\n\n![logo](/images/logo.png)\n"),
		"/images/logo.png":    pngData,
	})

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export/epub?path=/docs/with-image.md")
	if err != nil {
		t.Fatalf("GET export/epub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	assertValidEPUBZip(t, data)

	// Verify the EPUB contains the image.
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	found := false
	for _, f := range r.File {
		if strings.Contains(f.Name, ".png") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected image file in EPUB, none found")
	}
}

func TestExportEPUB_TitleAuthorOverride(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "epub-meta")

	bulkUploadRequest(t, srv, vaultName, map[string]any{
		"force": true,
	}, map[string][]byte{
		"/doc.md": []byte("# Original Title\n\nContent."),
	})

	params := url.Values{
		"path":   {"/doc.md"},
		"title":  {"Custom Title"},
		"author": {"Custom Author"},
	}
	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export/epub?" + params.Encode())
	if err != nil {
		t.Fatalf("GET export/epub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify Content-Disposition uses custom title.
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "Custom Title.epub") {
		t.Errorf("Content-Disposition = %q, want filename containing 'Custom Title.epub'", cd)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	assertValidEPUBZip(t, data)
}

func TestExportEPUB_MissingPath(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "epub-no-path")

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export/epub")
	if err != nil {
		t.Fatalf("GET export/epub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var errResp struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Detail == "" {
		t.Error("expected non-empty error detail")
	}
}

func TestExportEPUB_NonexistentPath(t *testing.T) {
	srv, _, vaultName := setupBulkServer(t, "epub-404")

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/export/epub?path=/nonexistent/")
	if err != nil {
		t.Fatalf("GET export/epub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

func assertValidEPUBZip(t *testing.T, data []byte) {
	t.Helper()

	if len(data) == 0 {
		t.Fatal("EPUB data is empty")
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid ZIP: %v", err)
	}

	foundContainer := false
	for _, f := range r.File {
		if f.Name == "META-INF/container.xml" {
			foundContainer = true
			break
		}
	}
	if !foundContainer {
		t.Error("missing META-INF/container.xml in EPUB")
	}
}
