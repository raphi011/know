package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	hash := "abc123"
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID:     vaultID,
		Path:        "/docs/hello.md",
		Title:       "Hello World",
		Content:     "---\ntitle: Hello\n---\nHello world content",
		Labels:      []string{"test", "greeting"},
		ContentHash: &hash,
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	if doc.Title != "Hello World" {
		t.Errorf("Expected title 'Hello World', got %q", doc.Title)
	}
	if doc.Path != "/docs/hello.md" {
		t.Errorf("Expected path '/docs/hello.md', got %q", doc.Path)
	}
	if len(doc.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(doc.Labels))
	}
}

func TestGetFileByPath(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/unique-path.md",
		Title:   "Unique Path Doc",
		Content: "content",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/unique-path.md")
	if err != nil {
		t.Fatalf("GetFileByPath failed: %v", err)
	}
	if doc == nil {
		t.Fatal("GetFileByPath returned nil")
	}
	if doc.Title != "Unique Path Doc" {
		t.Errorf("Expected title 'Unique Path Doc', got %q", doc.Title)
	}

	notFound, err := testDB.GetFileByPath(ctx, vaultID, "/nonexistent.md")
	if err != nil {
		t.Fatalf("GetFileByPath nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent path")
	}
}

func TestListFiles(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	for _, path := range []string{"/a/doc1.md", "/a/doc2.md", "/b/doc3.md"} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Title:   "Doc " + path,
			Content: "content of " + path,
			Labels:  []string{"test"},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", path, err)
		}
	}

	// List all
	docs, err := testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("Expected 3 docs, got %d", len(docs))
	}

	// List by folder
	folder := "/a/"
	docs, err = testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, Folder: &folder})
	if err != nil {
		t.Fatalf("ListFiles with folder failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("Expected 2 docs in /a/, got %d", len(docs))
	}
}

func TestUpdateFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/update-test.md",
		Title:   "Original",
		Content: "original",
		Labels:  []string{"old"},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	updated, err := testDB.UpdateFile(ctx, docID, models.FileInput{
		Content: "new content",
		Title:   "Updated Title",
		Labels:  []string{"new"},
	})
	if err != nil {
		t.Fatalf("UpdateFile failed: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got %q", updated.Title)
	}
}

func TestDeleteFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/delete-test.md",
		Title:   "Delete Me",
		Content: "content",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.DeleteFile(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	gone, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetFileByID after delete error: %v", err)
	}
	if gone != nil {
		t.Error("File should be nil after delete")
	}
}

func TestDeleteFileAtomic(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create file with chunks, wiki links, and label edges
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/atomic-delete.md",
		Title:   "Atomic Delete",
		Content: "content",
		Labels:  []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Chunk 1", Position: 0, Embedding: dummyEmbedding()},
		{FileID: docID, Text: "Chunk 2", Position: 1, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	// Outgoing wiki link from the file being deleted
	if err := testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "other"},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	// Create a second file with a wiki link pointing TO the file being deleted (incoming link)
	otherDoc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/other.md",
		Title:   "Other",
		Content: "links to atomic-delete",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile (other) failed: %v", err)
	}
	otherDocID := models.MustRecordIDString(otherDoc.ID)
	if err := testDB.CreateWikiLinks(ctx, otherDocID, vaultID, []WikiLinkInput{
		{RawTarget: "atomic-delete", ToFileID: &docID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks (incoming) failed: %v", err)
	}

	if err := testDB.SyncFileLabels(ctx, docID, vaultID, []string{"test"}); err != nil {
		t.Fatalf("SyncFileLabels failed: %v", err)
	}

	// Delete atomically
	if err := testDB.DeleteFileAtomic(ctx, docID); err != nil {
		t.Fatalf("DeleteFileAtomic failed: %v", err)
	}

	// Verify file is gone
	gone, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetFileByID after atomic delete error: %v", err)
	}
	if gone != nil {
		t.Error("File should be nil after atomic delete")
	}

	// Verify chunks are gone
	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after atomic delete error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks after atomic delete, got %d", len(chunks))
	}

	// Verify outgoing wiki links are gone
	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks after atomic delete error: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("Expected 0 outgoing wiki links after atomic delete, got %d", len(links))
	}

	// Verify incoming wiki links were unresolved (to_file set to NONE)
	incomingLinks, err := testDB.GetWikiLinksToFile(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinksToFile after atomic delete error: %v", err)
	}
	if len(incomingLinks) != 0 {
		t.Errorf("Expected 0 incoming wiki links after atomic delete, got %d", len(incomingLinks))
	}
	// The link itself should still exist on the other file, just unresolved
	otherLinks, err := testDB.GetWikiLinks(ctx, otherDocID)
	if err != nil {
		t.Fatalf("GetWikiLinks for other doc error: %v", err)
	}
	if len(otherLinks) != 1 {
		t.Errorf("Expected 1 wiki link on other doc, got %d", len(otherLinks))
	}

	// Verify label edges are gone
	labels, err := testDB.GetLabelsForFile(ctx, docID)
	if err != nil {
		t.Fatalf("GetLabelsForFile after atomic delete error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("Expected 0 labels after atomic delete, got %d", len(labels))
	}
}

