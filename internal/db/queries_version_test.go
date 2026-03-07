package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func createTestDocument(t *testing.T, ctx context.Context, vaultID string) *models.Document {
	t.Helper()
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        fmt.Sprintf("/version-test-%d.md", time.Now().UnixNano()),
		Title:       "Version Test Doc",
		Content:     "version test content",
		ContentBody: "version test content",
		Source:      models.SourceManual,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("Failed to create test document: %v", err)
	}
	return doc
}

func TestCreateVersion(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	v, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
		DocumentID:  docID,
		VaultID:     vaultID,
		Content:     "version 1 content",
		ContentHash: "hash1",
		Title:       "Version 1",
		Source:      models.SourceManual,
	}, 1)
	if err != nil {
		t.Fatalf("CreateVersion failed: %v", err)
	}
	if v.Version != 1 {
		t.Errorf("Expected version 1, got %d", v.Version)
	}
	if v.Title != "Version 1" {
		t.Errorf("Expected title 'Version 1', got %q", v.Title)
	}
	if v.Content != "version 1 content" {
		t.Errorf("Expected content 'version 1 content', got %q", v.Content)
	}
	if v.ContentHash != "hash1" {
		t.Errorf("Expected content_hash 'hash1', got %q", v.ContentHash)
	}
}

func TestGetVersion(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	created, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
		DocumentID:  docID,
		VaultID:     vaultID,
		Content:     "get version content",
		ContentHash: "hash-get",
		Title:       "Get Version",
		Source:      models.SourceManual,
	}, 1)
	if err != nil {
		t.Fatalf("CreateVersion failed: %v", err)
	}
	createdID := models.MustRecordIDString(created.ID)

	got, err := testDB.GetVersion(ctx, createdID)
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetVersion returned nil")
	}
	if got.Title != "Get Version" {
		t.Errorf("Expected title 'Get Version', got %q", got.Title)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}
}

func TestGetVersionByNumber(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	_, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
		DocumentID:  docID,
		VaultID:     vaultID,
		Content:     "by number content",
		ContentHash: "hash-num",
		Title:       "By Number",
		Source:      models.SourceManual,
	}, 1)
	if err != nil {
		t.Fatalf("CreateVersion failed: %v", err)
	}

	got, err := testDB.GetVersionByNumber(ctx, docID, 1)
	if err != nil {
		t.Fatalf("GetVersionByNumber failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetVersionByNumber returned nil")
	}
	if got.Title != "By Number" {
		t.Errorf("Expected title 'By Number', got %q", got.Title)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}
}

func TestListVersions(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	for i := 1; i <= 3; i++ {
		_, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
			DocumentID:  docID,
			VaultID:     vaultID,
			Content:     fmt.Sprintf("content v%d", i),
			ContentHash: fmt.Sprintf("hash-%d", i),
			Title:       fmt.Sprintf("Version %d", i),
			Source:      models.SourceManual,
		}, i)
		if err != nil {
			t.Fatalf("CreateVersion %d failed: %v", i, err)
		}
	}

	versions, err := testDB.ListVersions(ctx, docID, 2, 0)
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("Expected 2 versions with limit=2, got %d", len(versions))
	}
}

func TestGetLatestVersion(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	for i := 1; i <= 2; i++ {
		_, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
			DocumentID:  docID,
			VaultID:     vaultID,
			Content:     fmt.Sprintf("content v%d", i),
			ContentHash: fmt.Sprintf("hash-%d", i),
			Title:       fmt.Sprintf("Version %d", i),
			Source:      models.SourceManual,
		}, i)
		if err != nil {
			t.Fatalf("CreateVersion %d failed: %v", i, err)
		}
	}

	latest, err := testDB.GetLatestVersion(ctx, docID)
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestVersion returned nil")
	}
	if latest.Version != 2 {
		t.Errorf("Expected latest version 2, got %d", latest.Version)
	}
}

func TestCountVersions(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	for i := 1; i <= 3; i++ {
		_, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
			DocumentID:  docID,
			VaultID:     vaultID,
			Content:     fmt.Sprintf("content v%d", i),
			ContentHash: fmt.Sprintf("hash-%d", i),
			Title:       fmt.Sprintf("Version %d", i),
			Source:      models.SourceManual,
		}, i)
		if err != nil {
			t.Fatalf("CreateVersion %d failed: %v", i, err)
		}
	}

	count, err := testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("CountVersions failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestDeleteOldestVersions(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	for i := 1; i <= 5; i++ {
		_, err := testDB.CreateVersion(ctx, models.DocumentVersionInput{
			DocumentID:  docID,
			VaultID:     vaultID,
			Content:     fmt.Sprintf("content v%d", i),
			ContentHash: fmt.Sprintf("hash-%d", i),
			Title:       fmt.Sprintf("Version %d", i),
			Source:      models.SourceManual,
		}, i)
		if err != nil {
			t.Fatalf("CreateVersion %d failed: %v", i, err)
		}
	}

	deleted, err := testDB.DeleteOldestVersions(ctx, docID, 2)
	if err != nil {
		t.Fatalf("DeleteOldestVersions failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("Expected 3 deleted, got %d", deleted)
	}

	remaining, err := testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("CountVersions after delete failed: %v", err)
	}
	if remaining != 2 {
		t.Errorf("Expected 2 remaining versions, got %d", remaining)
	}
}

func TestNextVersionNumber(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)
	doc := createTestDocument(t, ctx, vaultID)
	docID := models.MustRecordIDString(doc.ID)

	// No versions yet — should return 1
	next, err := testDB.NextVersionNumber(ctx, docID)
	if err != nil {
		t.Fatalf("NextVersionNumber failed: %v", err)
	}
	if next != 1 {
		t.Errorf("Expected next version 1 when no versions exist, got %d", next)
	}

	// Create version 1
	_, err = testDB.CreateVersion(ctx, models.DocumentVersionInput{
		DocumentID:  docID,
		VaultID:     vaultID,
		Content:     "content v1",
		ContentHash: "hash-1",
		Title:       "Version 1",
		Source:      models.SourceManual,
	}, 1)
	if err != nil {
		t.Fatalf("CreateVersion failed: %v", err)
	}

	next, err = testDB.NextVersionNumber(ctx, docID)
	if err != nil {
		t.Fatalf("NextVersionNumber after v1 failed: %v", err)
	}
	if next != 2 {
		t.Errorf("Expected next version 2 after creating v1, got %d", next)
	}
}
