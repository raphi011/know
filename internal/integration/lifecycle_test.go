package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	v1db "github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/auth"
	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/document"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"
	"github.com/raphaelgruber/memcp-go/internal/v2/search"
	"github.com/raphaelgruber/memcp-go/internal/v2/vault"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	testDB        *v2db.Client
	testContainer testcontainers.Container
)

func TestMain(m *testing.M) {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	ctx := context.Background()

	var err error
	testContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "surrealdb/surrealdb:v3.0.0-beta.1",
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

	testDB, err = v2db.NewClient(ctx, v1db.Config{
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

	_, err = testDB.CreateToken(ctx, userID, tokenHash, "test-token", []string{vaultID})
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

	docSvc := document.NewService(testDB, nil) // no embedder

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
		Source: models.SourceScrape,
	})
	if err != nil {
		t.Fatalf("create doc1: %v", err)
	}
	doc1ID, err := models.RecordIDString(doc1.ID)
	if err != nil {
		t.Fatalf("doc1 ID: %v", err)
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
		Source: models.SourceScrape,
	})
	if err != nil {
		t.Fatalf("create doc2: %v", err)
	}
	doc2ID, err := models.RecordIDString(doc2.ID)
	if err != nil {
		t.Fatalf("doc2 ID: %v", err)
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

	// --- BM25 Search ---

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
		rID, err := models.RecordIDString(r.Document.ID)
		if err != nil {
			t.Fatalf("extract search result doc ID: %v", err)
		}
		if rID == doc1ID {
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
		Source: models.SourceScrape,
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