func TestMoveFileAtomic(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/old/move-test.md",
		Title:   "Move Test",
		Content: "content",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	moved, err := testDB.MoveFileAtomic(ctx, vaultID, docID, "/new/subdir/move-test.md")
	if err != nil {
		t.Fatalf("MoveFileAtomic failed: %v", err)
	}
	if moved.Path != "/new/subdir/move-test.md" {
		t.Errorf("Expected path '/new/subdir/move-test.md', got %q", moved.Path)
	}
	expectedStem := models.FilenameStem("/new/subdir/move-test.md")
	if moved.Stem != expectedStem {
		t.Errorf("Expected stem %q, got %q", expectedStem, moved.Stem)
	}

	// Verify ancestor folders were created atomically
	folder, err := testDB.GetFolderByPath(ctx, vaultID, "/new/subdir")
	if err != nil {
		t.Fatalf("GetFolderByPath /new/subdir error: %v", err)
	}
	if folder == nil {
		t.Error("Expected folder /new/subdir to exist after atomic move")
	}

	folder, err = testDB.GetFolderByPath(ctx, vaultID, "/new")
	if err != nil {
		t.Fatalf("GetFolderByPath /new error: %v", err)
	}
	if folder == nil {
		t.Error("Expected folder /new to exist after atomic move")
	}
}

func TestMoveByPrefixAtomic(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create folder + files under /src/
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	srcPath := "/src-" + ts + "/"
	dstPath := "/dst-" + ts + "/"

	if _, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID:  vaultID,
		Path:     strings.TrimSuffix(srcPath, "/"),
		Title:    "src",
		IsFolder: true,
		Labels:   []string{},
	}); err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}

	for _, name := range []string{"a.md", "b.md"} {
		if _, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    srcPath + name,
			Title:   name,
			Content: "content",
			Labels:  []string{},
		}); err != nil {
			t.Fatalf("CreateFile %s failed: %v", name, err)
		}
	}

	count, err := testDB.MoveByPrefixAtomic(ctx, vaultID, srcPath, dstPath)
	if err != nil {
		t.Fatalf("MoveByPrefixAtomic failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 moved files, got %d", count)
	}

	// Verify files are at new paths
	for _, name := range []string{"a.md", "b.md"} {
		f, err := testDB.GetFileByPath(ctx, vaultID, dstPath+name)
		if err != nil {
			t.Fatalf("GetFileByPath %s error: %v", dstPath+name, err)
		}
		if f == nil {
			t.Errorf("Expected file at %s after prefix move", dstPath+name)
		}
		// Verify stem was recomputed
		expectedStem := models.FilenameStem(dstPath + name)
		if f != nil && f.Stem != expectedStem {
			t.Errorf("Expected stem %q for %s, got %q", expectedStem, name, f.Stem)
		}
	}

	// Verify old files are gone
	for _, name := range []string{"a.md", "b.md"} {
		f, err := testDB.GetFileByPath(ctx, vaultID, srcPath+name)
		if err != nil {
			t.Fatalf("GetFileByPath %s error: %v", srcPath+name, err)
		}
		if f != nil {
			t.Errorf("Expected no file at old path %s", srcPath+name)
		}
	}

	// Verify destination folder was created
	dstFolderPath := strings.TrimSuffix(dstPath, "/")
	folder, err := testDB.GetFolderByPath(ctx, vaultID, dstFolderPath)
	if err != nil {
		t.Fatalf("GetFolderByPath %s error: %v", dstFolderPath, err)
	}
	if folder == nil {
		t.Errorf("Expected folder %s to exist after prefix move", dstFolderPath)
	}

	// Verify source folder was moved (not left behind as a ghost)
	srcFolderPath := strings.TrimSuffix(srcPath, "/")
	srcFolder, err := testDB.GetFolderByPath(ctx, vaultID, srcFolderPath)
	if err != nil {
		t.Fatalf("GetFolderByPath %s error: %v", srcFolderPath, err)
	}
	if srcFolder != nil {
		t.Errorf("Expected source folder %s to be gone after prefix move", srcFolderPath)
	}
}

func TestMoveFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/old-path.md",
		Title:   "Move Test",
		Content: "content",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	moved, err := testDB.MoveFile(ctx, docID, "/new-path.md")
	if err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}
	if moved.Path != "/new-path.md" {
		t.Errorf("Expected path '/new-path.md', got %q", moved.Path)
	}
}

func TestListFiles_LabelFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	for _, doc := range []struct {
		path   string
		title  string
		labels []string
	}{
		{"/projects/go-app.md", "Go App", []string{"go", "project"}},
		{"/projects/rust-app.md", "Rust App", []string{"rust", "project"}},
		{"/notes/setup.md", "Setup Guide", []string{"guide"}},
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    doc.path,
			Title:   doc.title,
			Content: "# " + doc.title,
			Labels:  doc.labels,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", doc.path, err)
		}
	}

	docs, err := testDB.ListFiles(ctx, ListFilesFilter{
		VaultID: vaultID,
		Labels:  []string{"go"},
	})
	if err != nil {
		t.Fatalf("list by label: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with label 'go', got %d", len(docs))
	}

	folder := "/projects/"
	docs, err = testDB.ListFiles(ctx, ListFilesFilter{
		VaultID: vaultID,
		Folder:  &folder,
		Labels:  []string{"project"},
	})
	if err != nil {
		t.Fatalf("list by folder+label: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs in /projects/ with label 'project', got %d", len(docs))
	}
}

func TestGetFileByID(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/get-by-id-" + suffix + ".md",
		Title:   "GetByID Test",
		Content: "get by id content",
		Labels:  []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	fetched, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetFileByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetFileByID returned nil for existing file")
	}
	if fetched.Title != "GetByID Test" {
		t.Errorf("Expected title 'GetByID Test', got %q", fetched.Title)
	}

	notFound, err := testDB.GetFileByID(ctx, "file:nonexistent_"+suffix)
	if err != nil {
		t.Fatalf("GetFileByID nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent file ID")
	}
}

func TestDeleteFilesByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	paths := []string{
		"/prefix-" + suffix + "/a.md",
		"/prefix-" + suffix + "/b.md",
		"/other-" + suffix + "/c.md",
	}
	for _, path := range paths {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Title:   "Doc " + path,
			Content: "content of " + path,
			Labels:  []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", path, err)
		}
	}

	count, err := testDB.DeleteFilesByPrefix(ctx, vaultID, "/prefix-"+suffix+"/")
	if err != nil {
		t.Fatalf("DeleteFilesByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 deleted files, got %d", count)
	}

	remaining, err := testDB.GetFileByPath(ctx, vaultID, "/other-"+suffix+"/c.md")
	if err != nil {
		t.Fatalf("GetFileByPath failed: %v", err)
	}
	if remaining == nil {
		t.Error("File at /other/ path should still exist after prefix delete")
	}
}

func TestMoveFilesByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, path := range []string{"/old-" + suffix + "/a.md", "/old-" + suffix + "/b.md"} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Title:   "Doc " + path,
			Content: "content of " + path,
			Labels:  []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", path, err)
		}
	}

	count, err := testDB.MoveFilesByPrefix(ctx, vaultID, "/old-"+suffix+"/", "/new-"+suffix+"/")
	if err != nil {
		t.Fatalf("MoveFilesByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 moved files, got %d", count)
	}

	docA, err := testDB.GetFileByPath(ctx, vaultID, "/new-"+suffix+"/a.md")
	if err != nil {
		t.Fatalf("GetFileByPath new/a.md failed: %v", err)
	}
	if docA == nil {
		t.Error("File a.md should exist at new path after move")
	}

	docB, err := testDB.GetFileByPath(ctx, vaultID, "/new-"+suffix+"/b.md")
	if err != nil {
		t.Fatalf("GetFileByPath new/b.md failed: %v", err)
	}
	if docB == nil {
		t.Error("File b.md should exist at new path after move")
	}
}

