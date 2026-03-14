package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/search"
	"github.com/raphi011/know/internal/vault"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	testDB        *db.Client
	testContainer testcontainers.Container
)

func TestMain(m *testing.M) {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	ctx := context.Background()

	var err error
	testContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "surrealdb/surrealdb:v3.0.2",
			ExposedPorts: []string{"8000/tcp"},
			Cmd:          []string{"start", "--log", "info", "--user", "root", "--pass", "root"},
			WaitingFor:   wait.ForLog("Started web server").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		log.Fatalf("start SurrealDB container: %v", err)
	}

	host, err := testContainer.Host(ctx)
	if err != nil {
		log.Fatalf("get container host: %v", err)
	}
	if host == "" || host == "null" || host == "192.168.64.2" {
		host = "localhost"
	}
	mappedPort, err := testContainer.MappedPort(ctx, "8000")
	if err != nil {
		log.Fatalf("get mapped port: %v", err)
	}

	testDB, err = db.NewClient(ctx, db.Config{
		URL:       fmt.Sprintf("ws://%s:%s/rpc", host, mappedPort.Port()),
		Namespace: "test_integration",
		Database:  "test_integration",
		Username:  "root",
		Password:  "root",
		AuthLevel: "root",
	}, nil, nil)
	if err != nil {
		log.Fatalf("create DB client: %v", err)
	}

	if err := testDB.InitSchema(ctx, 4); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	code := m.Run()

	if err := testDB.Close(ctx); err != nil {
		log.Printf("close DB: %v", err)
	}
	if err := testContainer.Terminate(ctx); err != nil {
		log.Printf("terminate container: %v", err)
	}

	os.Exit(code)
}

