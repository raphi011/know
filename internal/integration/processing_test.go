package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/vault"
)

// setupProcessingTest creates a vault and file service for processing tests.
func setupProcessingTest(t *testing.T, ctx context.Context) (string, *file.Service) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "proc-test-user-" + suffix})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "proc-test-" + suffix})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := models.MustRecordIDString(v.ID)

	fileSvc := file.NewService(testDB, blob.NewFS(t.TempDir()), nil, parser.DefaultChunkConfig(), file.VersionConfig{
		CoalesceMinutes: 10,
		RetentionCount:  50,
	}, nil, 0)

	return vaultID, fileSvc
}

func TestCreate_DefersChunksAndWikiLinks(t *testing.T) {
	ctx := context.Background()
	vaultID, fileSvc := setupProcessingTest(t, ctx)

	longContent := "# Big Document\n\n" + strings.Repeat("This is a paragraph of content that should be chunked. ", 200)
	wikiContent := longContent + "\n\nSee also [[Other Page]]."

	doc, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/deferred-test.md",
		Content: wikiContent,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks before ProcessAllPending, got %d", len(chunks))
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 wiki-links before ProcessAllPending, got %d", len(links))
	}

	if err := fileSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	chunks, err = testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after processing: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected chunks after ProcessAllPending, got 0")
	}

	links, err = testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("get wiki-links after processing: %v", err)
	}
	if len(links) == 0 {
		t.Error("expected wiki-links after ProcessAllPending, got 0")
	}

}

func TestProcessAllPending_EmptyIsNoOp(t *testing.T) {
	ctx := context.Background()
	_, fileSvc := setupProcessingTest(t, ctx)

	if err := fileSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("ProcessAllPending on empty vault: %v", err)
	}
}

func TestProcessAllPending_ProcessesMultipleFiles(t *testing.T) {
	ctx := context.Background()
	vaultID, fileSvc := setupProcessingTest(t, ctx)

	for i := range 5 {
		_, err := fileSvc.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    fmt.Sprintf("/multi-%d.md", i),
			Content: fmt.Sprintf("# Doc %d\n\nContent for document %d.", i, i),
		})
		if err != nil {
			t.Fatalf("create doc %d: %v", i, err)
		}
	}

	unprocessed, err := testDB.ListUnprocessedFiles(ctx, 100)
	if err != nil {
		t.Fatalf("list unprocessed: %v", err)
	}
	if len(unprocessed) < 5 {
		t.Fatalf("expected at least 5 unprocessed, got %d", len(unprocessed))
	}

	if err := fileSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	unprocessed, err = testDB.ListUnprocessedFiles(ctx, 100)
	if err != nil {
		t.Fatalf("list unprocessed after: %v", err)
	}
	if len(unprocessed) != 0 {
		t.Errorf("expected 0 unprocessed after ProcessAllPending, got %d", len(unprocessed))
	}
}

func TestProcessDocument_Idempotent(t *testing.T) {
	ctx := context.Background()
	vaultID, fileSvc := setupProcessingTest(t, ctx)

	longContent := "# Idempotent Test\n\n" + strings.Repeat("Content that will produce chunks. ", 200)
	longContent += "\n\nSee also [[Some Link]]."

	doc, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/idempotent-test.md",
		Content: longContent,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := fileSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("first process: %v", err)
	}

	chunks1, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks: %v", err)
	}
	links1, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}

	latest, err := testDB.GetFileByID(ctx, docID)
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if err := fileSvc.ProcessFile(ctx, latest); err != nil {
		t.Fatalf("second process: %v", err)
	}

	chunks2, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after second process: %v", err)
	}
	links2, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("get wiki-links after second process: %v", err)
	}

	if len(chunks1) != len(chunks2) {
		t.Errorf("chunk count changed after reprocess: %d → %d", len(chunks1), len(chunks2))
	}
	if len(links1) != len(links2) {
		t.Errorf("wiki-link count changed after reprocess: %d → %d", len(links1), len(links2))
	}
}