func TestListLabels(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, doc := range []struct {
		path   string
		labels []string
	}{
		{"/labels-" + suffix + "/a.md", []string{"alpha", "beta"}},
		{"/labels-" + suffix + "/b.md", []string{"beta", "gamma"}},
		{"/labels-" + suffix + "/c.md", []string{"delta"}},
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    doc.path,
			Title:   "Doc " + doc.path,
			Content: "content",
			Labels:  doc.labels,
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", doc.path, err)
		}
	}

	labels, err := testDB.ListLabels(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListLabels failed: %v", err)
	}

	expected := map[string]bool{"alpha": false, "beta": false, "gamma": false, "delta": false}
	for _, l := range labels {
		if _, ok := expected[l]; ok {
			expected[l] = true
		}
	}
	for label, found := range expected {
		if !found {
			t.Errorf("Expected label %q not found in ListLabels result", label)
		}
	}
}

func TestListLabelsWithCounts(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Empty vault should return empty slice
	emptyCounts, err := testDB.ListLabelsWithCounts(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListLabelsWithCounts on empty vault failed: %v", err)
	}
	if len(emptyCounts) != 0 {
		t.Errorf("expected 0 label counts on empty vault, got %d", len(emptyCounts))
	}

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, doc := range []struct {
		path   string
		labels []string
	}{
		{"/lc-" + suffix + "/a.md", []string{"go", "project"}},
		{"/lc-" + suffix + "/b.md", []string{"go", "tutorial"}},
		{"/lc-" + suffix + "/c.md", []string{"rust"}},
		{"/lc-" + suffix + "/d.md", nil},        // no labels — must not produce phantom entries
		{"/lc-" + suffix + "/e.md", []string{}}, // empty labels — must not produce phantom entries
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    doc.path,
			Title:   "Doc " + doc.path,
			Content: "content",
			Labels:  doc.labels,
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", doc.path, err)
		}
	}

	counts, err := testDB.ListLabelsWithCounts(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListLabelsWithCounts failed: %v", err)
	}

	// Should be sorted by count desc: go(2), then project/tutorial/rust(1 each)
	// Files with nil or empty labels must NOT produce phantom entries
	if len(counts) != 4 {
		t.Fatalf("expected 4 label counts, got %d: %v", len(counts), counts)
	}
	if counts[0].Label != "go" || counts[0].Count != 2 {
		t.Errorf("expected first label to be go(2), got %s(%d)", counts[0].Label, counts[0].Count)
	}

	countMap := map[string]int{}
	for _, lc := range counts {
		countMap[lc.Label] = lc.Count
	}
	for _, label := range []string{"project", "tutorial", "rust"} {
		if countMap[label] != 1 {
			t.Errorf("expected label %q count 1, got %d", label, countMap[label])
		}
	}
}

func TestUpsertFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	path := "/upsert-" + suffix + ".md"

	// First upsert: should create
	doc, created, previousDoc, err := testDB.UpsertFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    path,
		Title:   "Upsert Original",
		Content: "original content",
		Labels:  []string{"v1"},
	})
	if err != nil {
		t.Fatalf("UpsertFile (create) failed: %v", err)
	}
	if !created {
		t.Error("Expected created=true for first upsert")
	}
	if previousDoc != nil {
		t.Error("Expected previousDoc=nil for first upsert")
	}
	if doc.Title != "Upsert Original" {
		t.Errorf("Expected title 'Upsert Original', got %q", doc.Title)
	}

	// Second upsert: should update
	doc2, created2, previousDoc2, err := testDB.UpsertFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    path,
		Title:   "Upsert Updated",
		Content: "updated content",
		Labels:  []string{"v2"},
	})
	if err != nil {
		t.Fatalf("UpsertFile (update) failed: %v", err)
	}
	if created2 {
		t.Error("Expected created=false for second upsert")
	}
	if previousDoc2 == nil {
		t.Fatal("Expected previousDoc to be set for second upsert")
	}
	if previousDoc2.Title != "Upsert Original" {
		t.Errorf("Expected previousDoc title 'Upsert Original', got %q", previousDoc2.Title)
	}
	if doc2.Title != "Upsert Updated" {
		t.Errorf("Expected title 'Upsert Updated', got %q", doc2.Title)
	}
}

