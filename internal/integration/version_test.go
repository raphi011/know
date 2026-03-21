package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/vault"
)

func itoa(i int) string { return strconv.Itoa(i) }

func TestVersionLifecycle(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "version-test-user"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		t.Fatalf("user ID: %v", err)
	}

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "version-test-vault"})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		t.Fatalf("vault ID: %v", err)
	}

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{
		CoalesceMinutes: 0,
		RetentionCount:  50,
	}, nil, 0)

	doc, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 1\n\nOriginal content.\n",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		t.Fatalf("doc ID: %v", err)
	}

	versions, err := testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list versions after create: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions after first create, got %d", len(versions))
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 2\n\nUpdated content.\n",
	})
	if err != nil {
		t.Fatalf("update doc v2: %v", err)
	}

	versions, err = testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list versions after v2: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version after first update, got %d", len(versions))
	}
	if versions[0].Version != 1 {
		t.Errorf("version number: got %d, want 1", versions[0].Version)
	}
	if versions[0].Title != "Version 1" {
		t.Errorf("version title: got %q, want %q", versions[0].Title, "Version 1")
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 3\n\nThird revision.\n",
	})
	if err != nil {
		t.Fatalf("update doc v3: %v", err)
	}

	versions, err = testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list versions after v3: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 3\n\nThird revision.\n",
	})
	if err != nil {
		t.Fatalf("no-op update: %v", err)
	}

	count, err := testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 versions after no-op update, got %d", count)
	}

	v1 := versions[len(versions)-1]
	v1ID, err := models.RecordIDString(v1.ID)
	if err != nil {
		t.Fatalf("v1 ID: %v", err)
	}

	restored, err := fileSvc.Rollback(ctx, vaultID, docID, v1ID)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if restored.Hash == nil || *restored.Hash != v1.Hash {
		t.Errorf("rollback content_hash mismatch:\ngot:  %v\nwant: %q", restored.Hash, v1.Hash)
	}

	count, err = testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 versions after rollback, got %d", count)
	}

	if err := fileSvc.Delete(ctx, vaultID, "/docs/versioned.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	afterDelete, err := testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Errorf("versions after cascade: got %d, want 0", len(afterDelete))
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}

func TestVersionCoalescing(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "coalesce-test-user"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		t.Fatalf("user ID: %v", err)
	}

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "coalesce-test-vault"})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		t.Fatalf("vault ID: %v", err)
	}

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{
		CoalesceMinutes: 60,
		RetentionCount:  50,
	}, nil, 0)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/coalesce.md",
		Content: "v1\n",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/docs/coalesce.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		t.Fatalf("doc ID: %v", err)
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/coalesce.md",
		Content: "v2\n",
	})
	if err != nil {
		t.Fatalf("update v2: %v", err)
	}

	count, err := testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("count after v2: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 version after first update, got %d", count)
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/coalesce.md",
		Content: "v3\n",
	})
	if err != nil {
		t.Fatalf("update v3: %v", err)
	}

	count, err = testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("count after v3: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 version after coalesced update, got %d", count)
	}

	if err := fileSvc.Delete(ctx, vaultID, "/docs/coalesce.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}

func TestVersionRetention(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "retention-test-user"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		t.Fatalf("user ID: %v", err)
	}

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "retention-test-vault"})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		t.Fatalf("vault ID: %v", err)
	}

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{
		CoalesceMinutes: 0,
		RetentionCount:  3,
	}, nil, 0)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/docs/retention.md",
		Content: "v0\n",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/docs/retention.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		t.Fatalf("doc ID: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_, err = fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    "/docs/retention.md",
			Content: "v" + itoa(i) + "\n",
		})
		if err != nil {
			t.Fatalf("update v%d: %v", i, err)
		}
	}

	count, err := testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 versions (retention cap), got %d", count)
	}

	versions, err := testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	if versions[0].Version != 5 {
		t.Errorf("newest version: got %d, want 5", versions[0].Version)
	}
	if versions[2].Version != 3 {
		t.Errorf("oldest kept version: got %d, want 3", versions[2].Version)
	}

	if err := fileSvc.Delete(ctx, vaultID, "/docs/retention.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}
