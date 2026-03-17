package db

import (
	"context"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestCreateVaultMember(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	member, err := testDB.CreateVaultMember(ctx, userID, vaultID, models.RoleRead)
	if err != nil {
		t.Fatalf("CreateVaultMember failed: %v", err)
	}
	if member == nil {
		t.Fatal("CreateVaultMember returned nil")
	}
	if member.Role != string(models.RoleRead) {
		t.Errorf("expected role %q, got %q", models.RoleRead, member.Role)
	}
	if models.MustRecordIDString(member.User) != userID {
		t.Errorf("expected user %q, got %q", userID, models.MustRecordIDString(member.User))
	}
	if models.MustRecordIDString(member.Vault) != vaultID {
		t.Errorf("expected vault %q, got %q", vaultID, models.MustRecordIDString(member.Vault))
	}
	if member.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestGetVaultMemberships(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	vault1 := createTestVault(t, ctx, userID)
	vault1ID := models.MustRecordIDString(vault1.ID)
	vault2 := createTestVault(t, ctx, userID)
	vault2ID := models.MustRecordIDString(vault2.ID)

	_, err := testDB.CreateVaultMember(ctx, userID, vault1ID, models.RoleRead)
	if err != nil {
		t.Fatalf("CreateVaultMember vault1 failed: %v", err)
	}
	_, err = testDB.CreateVaultMember(ctx, userID, vault2ID, models.RoleWrite)
	if err != nil {
		t.Fatalf("CreateVaultMember vault2 failed: %v", err)
	}

	memberships, err := testDB.GetVaultMemberships(ctx, userID)
	if err != nil {
		t.Fatalf("GetVaultMemberships failed: %v", err)
	}

	vaultIDs := make(map[string]bool)
	for _, m := range memberships {
		vaultIDs[models.MustRecordIDString(m.Vault)] = true
	}
	if !vaultIDs[vault1ID] {
		t.Errorf("expected vault1 %q in memberships", vault1ID)
	}
	if !vaultIDs[vault2ID] {
		t.Errorf("expected vault2 %q in memberships", vault2ID)
	}
}

func TestGetVaultMembers(t *testing.T) {
	ctx := context.Background()
	owner := createTestUser(t, ctx)
	ownerID := models.MustRecordIDString(owner.ID)
	vault := createTestVault(t, ctx, ownerID)
	vaultID := models.MustRecordIDString(vault.ID)

	user2 := createTestUser(t, ctx)
	user2ID := models.MustRecordIDString(user2.ID)

	_, err := testDB.CreateVaultMember(ctx, ownerID, vaultID, models.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateVaultMember owner failed: %v", err)
	}
	_, err = testDB.CreateVaultMember(ctx, user2ID, vaultID, models.RoleRead)
	if err != nil {
		t.Fatalf("CreateVaultMember user2 failed: %v", err)
	}

	members, err := testDB.GetVaultMembers(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVaultMembers failed: %v", err)
	}

	userIDs := make(map[string]bool)
	for _, m := range members {
		userIDs[models.MustRecordIDString(m.User)] = true
	}
	if !userIDs[ownerID] {
		t.Errorf("expected owner %q in members", ownerID)
	}
	if !userIDs[user2ID] {
		t.Errorf("expected user2 %q in members", user2ID)
	}
}