func TestGetFilesByStem(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	// Two files that share stem "my-note" (different folders)
	for _, p := range []string{
		"/folder-a-" + suffix + "/my-note.md",
		"/folder-b-" + suffix + "/my-note.md",
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "My Note", Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
	}

	files, err := testDB.GetFilesByStem(ctx, vaultID, "my-note")
	if err != nil {
		t.Fatalf("GetFilesByStem failed: %v", err)
	}
	if len(files) < 2 {
		t.Errorf("Expected at least 2 files with stem 'my-note', got %d", len(files))
	}
}

func TestCountFilesByStem(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	stem := "count-stem-" + suffix
	for _, p := range []string{
		"/count-stem-dir-a/" + stem + ".md",
		"/count-stem-dir-b/" + stem + ".md",
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "Count Stem", Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
	}

	count, err := testDB.CountFilesByStem(ctx, vaultID, stem)
	if err != nil {
		t.Fatalf("CountFilesByStem failed: %v", err)
	}

	files, err := testDB.GetFilesByStem(ctx, vaultID, stem)
	if err != nil {
		t.Fatalf("GetFilesByStem failed: %v", err)
	}
	if count != len(files) {
		t.Errorf("CountFilesByStem=%d does not match GetFilesByStem len=%d", count, len(files))
	}
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func TestListFilesByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	docsPrefix := "/pfx-docs-" + suffix + "/"
	otherPrefix := "/pfx-other-" + suffix + "/"
	for _, p := range []string{docsPrefix + "a.md", docsPrefix + "b.md", otherPrefix + "c.md"} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "Prefix Test", Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
	}

	files, err := testDB.ListFilesByPrefix(ctx, vaultID, docsPrefix)
	if err != nil {
		t.Fatalf("ListFilesByPrefix failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 files under docs prefix, got %d", len(files))
	}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, docsPrefix) {
			t.Errorf("Unexpected path %q returned for prefix %q", f.Path, docsPrefix)
		}
	}
}

func TestUpdateFileTranscript(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/transcript-" + suffix + ".md", Title: "Transcript Test",
		ContentLength: 8, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	newHash := "abc123hash"
	newLen := 20
	if err := testDB.UpdateFileTranscript(ctx, docID, newHash, newLen); err != nil {
		t.Fatalf("UpdateFileTranscript failed: %v", err)
	}

	fetched, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetFileByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetFileByID returned nil")
	}
	if fetched.ContentHash == nil || *fetched.ContentHash != newHash {
		t.Errorf("Expected content_hash %q, got %v", newHash, fetched.ContentHash)
	}
	if fetched.ContentLength != newLen {
		t.Errorf("Expected content_length %d, got %d", newLen, fetched.ContentLength)
	}
}

func TestBatchUpdateFileAccess(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	var fileIDs []string
	for _, p := range []string{
		"/access-a-" + suffix + ".md",
		"/access-b-" + suffix + ".md",
	} {
		doc, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "Access Test", Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
		fileIDs = append(fileIDs, models.MustRecordIDString(doc.ID))
	}

	if err := testDB.BatchUpdateFileAccess(ctx, fileIDs); err != nil {
		t.Fatalf("BatchUpdateFileAccess failed: %v", err)
	}

	for _, id := range fileIDs {
		f, err := testDB.GetFileByID(ctx, id)
		if err != nil {
			t.Fatalf("GetFileByID failed: %v", err)
		}
		if f == nil {
			t.Fatal("GetFileByID returned nil")
		}
		if f.AccessCount != 1 {
			t.Errorf("Expected access_count 1, got %d for file %s", f.AccessCount, id)
		}
	}
}

func TestSetFileAccessCount(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/setaccess-" + suffix + ".md", Title: "Set Access",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.SetFileAccessCount(ctx, docID, 42); err != nil {
		t.Fatalf("SetFileAccessCount failed: %v", err)
	}

	fetched, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetFileByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetFileByID returned nil")
	}
	if fetched.AccessCount != 42 {
		t.Errorf("Expected access_count 42, got %d", fetched.AccessCount)
	}
}

