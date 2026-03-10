package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/vault"
)

// setupVault is a test helper that creates a user and vault, returning the vault ID.
func setupVault(t *testing.T, ctx context.Context, suffix string) (string, *vault.Service) {
	t.Helper()
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "folder-test-user-" + suffix})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "folder-test-" + suffix})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)
	return vaultID, vaultSvc
}

// TestFolderAutoCreate verifies that creating a document with a nested path
// automatically creates all parent folders.
func TestFolderAutoCreate(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "autocreate-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create a document at /guides/sub/file.md
	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/guides/sub/file.md",
		Content: "# Guide",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Verify folders /guides and /guides/sub were created
	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}

	folderPaths := make(map[string]bool)
	for _, f := range folders {
		folderPaths[f.Path] = true
	}

	if !folderPaths["/guides"] {
		t.Error("expected /guides folder to be auto-created")
	}
	if !folderPaths["/guides/sub"] {
		t.Error("expected /guides/sub folder to be auto-created")
	}

	// Cleanup
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderRootDocNoFolders verifies that a root-level document creates no folders.
func TestFolderRootDocNoFolders(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "rootdoc-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/readme.md",
		Content: "# Readme",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) != 0 {
		t.Errorf("expected no folders for root-level doc, got %d: %v", len(folders), folders)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderEnsureIdempotency verifies that EnsureFolders is idempotent.
func TestFolderEnsureIdempotency(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "idempotent-"+fmt.Sprint(time.Now().UnixNano()))

	// Ensure folders twice for the same path
	if err := testDB.EnsureFolders(ctx, vaultID, "/guides/sub/file.md"); err != nil {
		t.Fatalf("first EnsureFolders: %v", err)
	}
	if err := testDB.EnsureFolders(ctx, vaultID, "/guides/sub/other.md"); err != nil {
		t.Fatalf("second EnsureFolders: %v", err)
	}

	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}

	// Should have exactly 2 folders: /guides and /guides/sub (no duplicates)
	if len(folders) != 2 {
		t.Errorf("expected 2 folders, got %d: %v", len(folders), folders)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderEmptyPersistence verifies that empty folders persist after doc deletion.
func TestFolderEmptyPersistence(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "empty-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create a document to auto-create folders
	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/notes/file.md",
		Content: "# Note",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Delete the document (single doc delete does NOT clean up folders)
	if err := docSvc.Delete(ctx, vaultID, "/notes/file.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}

	// Folder should still exist
	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) != 1 {
		t.Errorf("expected 1 folder (empty /notes), got %d", len(folders))
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderExplicitCreate verifies that folders can be created explicitly (empty).
func TestFolderExplicitCreate(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "explicit-"+fmt.Sprint(time.Now().UnixNano()))

	folder, err := vaultSvc.CreateFolder(ctx, vaultID, "/projects/new-idea")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if folder.Path != "/projects/new-idea" {
		t.Errorf("folder path: got %q, want %q", folder.Path, "/projects/new-idea")
	}
	if folder.Name != "new-idea" {
		t.Errorf("folder name: got %q, want %q", folder.Name, "new-idea")
	}

	// Ancestor /projects should also exist
	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	folderPaths := make(map[string]bool)
	for _, f := range folders {
		folderPaths[f.Path] = true
	}
	if !folderPaths["/projects"] {
		t.Error("expected /projects ancestor folder to be auto-created")
	}
	if !folderPaths["/projects/new-idea"] {
		t.Error("expected /projects/new-idea folder to exist")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderDeleteCascade verifies that deleting a folder cascades to child
// folders and documents.
func TestFolderDeleteCascade(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "cascade-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create docs under /guides/
	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    p,
			Content: "# Doc at " + p,
			Source:  models.SourceManual,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", p, err)
		}
	}

	// Also create a doc outside /guides/
	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/other/c.md",
		Content: "# Other",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create other doc: %v", err)
	}

	// Delete /guides folder — should cascade
	if err := vaultSvc.DeleteFolder(ctx, vaultID, "/guides"); err != nil {
		t.Fatalf("delete folder: %v", err)
	}

	// Verify /guides docs are gone
	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted after folder cascade", p)
		}
	}

	// Verify /guides folders are gone
	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	for _, f := range folders {
		if f.Path == "/guides" || f.Path == "/guides/sub" {
			t.Errorf("folder %s should be deleted", f.Path)
		}
	}

	// Verify /other/c.md still exists
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/other/c.md")
	if err != nil {
		t.Fatalf("get other doc: %v", err)
	}
	if doc == nil {
		t.Error("doc /other/c.md should still exist")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderMoveBasic verifies that moving a folder updates folder records,
// child folder records, and documents.
func TestFolderMoveBasic(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "move-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create docs under /guides/ and /guides/sub/
	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    p,
			Content: "# Doc at " + p,
			Source:  models.SourceManual,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", p, err)
		}
	}

	// Also create a doc outside /guides/ that should not be affected
	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/other/c.md",
		Content: "# Other",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create other doc: %v", err)
	}

	// Move /guides to /docs
	if err := vaultSvc.MoveFolder(ctx, vaultID, "/guides", "/docs"); err != nil {
		t.Fatalf("move folder: %v", err)
	}

	// Verify folder records were moved
	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	folderPaths := make(map[string]bool)
	for _, f := range folders {
		folderPaths[f.Path] = true
	}
	if folderPaths["/guides"] || folderPaths["/guides/sub"] {
		t.Error("old folder paths should not exist after move")
	}
	if !folderPaths["/docs"] {
		t.Error("expected /docs folder after move")
	}
	if !folderPaths["/docs/sub"] {
		t.Error("expected /docs/sub folder after move")
	}

	// Verify documents were moved
	for _, p := range []string{"/docs/a.md", "/docs/sub/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc == nil {
			t.Errorf("expected document at %s after move", p)
		}
	}

	// Verify old document paths are gone
	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc != nil {
			t.Errorf("document at old path %s should not exist after move", p)
		}
	}

	// Verify /other/c.md is unaffected
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/other/c.md")
	if err != nil {
		t.Fatalf("get other doc: %v", err)
	}
	if doc == nil {
		t.Error("doc /other/c.md should still exist")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderMoveCreatesAncestors verifies that moving a folder to a nested
// destination creates ancestor folders at the destination.
func TestFolderMoveCreatesAncestors(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "move-ancestors-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/guides/a.md",
		Content: "# Guide",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Move /guides to /archive/old/guides — should create /archive and /archive/old
	if err := vaultSvc.MoveFolder(ctx, vaultID, "/guides", "/archive/old/guides"); err != nil {
		t.Fatalf("move folder: %v", err)
	}

	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	folderPaths := make(map[string]bool)
	for _, f := range folders {
		folderPaths[f.Path] = true
	}

	for _, expected := range []string{"/archive", "/archive/old", "/archive/old/guides"} {
		if !folderPaths[expected] {
			t.Errorf("expected folder %s to exist after move", expected)
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderListByParent verifies that ListFolders with parentPath filters
// to immediate children only.
func TestFolderListByParent(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "listparent-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create a nested structure: /a/b/c/file.md
	_, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/a/b/c/file.md",
		Content: "# Deep",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Also create /a/sibling/file.md
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/a/sibling/file.md",
		Content: "# Sibling",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// List children of /a — should return /a/b and /a/sibling (not /a/b/c)
	parentPath := "/a"
	children, err := vaultSvc.ListFolders(ctx, vaultID, &parentPath)
	if err != nil {
		t.Fatalf("list folders by parent: %v", err)
	}

	childPaths := make(map[string]bool)
	for _, f := range children {
		childPaths[f.Path] = true
	}

	if !childPaths["/a/b"] {
		t.Error("expected /a/b as child of /a")
	}
	if !childPaths["/a/sibling"] {
		t.Error("expected /a/sibling as child of /a")
	}
	if childPaths["/a/b/c"] {
		t.Error("/a/b/c should not be an immediate child of /a")
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children of /a, got %d", len(children))
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestFolderListAll verifies that ListFolders returns all vault folders.
func TestFolderListAll(t *testing.T) {
	ctx := context.Background()
	vaultID, vaultSvc := setupVault(t, ctx, "listall-"+fmt.Sprint(time.Now().UnixNano()))

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	paths := []string{
		"/guides/getting-started.md",
		"/guides/advanced/topic.md",
		"/memories/note.md",
		"/projects/alpha.md",
	}
	for _, p := range paths {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    p,
			Content: "# Doc",
			Source:  models.SourceManual,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", p, err)
		}
	}

	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}

	expected := map[string]bool{
		"/guides":          true,
		"/guides/advanced": true,
		"/memories":        true,
		"/projects":        true,
	}

	got := make(map[string]bool)
	for _, f := range folders {
		got[f.Path] = true
	}

	for path := range expected {
		if !got[path] {
			t.Errorf("missing expected folder: %s", path)
		}
	}
	if len(folders) != len(expected) {
		t.Errorf("folder count: got %d, want %d", len(folders), len(expected))
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}
