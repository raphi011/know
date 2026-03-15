package db

import (
	"context"
	"fmt"
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
		Content:  "---\ntitle: Hello\n---\nHello world content",
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
