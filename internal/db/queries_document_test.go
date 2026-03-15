package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	hash := "abc123"
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/docs/hello.md",
		Title:       "Hello World",
		Content:     "---\ntitle: Hello\n---\nHello world content",
		ContentBody: "Hello world content",
		Labels:      []string{"test", "greeting"},
		ContentHash: &hash,
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
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

func TestGetDocumentByPath(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	_, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/unique-path.md",
		Title:       "Unique Path Doc",
		Content:     "content",
		ContentBody: "content",
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/unique-path.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath failed: %v", err)
	}
	if doc == nil {
		t.Fatal("GetDocumentByPath returned nil")
	}
	if doc.Title != "Unique Path Doc" {
		t.Errorf("Expected title 'Unique Path Doc', got %q", doc.Title)
	}

	notFound, err := testDB.GetDocumentByPath(ctx, vaultID, "/nonexistent.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent path")
	}
}

func TestListDocuments(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	for _, path := range []string{"/a/doc1.md", "/a/doc2.md", "/b/doc3.md"} {
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        path,
			Title:       "Doc " + path,
			Content:     "content of " + path,
			ContentBody: "content of " + path,
			Labels:      []string{"test"},
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", path, err)
		}
	}

	// List all
	docs, err := testDB.ListDocuments(ctx, ListDocumentsFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("Expected 3 docs, got %d", len(docs))
	}

	// List by folder
	folder := "/a/"
	docs, err = testDB.ListDocuments(ctx, ListDocumentsFilter{VaultID: vaultID, Folder: &folder})
	if err != nil {
		t.Fatalf("ListDocuments with folder failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("Expected 2 docs in /a/, got %d", len(docs))
	}
}

func TestUpdateDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/update-test.md",
		Title:       "Original",
		Content:     "original",
		ContentBody: "original",
		Labels:      []string{"old"},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	updated, err := testDB.UpdateDocument(ctx, docID, models.DocumentInput{
		Content:     "new content",
		ContentBody: "new content",
		Title:       "Updated Title",
		Labels:      []string{"new"},
	})
	if err != nil {
		t.Fatalf("UpdateDocument failed: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got %q", updated.Title)
	}
	if updated.ContentBody != "new content" {
		t.Errorf("Expected content_body 'new content', got %q", updated.ContentBody)
	}
}

func TestDeleteDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/delete-test.md",
		Title:       "Delete Me",
		Content:     "content",
		ContentBody: "content",
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.DeleteDocument(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	gone, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetDocumentByID after delete error: %v", err)
	}
	if gone != nil {
		t.Error("Document should be nil after delete")
	}
}

func TestMoveDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/old-path.md",
		Title:       "Move Test",
		Content:     "content",
		ContentBody: "content",
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	moved, err := testDB.MoveDocument(ctx, docID, "/new-path.md")
	if err != nil {
		t.Fatalf("MoveDocument failed: %v", err)
	}
	if moved.Path != "/new-path.md" {
		t.Errorf("Expected path '/new-path.md', got %q", moved.Path)
	}
}

func TestListDocuments_LabelFilter(t *testing.T) {
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
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        doc.path,
			Title:       doc.title,
			Content:     "# " + doc.title,
			ContentBody: doc.title,
			Labels:      doc.labels,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", doc.path, err)
		}
	}

	docs, err := testDB.ListDocuments(ctx, ListDocumentsFilter{
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
	docs, err = testDB.ListDocuments(ctx, ListDocumentsFilter{
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

func TestGetDocumentByID(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/get-by-id-" + suffix + ".md",
		Title:       "GetByID Test",
		Content:     "get by id content",
		ContentBody: "get by id content",
		Labels:      []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	fetched, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetDocumentByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetDocumentByID returned nil for existing document")
	}
	if fetched.Title != "GetByID Test" {
		t.Errorf("Expected title 'GetByID Test', got %q", fetched.Title)
	}

	notFound, err := testDB.GetDocumentByID(ctx, "document:nonexistent_"+suffix)
	if err != nil {
		t.Fatalf("GetDocumentByID nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent document ID")
	}
}

func TestDeleteDocumentsByPrefix(t *testing.T) {
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
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        path,
			Title:       "Doc " + path,
			Content:     "content of " + path,
			ContentBody: "content of " + path,
			Labels:      []string{},
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", path, err)
		}
	}

	count, err := testDB.DeleteDocumentsByPrefix(ctx, vaultID, "/prefix-"+suffix+"/")
	if err != nil {
		t.Fatalf("DeleteDocumentsByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 deleted documents, got %d", count)
	}

	remaining, err := testDB.GetDocumentByPath(ctx, vaultID, "/other-"+suffix+"/c.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath failed: %v", err)
	}
	if remaining == nil {
		t.Error("Document at /other/ path should still exist after prefix delete")
	}
}

func TestMoveDocumentsByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for _, path := range []string{"/old-" + suffix + "/a.md", "/old-" + suffix + "/b.md"} {
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        path,
			Title:       "Doc " + path,
			Content:     "content of " + path,
			ContentBody: "content of " + path,
			Labels:      []string{},
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", path, err)
		}
	}

	count, err := testDB.MoveDocumentsByPrefix(ctx, vaultID, "/old-"+suffix+"/", "/new-"+suffix+"/")
	if err != nil {
		t.Fatalf("MoveDocumentsByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 moved documents, got %d", count)
	}

	docA, err := testDB.GetDocumentByPath(ctx, vaultID, "/new-"+suffix+"/a.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath new/a.md failed: %v", err)
	}
	if docA == nil {
		t.Error("Document a.md should exist at new path after move")
	}

	docB, err := testDB.GetDocumentByPath(ctx, vaultID, "/new-"+suffix+"/b.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath new/b.md failed: %v", err)
	}
	if docB == nil {
		t.Error("Document b.md should exist at new path after move")
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
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        doc.path,
			Title:       "Doc " + doc.path,
			Content:     "content",
			ContentBody: "content",
			Labels:      doc.labels,
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", doc.path, err)
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
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        doc.path,
			Title:       "Doc " + doc.path,
			Content:     "content",
			ContentBody: "content",
			Labels:      doc.labels,
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", doc.path, err)
		}
	}

	counts, err := testDB.ListLabelsWithCounts(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListLabelsWithCounts failed: %v", err)
	}

	// Should be sorted by count desc: go(2), then project/tutorial/rust(1 each)
	// Documents with nil or empty labels must NOT produce phantom entries
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

func TestUpsertDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	path := "/upsert-" + suffix + ".md"

	// First upsert: should create
	doc, created, previousDoc, err := testDB.UpsertDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        path,
		Title:       "Upsert Original",
		Content:     "original content",
		ContentBody: "original content",
		Labels:      []string{"v1"},
	})
	if err != nil {
		t.Fatalf("UpsertDocument (create) failed: %v", err)
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
	doc2, created2, previousDoc2, err := testDB.UpsertDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        path,
		Title:       "Upsert Updated",
		Content:     "updated content",
		ContentBody: "updated content",
		Labels:      []string{"v2"},
	})
	if err != nil {
		t.Fatalf("UpsertDocument (update) failed: %v", err)
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
