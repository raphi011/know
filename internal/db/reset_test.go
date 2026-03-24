package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestListAllTokens(t *testing.T) {
	ctx := context.Background()
	user1 := createTestUser(t, ctx)
	user1ID := models.MustRecordIDString(user1.ID)
	user2 := createTestUser(t, ctx)
	user2ID := models.MustRecordIDString(user2.ID)

	hash1 := hashString("token1-" + user1ID)
	hash2 := hashString("token2-" + user2ID)

	_, err := testDB.CreateToken(ctx, user1ID, hash1, "t1", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create token 1: %v", err)
	}
	_, err = testDB.CreateToken(ctx, user2ID, hash2, "t2", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create token 2: %v", err)
	}

	tokens, err := testDB.ListAllTokens(ctx)
	if err != nil {
		t.Fatalf("ListAllTokens: %v", err)
	}
	if len(tokens) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
	}

	hashes := make(map[string]bool)
	for _, tok := range tokens {
		hashes[tok.TokenHash] = true
	}
	if !hashes[hash1] {
		t.Error("expected token1 hash in results")
	}
	if !hashes[hash2] {
		t.Error("expected token2 hash in results")
	}
}

func TestListAllVaultMembers(t *testing.T) {
	ctx := context.Background()
	user1 := createTestUser(t, ctx)
	user1ID := models.MustRecordIDString(user1.ID)
	user2 := createTestUser(t, ctx)
	user2ID := models.MustRecordIDString(user2.ID)

	vault := createTestVault(t, ctx, user1ID)
	vaultID := models.MustRecordIDString(vault.ID)

	_, err := testDB.CreateVaultMember(ctx, user1ID, vaultID, models.RoleAdmin)
	if err != nil {
		t.Fatalf("create member 1: %v", err)
	}
	_, err = testDB.CreateVaultMember(ctx, user2ID, vaultID, models.RoleRead)
	if err != nil {
		t.Fatalf("create member 2: %v", err)
	}

	members, err := testDB.ListAllVaultMembers(ctx)
	if err != nil {
		t.Fatalf("ListAllVaultMembers: %v", err)
	}
	if len(members) < 2 {
		t.Fatalf("expected at least 2 members, got %d", len(members))
	}
}

