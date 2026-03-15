package db

import (
	"context"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestCreateWikiLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/link-source.md", Title: "Link Source",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "Some Target"},
		{RawTarget: "Another Target"},
	})
	if err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("Expected 2 wiki links, got %d", len(links))
	}
}

func TestGetBacklinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	docA, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/backlink-a.md", Title: "Doc A",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile docA failed: %v", err)
	}
	docB, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/backlink-b.md", Title: "Doc B",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile docB failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	if err := testDB.CreateWikiLinks(ctx, docAID, vaultID, []WikiLinkInput{
		{RawTarget: "Doc B", ToFileID: &docBID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	backlinks, err := testDB.GetBacklinks(ctx, docBID)
	if err != nil {
		t.Fatalf("GetBacklinks failed: %v", err)
	}
	if len(backlinks) != 1 {
		t.Errorf("Expected 1 backlink, got %d", len(backlinks))
	}
}

func TestResolveDanglingLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	docA, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/dangling-source.md", Title: "Source",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile docA failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)

	if err := testDB.CreateWikiLinks(ctx, docAID, vaultID, []WikiLinkInput{
		{RawTarget: "Future Doc"},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0].ToFile != nil {
		t.Fatal("Link should be dangling (to_file nil)")
	}

	docB, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/future-doc.md", Title: "Future Doc",
		Content: "arrived", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile docB failed: %v", err)
	}
	docBID := models.MustRecordIDString(docB.ID)

	resolved, err := testDB.ResolveDanglingLinks(ctx, vaultID, "Future Doc", docBID)
	if err != nil {
		t.Fatalf("ResolveDanglingLinks failed: %v", err)
	}
	if resolved != 1 {
		t.Errorf("Expected 1 resolved link, got %d", resolved)
	}

	links, err = testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("GetWikiLinks after resolve failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Error("Link should now be resolved")
	}
}

func TestUnresolveWikiLinksToFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	docA, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/unresolve-a.md", Title: "Unresolve A",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile A failed: %v", err)
	}
	docB, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/unresolve-b.md", Title: "Unresolve B",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile B failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	// Create a resolved link from A -> B
	if err := testDB.CreateWikiLinks(ctx, docAID, vaultID, []WikiLinkInput{
		{RawTarget: "Unresolve B", ToFileID: &docBID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	// Verify link is resolved
	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 1 || links[0].ToFile == nil {
		t.Fatal("Link should be resolved initially")
	}

	// Unresolve links pointing to B
	count, err := testDB.UnresolveWikiLinksToFile(ctx, docBID)
	if err != nil {
		t.Fatalf("UnresolveWikiLinksToFile failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 unresolved link, got %d", count)
	}

	// Verify link is now dangling but still exists
	links, err = testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("GetWikiLinks after unresolve failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link (preserved), got %d", len(links))
	}
	if links[0].ToFile != nil {
		t.Error("Link should be dangling after unresolve")
	}
	if links[0].RawTarget != "Unresolve B" {
		t.Errorf("RawTarget should be preserved, got %q", links[0].RawTarget)
	}
}

func TestUpdateWikiLinkRawTargets(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/raw-target-src.md", Title: "Raw Target Source",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "/old/path.md"},
		{RawTarget: "Unrelated Target"},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	count, err := testDB.UpdateWikiLinkRawTargets(ctx, vaultID, "/old/path.md", "/new/path.md")
	if err != nil {
		t.Fatalf("UpdateWikiLinkRawTargets failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 updated, got %d", count)
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	found := false
	for _, l := range links {
		if l.RawTarget == "/new/path.md" {
			found = true
		}
		if l.RawTarget == "/old/path.md" {
			t.Error("Old raw_target should not exist after update")
		}
	}
	if !found {
		t.Error("Expected to find link with new raw_target /new/path.md")
	}
}

func TestUpdateWikiLinkRawTargetsByPrefix(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/prefix-src.md", Title: "Prefix Source",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "/old-folder/a.md"},
		{RawTarget: "/old-folder/sub/b.md"},
		{RawTarget: "/other/c.md"},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	count, err := testDB.UpdateWikiLinkRawTargetsByPrefix(ctx, vaultID, "/old-folder/", "/new-folder/")
	if err != nil {
		t.Fatalf("UpdateWikiLinkRawTargetsByPrefix failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 updated, got %d", count)
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	targets := make(map[string]bool)
	for _, l := range links {
		targets[l.RawTarget] = true
	}
	if !targets["/new-folder/a.md"] {
		t.Error("Expected /new-folder/a.md")
	}
	if !targets["/new-folder/sub/b.md"] {
		t.Error("Expected /new-folder/sub/b.md")
	}
	if !targets["/other/c.md"] {
		t.Error("Expected /other/c.md to be unchanged")
	}
}

func TestDeleteWikiLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/delete-links-test.md", Title: "Delete Links",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "Target One"},
		{RawTarget: "Target Two"},
	})
	if err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("Expected 2 wiki links, got %d", len(links))
	}

	err = testDB.DeleteWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteWikiLinks failed: %v", err)
	}

	links, err = testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks after delete failed: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("Expected 0 wiki links after delete, got %d", len(links))
	}
}