func TestGetFilesByAllLabels(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, d := range []struct {
		path   string
		labels []string
	}{
		{"/allabels-ab-" + suffix + ".md", []string{"lbl-a", "lbl-b"}},
		{"/allabels-ac-" + suffix + ".md", []string{"lbl-a", "lbl-c"}},
		{"/allabels-abc-" + suffix + ".md", []string{"lbl-a", "lbl-b", "lbl-c"}},
	} {
		doc, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: d.path, Title: "AllLabels", Content: "content", Labels: d.labels,
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", d.path, err)
		}
		docID := models.MustRecordIDString(doc.ID)
		if err := testDB.SyncFileLabels(ctx, docID, vaultID, d.labels); err != nil {
			t.Fatalf("SyncFileLabels %s failed: %v", d.path, err)
		}
	}

	// Files with both lbl-a and lbl-b: ab and abc (2 files)
	files, err := testDB.GetFilesByAllLabels(ctx, vaultID, []string{"lbl-a", "lbl-b"})
	if err != nil {
		t.Fatalf("GetFilesByAllLabels failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 files with labels [lbl-a, lbl-b], got %d", len(files))
	}
}

func TestGetFileMetaByPath(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	p := "/filemeta-" + suffix + ".md"
	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: p, Title: "File Meta Test", Content: "meta content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	meta, err := testDB.GetFileMetaByPath(ctx, vaultID, p)
	if err != nil {
		t.Fatalf("GetFileMetaByPath failed: %v", err)
	}
	if meta == nil {
		t.Fatal("GetFileMetaByPath returned nil for existing file")
	}
	if meta.Path != p {
		t.Errorf("Expected path %q, got %q", p, meta.Path)
	}
	// FileMeta is a lightweight projection — no content, title not projected by GetFileMetaByPath
	if meta.IsFolder {
		t.Error("Expected is_folder=false for a regular file")
	}

	notFound, err := testDB.GetFileMetaByPath(ctx, vaultID, "/nonexistent-"+suffix+".md")
	if err != nil {
		t.Fatalf("GetFileMetaByPath nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent path")
	}
}

func TestGetFileMetaByPaths(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	p1 := "/meta-batch-a-" + suffix + ".md"
	p2 := "/meta-batch-b-" + suffix + ".md"
	pMissing := "/meta-batch-missing-" + suffix + ".md"

	for _, p := range []string{p1, p2} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "Batch Meta", Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
	}

	// Query with 2 existing + 1 missing path
	m, err := testDB.GetFileMetaByPaths(ctx, vaultID, []string{p1, p2, pMissing})
	if err != nil {
		t.Fatalf("GetFileMetaByPaths failed: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(m))
	}
	if m[p1] == nil {
		t.Error("Expected entry for p1")
	}
	if m[p2] == nil {
		t.Error("Expected entry for p2")
	}
	if m[pMissing] != nil {
		t.Error("Expected nil for missing path")
	}

	// Empty input returns empty map
	empty, err := testDB.GetFileMetaByPaths(ctx, vaultID, []string{})
	if err != nil {
		t.Fatalf("GetFileMetaByPaths empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(empty))
	}
}

func TestListFileMetas(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, p := range []string{
		"/listmeta-" + suffix + "/a.md",
		"/listmeta-" + suffix + "/b.md",
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "ListMeta", Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
	}

	folder := "/listmeta-" + suffix + "/"
	metas, err := testDB.ListFileMetas(ctx, ListFilesFilter{VaultID: vaultID, Folder: &folder})
	if err != nil {
		t.Fatalf("ListFileMetas failed: %v", err)
	}
	if len(metas) != 2 {
		t.Errorf("Expected 2 file metas, got %d", len(metas))
	}
}

func TestListChildFolders(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	// Create folder structure: /a/, /a/b/, /a/c/, /a/b/d/
	for _, fp := range []string{
		"/cf-" + suffix,
		"/cf-" + suffix + "/b",
		"/cf-" + suffix + "/c",
		"/cf-" + suffix + "/b/d",
	} {
		_, err := testDB.CreateFolder(ctx, vaultID, fp)
		if err != nil {
			t.Fatalf("CreateFolder %s failed: %v", fp, err)
		}
	}

	children, err := testDB.ListChildFolders(ctx, vaultID, "/cf-"+suffix)
	if err != nil {
		t.Fatalf("ListChildFolders failed: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("Expected 2 immediate child folders, got %d", len(children))
	}
	paths := map[string]bool{}
	for _, ch := range children {
		paths[ch.Path] = true
	}
	if !paths["/cf-"+suffix+"/b"] {
		t.Errorf("Expected child /cf-%s/b, not found in %v", suffix, paths)
	}
	if !paths["/cf-"+suffix+"/c"] {
		t.Errorf("Expected child /cf-%s/c, not found in %v", suffix, paths)
	}
}

func TestRecomputeStems(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/old-stem-" + suffix + ".md", Title: "Old Stem",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	// Move the file to a new path
	newPath := "/new-stem-" + suffix + ".md"
	if _, err := testDB.MoveFile(ctx, docID, newPath); err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}

	// RecomputeStems for the new path prefix
	if err := testDB.RecomputeStems(ctx, vaultID, "/new-stem-"+suffix); err != nil {
		t.Fatalf("RecomputeStems failed: %v", err)
	}

	fetched, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetFileByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetFileByID returned nil")
	}
	expectedStem := models.FilenameStem(newPath)
	if fetched.Stem != expectedStem {
		t.Errorf("Expected stem %q, got %q", expectedStem, fetched.Stem)
	}
}

