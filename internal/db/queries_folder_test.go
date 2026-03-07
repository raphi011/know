package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
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
	if folder.Name != folderPath[1:] { // Base name without leading slash
		t.Errorf("Expected name %q, got %q", folderPath[1:], folder.Name)
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

	folderPath := fmt.Sprintf("/df-%d", time.Now().UnixNano())
	_, err := testDB.CreateFolder(ctx, vaultID, folderPath)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}

	err = testDB.DeleteFolder(ctx, vaultID, folderPath)
	if err != nil {
		t.Fatalf("DeleteFolder failed: %v", err)
	}

	got, err := testDB.GetFolderByPath(ctx, vaultID, folderPath)
	if err != nil {
		t.Fatalf("GetFolderByPath after delete failed: %v", err)
	}
	if got != nil {
		t.Error("Folder should be nil after delete")
	}
}

func TestDeleteFoldersByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/dfp-%d", time.Now().UnixNano())
	for _, p := range []string{prefix + "/docs/a", prefix + "/docs/b", prefix + "/other"} {
		_, err := testDB.CreateFolder(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("CreateFolder %s failed: %v", p, err)
		}
	}

	count, err := testDB.DeleteFoldersByPrefix(ctx, vaultID, prefix+"/docs/")
	if err != nil {
		t.Fatalf("DeleteFoldersByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 deleted, got %d", count)
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

func TestMoveFoldersByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := fmt.Sprintf("/mfp-%d", time.Now().UnixNano())
	for _, p := range []string{prefix + "/old/a", prefix + "/old/b"} {
		_, err := testDB.CreateFolder(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("CreateFolder %s failed: %v", p, err)
		}
	}

	count, err := testDB.MoveFoldersByPrefix(ctx, vaultID, prefix+"/old/", prefix+"/new/")
	if err != nil {
		t.Fatalf("MoveFoldersByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 moved, got %d", count)
	}
}
