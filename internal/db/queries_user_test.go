package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateUser(t *testing.T) {
	ctx := context.Background()

	name := fmt.Sprintf("user-%d", time.Now().UnixNano())
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: name})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if user.Name != name {
		t.Errorf("Expected name %q, got %q", name, user.Name)
	}
}

func TestCreateUserWithID(t *testing.T) {
	ctx := context.Background()

	customID := fmt.Sprintf("custom-user-id-%d", time.Now().UnixNano())
	name := fmt.Sprintf("user-custom-%d", time.Now().UnixNano())
	user, err := testDB.CreateUserWithID(ctx, customID, models.UserInput{Name: name})
	if err != nil {
		t.Fatalf("CreateUserWithID failed: %v", err)
	}

	userID := models.MustRecordIDString(user.ID)
	if userID != customID {
		t.Errorf("Expected user ID %q, got %q", customID, userID)
	}
	if user.Name != name {
		t.Errorf("Expected name %q, got %q", name, user.Name)
	}
}

func TestGetUser(t *testing.T) {
	ctx := context.Background()

	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	retrieved, err := testDB.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetUser returned nil")
	}
	if retrieved.Name != user.Name {
		t.Errorf("Expected name %q, got %q", user.Name, retrieved.Name)
	}

	nonExistent, err := testDB.GetUser(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetUser non-existent should not error: %v", err)
	}
	if nonExistent != nil {
		t.Error("GetUser non-existent should return nil")
	}
}

func TestUpdateUserSystemAdmin(t *testing.T) {
	ctx := context.Background()

	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	// Set admin=true
	if err := testDB.UpdateUserSystemAdmin(ctx, userID, true); err != nil {
		t.Fatalf("UpdateUserSystemAdmin true failed: %v", err)
	}

	got, err := testDB.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetUser returned nil")
	}
	if !got.IsSystemAdmin {
		t.Error("Expected is_system_admin=true")
	}

	// Set admin=false
	if err := testDB.UpdateUserSystemAdmin(ctx, userID, false); err != nil {
		t.Fatalf("UpdateUserSystemAdmin false failed: %v", err)
	}

	got2, err := testDB.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got2 == nil {
		t.Fatal("GetUser returned nil")
	}
	if got2.IsSystemAdmin {
		t.Error("Expected is_system_admin=false")
	}
}

