package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func insertTestShareLink(t *testing.T, ctx context.Context, userID, vaultID, hash, path string) {
	t.Helper()
	sql := `INSERT INTO share_link {
		vault: type::record("vault", $vault_id),
		token_hash: $hash,
		path: $path,
		is_folder: false,
		created_by: type::record("user", $user_id)
	}`
	_, err := surrealdb.Query[any](ctx, testDB.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"hash":     hash,
		"path":     path,
		"user_id":  bareID("user", userID),
	})
	if err != nil {
		t.Fatalf("insert share_link failed: %v", err)
	}
}

func TestGetShareLinkByHash(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	hash := fmt.Sprintf("share-hash-%d", time.Now().UnixNano())
	path := "/notes/secret.md"

	insertTestShareLink(t, ctx, userID, vaultID, hash, path)

	link, err := testDB.GetShareLinkByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetShareLinkByHash failed: %v", err)
	}
	if link == nil {
		t.Fatal("GetShareLinkByHash returned nil")
	}
	if link.TokenHash != hash {
		t.Errorf("expected token_hash %q, got %q", hash, link.TokenHash)
	}
	if link.Path != path {
		t.Errorf("expected path %q, got %q", path, link.Path)
	}
	if link.IsFolder {
		t.Error("expected is_folder to be false")
	}
	if models.MustRecordIDString(link.Vault) != vaultID {
		t.Errorf("expected vault %q, got %q", vaultID, models.MustRecordIDString(link.Vault))
	}
	if models.MustRecordIDString(link.CreatedBy) != userID {
		t.Errorf("expected created_by %q, got %q", userID, models.MustRecordIDString(link.CreatedBy))
	}
	if link.ExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil")
	}
}

func TestGetShareLinkByHash_NotFound(t *testing.T) {
	ctx := context.Background()

	link, err := testDB.GetShareLinkByHash(ctx, "nonexistent-hash-"+fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("GetShareLinkByHash failed: %v", err)
	}
	if link != nil {
		t.Errorf("expected nil for nonexistent hash, got %+v", link)
	}
}
