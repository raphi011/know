package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// createTestArchive builds a tar.gz in memory with the given entries.
func createTestArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, data := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("write tar data %s: %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

// writeTestArchive writes a tar.gz to a temp file and returns its path.
func writeTestArchive(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	data := createTestArchive(t, entries)
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func TestCollectArchiveEntries_SupportedFiles(t *testing.T) {
	path := writeTestArchive(t, map[string][]byte{
		"docs/readme.md":   []byte("# Hello"),
		"docs/nested/a.md": []byte("# Nested"),
		"images/logo.png":  {0x89, 0x50, 0x4E, 0x47},
		"config.yaml":      []byte("key: value"),
		"data.json":        []byte("{}"),
		"notes/b.markdown": []byte("# B"),
	})

	entries, skipped, err := collectArchiveEntries(path)
	if err != nil {
		t.Fatalf("collectArchiveEntries: %v", err)
	}

	// Should include .md, .markdown, and .png; skip .yaml and .json
	if got := len(entries); got != 4 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.path
		}
		t.Fatalf("expected 4 entries, got %d: %v", got, names)
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", skipped)
	}

	// Verify data integrity
	found := make(map[string][]byte)
	for _, e := range entries {
		found[e.path] = e.data
	}
	if string(found["docs/readme.md"]) != "# Hello" {
		t.Errorf("readme content mismatch: %q", string(found["docs/readme.md"]))
	}
}

func TestCollectArchiveEntries_SkipsDirectories(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a directory entry
	if err := tw.WriteHeader(&tar.Header{
		Name:     "docs/",
		Typeflag: tar.TypeDir,
	}); err != nil {
		t.Fatalf("write dir header: %v", err)
	}

	// Add a regular file
	data := []byte("# Hello")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "docs/readme.md",
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write file header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write file data: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	entries, _, err := collectArchiveEntries(path)
	if err != nil {
		t.Fatalf("collectArchiveEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestCollectArchiveEntries_SkipsSymlinks(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a symlink entry with .md extension
	if err := tw.WriteHeader(&tar.Header{
		Name:     "link.md",
		Typeflag: tar.TypeSymlink,
		Linkname: "real.md",
	}); err != nil {
		t.Fatalf("write symlink header: %v", err)
	}

	// Add a regular file
	data := []byte("# Real")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "real.md",
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write file header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write file data: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	entries, _, err := collectArchiveEntries(path)
	if err != nil {
		t.Fatalf("collectArchiveEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (only real.md), got %d", len(entries))
	}
}

func TestCollectArchiveEntries_RejectsPathTraversal(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"dot-dot prefix", "../etc/passwd.md"},
		{"dot-dot nested", "docs/../../etc/shadow.md"},
		{"absolute path", "/etc/passwd.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestArchive(t, map[string][]byte{
				tt.path: []byte("malicious"),
			})

			_, _, err := collectArchiveEntries(path)
			if err == nil {
				t.Fatal("expected error for unsafe path, got nil")
			}
			if got := err.Error(); !bytes.Contains([]byte(got), []byte("unsafe path")) {
				t.Errorf("expected 'unsafe path' in error, got: %s", got)
			}
		})
	}
}

func TestCollectArchiveEntries_EmptyArchive(t *testing.T) {
	path := writeTestArchive(t, map[string][]byte{})

	entries, skipped, err := collectArchiveEntries(path)
	if err != nil {
		t.Fatalf("collectArchiveEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
}

func TestCollectArchiveEntries_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.tar.gz")
	if err := os.WriteFile(path, []byte("not a gzip file"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, _, err := collectArchiveEntries(path)
	if err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}

func TestIsArchiveFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"backup.tar.gz", true},
		{"backup.tgz", true},
		{"BACKUP.TAR.GZ", true},
		{"backup.TGZ", true},
		{"file.gz", false},
		{"file.tar", false},
		{"file.zip", false},
		{"file.md", false},
		{"path/to/archive.tar.gz", true},
	}
	for _, tt := range tests {
		if got := isArchiveFile(tt.path); got != tt.want {
			t.Errorf("isArchiveFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
