package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
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

	// Create a file in the vault to test cascade
	hash := "testhash"
	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/test.md",
		Title:   "Test",
		Content: "content",
		Hash:    &hash,
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
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

func TestCreateVaultWithOwner(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	vault, err := testDB.CreateVaultWithOwner(ctx, userID, models.VaultInput{
		Name: fmt.Sprintf("owned-vault-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("CreateVaultWithOwner failed: %v", err)
	}
	if vault.Owner == nil {
		t.Fatal("Expected vault to have an owner")
	}
	ownerID := models.MustRecordIDString(*vault.Owner)
	if ownerID != userID {
		t.Errorf("Expected owner %q, got %q", userID, ownerID)
	}

	// Shared vault (no owner) should have nil owner
	shared, err := testDB.CreateVault(ctx, userID, models.VaultInput{
		Name: fmt.Sprintf("shared-vault-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("CreateVault failed: %v", err)
	}
	if shared.Owner != nil {
		t.Error("Shared vault should not have an owner")
	}
}

func TestGetVaultByOwner(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	// No vault yet
	notFound, err := testDB.GetVaultByOwner(ctx, userID)
	if err != nil {
		t.Fatalf("GetVaultByOwner should not error for missing: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for user with no owned vault")
	}

	// Create owned vault
	created, err := testDB.CreateVaultWithOwner(ctx, userID, models.VaultInput{
		Name: fmt.Sprintf("findable-vault-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("CreateVaultWithOwner failed: %v", err)
	}

	found, err := testDB.GetVaultByOwner(ctx, userID)
	if err != nil {
		t.Fatalf("GetVaultByOwner failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetVaultByOwner returned nil for user with owned vault")
	}
	createdID := models.MustRecordIDString(created.ID)
	foundID := models.MustRecordIDString(found.ID)
	if foundID != createdID {
		t.Errorf("Expected vault ID %q, got %q", createdID, foundID)
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

func TestUpdateVaultSettings(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	settings := models.VaultSettings{
		MemoryPath:   "/custom-memories",
		TemplatePath: "/custom-templates",
		RRFK:         80,
	}
	updated, err := testDB.UpdateVaultSettings(ctx, vaultID, settings)
	if err != nil {
		t.Fatalf("UpdateVaultSettings failed: %v", err)
	}
	if updated == nil {
		t.Fatal("UpdateVaultSettings returned nil")
	}
	if updated.Settings == nil {
		t.Fatal("Expected settings to be set")
	}
	if updated.Settings.MemoryPath != "/custom-memories" {
		t.Errorf("Expected memory_path '/custom-memories', got %q", updated.Settings.MemoryPath)
	}
	if updated.Settings.RRFK != 80 {
		t.Errorf("Expected rrf_k 80, got %d", updated.Settings.RRFK)
	}

	// Re-fetch to confirm persistence
	fetched, err := testDB.GetVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVault after settings update failed: %v", err)
	}
	if fetched == nil || fetched.Settings == nil {
		t.Fatal("GetVault returned nil or nil settings")
	}
	if fetched.Settings.TemplatePath != "/custom-templates" {
		t.Errorf("Expected template_path '/custom-templates', got %q", fetched.Settings.TemplatePath)
	}
}

func TestGetVaultInfo(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	// Create a file with chunks and labels so stats are non-zero
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/info-test-" + suffix + ".md",
		Title:   "Info Test",
		Content: "content for info test",
		Labels:  []string{"info-label"},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Info chunk", Position: 0, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	info, err := testDB.GetVaultInfo(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVaultInfo failed: %v", err)
	}
	if info == nil {
		t.Fatal("GetVaultInfo returned nil")
	}
	if info.DocumentCount < 1 {
		t.Errorf("Expected DocumentCount >= 1, got %d", info.DocumentCount)
	}
	if info.ChunkTotal < 1 {
		t.Errorf("Expected ChunkTotal >= 1, got %d", info.ChunkTotal)
	}
}

func TestGetVaultsByIDs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	v1 := createTestVault(t, ctx, userID)
	v1ID := models.MustRecordIDString(v1.ID)
	v2 := createTestVault(t, ctx, userID)
	v2ID := models.MustRecordIDString(v2.ID)

	t.Run("returns matching vaults", func(t *testing.T) {
		vaults, err := testDB.GetVaultsByIDs(ctx, []string{v1ID, v2ID})
		if err != nil {
			t.Fatalf("GetVaultsByIDs failed: %v", err)
		}
		if len(vaults) != 2 {
			t.Fatalf("Expected 2 vaults, got %d", len(vaults))
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		vaults, err := testDB.GetVaultsByIDs(ctx, []string{})
		if err != nil {
			t.Fatalf("GetVaultsByIDs with empty input failed: %v", err)
		}
		if vaults != nil {
			t.Errorf("Expected nil, got %v", vaults)
		}
	})

	t.Run("non-existent IDs are excluded", func(t *testing.T) {
		vaults, err := testDB.GetVaultsByIDs(ctx, []string{v1ID, "nonexistent"})
		if err != nil {
			t.Fatalf("GetVaultsByIDs failed: %v", err)
		}
		if len(vaults) != 1 {
			t.Fatalf("Expected 1 vault, got %d", len(vaults))
		}
	})

	t.Run("handles prefixed IDs", func(t *testing.T) {
		vaults, err := testDB.GetVaultsByIDs(ctx, []string{"vault:" + v1ID})
		if err != nil {
			t.Fatalf("GetVaultsByIDs with prefixed ID failed: %v", err)
		}
		if len(vaults) != 1 {
			t.Fatalf("Expected 1 vault, got %d", len(vaults))
		}
	})
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
		_, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    p,
			Title:   fmt.Sprintf("Doc %d", i),
			Content: "content",
			Hash:    &hash,
			Labels:  []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile for path %q failed: %v", p, err)
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
