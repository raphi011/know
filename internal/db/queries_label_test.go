package db

import (
	"context"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestEnsureLabel(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// First call creates
	id1, err := testDB.EnsureLabel(ctx, vaultID, "go")
	if err != nil {
		t.Fatalf("EnsureLabel failed: %v", err)
	}
	if id1 == "" {
		t.Fatal("Expected non-empty label ID")
	}

	// Second call returns same ID (upsert)
	id2, err := testDB.EnsureLabel(ctx, vaultID, "go")
	if err != nil {
		t.Fatalf("EnsureLabel second call failed: %v", err)
	}
	if id1 != id2 {
		t.Errorf("Expected same label ID on upsert: got %q and %q", id1, id2)
	}

	// Different name creates different label
	id3, err := testDB.EnsureLabel(ctx, vaultID, "rust")
	if err != nil {
		t.Fatalf("EnsureLabel different name failed: %v", err)
	}
	if id3 == id1 {
		t.Error("Different label name should produce different ID")
	}
}

func TestSyncFileLabels(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/label-sync.md", Title: "Label Sync",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	// Sync with initial labels
	if err := testDB.SyncFileLabels(ctx, docID, vaultID, []string{"go", "backend"}); err != nil {
		t.Fatalf("SyncFileLabels failed: %v", err)
	}

	labels, err := testDB.GetLabelsForFile(ctx, docID)
	if err != nil {
		t.Fatalf("GetLabelsForFile failed: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("Expected 2 labels, got %d: %v", len(labels), labels)
	}

	// Sync with different labels — old edges should be removed
	if err := testDB.SyncFileLabels(ctx, docID, vaultID, []string{"rust"}); err != nil {
		t.Fatalf("SyncFileLabels second call failed: %v", err)
	}

	labels, err = testDB.GetLabelsForFile(ctx, docID)
	if err != nil {
		t.Fatalf("GetLabelsForFile after resync failed: %v", err)
	}
	if len(labels) != 1 {
		t.Errorf("Expected 1 label after resync, got %d: %v", len(labels), labels)
	}
	if len(labels) > 0 && labels[0] != "rust" {
		t.Errorf("Expected label 'rust', got %q", labels[0])
	}

	// Sync with empty labels — all edges should be removed
	if err := testDB.SyncFileLabels(ctx, docID, vaultID, nil); err != nil {
		t.Fatalf("SyncFileLabels empty failed: %v", err)
	}

	labels, err = testDB.GetLabelsForFile(ctx, docID)
	if err != nil {
		t.Fatalf("GetLabelsForFile after empty sync failed: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("Expected 0 labels after empty sync, got %d: %v", len(labels), labels)
	}
}

func TestGetFilesByLabel(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc1, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/label-query-1.md", Title: "Label Query 1",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile 1 failed: %v", err)
	}
	doc2, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/label-query-2.md", Title: "Label Query 2",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile 2 failed: %v", err)
	}
	doc1ID := models.MustRecordIDString(doc1.ID)
	doc2ID := models.MustRecordIDString(doc2.ID)

	// Both docs get "shared" label, only doc1 gets "unique"
	if err := testDB.SyncFileLabels(ctx, doc1ID, vaultID, []string{"shared", "unique"}); err != nil {
		t.Fatalf("SyncFileLabels doc1 failed: %v", err)
	}
	if err := testDB.SyncFileLabels(ctx, doc2ID, vaultID, []string{"shared"}); err != nil {
		t.Fatalf("SyncFileLabels doc2 failed: %v", err)
	}

	// Query by "shared" — should return both docs
	docs, err := testDB.GetFilesByLabel(ctx, vaultID, "shared")
	if err != nil {
		t.Fatalf("GetFilesByLabel failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("Expected 2 docs with 'shared' label, got %d", len(docs))
	}

	// Query by "unique" — should return only doc1
	docs, err = testDB.GetFilesByLabel(ctx, vaultID, "unique")
	if err != nil {
		t.Fatalf("GetFilesByLabel unique failed: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("Expected 1 doc with 'unique' label, got %d", len(docs))
	}

	// Query by nonexistent label — should return empty
	docs, err = testDB.GetFilesByLabel(ctx, vaultID, "nonexistent")
	if err != nil {
		t.Fatalf("GetFilesByLabel nonexistent failed: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("Expected 0 docs with 'nonexistent' label, got %d", len(docs))
	}
}