// TestFullLifecycle exercises the complete v2 pipeline:
// user → vault → token → documents → wiki-links → search → delete cascade
func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	// --- Bootstrap: user, vault, token ---

	user, err := testDB.CreateUser(ctx, models.UserInput{
		Name: "integration-test-user",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		t.Fatalf("user ID: %v", err)
	}

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "test-vault"})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		t.Fatalf("vault ID: %v", err)
	}

	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	_ = rawToken // would be used by CLI

	_, err = testDB.CreateToken(ctx, userID, tokenHash, "test-token")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// Verify token lookup
	tok, err := testDB.GetTokenByHash(ctx, auth.HashToken(rawToken))
	if err != nil {
		t.Fatalf("get token by hash: %v", err)
	}
	if tok == nil {
		t.Fatal("expected token, got nil")
	}

	// --- Documents with wiki-links ---

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0) // no embedder

	doc1, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/projects/alpha.md",
		Content: `---
title: Project Alpha
labels: [go, backend]
type: project
---

# Project Alpha

See also [[Beta Notes]] and [[missing-page]].

#architecture #go
`,
	})
	if err != nil {
		t.Fatalf("create doc1: %v", err)
	}
	doc1ID, err := models.RecordIDString(doc1.ID)
	if err != nil {
		t.Fatalf("doc1 ID: %v", err)
	}

	// Process pending documents (deferred pipeline: chunks, wiki-links, etc.)
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Verify doc1 was parsed correctly
	if doc1.Title != "Project Alpha" {
		t.Errorf("doc1 title: got %q, want %q", doc1.Title, "Project Alpha")
	}
	// Labels: frontmatter [go, backend] + inline #architecture #go → [go, backend, architecture]
	if len(doc1.Labels) < 3 {
		t.Errorf("doc1 labels: got %v, want at least 3", doc1.Labels)
	}

	// Check wiki-links: "Beta Notes" should be dangling, "missing-page" should be dangling
	links, err := testDB.GetWikiLinks(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("wiki-links: got %d, want 2", len(links))
	}
	for _, l := range links {
		if l.ToDoc != nil {
			t.Errorf("wiki-link %q should be dangling", l.RawTarget)
		}
	}

	// --- Create second doc that resolves "Beta Notes" ---

	doc2, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/notes/beta-notes.md",
		Content: `---
title: Beta Notes
labels: [notes]
---

# Beta Notes

Some notes about the beta project.
`,
	})
	if err != nil {
		t.Fatalf("create doc2: %v", err)
	}
	doc2ID, err := models.RecordIDString(doc2.ID)
	if err != nil {
		t.Fatalf("doc2 ID: %v", err)
	}

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending after doc2: %v", err)
	}

	// Verify dangling link to "Beta Notes" is now resolved
	links, err = testDB.GetWikiLinks(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get wiki-links after doc2: %v", err)
	}

	var resolvedCount int
	for _, l := range links {
		if l.ToDoc != nil {
			resolvedCount++
			toID, err := models.RecordIDString(*l.ToDoc)
			if err != nil {
				t.Fatalf("extract toDoc ID: %v", err)
			}
			if toID != doc2ID {
				t.Errorf("resolved link points to %q, want %q", toID, doc2ID)
			}
		}
	}
	if resolvedCount != 1 {
		t.Errorf("resolved links: got %d, want 1", resolvedCount)
	}

	// --- BM25 Chunk Search ---

	// Verify chunks exist for doc1 before searching
	doc1Chunks, err := testDB.GetChunks(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get doc1 chunks: %v", err)
	}
	if len(doc1Chunks) == 0 {
		t.Fatal("expected chunks for doc1, got none")
	}
	t.Logf("doc1 has %d chunks, first chunk content: %q", len(doc1Chunks), doc1Chunks[0].Content[:min(80, len(doc1Chunks[0].Content))])

	// Also verify direct BM25 chunk search at DB level
	directResults, err := testDB.BM25ChunkSearch(ctx, "alpha", db.SearchFilter{
		VaultID: vaultID,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("direct BM25ChunkSearch: %v", err)
	}
	t.Logf("direct BM25ChunkSearch returned %d results", len(directResults))

	searchSvc := search.NewService(testDB, nil) // no embedder → BM25 only

	results, err := searchSvc.Search(ctx, search.SearchInput{
		VaultID: vaultID,
		Query:   "alpha",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("search returned no results for 'alpha'")
	}
	// BM25 now searches chunks, so MatchedChunks should be populated
	if len(results[0].MatchedChunks) == 0 {
		t.Error("expected MatchedChunks to be populated from BM25 chunk search")
	}

	// Search with label filter
	results, err = searchSvc.Search(ctx, search.SearchInput{
		VaultID: vaultID,
		Query:   "project",
		Labels:  []string{"notes"},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("search with label: %v", err)
	}
	// Only doc2 has "notes" label and content about "project"
	for _, r := range results {
		if r.DocumentID == doc1ID {
			t.Error("search with label=notes should not return doc1")
		}
	}

	// --- Folders ---

	folders, err := vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) < 2 {
		t.Errorf("folders: got %d, want at least 2 (/projects, /notes)", len(folders))
	}

	// --- Delete doc1 → cascade should remove wiki-links, chunks ---

	if err := docSvc.Delete(ctx, vaultID, "/projects/alpha.md"); err != nil {
		t.Fatalf("delete doc1: %v", err)
	}

	// Wiki-links from doc1 should be gone
	links, err = testDB.GetWikiLinks(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get wiki-links after delete: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("wiki-links after delete: got %d, want 0", len(links))
	}

	// doc1 should be gone
	gone, err := testDB.GetDocumentByID(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get deleted doc1: %v", err)
	}
	if gone != nil {
		t.Error("doc1 should be nil after delete")
	}

	// doc2 should still exist
	still, err := testDB.GetDocumentByID(ctx, doc2ID)
	if err != nil {
		t.Fatalf("get doc2 after doc1 delete: %v", err)
	}
	if still == nil {
		t.Error("doc2 should still exist")
	}

	// --- Upsert (update via create) ---

	doc2Updated, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/notes/beta-notes.md",
		Content: `---
title: Beta Notes (Updated)
labels: [notes, updated]
---

# Beta Notes (Updated)

Updated content for beta.
`,
	})
	if err != nil {
		t.Fatalf("upsert doc2: %v", err)
	}
	if doc2Updated.Title != "Beta Notes (Updated)" {
		t.Errorf("upserted title: got %q, want %q", doc2Updated.Title, "Beta Notes (Updated)")
	}

	// --- Cleanup vault ---

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}