func TestGetUserByOIDCSubject(t *testing.T) {
	ctx := context.Background()

	provider := "https://accounts.google.com"
	subject := fmt.Sprintf("oidc-sub-%d", time.Now().UnixNano())
	name := fmt.Sprintf("oidc-user-%d", time.Now().UnixNano())
	email := fmt.Sprintf("oidc-%d@example.com", time.Now().UnixNano())

	// Create user with OIDC fields
	created, err := testDB.CreateUserFromOIDC(ctx, provider, subject, name, email)
	if err != nil {
		t.Fatalf("CreateUserFromOIDC failed: %v", err)
	}

	// Find by provider+subject
	found, err := testDB.GetUserByOIDCSubject(ctx, provider, subject)
	if err != nil {
		t.Fatalf("GetUserByOIDCSubject failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetUserByOIDCSubject returned nil for existing user")
	}
	if found.Name != created.Name {
		t.Errorf("Expected name %q, got %q", created.Name, found.Name)
	}

	// Not found
	notFound, err := testDB.GetUserByOIDCSubject(ctx, provider, "nonexistent-subject")
	if err != nil {
		t.Errorf("GetUserByOIDCSubject non-existent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("GetUserByOIDCSubject non-existent should return nil")
	}
}

func TestGetUserByEmail(t *testing.T) {
	ctx := context.Background()

	email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	name := fmt.Sprintf("email-user-%d", time.Now().UnixNano())

	_, err := testDB.CreateUser(ctx, models.UserInput{Name: name, Email: &email})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Find by email
	found, err := testDB.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetUserByEmail returned nil for existing user")
	}
	if found.Name != name {
		t.Errorf("Expected name %q, got %q", name, found.Name)
	}

	// Not found
	notFound, err := testDB.GetUserByEmail(ctx, "nonexistent@example.com")
	if err != nil {
		t.Errorf("GetUserByEmail non-existent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("GetUserByEmail non-existent should return nil")
	}
}

func TestCreateUserFromOIDC(t *testing.T) {
	ctx := context.Background()

	provider := "https://accounts.google.com"
	subject := fmt.Sprintf("oidc-create-%d", time.Now().UnixNano())
	name := fmt.Sprintf("oidc-create-user-%d", time.Now().UnixNano())
	email := fmt.Sprintf("oidc-create-%d@example.com", time.Now().UnixNano())

	user, err := testDB.CreateUserFromOIDC(ctx, provider, subject, name, email)
	if err != nil {
		t.Fatalf("CreateUserFromOIDC failed: %v", err)
	}
	if user.Name != name {
		t.Errorf("Expected name %q, got %q", name, user.Name)
	}
	if user.Email == nil || *user.Email != email {
		t.Errorf("Expected email %q, got %v", email, user.Email)
	}
	if user.OIDCProvider == nil || *user.OIDCProvider != provider {
		t.Errorf("Expected oidc_provider %q, got %v", provider, user.OIDCProvider)
	}
	if user.OIDCSubject == nil || *user.OIDCSubject != subject {
		t.Errorf("Expected oidc_subject %q, got %v", subject, user.OIDCSubject)
	}
}

func TestLinkOIDCIdentity(t *testing.T) {
	ctx := context.Background()

	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	provider := "https://accounts.google.com"
	subject := fmt.Sprintf("link-sub-%d", time.Now().UnixNano())

	if err := testDB.LinkOIDCIdentity(ctx, userID, provider, subject); err != nil {
		t.Fatalf("LinkOIDCIdentity failed: %v", err)
	}

	// Verify fields updated
	got, err := testDB.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetUser returned nil")
	}
	if got.OIDCProvider == nil || *got.OIDCProvider != provider {
		t.Errorf("Expected oidc_provider %q, got %v", provider, got.OIDCProvider)
	}
	if got.OIDCSubject == nil || *got.OIDCSubject != subject {
		t.Errorf("Expected oidc_subject %q, got %v", subject, got.OIDCSubject)
	}

	// Verify findable by OIDC subject
	found, err := testDB.GetUserByOIDCSubject(ctx, provider, subject)
	if err != nil {
		t.Fatalf("GetUserByOIDCSubject failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetUserByOIDCSubject returned nil after linking")
	}
}

func TestListUsers(t *testing.T) {
	ctx := context.Background()

	name1 := fmt.Sprintf("list-user-a-%d", time.Now().UnixNano())
	name2 := fmt.Sprintf("list-user-b-%d", time.Now().UnixNano())

	if _, err := testDB.CreateUser(ctx, models.UserInput{Name: name1}); err != nil {
		t.Fatalf("CreateUser 1 failed: %v", err)
	}
	if _, err := testDB.CreateUser(ctx, models.UserInput{Name: name2}); err != nil {
		t.Fatalf("CreateUser 2 failed: %v", err)
	}

	users, err := testDB.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) < 2 {
		t.Fatalf("Expected at least 2 users, got %d", len(users))
	}

	// Verify created users appear in results
	found := map[string]bool{}
	for _, u := range users {
		found[u.Name] = true
	}
	if !found[name1] {
		t.Errorf("Expected user %q in list", name1)
	}
	if !found[name2] {
		t.Errorf("Expected user %q in list", name2)
	}

	// Verify ordering (ASC by name)
	for i := 1; i < len(users); i++ {
		if users[i].Name < users[i-1].Name {
			t.Errorf("Users not ordered by name: %q < %q", users[i].Name, users[i-1].Name)
		}
	}
}

