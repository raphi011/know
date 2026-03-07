package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/vault"
)

func itoa(i int) string { return strconv.Itoa(i) }

// TestVersionLifecycle exercises: version creation on update, coalescing,
// retention cap, rollback, and cascade delete.
func TestVersionLifecycle(t *testing.T) {
	ctx := context.Background()

	// --- Bootstrap ---

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

	// Use 0 coalesce minutes so every content change creates a version
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{
		CoalesceMinutes: 0,
		RetentionCount:  50,
	})

	// --- Test: first create does NOT create a version ---

	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 1\n\nOriginal content.\n",
		Source:  models.SourceManual,
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

	// --- Test: update creates version of old content ---

	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 2\n\nUpdated content.\n",
		Source:  models.SourceManual,
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

	// --- Test: second update creates second version ---

	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 3\n\nThird revision.\n",
		Source:  models.SourceManual,
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

	// --- Test: update with same content does NOT create version ---

	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/versioned.md",
		Content: "# Version 3\n\nThird revision.\n",
		Source:  models.SourceManual,
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

	// --- Test: rollback restores old content and creates version of pre-rollback state ---

	// Get the first version to rollback to
	v1 := versions[len(versions)-1] // oldest (version 1)
	v1ID, err := models.RecordIDString(v1.ID)
	if err != nil {
		t.Fatalf("v1 ID: %v", err)
	}

	restored, err := docSvc.Rollback(ctx, vaultID, docID, v1ID)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if restored.Content != v1.Content {
		t.Errorf("rollback content mismatch:\ngot:  %q\nwant: %q", restored.Content, v1.Content)
	}
	if restored.Source != models.SourceRollback {
		t.Errorf("rollback source: got %q, want %q", restored.Source, models.SourceRollback)
	}

	// Should have 3 versions now (v1=original, v2=first update, v3=pre-rollback)
	count, err = testDB.CountVersions(ctx, docID)
	if err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 versions after rollback, got %d", count)
	}

	// --- Test: cascade delete ---

	if err := docSvc.Delete(ctx, vaultID, "/docs/versioned.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}

	// Versions should be gone via cascade event
	// Give SurrealDB a moment for async cascade
	time.Sleep(100 * time.Millisecond)

	afterDelete, err := testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Errorf("versions after cascade: got %d, want 0", len(afterDelete))
	}

	// --- Cleanup ---

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}

// TestVersionCoalescing verifies that rapid updates within the coalesce window
// do not create versions.
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

	// Use a very large coalesce window (60 min) to prevent versions
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{
		CoalesceMinutes: 60,
		RetentionCount:  50,
	})

	// Create initial document
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/coalesce.md",
		Content: "v1\n",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/docs/coalesce.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		t.Fatalf("doc ID: %v", err)
	}

	// First update creates version 1 (no prior version exists)
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/coalesce.md",
		Content: "v2\n",
		Source:  models.SourceManual,
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

	// Second update should be coalesced (version was just created)
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/coalesce.md",
		Content: "v3\n",
		Source:  models.SourceManual,
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

	// --- Cleanup ---

	if err := docSvc.Delete(ctx, vaultID, "/docs/coalesce.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}

// TestVersionRetention verifies that the retention cap prunes old versions.
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

	// Retention cap of 3
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{
		CoalesceMinutes: 0,
		RetentionCount:  3,
	})

	// Create document
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/retention.md",
		Content: "v0\n",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/docs/retention.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		t.Fatalf("doc ID: %v", err)
	}

	// Create 5 updates (should produce 5 versions, but retention keeps only 3)
	for i := 1; i <= 5; i++ {
		_, err = docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    "/docs/retention.md",
			Content: "v" + itoa(i) + "\n",
			Source:  models.SourceManual,
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

	// Verify newest versions are kept (highest version numbers)
	versions, err := testDB.ListVersions(ctx, docID, 100, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// versions are newest first, so version 5, 4, 3
	if versions[0].Version != 5 {
		t.Errorf("newest version: got %d, want 5", versions[0].Version)
	}
	if versions[2].Version != 3 {
		t.Errorf("oldest kept version: got %d, want 3", versions[2].Version)
	}

	// --- Cleanup ---

	if err := docSvc.Delete(ctx, vaultID, "/docs/retention.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}