// TestDeleteByPrefix verifies that DeleteByPrefix removes all documents under
// a path prefix while leaving documents outside the prefix untouched.
func TestDeleteByPrefix(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "delete-prefix-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "delete-prefix-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create 3 documents: 2 under /test-prefix/, 1 under /other/
	for _, path := range []string{"/test-prefix/a.md", "/test-prefix/b.md", "/other/c.md"} {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	// Delete by prefix
	count, err := docSvc.DeleteByPrefix(ctx, vaultID, "/test-prefix")
	if err != nil {
		t.Fatalf("delete by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("deleted count: got %d, want 2", count)
	}

	// Verify prefix docs are gone
	for _, path := range []string{"/test-prefix/a.md", "/test-prefix/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted", path)
		}
	}

	// Verify /other/c.md still exists
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/other/c.md")
	if err != nil {
		t.Fatalf("get doc /other/c.md: %v", err)
	}
	if doc == nil {
		t.Error("doc /other/c.md should still exist")
	}

	// Cleanup
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveByPrefix verifies that MoveByPrefix renames all documents under a
// path prefix while leaving documents outside the prefix untouched.
func TestMoveByPrefix(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "move-prefix-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "move-prefix-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create 2 documents under /old-folder/
	for _, path := range []string{"/old-folder/a.md", "/old-folder/b.md"} {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	// Move /old-folder → /new-folder
	count, err := docSvc.MoveByPrefix(ctx, vaultID, "/old-folder", "/new-folder")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("moved count: got %d, want 2", count)
	}

	// Verify old paths are gone
	for _, path := range []string{"/old-folder/a.md", "/old-folder/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc != nil {
			t.Errorf("doc %s should no longer exist at old path", path)
		}
	}

	// Verify new paths exist
	for _, path := range []string{"/new-folder/a.md", "/new-folder/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc == nil {
			t.Errorf("doc %s should exist at new path", path)
		}
	}

	// Cleanup
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestDeleteByPrefix_BoundaryCollision verifies that deleting "/test-prefix" does not
// affect documents under "/test-prefix-extra" or "/test-prefixed" — only "/test-prefix/".
func TestDeleteByPrefix_BoundaryCollision(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "delete-boundary-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "delete-boundary-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create docs under similar-looking prefixes
	paths := []string{
		"/test-prefix/a.md",
		"/test-prefix/sub/b.md",
		"/test-prefix-extra/c.md",
		"/test-prefixed/d.md",
	}
	for _, path := range paths {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	// Delete by prefix — should only affect /test-prefix/
	count, err := docSvc.DeleteByPrefix(ctx, vaultID, "/test-prefix")
	if err != nil {
		t.Fatalf("delete by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("deleted count: got %d, want 2", count)
	}

	// Verify /test-prefix/ docs are gone
	for _, path := range []string{"/test-prefix/a.md", "/test-prefix/sub/b.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted", path)
		}
	}

	// Verify similar-prefix docs are NOT deleted
	for _, path := range []string{"/test-prefix-extra/c.md", "/test-prefixed/d.md"} {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc == nil {
			t.Errorf("doc %s should still exist (different prefix)", path)
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveByPrefix_NestedSubfolders verifies that moving a prefix correctly
// preserves the full relative path structure for deeply nested documents.
func TestMoveByPrefix_NestedSubfolders(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "move-nested-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "move-nested-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create deeply nested documents
	paths := []string{
		"/old-folder/a.md",
		"/old-folder/sub/b.md",
		"/old-folder/sub/deep/c.md",
	}
	for _, path := range paths {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := docSvc.MoveByPrefix(ctx, vaultID, "/old-folder", "/new-folder")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 3 {
		t.Errorf("moved count: got %d, want 3", count)
	}

	// Verify new paths preserve relative structure
	expectedNewPaths := []string{
		"/new-folder/a.md",
		"/new-folder/sub/b.md",
		"/new-folder/sub/deep/c.md",
	}
	for _, path := range expectedNewPaths {
		doc, err := testDB.GetDocumentByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc == nil {
			t.Errorf("doc %s should exist at new path", path)
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveByPrefix_BoundaryCollision verifies that moving "/guides" does not
// affect documents under "/guides-extra".
func TestMoveByPrefix_BoundaryCollision(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "move-boundary-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "move-boundary-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	paths := []string{
		"/guides/a.md",
		"/guides-extra/b.md",
	}
	for _, path := range paths {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := docSvc.MoveByPrefix(ctx, vaultID, "/guides", "/tutorials")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 1 {
		t.Errorf("moved count: got %d, want 1", count)
	}

	// /guides-extra/b.md should be untouched
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/guides-extra/b.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("doc /guides-extra/b.md should still exist")
	}

	// /tutorials/a.md should exist
	doc, err = testDB.GetDocumentByPath(ctx, vaultID, "/tutorials/a.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("doc /tutorials/a.md should exist after move")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveByPrefix_SamePrefix verifies that moving to the same prefix is a no-op.
func TestMoveByPrefix_SamePrefix(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "move-same-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "move-same-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/folder/a.md",
		Content: "# Doc",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Move to same prefix — should return 0 (no-op)
	count, err := docSvc.MoveByPrefix(ctx, vaultID, "/folder", "/folder")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 0 {
		t.Errorf("moved count: got %d, want 0 (same prefix)", count)
	}

	// Document should still be at original path
	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/folder/a.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("doc should still exist at original path")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestDeleteUnresolvesIncomingWikiLinks verifies that deleting a document makes
// incoming wiki_links dangling (to_doc = NONE) rather than deleting them.
func TestDeleteUnresolvesIncomingWikiLinks(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "unresolve-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "unresolve-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create doc B (target)
	docB, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/target.md",
		Content: "# Target\n\nTarget content.",
	})
	if err != nil {
		t.Fatalf("create target doc: %v", err)
	}
	docBID := models.MustRecordIDString(docB.ID)
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Create doc A (source) that links to Target
	docA, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/source.md",
		Content: "# Source\n\nSee [[Target]].",
	})
	if err != nil {
		t.Fatalf("create source doc: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Verify link from A to B is resolved
	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToDoc == nil {
		t.Fatal("wiki-link should be resolved before delete")
	}

	// Delete doc B
	if err := docSvc.Delete(ctx, vaultID, "/target.md"); err != nil {
		t.Fatalf("delete target doc: %v", err)
	}

	// Verify doc B is gone
	gone, err := testDB.GetDocumentByID(ctx, docBID)
	if err != nil {
		t.Fatalf("get deleted doc: %v", err)
	}
	if gone != nil {
		t.Error("target doc should be deleted")
	}

	// Verify wiki-link from A still exists but is now dangling
	links, err = testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("get wiki-links after delete: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link preserved, got %d", len(links))
	}
	if links[0].ToDoc != nil {
		t.Error("wiki-link should be dangling after target deleted")
	}
	if links[0].RawTarget != "Target" {
		t.Errorf("raw_target should be preserved, got %q", links[0].RawTarget)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveUpdatesWikiLinkRawTargets verifies that moving a document updates
// raw_target in wiki_links that reference the old path.
func TestMoveUpdatesWikiLinkRawTargets(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "move-rawt-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "move-rawt-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create target doc
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/old/target.md",
		Content: "# Target",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Create source doc linking by path
	docA, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/source.md",
		Content: "# Source\n\nSee [[/old/target.md]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Move target doc
	_, err = docSvc.Move(ctx, vaultID, "/old/target.md", "/new/target.md")
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	// Verify wiki-link raw_target was updated
	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("get wiki-links after move: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].RawTarget != "/new/target.md" {
		t.Errorf("expected raw_target '/new/target.md', got %q", links[0].RawTarget)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveByPrefixUpdatesWikiLinkRawTargets verifies that MoveByPrefix
// updates raw_targets for all wiki_links referencing paths under the old prefix.
func TestMoveByPrefixUpdatesWikiLinkRawTargets(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "mbp-rawt-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "mbp-rawt-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create docs under /old-dir/
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/old-dir/a.md",
		Content: "# A",
	})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Create source that references paths under /old-dir/
	src, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/source.md",
		Content: "# Source\n\nSee [[/old-dir/a.md]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	srcID := models.MustRecordIDString(src.ID)
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Move by prefix
	count, err := docSvc.MoveByPrefix(ctx, vaultID, "/old-dir", "/new-dir")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 moved, got %d", count)
	}

	// Verify raw_target updated
	links, err := testDB.GetWikiLinks(ctx, srcID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].RawTarget != "/new-dir/a.md" {
		t.Errorf("expected raw_target '/new-dir/a.md', got %q", links[0].RawTarget)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestProcessRelatesToDeleteThenRecreate verifies that updating a document's
// relates_to frontmatter removes stale relations and creates new ones.
func TestProcessRelatesToDeleteThenRecreate(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "relto-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "relto-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create target docs
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/target-a.md",
		Content: "# Target A",
	})
	if err != nil {
		t.Fatalf("create target-a: %v", err)
	}
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/target-b.md",
		Content: "# Target B",
	})
	if err != nil {
		t.Fatalf("create target-b: %v", err)
	}
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Create source doc with relates_to: [Target A]
	src, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/relto-src.md",
		Content: "---\ntitle: Source\nrelates_to:\n  - Target A\n---\n# Source",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	srcID := models.MustRecordIDString(src.ID)
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Verify relation to Target A exists
	rels, err := testDB.GetRelations(ctx, srcID)
	if err != nil {
		t.Fatalf("get relations: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(rels))
	}

	// Update source to relates_to: [Target B] (removing Target A)
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/relto-src.md",
		Content: "---\ntitle: Source\nrelates_to:\n  - Target B\n---\n# Source Updated",
	})
	if err != nil {
		t.Fatalf("update source: %v", err)
	}
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending after update: %v", err)
	}

	// Verify: only relation to Target B exists (Target A was cleaned up)
	rels, err = testDB.GetRelations(ctx, srcID)
	if err != nil {
		t.Fatalf("get relations after update: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relation after update, got %d", len(rels))
	}

	// Update source to remove all relates_to
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/relto-src.md",
		Content: "---\ntitle: Source\n---\n# Source No Relations",
	})
	if err != nil {
		t.Fatalf("update source no rels: %v", err)
	}
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending after remove: %v", err)
	}

	// Verify: no frontmatter relations remain
	rels, err = testDB.GetRelations(ctx, srcID)
	if err != nil {
		t.Fatalf("get relations after remove: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("expected 0 relations after removing relates_to, got %d", len(rels))
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestLabelGraph verifies that ProcessDocument creates has_label edges
// and that label graph queries work correctly.
func TestLabelGraph(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "label-graph-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "label-graph-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create docs with labels
	doc1, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/labeled-1.md",
		Content: "---\nlabels: [go, backend]\n---\n# Doc 1",
	})
	if err != nil {
		t.Fatalf("create doc1: %v", err)
	}
	doc1ID := models.MustRecordIDString(doc1.ID)

	doc2, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/labeled-2.md",
		Content: "---\nlabels: [go, frontend]\n---\n# Doc 2",
	})
	if err != nil {
		t.Fatalf("create doc2: %v", err)
	}
	doc2ID := models.MustRecordIDString(doc2.ID)

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Verify labels for doc1
	labels, err := testDB.GetLabelsForDocument(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get labels for doc1: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("expected 2 labels for doc1, got %d: %v", len(labels), labels)
	}

	// Verify labels for doc2
	labels, err = testDB.GetLabelsForDocument(ctx, doc2ID)
	if err != nil {
		t.Fatalf("get labels for doc2: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("expected 2 labels for doc2, got %d: %v", len(labels), labels)
	}

	// Query by "go" — should return both
	docs, err := testDB.GetDocumentsByLabel(ctx, vaultID, "go")
	if err != nil {
		t.Fatalf("get docs by label 'go': %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs with 'go' label, got %d", len(docs))
	}

	// Query by "backend" — should return only doc1
	docs, err = testDB.GetDocumentsByLabel(ctx, vaultID, "backend")
	if err != nil {
		t.Fatalf("get docs by label 'backend': %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with 'backend' label, got %d", len(docs))
	}

	// Update doc1 to change labels (remove "backend", add "infra")
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/labeled-1.md",
		Content: "---\nlabels: [go, infra]\n---\n# Doc 1 Updated",
	})
	if err != nil {
		t.Fatalf("update doc1: %v", err)
	}
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending after update: %v", err)
	}

	// Verify "backend" no longer returns doc1
	docs, err = testDB.GetDocumentsByLabel(ctx, vaultID, "backend")
	if err != nil {
		t.Fatalf("get docs by label 'backend' after update: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs with 'backend' label after update, got %d", len(docs))
	}

	// Verify "infra" returns doc1
	docs, err = testDB.GetDocumentsByLabel(ctx, vaultID, "infra")
	if err != nil {
		t.Fatalf("get docs by label 'infra': %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with 'infra' label, got %d", len(docs))
	}

	// Delete doc1 — has_label edges should be cleaned up
	if err := docSvc.Delete(ctx, vaultID, "/labeled-1.md"); err != nil {
		t.Fatalf("delete doc1: %v", err)
	}

	labels, err = testDB.GetLabelsForDocument(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get labels for deleted doc1: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected 0 labels for deleted doc, got %d: %v", len(labels), labels)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestSyncChunks_PreservesUnchangedChunks verifies the content-based chunk diffing:
// unchanged chunks keep their embeddings, changed chunks get new embed_at, removed chunks are deleted.
func TestSyncChunks_PreservesUnchangedChunks(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "sync-chunks-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "sync-test-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	// nil embedder — embed_at should NOT be set on any chunks
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// --- Step 1: Create a document with initial content ---
	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/sync-test.md",
		Content: "# Title\n\nFirst paragraph content.\n\nSecond paragraph content.",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Get initial chunks
	initialChunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get initial chunks: %v", err)
	}
	if len(initialChunks) == 0 {
		t.Fatal("expected at least 1 chunk after create")
	}

	// Verify nil embedder means no embed_at set
	for _, c := range initialChunks {
		if c.EmbedAt != nil {
			t.Error("with nil embedder, embed_at should be nil")
		}
	}

	initialContent := initialChunks[0].Content
	initialID := models.MustRecordIDString(initialChunks[0].ID)

	// --- Step 2: Update with same content — chunks should be preserved ---
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/sync-test.md",
		Content: "# Title\n\nFirst paragraph content.\n\nSecond paragraph content.",
	})
	if err != nil {
		t.Fatalf("upsert same content: %v", err)
	}

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending step 2: %v", err)
	}

	afterSameUpdate, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after same-content update: %v", err)
	}
	if len(afterSameUpdate) != len(initialChunks) {
		t.Errorf("chunk count changed on same content: was %d, now %d", len(initialChunks), len(afterSameUpdate))
	}

	// The first chunk should have the same ID (preserved, not recreated)
	afterID := models.MustRecordIDString(afterSameUpdate[0].ID)
	if afterID != initialID {
		t.Errorf("chunk ID changed on same content: was %q, now %q", initialID, afterID)
	}
	if afterSameUpdate[0].Content != initialContent {
		t.Error("chunk content should be unchanged")
	}

	// --- Step 3: Update with different content — old chunks deleted, new created ---
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/sync-test.md",
		Content: "# New Title\n\nCompletely different content here.",
	})
	if err != nil {
		t.Fatalf("upsert new content: %v", err)
	}

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending step 3: %v", err)
	}

	afterNewContent, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after new content: %v", err)
	}
	if len(afterNewContent) == 0 {
		t.Fatal("expected chunks after content update")
	}

	// Old chunk ID should no longer exist (content changed → deleted + new created)
	for _, c := range afterNewContent {
		cID := models.MustRecordIDString(c.ID)
		if cID == initialID {
			t.Error("old chunk should be deleted when content changes")
		}
	}

	// --- Cleanup ---
	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestSyncChunks_PartialUpdate verifies that when some chunks change and others don't,
