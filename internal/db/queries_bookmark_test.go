package db

import (
	"context"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateAndListBookmarks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create a test file
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/bookmark-test.md", Title: "Bookmark Test",
		Size: 10, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	fileID := models.MustRecordIDString(doc.ID)

	// List bookmarks — should be empty
	files, err := testDB.ListBookmarks(ctx, userID, vaultID)
	if err != nil {
		t.Fatalf("ListBookmarks (empty): %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 bookmarks, got %d", len(files))
	}

	// Create bookmark
	if err := testDB.CreateBookmark(ctx, userID, fileID, vaultID); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	// List bookmarks — should have 1
	files, err = testDB.ListBookmarks(ctx, userID, vaultID)
	if err != nil {
		t.Fatalf("ListBookmarks (1): %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(files))
	}
	if files[0].Path != "/bookmark-test.md" {
		t.Errorf("expected path /bookmark-test.md, got %s", files[0].Path)
	}
}

func TestCreateBookmarkIdempotent(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/idempotent-test.md", Title: "Idempotent",
		Size: 5, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	fileID := models.MustRecordIDString(doc.ID)

	// Create same bookmark twice — should not error
	if err := testDB.CreateBookmark(ctx, userID, fileID, vaultID); err != nil {
		t.Fatalf("CreateBookmark (1st): %v", err)
	}
	if err := testDB.CreateBookmark(ctx, userID, fileID, vaultID); err != nil {
		t.Fatalf("CreateBookmark (2nd): %v", err)
	}

	files, err := testDB.ListBookmarks(ctx, userID, vaultID)
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 bookmark after double-create, got %d", len(files))
	}
}

func TestDeleteBookmark(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/delete-test.md", Title: "Delete Test",
		Size: 5, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	fileID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateBookmark(ctx, userID, fileID, vaultID); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	// Delete bookmark
	if err := testDB.DeleteBookmark(ctx, userID, fileID); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}

	// Should be empty
	files, err := testDB.ListBookmarks(ctx, userID, vaultID)
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 bookmarks after delete, got %d", len(files))
	}

	// Delete again — no-op, should not error
	if err := testDB.DeleteBookmark(ctx, userID, fileID); err != nil {
		t.Fatalf("DeleteBookmark (2nd): %v", err)
	}
}

func TestBookmarkCascadeOnFileDelete(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/cascade-test.md", Title: "Cascade",
		Size: 5, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	fileID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateBookmark(ctx, userID, fileID, vaultID); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	// Delete the file — bookmark should cascade
	if err := testDB.DeleteFile(ctx, fileID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	// Wait for async cascade event to complete
	time.Sleep(100 * time.Millisecond)

	files, err := testDB.ListBookmarks(ctx, userID, vaultID)
	if err != nil {
		t.Fatalf("ListBookmarks after cascade: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 bookmarks after file delete cascade, got %d", len(files))
	}
}
