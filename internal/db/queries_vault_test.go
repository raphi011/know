package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestCreateVault(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	desc := "A test vault"
	vault, err := testDB.CreateVault(ctx, userID, models.VaultInput{
		Name:        "My Vault",
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("CreateVault failed: %v", err)
	}
	if vault.Name != "My Vault" {
		t.Errorf("Expected name 'My Vault', got %q", vault.Name)
	}
	if vault.Description == nil || *vault.Description != "A test vault" {
		t.Errorf("Expected description 'A test vault', got %v", vault.Description)
	}
}

func TestGetVault(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	retrieved, err := testDB.GetVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVault failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetVault returned nil")
	}
	if retrieved.Name != vault.Name {
		t.Errorf("Expected name %q, got %q", vault.Name, retrieved.Name)
	}

	nonExistent, err := testDB.GetVault(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetVault non-existent should not error: %v", err)
	}
	if nonExistent != nil {
		t.Error("GetVault non-existent should return nil")
	}
}

func TestListVaults(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	_ = createTestVault(t, ctx, userID)

	vaults, err := testDB.ListVaults(ctx)
	if err != nil {
		t.Fatalf("ListVaults failed: %v", err)
	}
	if len(vaults) == 0 {
		t.Error("ListVaults should return at least one vault")
	}
}

func TestDeleteVault(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create a document in the vault to test cascade
	hash := "testhash"
	_, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/test.md",
		Title:       "Test",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		ContentHash: &hash,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	err = testDB.DeleteVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("DeleteVault failed: %v", err)
	}

	deleted, err := testDB.GetVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVault after delete failed: %v", err)
	}
	if deleted != nil {
		t.Error("Vault should be nil after delete")
	}
}

func TestCreateVaultWithID(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	customID := fmt.Sprintf("custom-vault-id-%d", time.Now().UnixNano())
	vault, err := testDB.CreateVaultWithID(ctx, customID, userID, models.VaultInput{
		Name: "Vault With Custom ID",
	})
	if err != nil {
		t.Fatalf("CreateVaultWithID failed: %v", err)
	}

	vaultID := models.MustRecordIDString(vault.ID)
	if vaultID != customID {
		t.Errorf("Expected vault ID to contain %q, got %q", customID, vaultID)
	}
	if vault.Name != "Vault With Custom ID" {
		t.Errorf("Expected name 'Vault With Custom ID', got %q", vault.Name)
	}
}

func TestGetVaultByName(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	vaultName := fmt.Sprintf("unique-vault-%d", time.Now().UnixNano())
	_, err := testDB.CreateVault(ctx, userID, models.VaultInput{
		Name: vaultName,
	})
	if err != nil {
		t.Fatalf("CreateVault failed: %v", err)
	}

	found, err := testDB.GetVaultByName(ctx, vaultName)
	if err != nil {
		t.Fatalf("GetVaultByName failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetVaultByName returned nil for existing vault")
	}
	if found.Name != vaultName {
		t.Errorf("Expected name %q, got %q", vaultName, found.Name)
	}

	notFound, err := testDB.GetVaultByName(ctx, "nonexistent-vault-name")
	if err != nil {
		t.Errorf("GetVaultByName non-existent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("GetVaultByName non-existent should return nil")
	}
}

func TestListDocumentPaths(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	paths := []string{"/docs/a.md", "/docs/b.md", "/notes/c.md"}
	for i, p := range paths {
		hash := fmt.Sprintf("hash-%d-%d", i, time.Now().UnixNano())
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        p,
			Title:       fmt.Sprintf("Doc %d", i),
			Content:     "content",
			ContentBody: "content",
			Source:      models.SourceManual,
			ContentHash: &hash,
			Labels:      []string{},
		})
		if err != nil {
			t.Fatalf("CreateDocument for path %q failed: %v", p, err)
		}
	}

	result, err := testDB.ListDocumentPaths(ctx, vaultID)
	if err != nil {
		t.Fatalf("ListDocumentPaths failed: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("Expected 3 paths, got %d", len(result))
	}

	pathSet := map[string]bool{}
	for _, p := range result {
		pathSet[p] = true
	}
	for _, p := range paths {
		if !pathSet[p] {
			t.Errorf("Expected path %q in results", p)
		}
	}
}
