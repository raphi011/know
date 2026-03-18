package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/vault"
)

func setupVault(t *testing.T, ctx context.Context, suffix string) (string, string, *vault.Service) {
	t.Helper()
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "folder-test-user-" + suffix})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultName := "folder-test-" + suffix
	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: vaultName})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)
	return vaultID, vaultName, vaultSvc
}

func TestFolderAutoCreate(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "autocreate-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/guides/sub/file.md",
		Content: "# Guide",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

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

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

func TestFolderRootDocNoFolders(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "rootdoc-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/readme.md",
		Content: "# Readme",
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

func TestFolderEnsureIdempotency(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "idempotent-"+fmt.Sprint(time.Now().UnixNano()))

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

	if len(folders) != 2 {
		t.Errorf("expected 2 folders, got %d: %v", len(folders), folders)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

func TestFolderEmptyPersistence(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "empty-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/notes/file.md",
		Content: "# Note",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	if err := fileSvc.Delete(ctx, vaultID, "/notes/file.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}

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

func TestFolderExplicitCreate(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "explicit-"+fmt.Sprint(time.Now().UnixNano()))

	folder, err := vaultSvc.CreateFolder(ctx, vaultID, "/projects/new-idea")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if folder.Path != "/projects/new-idea" {
		t.Errorf("folder path: got %q, want %q", folder.Path, "/projects/new-idea")
	}
	if folder.Title != "new-idea" {
		t.Errorf("folder title: got %q, want %q", folder.Title, "new-idea")
	}

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

func TestFolderDeleteCascade(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "cascade-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    p,
			Content: "# Doc at " + p,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", p, err)
		}
	}

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/other/c.md",
		Content: "# Other",
	})
	if err != nil {
		t.Fatalf("create other doc: %v", err)
	}

	if err := vaultSvc.DeleteFolder(ctx, vaultID, "/guides"); err != nil {
		t.Fatalf("delete folder: %v", err)
	}

	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted after folder cascade", p)
		}
	}

	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	for _, f := range folders {
		if f.Path == "/guides" || f.Path == "/guides/sub" {
			t.Errorf("folder %s should be deleted", f.Path)
		}
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/other/c.md")
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

func TestFolderMoveBasic(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "move-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    p,
			Content: "# Doc at " + p,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", p, err)
		}
	}

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/other/c.md",
		Content: "# Other",
	})
	if err != nil {
		t.Fatalf("create other doc: %v", err)
	}

	if err := vaultSvc.MoveFolder(ctx, vaultID, "/guides", "/docs"); err != nil {
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
	if folderPaths["/guides"] || folderPaths["/guides/sub"] {
		t.Error("old folder paths should not exist after move")
	}
	if !folderPaths["/docs"] {
		t.Error("expected /docs folder after move")
	}
	if !folderPaths["/docs/sub"] {
		t.Error("expected /docs/sub folder after move")
	}

	for _, p := range []string{"/docs/a.md", "/docs/sub/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc == nil {
			t.Errorf("expected file at %s after move", p)
		}
	}

	for _, p := range []string{"/guides/a.md", "/guides/sub/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, p)
		if err != nil {
			t.Fatalf("get doc %s: %v", p, err)
		}
		if doc != nil {
			t.Errorf("file at old path %s should not exist after move", p)
		}
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/other/c.md")
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

func TestFolderMoveCreatesAncestors(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "move-ancestors-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/guides/a.md",
		Content: "# Guide",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

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

func TestFolderListByParent(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "listparent-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/a/b/c/file.md",
		Content: "# Deep",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/a/sibling/file.md",
		Content: "# Sibling",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

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

func TestFolderListAll(t *testing.T) {
	ctx := context.Background()
	vaultID, _, vaultSvc := setupVault(t, ctx, "listall-"+fmt.Sprint(time.Now().UnixNano()))

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	paths := []string{
		"/guides/getting-started.md",
		"/guides/advanced/topic.md",
		"/memories/note.md",
		"/projects/alpha.md",
	}
	for _, p := range paths {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    p,
			Content: "# Doc",
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
