package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateFolder(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	folderPath := fmt.Sprintf("/docs-%d", time.Now().UnixNano())
	folder, err := testDB.CreateFolder(ctx, vaultID, folderPath)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	if folder.Path != folderPath {
		t.Errorf("Expected path %q, got %q", folderPath, folder.Path)
	}
}

func TestEnsureFolders(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/ef-%d", time.Now().UnixNano())
	docPath := prefix + "/b/c/doc.md"

	err := testDB.EnsureFolders(ctx, vaultID, docPath)
	if err != nil {
		t.Fatalf("EnsureFolders failed: %v", err)
	}

	folders, err := testDB.ListFolders(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListFolders failed: %v", err)
	}

	// Should have created prefix, prefix/b, prefix/b/c
	expected := map[string]bool{
		prefix:          false,
		prefix + "/b":   false,
		prefix + "/b/c": false,
	}
	for _, f := range folders {
		if _, ok := expected[f.Path]; ok {
			expected[f.Path] = true
		}
	}
	for p, found := range expected {
		if !found {
			t.Errorf("Expected folder %q to exist", p)
		}
	}
}

func TestEnsureFolderPath(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/efp-%d", time.Now().UnixNano())
	deepPath := prefix + "/a/b/c"

	if err := testDB.EnsureFolderPath(ctx, vaultID, deepPath); err != nil {
		t.Fatalf("EnsureFolderPath failed: %v", err)
	}

	// All ancestor folders should exist
	for _, fp := range []string{prefix, prefix + "/a", prefix + "/a/b", deepPath} {
		got, err := testDB.GetFolderByPath(ctx, vaultID, fp)
		if err != nil {
			t.Fatalf("GetFolderByPath %s failed: %v", fp, err)
		}
		if got == nil {
			t.Errorf("expected folder %s to exist after EnsureFolderPath", fp)
		}
	}

	// Idempotent — calling again must not error
	if err := testDB.EnsureFolderPath(ctx, vaultID, deepPath); err != nil {
		t.Fatalf("EnsureFolderPath (idempotent) failed: %v", err)
	}

	// Root path should be a no-op
	if err := testDB.EnsureFolderPath(ctx, vaultID, "/"); err != nil {
		t.Fatalf("EnsureFolderPath root failed: %v", err)
	}
}

func TestListFolders(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/lf-%d", time.Now().UnixNano())
	for _, p := range []string{prefix + "/a", prefix + "/b", prefix + "/c"} {
		_, err := testDB.CreateFolder(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("CreateFolder %s failed: %v", p, err)
		}
	}

	folders, err := testDB.ListFolders(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListFolders failed: %v", err)
	}
	if len(folders) < 3 {
		t.Errorf("Expected at least 3 folders, got %d", len(folders))
	}
}

func TestGetFolderByPath(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	folderPath := fmt.Sprintf("/gfbp-%d", time.Now().UnixNano())
	_, err := testDB.CreateFolder(ctx, vaultID, folderPath)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}

	got, err := testDB.GetFolderByPath(ctx, vaultID, folderPath)
	if err != nil {
		t.Fatalf("GetFolderByPath failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetFolderByPath returned nil for existing folder")
	}
	if got.Path != folderPath {
		t.Errorf("Expected path %q, got %q", folderPath, got.Path)
	}

	// Test nonexistent
	notFound, err := testDB.GetFolderByPath(ctx, vaultID, "/nonexistent-folder-path")
	if err != nil {
		t.Fatalf("GetFolderByPath nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent folder path")
	}
}

