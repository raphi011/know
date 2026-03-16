package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
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

// initGitRepo creates a git repo in dir with the given files committed,
// plus an optional .gitignore.
func initGitRepo(t *testing.T, dir string, files map[string]string, gitignore string) {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")

	if gitignore != "" {
		writeFile(t, filepath.Join(dir, ".gitignore"), gitignore)
	}

	for path, content := range files {
		abs := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, abs, content)
	}

	run("git", "add", "-A")
	run("git", "commit", "-m", "init", "--no-gpg-sign")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectImportFiles_GitIgnore(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"notes/hello.md":  "# Hello",
		"notes/world.md":  "# World",
		"images/logo.png": "png-data",
	}, "ignored/\n")

	// Add an ignored directory with a markdown file after commit.
	ignoredDir := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ignoredDir, "skip.md"), "# Skipped")

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(dir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// Should include hello.md, world.md, logo.png — NOT skip.md
	if got := len(files); got != 3 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = filepath.Base(f)
		}
		t.Fatalf("expected 3 files, got %d: %v", got, names)
	}

	for _, f := range files {
		if filepath.Base(f) == "skip.md" {
			t.Error("gitignored file skip.md should not be included")
		}
	}
}

func TestCollectImportFiles_GitTrackedDotfile(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"notes/hello.md":          "# Hello",
		".special/tracked-dot.md": "# Tracked dotfile",
	}, "")

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(dir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// Git tracks .special/tracked-dot.md, so it should be included.
	var found bool
	for _, f := range files {
		if filepath.Base(f) == "tracked-dot.md" {
			found = true
		}
	}
	if !found {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f
		}
		t.Fatalf("tracked dotfile should be included, got: %v", names)
	}
}

func TestCollectImportFiles_NonGitSkipsDotfiles(t *testing.T) {
	dir := t.TempDir()

	// Create files without git init.
	for path, content := range map[string]string{
		"notes/hello.md":           "# Hello",
		".obsidian/templates/d.md": "# Template",
		".hidden.md":               "# Hidden",
	} {
		abs := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, abs, content)
	}

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(dir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	if got := len(files); got != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f
		}
		t.Fatalf("expected 1 file (hello.md), got %d: %v", got, names)
	}
	if filepath.Base(files[0]) != "hello.md" {
		t.Errorf("expected hello.md, got %s", filepath.Base(files[0]))
	}
}

func TestCollectImportFiles_NoIgnoreIncludesAll(t *testing.T) {
	dir := t.TempDir()

	for path, content := range map[string]string{
		"notes/hello.md":           "# Hello",
		".obsidian/templates/d.md": "# Template",
		".hidden.md":               "# Hidden",
	} {
		abs := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, abs, content)
	}

	importNoIgnore = true
	defer func() { importNoIgnore = false }()

	files, _, _, err := collectImportFiles(dir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// Should include all 3 markdown files.
	if got := len(files); got != 3 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f
		}
		t.Fatalf("expected 3 files, got %d: %v", got, names)
	}
}

func TestCollectImportFiles_GitNonRecursive(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"top.md":           "# Top",
		"sub/nested.md":    "# Nested",
		"sub/deep/deep.md": "# Deep",
	}, "")

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(dir, false)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// Non-recursive: only top.md.
	if got := len(files); got != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f
		}
		t.Fatalf("expected 1 file (top.md), got %d: %v", got, names)
	}
	if filepath.Base(files[0]) != "top.md" {
		t.Errorf("expected top.md, got %s", filepath.Base(files[0]))
	}
}

func TestCollectImportFiles_NoIgnoreInGitRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"hello.md": "# Hello",
	}, "ignored/\n")

	// Add an ignored directory with a markdown file.
	ignoredDir := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ignoredDir, "secret.md"), "# Secret")

	oldNoIgnore := importNoIgnore
	importNoIgnore = true
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(dir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// --no-ignore should include the gitignored file.
	if got := len(files); got != 2 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = filepath.Base(f)
		}
		t.Fatalf("expected 2 files (hello.md, secret.md), got %d: %v", got, names)
	}
}

func TestCollectImportFiles_GitUntrackedFile(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"committed.md": "# Committed",
	}, "")

	// Add an untracked file after commit — should be included.
	writeFile(t, filepath.Join(dir, "untracked.md"), "# Untracked")

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(dir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	if got := len(files); got != 2 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = filepath.Base(f)
		}
		t.Fatalf("expected 2 files (committed.md, untracked.md), got %d: %v", got, names)
	}
}

func TestCollectImportFiles_GitSubdirectory(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"top.md":           "# Top",
		"sub/nested.md":    "# Nested",
		"sub/deep/deep.md": "# Deep",
		"other/other.md":   "# Other",
	}, "")

	// Import only the sub/ directory.
	subDir := filepath.Join(dir, "sub")

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, _, _, err := collectImportFiles(subDir, true)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// Should include nested.md and deep.md — NOT top.md or other.md.
	if got := len(files); got != 2 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = filepath.Base(f)
		}
		t.Fatalf("expected 2 files (nested.md, deep.md), got %d: %v", got, names)
	}

	for _, f := range files {
		base := filepath.Base(f)
		if base != "nested.md" && base != "deep.md" {
			t.Errorf("unexpected file: %s", f)
		}
	}
}

func TestCollectImportFiles_NonGitNonRecursiveSkipsDotfiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "visible.md"), "# Visible")
	writeFile(t, filepath.Join(dir, ".hidden.md"), "# Hidden")
	if err := os.MkdirAll(filepath.Join(dir, ".dotdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldNoIgnore := importNoIgnore
	importNoIgnore = false
	defer func() { importNoIgnore = oldNoIgnore }()

	files, skippedDirs, _, err := collectImportFiles(dir, false)
	if err != nil {
		t.Fatalf("collectImportFiles: %v", err)
	}

	// Should include only visible.md.
	if got := len(files); got != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = filepath.Base(f)
		}
		t.Fatalf("expected 1 file (visible.md), got %d: %v", got, names)
	}
	if filepath.Base(files[0]) != "visible.md" {
		t.Errorf("expected visible.md, got %s", filepath.Base(files[0]))
	}
	// .dotdir and subdir should both be counted as skipped.
	if skippedDirs != 2 {
		t.Errorf("expected 2 skipped dirs (.dotdir + subdir), got %d", skippedDirs)
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
