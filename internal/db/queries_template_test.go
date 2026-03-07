package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestCreateTemplate(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	desc := "A test template"
	tmpl, err := testDB.CreateTemplate(ctx, models.TemplateInput{
		VaultID:      &vaultID,
		Name:         "template-" + suffix,
		Description:  &desc,
		Content:      "Hello {{.Name}}",
		IsAITemplate: true,
	})
	if err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	if tmpl.Name != "template-"+suffix {
		t.Errorf("Expected name %q, got %q", "template-"+suffix, tmpl.Name)
	}
	if tmpl.Content != "Hello {{.Name}}" {
		t.Errorf("Expected content 'Hello {{.Name}}', got %q", tmpl.Content)
	}
	if tmpl.Description == nil || *tmpl.Description != desc {
		t.Errorf("Expected description %q, got %v", desc, tmpl.Description)
	}
	if !tmpl.IsAITemplate {
		t.Error("Expected IsAITemplate to be true")
	}
}

func TestGetTemplate(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	tmpl, err := testDB.CreateTemplate(ctx, models.TemplateInput{
		VaultID: &vaultID,
		Name:    "get-tmpl-" + suffix,
		Content: "content",
	})
	if err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	tmplID := models.MustRecordIDString(tmpl.ID)

	fetched, err := testDB.GetTemplate(ctx, tmplID)
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetTemplate returned nil for existing template")
	}
	if fetched.Name != "get-tmpl-"+suffix {
		t.Errorf("Expected name %q, got %q", "get-tmpl-"+suffix, fetched.Name)
	}

	// Nonexistent template should return nil
	notFound, err := testDB.GetTemplate(ctx, "template:nonexistent_"+suffix)
	if err != nil {
		t.Fatalf("GetTemplate nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent template")
	}
}

func TestListTemplates(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	// Create a vault-scoped template
	_, err := testDB.CreateTemplate(ctx, models.TemplateInput{
		VaultID: &vaultID,
		Name:    "vault-tmpl-" + suffix,
		Content: "vault content",
	})
	if err != nil {
		t.Fatalf("CreateTemplate (vault) failed: %v", err)
	}

	// Create a global template (no vault)
	_, err = testDB.CreateTemplate(ctx, models.TemplateInput{
		Name:    "global-tmpl-" + suffix,
		Content: "global content",
	})
	if err != nil {
		t.Fatalf("CreateTemplate (global) failed: %v", err)
	}

	// List all (nil vaultID) — should return at least both
	all, err := testDB.ListTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListTemplates (nil) failed: %v", err)
	}
	var foundVault, foundGlobal bool
	for _, tmpl := range all {
		if tmpl.Name == "vault-tmpl-"+suffix {
			foundVault = true
		}
		if tmpl.Name == "global-tmpl-"+suffix {
			foundGlobal = true
		}
	}
	if !foundVault {
		t.Error("ListTemplates (nil) should include vault-scoped template")
	}
	if !foundGlobal {
		t.Error("ListTemplates (nil) should include global template")
	}

	// List with specific vaultID
	vaultOnly, err := testDB.ListTemplates(ctx, &vaultID)
	if err != nil {
		t.Fatalf("ListTemplates (vaultID) failed: %v", err)
	}
	var foundVaultScoped bool
	for _, tmpl := range vaultOnly {
		if tmpl.Name == "vault-tmpl-"+suffix {
			foundVaultScoped = true
		}
	}
	if !foundVaultScoped {
		t.Error("ListTemplates (vaultID) should include vault-scoped template")
	}
}

func TestDeleteTemplate(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	tmpl, err := testDB.CreateTemplate(ctx, models.TemplateInput{
		VaultID: &vaultID,
		Name:    "delete-tmpl-" + suffix,
		Content: "to delete",
	})
	if err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	tmplID := models.MustRecordIDString(tmpl.ID)

	err = testDB.DeleteTemplate(ctx, tmplID)
	if err != nil {
		t.Fatalf("DeleteTemplate failed: %v", err)
	}

	gone, err := testDB.GetTemplate(ctx, tmplID)
	if err != nil {
		t.Fatalf("GetTemplate after delete error: %v", err)
	}
	if gone != nil {
		t.Error("Template should be nil after delete")
	}
}