func TestProvisionUser(t *testing.T) {
	ctx := context.Background()

	name := fmt.Sprintf("provision-%d", time.Now().UnixNano())
	email := fmt.Sprintf("provision-%d@example.com", time.Now().UnixNano())

	user, vault, err := testDB.ProvisionUser(ctx, models.UserInput{
		Name:  name,
		Email: &email,
	})
	if err != nil {
		t.Fatalf("ProvisionUser failed: %v", err)
	}

	if user.Name != name {
		t.Errorf("Expected user name %q, got %q", name, user.Name)
	}
	if user.Email == nil || *user.Email != email {
		t.Errorf("Expected email %q, got %v", email, user.Email)
	}

	// Vault should be named after the user and owned by them
	if vault.Name != name {
		t.Errorf("Expected vault name %q, got %q", name, vault.Name)
	}
	if vault.Owner == nil {
		t.Fatal("Expected vault to have an owner")
	}

	userID := models.MustRecordIDString(user.ID)
	ownerID := models.MustRecordIDString(*vault.Owner)
	if ownerID != userID {
		t.Errorf("Expected vault owner %q, got %q", userID, ownerID)
	}

	// Verify vault membership
	vaultID := models.MustRecordIDString(vault.ID)
	members, err := testDB.GetVaultMembers(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVaultMembers failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("Expected 1 vault member, got %d", len(members))
	}
	if members[0].Role != string(models.RoleAdmin) {
		t.Errorf("Expected admin role, got %q", members[0].Role)
	}
}

func TestProvisionUserFromOIDC(t *testing.T) {
	ctx := context.Background()

	provider := "https://accounts.google.com"
	subject := fmt.Sprintf("provision-oidc-sub-%d", time.Now().UnixNano())
	name := fmt.Sprintf("provision-oidc-%d", time.Now().UnixNano())
	email := fmt.Sprintf("provision-oidc-%d@example.com", time.Now().UnixNano())

	user, vault, err := testDB.ProvisionUserFromOIDC(ctx, provider, subject, name, email)
	if err != nil {
		t.Fatalf("ProvisionUserFromOIDC failed: %v", err)
	}

	// Verify user fields
	if user.Name != name {
		t.Errorf("Expected user name %q, got %q", name, user.Name)
	}
	if user.Email == nil || *user.Email != email {
		t.Errorf("Expected email %q, got %v", email, user.Email)
	}
	if user.OIDCProvider == nil || *user.OIDCProvider != provider {
		t.Errorf("Expected oidc_provider %q, got %v", provider, user.OIDCProvider)
	}
	if user.OIDCSubject == nil || *user.OIDCSubject != subject {
		t.Errorf("Expected oidc_subject %q, got %v", subject, user.OIDCSubject)
	}

	// Vault should be named after the user and owned by them
	if vault.Name != name {
		t.Errorf("Expected vault name %q, got %q", name, vault.Name)
	}
	if vault.Owner == nil {
		t.Fatal("Expected vault to have an owner")
	}

	userID := models.MustRecordIDString(user.ID)
	ownerID := models.MustRecordIDString(*vault.Owner)
	if ownerID != userID {
		t.Errorf("Expected vault owner %q, got %q", userID, ownerID)
	}

	// Verify vault membership
	vaultID := models.MustRecordIDString(vault.ID)
	members, err := testDB.GetVaultMembers(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVaultMembers failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("Expected 1 vault member, got %d", len(members))
	}
	if members[0].Role != string(models.RoleAdmin) {
		t.Errorf("Expected admin role, got %q", members[0].Role)
	}
}

func TestGetUserByName(t *testing.T) {
	ctx := context.Background()

	uniqueName := fmt.Sprintf("unique-user-%d", time.Now().UnixNano())
	_, err := testDB.CreateUser(ctx, models.UserInput{Name: uniqueName})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	found, err := testDB.GetUserByName(ctx, uniqueName)
	if err != nil {
		t.Fatalf("GetUserByName failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetUserByName returned nil for existing user")
	}
	if found.Name != uniqueName {
		t.Errorf("Expected name %q, got %q", uniqueName, found.Name)
	}

	notFound, err := testDB.GetUserByName(ctx, "nonexistent-user-name")
	if err != nil {
		t.Errorf("GetUserByName non-existent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("GetUserByName non-existent should return nil")
	}
}
