package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
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
// user → vault → token → files → wiki-links → search → delete cascade
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

	_, err = testDB.CreateToken(ctx, userID, tokenHash, "test-token", time.Now().Add(90*24*time.Hour))
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

	// --- Files with wiki-links ---

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0) // no embedder

	doc1, err := fileSvc.Create(ctx, models.FileInput{
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

	// Process pending files (deferred pipeline: chunks, wiki-links, etc.)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
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
		if l.ToFile != nil {
			t.Errorf("wiki-link %q should be dangling", l.RawTarget)
		}
	}

	// --- Create second doc that resolves "Beta Notes" ---

	doc2, err := fileSvc.Create(ctx, models.FileInput{
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

	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending after doc2: %v", err)
	}

	// Verify dangling link to "Beta Notes" is now resolved
	links, err = testDB.GetWikiLinks(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get wiki-links after doc2: %v", err)
	}

	var resolvedCount int
	for _, l := range links {
		if l.ToFile != nil {
			resolvedCount++
			toID, err := models.RecordIDString(*l.ToFile)
			if err != nil {
				t.Fatalf("extract toFile ID: %v", err)
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
	t.Logf("doc1 has %d chunks, first chunk content: %q", len(doc1Chunks), doc1Chunks[0].Text[:min(80, len(doc1Chunks[0].Text))])

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

	if err := fileSvc.Delete(ctx, vaultID, "/projects/alpha.md"); err != nil {
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
	gone, err := testDB.GetFileByID(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get deleted doc1: %v", err)
	}
	if gone != nil {
		t.Error("doc1 should be nil after delete")
	}

	// doc2 should still exist
	still, err := testDB.GetFileByID(ctx, doc2ID)
	if err != nil {
		t.Fatalf("get doc2 after doc1 delete: %v", err)
	}
	if still == nil {
		t.Error("doc2 should still exist")
	}

	// --- Upsert (update via create) ---

	doc2Updated, err := fileSvc.Create(ctx, models.FileInput{
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

// TestDeleteByPrefix verifies that DeleteByPrefix removes all files under
// a path prefix while leaving files outside the prefix untouched.
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	for _, path := range []string{"/test-prefix/a.md", "/test-prefix/b.md", "/other/c.md"} {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := fileSvc.DeleteByPrefix(ctx, vaultID, "/test-prefix")
	if err != nil {
		t.Fatalf("delete by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("deleted count: got %d, want 2", count)
	}

	for _, path := range []string{"/test-prefix/a.md", "/test-prefix/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted", path)
		}
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/other/c.md")
	if err != nil {
		t.Fatalf("get doc /other/c.md: %v", err)
	}
	if doc == nil {
		t.Error("doc /other/c.md should still exist")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveByPrefix verifies that MoveByPrefix renames all files under a
// path prefix while leaving files outside the prefix untouched.
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	for _, path := range []string{"/old-folder/a.md", "/old-folder/b.md"} {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := fileSvc.MoveByPrefix(ctx, vaultID, "/old-folder", "/new-folder")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("moved count: got %d, want 2", count)
	}

	for _, path := range []string{"/old-folder/a.md", "/old-folder/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc != nil {
			t.Errorf("doc %s should no longer exist at old path", path)
		}
	}

	for _, path := range []string{"/new-folder/a.md", "/new-folder/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
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

// TestDeleteByPrefix_BoundaryCollision verifies that deleting "/test-prefix" does not
// affect files under "/test-prefix-extra" or "/test-prefixed".
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	paths := []string{
		"/test-prefix/a.md",
		"/test-prefix/sub/b.md",
		"/test-prefix-extra/c.md",
		"/test-prefixed/d.md",
	}
	for _, path := range paths {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := fileSvc.DeleteByPrefix(ctx, vaultID, "/test-prefix")
	if err != nil {
		t.Fatalf("delete by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("deleted count: got %d, want 2", count)
	}

	for _, path := range []string{"/test-prefix/a.md", "/test-prefix/sub/b.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
		if err != nil {
			t.Fatalf("get doc %s: %v", path, err)
		}
		if doc != nil {
			t.Errorf("doc %s should be deleted", path)
		}
	}

	for _, path := range []string{"/test-prefix-extra/c.md", "/test-prefixed/d.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
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
// preserves the full relative path structure for deeply nested files.
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	paths := []string{"/old-folder/a.md", "/old-folder/sub/b.md", "/old-folder/sub/deep/c.md"}
	for _, path := range paths {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := fileSvc.MoveByPrefix(ctx, vaultID, "/old-folder", "/new-folder")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 3 {
		t.Errorf("moved count: got %d, want 3", count)
	}

	for _, path := range []string{"/new-folder/a.md", "/new-folder/sub/b.md", "/new-folder/sub/deep/c.md"} {
		doc, err := testDB.GetFileByPath(ctx, vaultID, path)
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
// affect files under "/guides-extra".
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	for _, path := range []string{"/guides/a.md", "/guides-extra/b.md"} {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Content: "# Doc at " + path,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", path, err)
		}
	}

	count, err := fileSvc.MoveByPrefix(ctx, vaultID, "/guides", "/tutorials")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 1 {
		t.Errorf("moved count: got %d, want 1", count)
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/guides-extra/b.md")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if doc == nil {
		t.Error("doc /guides-extra/b.md should still exist")
	}

	doc, err = testDB.GetFileByPath(ctx, vaultID, "/tutorials/a.md")
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/folder/a.md",
		Content: "# Doc",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	count, err := fileSvc.MoveByPrefix(ctx, vaultID, "/folder", "/folder")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 0 {
		t.Errorf("moved count: got %d, want 0 (same prefix)", count)
	}

	doc, err := testDB.GetFileByPath(ctx, vaultID, "/folder/a.md")
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

// TestDeleteUnresolvesIncomingWikiLinks verifies that deleting a file makes
// incoming wiki_links dangling (to_file = NONE) rather than deleting them.
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	docB, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/target.md",
		Content: "# Target\n\nTarget content.",
	})
	if err != nil {
		t.Fatalf("create target doc: %v", err)
	}
	docBID := models.MustRecordIDString(docB.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	docA, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/source.md",
		Content: "# Source\n\nSee [[Target]].",
	})
	if err != nil {
		t.Fatalf("create source doc: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Fatal("wiki-link should be resolved before delete")
	}

	if err := fileSvc.Delete(ctx, vaultID, "/target.md"); err != nil {
		t.Fatalf("delete target doc: %v", err)
	}

	gone, err := testDB.GetFileByID(ctx, docBID)
	if err != nil {
		t.Fatalf("get deleted doc: %v", err)
	}
	if gone != nil {
		t.Error("target doc should be deleted")
	}

	links, err = testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("get wiki-links after delete: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link preserved, got %d", len(links))
	}
	if links[0].ToFile != nil {
		t.Error("wiki-link should be dangling after target deleted")
	}
	if links[0].RawTarget != "Target" {
		t.Errorf("raw_target should be preserved, got %q", links[0].RawTarget)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestMoveUpdatesWikiLinkRawTargets verifies that moving a file updates
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/old/target.md",
		Content: "# Target",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	docA, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/source.md",
		Content: "# Source\n\nSee [[/old/target.md]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	_, err = fileSvc.Move(ctx, vaultID, "/old/target.md", "/new/target.md")
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("get wiki-links after move: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	// With stem-based resolution, raw_target is the shortest unambiguous target.
	// Since "target" is unique in the vault, raw_target should be just the stem.
	if links[0].RawTarget != "target" {
		t.Errorf("expected raw_target 'target', got %q", links[0].RawTarget)
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/old-dir/a.md",
		Content: "# A",
	})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	src, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/source.md",
		Content: "# Source\n\nSee [[/old-dir/a.md]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	srcID := models.MustRecordIDString(src.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	count, err := fileSvc.MoveByPrefix(ctx, vaultID, "/old-dir", "/new-dir")
	if err != nil {
		t.Fatalf("move by prefix: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 moved, got %d", count)
	}

	links, err := testDB.GetWikiLinks(ctx, srcID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	// With stem-based resolution, raw_target is the shortest unambiguous target.
	// Since "a" is unique in the vault, raw_target should be just the stem.
	if links[0].RawTarget != "a" {
		t.Errorf("expected raw_target 'a', got %q", links[0].RawTarget)
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	doc1, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/labeled-1.md",
		Content: "---\nlabels: [go, backend]\n---\n# Doc 1",
	})
	if err != nil {
		t.Fatalf("create doc1: %v", err)
	}
	doc1ID := models.MustRecordIDString(doc1.ID)

	doc2, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/labeled-2.md",
		Content: "---\nlabels: [go, frontend]\n---\n# Doc 2",
	})
	if err != nil {
		t.Fatalf("create doc2: %v", err)
	}
	doc2ID := models.MustRecordIDString(doc2.ID)

	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	labels, err := testDB.GetLabelsForFile(ctx, doc1ID)
	if err != nil {
		t.Fatalf("get labels for doc1: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("expected 2 labels for doc1, got %d: %v", len(labels), labels)
	}

	labels, err = testDB.GetLabelsForFile(ctx, doc2ID)
	if err != nil {
		t.Fatalf("get labels for doc2: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("expected 2 labels for doc2, got %d: %v", len(labels), labels)
	}

	docs, err := testDB.GetFilesByLabel(ctx, vaultID, "go")
	if err != nil {
		t.Fatalf("get docs by label 'go': %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs with 'go' label, got %d", len(docs))
	}

	docs, err = testDB.GetFilesByLabel(ctx, vaultID, "backend")
	if err != nil {
		t.Fatalf("get docs by label 'backend': %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with 'backend' label, got %d", len(docs))
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID, Path: "/labeled-1.md",
		Content: "---\nlabels: [go, infra]\n---\n# Doc 1 Updated",
	})
	if err != nil {
		t.Fatalf("update doc1: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending after update: %v", err)
	}

	docs, err = testDB.GetFilesByLabel(ctx, vaultID, "backend")
	if err != nil {
		t.Fatalf("get docs by label 'backend' after update: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs with 'backend' label after update, got %d", len(docs))
	}

	docs, err = testDB.GetFilesByLabel(ctx, vaultID, "infra")
	if err != nil {
		t.Fatalf("get docs by label 'infra': %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with 'infra' label, got %d", len(docs))
	}

	if err := fileSvc.Delete(ctx, vaultID, "/labeled-1.md"); err != nil {
		t.Fatalf("delete doc1: %v", err)
	}

	labels, err = testDB.GetLabelsForFile(ctx, doc1ID)
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

// TestSyncChunks_PreservesUnchangedChunks verifies the content-based chunk diffing.
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	doc, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/sync-test.md",
		Content: "# Title\n\nFirst paragraph content.\n\nSecond paragraph content.",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	initialChunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get initial chunks: %v", err)
	}
	if len(initialChunks) == 0 {
		t.Fatal("expected at least 1 chunk after create")
	}

	initialContent := initialChunks[0].Text
	initialID := models.MustRecordIDString(initialChunks[0].ID)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/sync-test.md",
		Content: "# Title\n\nFirst paragraph content.\n\nSecond paragraph content.",
	})
	if err != nil {
		t.Fatalf("upsert same content: %v", err)
	}

	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending step 2: %v", err)
	}

	afterSameUpdate, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after same-content update: %v", err)
	}
	if len(afterSameUpdate) != len(initialChunks) {
		t.Errorf("chunk count changed on same content: was %d, now %d", len(initialChunks), len(afterSameUpdate))
	}

	afterID := models.MustRecordIDString(afterSameUpdate[0].ID)
	if afterID != initialID {
		t.Errorf("chunk ID changed on same content: was %q, now %q", initialID, afterID)
	}
	if afterSameUpdate[0].Text != initialContent {
		t.Error("chunk content should be unchanged")
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/sync-test.md",
		Content: "# New Title\n\nCompletely different content here.",
	})
	if err != nil {
		t.Fatalf("upsert new content: %v", err)
	}

	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending step 3: %v", err)
	}

	afterNewContent, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after new content: %v", err)
	}
	if len(afterNewContent) == 0 {
		t.Fatal("expected chunks after content update")
	}

	for _, c := range afterNewContent {
		cID := models.MustRecordIDString(c.ID)
		if cID == initialID {
			t.Error("old chunk should be deleted when content changes")
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestSyncChunks_PartialUpdate verifies partial chunk updates preserve unchanged chunks.
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

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	longContent := "# Section A\n\nContent of section A that is preserved across updates.\n\n# Section B\n\nContent of section B that will change."

	doc, err := fileSvc.Create(ctx, models.FileInput{
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

	initialByContent := make(map[string]string)
	for _, c := range initialChunks {
		initialByContent[c.Text] = models.MustRecordIDString(c.ID)
	}

	updatedContent := "# Section A\n\nContent of section A that is preserved across updates.\n\n# Section B\n\nThis is completely new content for section B."

	_, err = fileSvc.Create(ctx, models.FileInput{
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

	for _, c := range updatedChunks {
		cID := models.MustRecordIDString(c.ID)
		if oldID, existed := initialByContent[c.Text]; existed {
			if cID != oldID {
				t.Errorf("unchanged chunk should keep ID: content=%q, old=%q, new=%q", c.Text[:min(30, len(c.Text))], oldID, cID)
			}
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestWikiLinkStemResolution verifies that wiki-links resolve via stem matching
// (case-insensitive, space/hyphen normalized).
func TestWikiLinkStemResolution(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "stem-resolve-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "stem-resolve-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create the target file first
	target, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/notes/vector-embeddings.md",
		Content: "# Vector Embeddings\n\nAll about embeddings.",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	targetID := models.MustRecordIDString(target.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending target: %v", err)
	}

	// Create source with lowercase hyphenated stem link
	source, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/source.md",
		Content: "# Source\n\nSee [[vector-embeddings]] for details.",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceID := models.MustRecordIDString(source.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending source: %v", err)
	}

	// Verify the link resolved
	links, err := testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Fatal("wiki-link should be resolved via stem match")
	}
	toID, err := models.RecordIDString(*links[0].ToFile)
	if err != nil {
		t.Fatalf("extract toFile ID: %v", err)
	}
	if toID != targetID {
		t.Errorf("resolved link points to %q, want %q", toID, targetID)
	}

	// Test case-insensitivity: create another source with mixed-case link
	source2, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/source2.md",
		Content: "# Source 2\n\nSee [[Vector-Embeddings]] for details.",
	})
	if err != nil {
		t.Fatalf("create source2: %v", err)
	}
	source2ID := models.MustRecordIDString(source2.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending source2: %v", err)
	}

	links2, err := testDB.GetWikiLinks(ctx, source2ID)
	if err != nil {
		t.Fatalf("get wiki-links source2: %v", err)
	}
	if len(links2) != 1 {
		t.Fatalf("expected 1 wiki-link from source2, got %d", len(links2))
	}
	if links2[0].ToFile == nil {
		t.Fatal("case-insensitive wiki-link should resolve")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestWikiLinkAmbiguity verifies that creating a second file with the same stem
// makes stem-only links ambiguous (dangling), and deleting it re-resolves them.
func TestWikiLinkAmbiguity(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "ambiguity-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "ambiguity-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Step 1: Create /a/notes.md
	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/a/notes.md",
		Content: "# Notes A",
	})
	if err != nil {
		t.Fatalf("create /a/notes.md: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Step 2: Create source with [[notes]] — should resolve (unique stem)
	source, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/source.md",
		Content: "# Source\n\nSee [[notes]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceID := models.MustRecordIDString(source.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending source: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Fatal("wiki-link should be resolved when stem is unique")
	}

	// Step 3: Create /b/notes.md — same stem, triggers ambiguity
	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/b/notes.md",
		Content: "# Notes B",
	})
	if err != nil {
		t.Fatalf("create /b/notes.md: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending after ambiguity: %v", err)
	}

	links, err = testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links after ambiguity: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile != nil {
		t.Error("wiki-link should be dangling when stem is ambiguous")
	}

	// Step 4: Delete /b/notes.md — ambiguity removed, should re-resolve
	if err := fileSvc.Delete(ctx, vaultID, "/b/notes.md"); err != nil {
		t.Fatalf("delete /b/notes.md: %v", err)
	}

	links, err = testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links after delete: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Error("wiki-link should re-resolve after ambiguity removed")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestWikiLinkMoveRecomputeTarget verifies that moving a file recomputes
// incoming raw_targets to the shortest unambiguous form.
func TestWikiLinkMoveRecomputeTarget(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "move-recompute-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "move-recompute-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create target
	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/notes/foo.md",
		Content: "# Foo",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Create source with [[foo]]
	source, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/source.md",
		Content: "# Source\n\nSee [[foo]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceID := models.MustRecordIDString(source.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending source: %v", err)
	}

	// Verify resolved
	links, err := testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Fatal("wiki-link should be resolved")
	}
	if links[0].RawTarget != "foo" {
		t.Errorf("raw_target before move: got %q, want %q", links[0].RawTarget, "foo")
	}

	// Move file
	_, err = fileSvc.Move(ctx, vaultID, "/notes/foo.md", "/archive/foo.md")
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	// Verify raw_target is still "foo" (unique stem, shortest form)
	links, err = testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links after move: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].RawTarget != "foo" {
		t.Errorf("raw_target after move: got %q, want %q", links[0].RawTarget, "foo")
	}
	if links[0].ToFile == nil {
		t.Error("wiki-link should still be resolved after move")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestWikiLinkPathSuffixDisambiguation verifies that [[a/notes]] resolves correctly
// when the stem "notes" is ambiguous but the path suffix disambiguates.
func TestWikiLinkPathSuffixDisambiguation(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "suffix-disambig-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "suffix-disambig-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create two files with same stem "notes"
	targetA, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/a/notes.md",
		Content: "# Notes A",
	})
	if err != nil {
		t.Fatalf("create /a/notes.md: %v", err)
	}
	targetAID := models.MustRecordIDString(targetA.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/b/notes.md",
		Content: "# Notes B",
	})
	if err != nil {
		t.Fatalf("create /b/notes.md: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Create source with path-qualified link [[a/notes]] — should resolve despite ambiguous stem
	source, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/source.md",
		Content: "# Source\n\nSee [[a/notes]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceID := models.MustRecordIDString(source.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending source: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 wiki-link, got %d", len(links))
	}
	if links[0].ToFile == nil {
		t.Fatal("path-qualified wiki-link should resolve despite ambiguous stem")
	}
	toID, err := models.RecordIDString(*links[0].ToFile)
	if err != nil {
		t.Fatalf("extract toFile ID: %v", err)
	}
	if toID != targetAID {
		t.Errorf("resolved to %q, want %q", toID, targetAID)
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestWikiLinkSQLNormalization verifies that the DB-side SQL normalization resolves
// links with spaces, underscores, .md extensions, and mixed case to the correct stem.
func TestWikiLinkSQLNormalization(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "sql-norm-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "sql-norm-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Create source with various link forms that should all resolve to "beta-notes"
	source, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/source.md",
		Content: "# Source\n\nSee [[Beta Notes]] and [[beta_notes.md]] and [[BETA-NOTES]].",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceID := models.MustRecordIDString(source.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending source: %v", err)
	}

	// All links should be dangling (target doesn't exist yet)
	links, err := testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 wiki-links, got %d", len(links))
	}
	for _, l := range links {
		if l.ToFile != nil {
			t.Errorf("link %q should be dangling before target exists", l.RawTarget)
		}
	}

	// Now create the target file — should resolve all dangling links via SQL normalization
	target, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/beta-notes.md",
		Content: "# Beta Notes",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	targetID := models.MustRecordIDString(target.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending target: %v", err)
	}

	// All three links should now be resolved
	links, err = testDB.GetWikiLinks(ctx, sourceID)
	if err != nil {
		t.Fatalf("get wiki-links after target creation: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 wiki-links, got %d", len(links))
	}
	for _, l := range links {
		if l.ToFile == nil {
			t.Errorf("link %q should be resolved after target creation", l.RawTarget)
			continue
		}
		toID, err := models.RecordIDString(*l.ToFile)
		if err != nil {
			t.Errorf("extract toFile ID for %q: %v", l.RawTarget, err)
			continue
		}
		if toID != targetID {
			t.Errorf("link %q resolved to %q, want %q", l.RawTarget, toID, targetID)
		}
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}

// TestWikiLinkStemOnlyVsPathQualifiedDuringAmbiguity verifies that when a stem
// becomes ambiguous, stem-only links ([[notes]]) become dangling but path-qualified
// links ([[a/notes]]) remain resolved.
func TestWikiLinkStemOnlyVsPathQualifiedDuringAmbiguity(t *testing.T) {
	ctx := context.Background()

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "qualified-vs-stem-user-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "qualified-vs-stem-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	// Step 1: Create /a/notes.md (unique stem)
	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/a/notes.md",
		Content: "# Notes A",
	})
	if err != nil {
		t.Fatalf("create /a/notes.md: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Step 2: Create sources — one with stem-only [[notes]], one with path-qualified [[a/notes]]
	stemSource, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/stem-source.md",
		Content: "# Stem Source\n\nSee [[notes]].",
	})
	if err != nil {
		t.Fatalf("create stem-source: %v", err)
	}
	stemSourceID := models.MustRecordIDString(stemSource.ID)

	qualifiedSource, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/qualified-source.md",
		Content: "# Qualified Source\n\nSee [[a/notes]].",
	})
	if err != nil {
		t.Fatalf("create qualified-source: %v", err)
	}
	qualifiedSourceID := models.MustRecordIDString(qualifiedSource.ID)
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending sources: %v", err)
	}

	// Both should be resolved when stem is unique
	stemLinks, err := testDB.GetWikiLinks(ctx, stemSourceID)
	if err != nil {
		t.Fatalf("get stem links: %v", err)
	}
	if len(stemLinks) != 1 || stemLinks[0].ToFile == nil {
		t.Fatal("stem-only link should be resolved when stem is unique")
	}

	qualifiedLinks, err := testDB.GetWikiLinks(ctx, qualifiedSourceID)
	if err != nil {
		t.Fatalf("get qualified links: %v", err)
	}
	if len(qualifiedLinks) != 1 || qualifiedLinks[0].ToFile == nil {
		t.Fatal("path-qualified link should be resolved when stem is unique")
	}

	// Step 3: Create /b/notes.md — introduces ambiguity
	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/b/notes.md",
		Content: "# Notes B",
	})
	if err != nil {
		t.Fatalf("create /b/notes.md: %v", err)
	}
	if err := fileSvc.ProcessAllPending(ctx, vaultID); err != nil {
		t.Fatalf("process pending after ambiguity: %v", err)
	}

	// Stem-only link should become dangling
	stemLinks, err = testDB.GetWikiLinks(ctx, stemSourceID)
	if err != nil {
		t.Fatalf("get stem links after ambiguity: %v", err)
	}
	if len(stemLinks) != 1 {
		t.Fatalf("expected 1 stem link, got %d", len(stemLinks))
	}
	if stemLinks[0].ToFile != nil {
		t.Error("stem-only link [[notes]] should be dangling when stem is ambiguous")
	}

	// Path-qualified link should remain resolved (UnresolveStemOnlyLinks only
	// affects links whose raw_target does NOT contain "/")
	qualifiedLinks, err = testDB.GetWikiLinks(ctx, qualifiedSourceID)
	if err != nil {
		t.Fatalf("get qualified links after ambiguity: %v", err)
	}
	if len(qualifiedLinks) != 1 {
		t.Fatalf("expected 1 qualified link, got %d", len(qualifiedLinks))
	}
	if qualifiedLinks[0].ToFile == nil {
		t.Error("path-qualified link [[a/notes]] should remain resolved during stem ambiguity")
	}

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("cleanup vault: %v", err)
	}
}
