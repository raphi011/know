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

func TestResolveDanglingLinksByStem(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Source file with a dangling link whose raw_target normalizes to stem "beta-notes"
	src, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/resolve-src.md", Title: "Resolve Src",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile src failed: %v", err)
	}
	srcID := models.MustRecordIDString(src.ID)

	// Target file at path "/beta-notes.md" — stem computed from path
	target, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/beta-notes.md", Title: "Beta Notes",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile target failed: %v", err)
	}
	targetID := models.MustRecordIDString(target.ID)

	// Create dangling link with raw_target "Beta Notes" (no ToFileID)
	if err := testDB.CreateWikiLinks(ctx, srcID, vaultID, []WikiLinkInput{
		{RawTarget: "Beta Notes"},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	count, err := testDB.ResolveDanglingLinksByStem(ctx, vaultID, "beta-notes", targetID)
	if err != nil {
		t.Fatalf("ResolveDanglingLinksByStem failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 resolved link, got %d", count)
	}

	links, err := testDB.GetWikiLinksToFile(ctx, targetID)
	if err != nil {
		t.Fatalf("GetWikiLinksToFile failed: %v", err)
	}
	if len(links) != 1 {
		t.Errorf("Expected 1 link pointing to target, got %d", len(links))
	}
}

func TestUnresolveStemOnlyLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	src, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/unresolve-stem-src.md", Title: "Unresolve Stem Src",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile src failed: %v", err)
	}
	srcID := models.MustRecordIDString(src.ID)

	target, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/some-page.md", Title: "Some Page",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile target failed: %v", err)
	}
	targetID := models.MustRecordIDString(target.ID)

	// Create a resolved link (with ToFileID set)
	if err := testDB.CreateWikiLinks(ctx, srcID, vaultID, []WikiLinkInput{
		{RawTarget: "Some Page", ToFileID: &targetID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	count, err := testDB.UnresolveStemOnlyLinks(ctx, vaultID, "some-page")
	if err != nil {
		t.Fatalf("UnresolveStemOnlyLinks failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 unresolved link, got %d", count)
	}

	links, err := testDB.GetWikiLinks(ctx, srcID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link preserved, got %d", len(links))
	}
	if links[0].ToFile != nil {
		t.Error("Expected to_file to be nil after unresolve")
	}
}

func TestGetWikiLinksToFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	docA, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/wlto-a.md", Title: "WLTO A",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile A failed: %v", err)
	}
	docB, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/wlto-b.md", Title: "WLTO B",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile B failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	if err := testDB.CreateWikiLinks(ctx, docAID, vaultID, []WikiLinkInput{
		{RawTarget: "WLTO B", ToFileID: &docBID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	links, err := testDB.GetWikiLinksToFile(ctx, docBID)
	if err != nil {
		t.Fatalf("GetWikiLinksToFile failed: %v", err)
	}
	if len(links) != 1 {
		t.Errorf("Expected 1 link to B, got %d", len(links))
	}
}

func TestBatchUpdateWikiLinkRawTargets(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/batch-raw-target.md", Title: "Batch Raw Target",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "Old Target One"},
		{RawTarget: "Old Target Two"},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("Expected 2 links, got %d", len(links))
	}

	linkIDs := []string{
		models.MustRecordIDString(links[0].ID),
		models.MustRecordIDString(links[1].ID),
	}

	if err := testDB.BatchUpdateWikiLinkRawTargets(ctx, linkIDs, "New Target"); err != nil {
		t.Fatalf("BatchUpdateWikiLinkRawTargets failed: %v", err)
	}

	updated, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks after batch update failed: %v", err)
	}
	for _, l := range updated {
		if l.RawTarget != "New Target" {
			t.Errorf("Expected raw_target 'New Target', got %q", l.RawTarget)
		}
	}
}

func TestGetWikiLinksWithTargetInfo(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	src, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/wlinfo-src.md", Title: "WLInfo Src",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile src failed: %v", err)
	}
	target, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/wlinfo-target.md", Title: "WLInfo Target",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile target failed: %v", err)
	}
	srcID := models.MustRecordIDString(src.ID)
	targetID := models.MustRecordIDString(target.ID)

	if err := testDB.CreateWikiLinks(ctx, srcID, vaultID, []WikiLinkInput{
		{RawTarget: "WLInfo Target", ToFileID: &targetID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	infos, err := testDB.GetWikiLinksWithTargetInfo(ctx, srcID)
	if err != nil {
		t.Fatalf("GetWikiLinksWithTargetInfo failed: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("Expected 1 link with target info, got %d", len(infos))
	}
	if infos[0].RawTarget != "WLInfo Target" {
		t.Errorf("Expected raw_target 'WLInfo Target', got %q", infos[0].RawTarget)
	}
	if infos[0].Path == nil || *infos[0].Path != "/wlinfo-target.md" {
		t.Errorf("Expected path '/wlinfo-target.md', got %v", infos[0].Path)
	}
	if infos[0].Title == nil || *infos[0].Title != "WLInfo Target" {
		t.Errorf("Expected title 'WLInfo Target', got %v", infos[0].Title)
	}
}