func TestDeleteFolder(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/df-%d", time.Now().UnixNano())
	parent := prefix + "/parent"
	child := parent + "/sub"
	sibling := prefix + "/sibling"

	for _, fp := range []string{parent, child, sibling} {
		if _, err := testDB.CreateFolder(ctx, vaultID, fp); err != nil {
			t.Fatalf("CreateFolder %s failed: %v", fp, err)
		}
	}

	// Delete parent — should cascade to child
	if err := testDB.DeleteFolder(ctx, vaultID, parent); err != nil {
		t.Fatalf("DeleteFolder failed: %v", err)
	}

	got, err := testDB.GetFolderByPath(ctx, vaultID, parent)
	if err != nil {
		t.Fatalf("GetFolderByPath parent failed: %v", err)
	}
	if got != nil {
		t.Error("parent folder should be deleted")
	}

	gotChild, err := testDB.GetFolderByPath(ctx, vaultID, child)
	if err != nil {
		t.Fatalf("GetFolderByPath child failed: %v", err)
	}
	if gotChild != nil {
		t.Error("child folder should be deleted with parent")
	}

	// Sibling should survive
	gotSibling, err := testDB.GetFolderByPath(ctx, vaultID, sibling)
	if err != nil {
		t.Fatalf("GetFolderByPath sibling failed: %v", err)
	}
	if gotSibling == nil {
		t.Error("sibling folder should survive parent deletion")
	}
}

func TestDeleteFoldersByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/dfp-%d", time.Now().UnixNano())
	base := prefix + "/docs"
	for _, p := range []string{base, base + "/a", base + "/a/nested", prefix + "/other"} {
		_, err := testDB.CreateFolder(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("CreateFolder %s failed: %v", p, err)
		}
	}

	// Delete base and all children
	count, err := testDB.DeleteFoldersByPrefix(ctx, vaultID, base)
	if err != nil {
		t.Fatalf("DeleteFoldersByPrefix failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 deleted (base + 2 children), got %d", count)
	}

	// All deleted folders should be gone
	for _, fp := range []string{base, base + "/a", base + "/a/nested"} {
		got, err := testDB.GetFolderByPath(ctx, vaultID, fp)
		if err != nil {
			t.Fatalf("GetFolderByPath %s failed: %v", fp, err)
		}
		if got != nil {
			t.Errorf("expected folder %s to be deleted", fp)
		}
	}

	// Verify /other survives
	survivor, err := testDB.GetFolderByPath(ctx, vaultID, prefix+"/other")
	if err != nil {
		t.Fatalf("GetFolderByPath failed: %v", err)
	}
	if survivor == nil {
		t.Error("Folder /other should survive deletion by prefix")
	}
}

func TestEnsureFolders_CachesRecentCalls(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	docPath := "/" + t.Name() + "/sub/doc.md"

	// First call should succeed (hits DB)
	if err := testDB.EnsureFolders(ctx, vaultID, docPath); err != nil {
		t.Fatalf("first EnsureFolders: %v", err)
	}

	// Verify folders were created
	folders, err := testDB.ListFolders(ctx, vaultID)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) == 0 {
		t.Fatal("expected folders after EnsureFolders")
	}

	// Second call for same path should succeed (from cache)
	if err := testDB.EnsureFolders(ctx, vaultID, docPath); err != nil {
		t.Fatalf("second EnsureFolders (cached): %v", err)
	}

	// Call for different file in same dir should also be cached
	if err := testDB.EnsureFolders(ctx, vaultID, "/"+t.Name()+"/sub/other.md"); err != nil {
		t.Fatalf("third EnsureFolders (same dir, cached): %v", err)
	}
}

func TestMoveFoldersByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/mfp-%d", time.Now().UnixNano())
	oldBase := prefix + "/old"
	newBase := prefix + "/new"
	for _, p := range []string{oldBase, oldBase + "/child"} {
		_, err := testDB.CreateFolder(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("CreateFolder %s failed: %v", p, err)
		}
	}

	count, err := testDB.MoveFoldersByPrefix(ctx, vaultID, oldBase, newBase)
	if err != nil {
		t.Fatalf("MoveFoldersByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 moved, got %d", count)
	}

	// Old paths should be gone
	for _, fp := range []string{oldBase, oldBase + "/child"} {
		got, err := testDB.GetFolderByPath(ctx, vaultID, fp)
		if err != nil {
			t.Fatalf("GetFolderByPath old %s failed: %v", fp, err)
		}
		if got != nil {
			t.Errorf("expected old folder %s to be gone after move", fp)
		}
	}

	// New paths should exist
	for _, fp := range []string{newBase, newBase + "/child"} {
		got, err := testDB.GetFolderByPath(ctx, vaultID, fp)
		if err != nil {
			t.Fatalf("GetFolderByPath new %s failed: %v", fp, err)
		}
		if got == nil {
			t.Errorf("expected new folder %s to exist after move", fp)
		}
	}
}