func TestListUnprocessedFiles(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/unprocessed-" + suffix + ".md", Title: "Unprocessed",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	// Create a parse pipeline job for this file
	if err := testDB.CreateJob(ctx, docID, "parse", 0); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	files, err := testDB.ListUnprocessedFiles(ctx, vaultID, 100)
	if err != nil {
		t.Fatalf("ListUnprocessedFiles failed: %v", err)
	}

	found := false
	for _, f := range files {
		if models.MustRecordIDString(f.ID) == docID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected file %s in unprocessed list, got %d files", docID, len(files))
	}
}

func TestListFiles_OrderBy(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, p := range []string{
		"/order-" + suffix + "/b.md",
		"/order-" + suffix + "/a.md",
	} {
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: p, Title: "Order " + p, Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %s failed: %v", p, err)
		}
	}

	folder := "/order-" + suffix + "/"

	// Default order (path ASC)
	docs, err := testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, Folder: &folder})
	if err != nil {
		t.Fatalf("ListFiles path ASC failed: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	if !strings.HasSuffix(docs[0].Path, "/a.md") {
		t.Errorf("expected first doc to be a.md (path ASC), got %q", docs[0].Path)
	}

	// Explicit order: created_at DESC → last created first
	docs, err = testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, Folder: &folder, OrderBy: OrderByCreatedAtDesc})
	if err != nil {
		t.Fatalf("ListFiles created_at DESC failed: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	// a.md was created second, so it should be first with DESC
	if !strings.HasSuffix(docs[0].Path, "/a.md") {
		t.Errorf("expected first doc to be a.md (created_at DESC), got %q", docs[0].Path)
	}

	// Invalid order must return error
	_, err = testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, OrderBy: FileOrderBy("DROP TABLE; --")})
	if err == nil {
		t.Error("expected error for invalid OrderBy, got nil")
	}
}

func TestListFiles_DocTypeFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	mdType := "markdown"
	audioType := "audio"
	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/dtype-" + suffix + "/note.md", Title: "Note",
		Content: "content", Labels: []string{}, DocType: &mdType,
	})
	if err != nil {
		t.Fatalf("CreateFile note failed: %v", err)
	}
	_, err = testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/dtype-" + suffix + "/recording.mp3", Title: "Recording",
		Content: "", Labels: []string{}, DocType: &audioType, MimeType: "audio/mpeg",
	})
	if err != nil {
		t.Fatalf("CreateFile recording failed: %v", err)
	}

	// Filter by doc_type
	docs, err := testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, DocType: &mdType})
	if err != nil {
		t.Fatalf("ListFiles by doc_type failed: %v", err)
	}
	for _, d := range docs {
		if d.DocType != nil && *d.DocType != mdType {
			t.Errorf("expected doc_type %q, got %q", mdType, *d.DocType)
		}
	}
	var found bool
	for _, d := range docs {
		if strings.HasSuffix(d.Path, "/note.md") {
			found = true
			break
		}
	}
	if !found {
		t.Error("note.md not found with doc_type filter")
	}
}