// only the changed chunks get new IDs (deleted + created).
func TestSyncChunks_PartialUpdate(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "partial-sync-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "partial-sync-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create document with content that uses heading-based splitting.
	longContent := "# Section A\n\nContent of section A that is preserved across updates.\n\n# Section B\n\nContent of section B that will change."

	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/partial-sync.md",
		Content: longContent,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	initialChunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get initial chunks: %v", err)
	}

	// Build content→ID map for initial state
	initialByContent := make(map[string]string)
	for _, c := range initialChunks {
		initialByContent[c.Content] = models.MustRecordIDString(c.ID)
	}

	// Update: keep section A, change section B
	updatedContent := "# Section A\n\nContent of section A that is preserved across updates.\n\n# Section B\n\nThis is completely new content for section B."

	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/partial-sync.md",
		Content: updatedContent,
	})
	if err != nil {
		t.Fatalf("update doc: %v", err)
	}

	updatedChunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get updated chunks: %v", err)
	}

	// Check which chunks were preserved vs recreated
	for _, c := range updatedChunks {
		cID := models.MustRecordIDString(c.ID)
		if oldID, existed := initialByContent[c.Content]; existed {
			// Content existed before — should have same ID (preserved)
			if cID != oldID {
				t.Errorf("unchanged chunk should keep ID: content=%q, old=%q, new=%q", c.Content[:min(30, len(c.Content))], oldID, cID)
			}
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}