func TestResetSchemaAndRestoreIdentity(t *testing.T) {
	ctx := context.Background()

	// Create a user with OIDC fields to exercise optionalString round-trip.
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	if err := testDB.UpdateUserSystemAdmin(ctx, userID, true); err != nil {
		t.Fatalf("set admin: %v", err)
	}
	if err := testDB.LinkOIDCIdentity(ctx, userID, "google", "subject-123"); err != nil {
		t.Fatalf("link oidc: %v", err)
	}

	// Create a private vault (with owner) and settings to exercise those branches.
	vault, err := testDB.CreateVaultWithOwner(ctx, userID, models.VaultInput{
		Name: "reset-test-vault",
	})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(vault.ID)

	settings := models.VaultSettings{MemoryPath: "/memories", RRFK: 42}
	_, err = testDB.UpdateVaultSettings(ctx, vaultID, settings)
	if err != nil {
		t.Fatalf("update vault settings: %v", err)
	}

	_, err = testDB.CreateVaultMember(ctx, userID, vaultID, models.RoleAdmin)
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	tokenHash := hashString("reset-test-token-" + userID)
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
	_, err = testDB.CreateToken(ctx, userID, tokenHash, "reset-test", expiresAt)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// Snapshot identity.
	snap, err := testDB.SnapshotIdentity(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if len(snap.Users) == 0 {
		t.Fatal("snapshot has no users")
	}
	if len(snap.Vaults) == 0 {
		t.Fatal("snapshot has no vaults")
	}

	// Reset schema (drops all tables, re-applies schema).
	if err := testDB.ResetSchema(ctx, 384); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	// Verify tables are empty after reset.
	users, err := testDB.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users after reset: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users after reset, got %d", len(users))
	}

	vaults, err := testDB.ListVaults(ctx)
	if err != nil {
		t.Fatalf("list vaults after reset: %v", err)
	}
	if len(vaults) != 0 {
		t.Errorf("expected 0 vaults after reset, got %d", len(vaults))
	}

	// Restore identity.
	if err := testDB.RestoreIdentity(ctx, snap); err != nil {
		t.Fatalf("restore identity: %v", err)
	}

	// Verify users are restored.
	restoredUsers, err := testDB.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users after restore: %v", err)
	}
	if len(restoredUsers) != len(snap.Users) {
		t.Errorf("expected %d users after restore, got %d", len(snap.Users), len(restoredUsers))
	}

	// Verify user fields: admin flag and OIDC fields.
	restoredUser, err := testDB.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("get user after restore: %v", err)
	}
	if restoredUser == nil {
		t.Fatal("restored user is nil")
	}
	if !restoredUser.IsSystemAdmin {
		t.Error("expected user to still be system admin after restore")
	}
	if restoredUser.OIDCProvider == nil || *restoredUser.OIDCProvider != "google" {
		t.Errorf("expected oidc_provider %q, got %v", "google", restoredUser.OIDCProvider)
	}
	if restoredUser.OIDCSubject == nil || *restoredUser.OIDCSubject != "subject-123" {
		t.Errorf("expected oidc_subject %q, got %v", "subject-123", restoredUser.OIDCSubject)
	}

	// Verify vault ID is preserved.
	restoredVaults, err := testDB.ListVaults(ctx)
	if err != nil {
		t.Fatalf("list vaults after restore: %v", err)
	}
	if len(restoredVaults) != len(snap.Vaults) {
		t.Errorf("expected %d vaults after restore, got %d", len(snap.Vaults), len(restoredVaults))
	}

	restoredVault, err := testDB.GetVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("get vault after restore: %v", err)
	}
	if restoredVault == nil {
		t.Fatal("restored vault is nil")
	}

	// Verify vault owner is preserved (private vault).
	if restoredVault.Owner == nil {
		t.Fatal("expected vault to have owner after restore")
	}
	ownerID, err := models.RecordIDString(*restoredVault.Owner)
	if err != nil {
		t.Fatalf("owner record id: %v", err)
	}
	if bareID("user", ownerID) != bareID("user", userID) {
		t.Errorf("expected vault owner %q, got %q", userID, ownerID)
	}

	// Verify vault settings are preserved.
	if restoredVault.Settings == nil {
		t.Fatal("expected vault to have settings after restore")
	}
	if restoredVault.Settings.MemoryPath != "/memories" {
		t.Errorf("expected settings.memory_path %q, got %q", "/memories", restoredVault.Settings.MemoryPath)
	}
	if restoredVault.Settings.RRFK != 42 {
		t.Errorf("expected settings.rrf_k %d, got %d", 42, restoredVault.Settings.RRFK)
	}

	// Verify token is restored with correct fields.
	tok, err := testDB.GetTokenByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("get token by hash after restore: %v", err)
	}
	if tok == nil {
		t.Fatal("restored token is nil")
	}
	if tok.Name != "reset-test" {
		t.Errorf("expected token name %q, got %q", "reset-test", tok.Name)
	}
	if tok.ExpiresAt == nil {
		t.Error("expected token expires_at to be preserved")
	} else if !tok.ExpiresAt.Truncate(time.Second).Equal(expiresAt) {
		t.Errorf("expected token expires_at %v, got %v", expiresAt, tok.ExpiresAt.Truncate(time.Second))
	}

	// Verify vault members are restored with correct role.
	members, err := testDB.GetVaultMembers(ctx, vaultID)
	if err != nil {
		t.Fatalf("get vault members after restore: %v", err)
	}
	if len(members) == 0 {
		t.Fatal("expected at least 1 vault member after restore")
	}
	foundAdmin := false
	for _, m := range members {
		mUserID, err := models.RecordIDString(m.User)
		if err != nil {
			t.Fatalf("record id from vault member: %v", err)
		}
		if bareID("user", mUserID) == bareID("user", userID) && m.Role == string(models.RoleAdmin) {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Error("expected vault member with admin role after restore")
	}
}

func TestAllTablesMatchesSchema(t *testing.T) {
	// Extract all table names from SchemaSQL and verify allTables() is complete.
	re := regexp.MustCompile(`DEFINE TABLE IF NOT EXISTS (\w+)`)
	matches := re.FindAllStringSubmatch(SchemaSQL(384), -1)

	schemaTables := make([]string, 0, len(matches))
	for _, m := range matches {
		schemaTables = append(schemaTables, m[1])
	}
	sort.Strings(schemaTables)

	resetTables := make([]string, len(allTables()))
	copy(resetTables, allTables())
	sort.Strings(resetTables)

	if len(schemaTables) != len(resetTables) {
		t.Fatalf("schema defines %d tables but allTables() has %d\n  schema: %v\n  allTables: %v",
			len(schemaTables), len(resetTables), schemaTables, resetTables)
	}

	for i := range schemaTables {
		if schemaTables[i] != resetTables[i] {
			t.Errorf("table mismatch at index %d: schema has %q, allTables has %q",
				i, schemaTables[i], resetTables[i])
		}
	}
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