func TestListFiles_MimeTypeFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/mime-" + suffix + "/doc.md", Title: "Markdown",
		Content: "content", Labels: []string{}, MimeType: "text/markdown",
	})
	if err != nil {
		t.Fatalf("CreateFile markdown failed: %v", err)
	}
	_, err = testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/mime-" + suffix + "/image.png", Title: "Image",
		Content: "", Labels: []string{}, MimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("CreateFile image failed: %v", err)
	}

	mime := "image/png"
	docs, err := testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, MimeType: &mime})
	if err != nil {
		t.Fatalf("ListFiles by mime_type failed: %v", err)
	}
	for _, d := range docs {
		if d.MimeType != mime {
			t.Errorf("expected mime_type %q, got %q", mime, d.MimeType)
		}
	}
	var found bool
	for _, d := range docs {
		if strings.HasSuffix(d.Path, "/image.png") {
			found = true
			break
		}
	}
	if !found {
		t.Error("image.png not found with mime_type filter")
	}
}

func TestListFiles_IsFolderFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	// Create a regular file
	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/isfolder-" + suffix + "/note.md", Title: "Note",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	// Create a folder
	_, err = testDB.CreateFolder(ctx, vaultID, "/isfolder-"+suffix+"/sub")
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}

	// Default ListFiles excludes folders
	docs, err := testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListFiles default failed: %v", err)
	}
	for _, d := range docs {
		if d.IsFolder {
			t.Errorf("default ListFiles should not return folders, got %q", d.Path)
		}
	}

	// Explicitly request folders only
	isFolder := true
	folder := "/isfolder-" + suffix + "/"
	folders, err := testDB.ListFiles(ctx, ListFilesFilter{VaultID: vaultID, IsFolder: &isFolder, Folder: &folder})
	if err != nil {
		t.Fatalf("ListFiles is_folder=true failed: %v", err)
	}
	if len(folders) != 1 {
		t.Errorf("expected 1 folder, got %d", len(folders))
	}
	for _, d := range folders {
		if !d.IsFolder {
			t.Errorf("expected is_folder=true, got false for %q", d.Path)
		}
	}
}

func TestListFiles_UpdatedSinceFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	// Create two files with a sleep between them so they get different updated_at values.
	oldDoc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/since-" + suffix + "/old.md", Title: "Old",
	})
	if err != nil {
		t.Fatalf("CreateFile old failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	newDoc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/since-" + suffix + "/new.md", Title: "New",
	})
	if err != nil {
		t.Fatalf("CreateFile new failed: %v", err)
	}

	// Sanity check: new.UpdatedAt must be strictly after old.UpdatedAt
	if !newDoc.UpdatedAt.After(oldDoc.UpdatedAt) {
		t.Fatalf("new.UpdatedAt (%v) should be after old.UpdatedAt (%v)", newDoc.UpdatedAt, oldDoc.UpdatedAt)
	}

	// Verify the filter works with a far-future cutoff first.
	folder := "/since-" + suffix + "/"
	farFuture := time.Now().Add(24 * time.Hour)
	farDocs, err := testDB.ListFiles(ctx, ListFilesFilter{
		VaultID: vaultID, Folder: &folder, UpdatedSince: &farFuture,
	})
	if err != nil {
		t.Fatalf("ListFiles with far-future cutoff failed: %v", err)
	}
	if len(farDocs) != 0 {
		t.Fatalf("expected 0 docs with far-future cutoff, got %d", len(farDocs))
	}

	// Use a cutoff between the two timestamps.
	mid := oldDoc.UpdatedAt.Add(newDoc.UpdatedAt.Sub(oldDoc.UpdatedAt) / 2)
	cutoff := mid
	docs, err := testDB.ListFiles(ctx, ListFilesFilter{
		VaultID:      vaultID,
		Folder:       &folder,
		UpdatedSince: &cutoff,
	})
	if err != nil {
		t.Fatalf("ListFiles with UpdatedSince failed: %v", err)
	}
	if len(docs) != 1 {
		for _, d := range docs {
			t.Logf("  doc: %s updated_at=%v", d.Path, d.UpdatedAt)
		}
		t.Logf("  cutoff=%v", cutoff)
		t.Fatalf("expected 1 doc with UpdatedSince, got %d", len(docs))
	}
	if !strings.HasSuffix(docs[0].Path, "/new.md") {
		t.Errorf("expected new.md, got %s", docs[0].Path)
	}

	// Query without UpdatedSince — should return both
	allDocs, err := testDB.ListFiles(ctx, ListFilesFilter{
		VaultID: vaultID,
		Folder:  &folder,
	})
	if err != nil {
		t.Fatalf("ListFiles without UpdatedSince failed: %v", err)
	}
	if len(allDocs) != 2 {
		t.Errorf("expected 2 docs total, got %d", len(allDocs))
	}
}
