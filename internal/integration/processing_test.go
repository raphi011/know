package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/vault"
)

// setupProcessingTest creates a vault and document service for processing tests.
func setupProcessingTest(t *testing.T, ctx context.Context) (string, *document.Service) {
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

	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{
		CoalesceMinutes: 10,
		RetentionCount:  50,
	}, nil, 0)

	return vaultID, docSvc
}

// TestCreate_DefersChunksAndWikiLinks verifies that Create stores the document
// but does NOT produce chunks or wiki-links. Those are deferred to ProcessAllPending.
func TestCreate_DefersChunksAndWikiLinks(t *testing.T) {
	ctx := context.Background()
	vaultID, docSvc := setupProcessingTest(t, ctx)

	// Content must exceed chunk threshold (6000 chars) to produce chunks
	longContent := "# Big Document\n\n" + strings.Repeat("This is a paragraph of content that should be chunked. ", 200)
	wikiContent := longContent + "\n\nSee also [[Other Page]]."

	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/deferred-test.md",
		Content: wikiContent,
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	// Before processing: no chunks should exist
	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks before ProcessAllPending, got %d", len(chunks))
	}

	// Before processing: no wiki-links should exist
	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("get wiki-links: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 wiki-links before ProcessAllPending, got %d", len(links))
	}

	// Document should be marked as unprocessed
	if doc.Processed {
		t.Error("expected processed=false after Create")
	}

	// Now process
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// After processing: chunks should exist
	chunks, err = testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("get chunks after processing: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected chunks after ProcessAllPending, got 0")
	}

	// After processing: wiki-link should exist
	links, err = testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("get wiki-links after processing: %v", err)
	}
	if len(links) == 0 {
		t.Error("expected wiki-links after ProcessAllPending, got 0")
	}

	// Document should now be marked as processed
	updated, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if !updated.Processed {
		t.Error("expected processed=true after ProcessAllPending")
	}
}

// TestProcessAllPending_EmptyIsNoOp verifies that ProcessAllPending returns
// immediately when there are no unprocessed documents.
func TestProcessAllPending_EmptyIsNoOp(t *testing.T) {
	ctx := context.Background()
	_, docSvc := setupProcessingTest(t, ctx)

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("ProcessAllPending on empty vault: %v", err)
	}
}

// TestProcessAllPending_ProcessesMultipleDocuments verifies that ProcessAllPending
// processes all unprocessed documents, not just the first batch.
func TestProcessAllPending_ProcessesMultipleDocuments(t *testing.T) {
	ctx := context.Background()
	vaultID, docSvc := setupProcessingTest(t, ctx)

	// Create several documents
	for i := 0; i < 5; i++ {
		_, err := docSvc.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    fmt.Sprintf("/multi-%d.md", i),
			Content: fmt.Sprintf("# Doc %d\n\nContent for document %d.", i, i),
			Source:  models.SourceManual,
		})
		if err != nil {
			t.Fatalf("create doc %d: %v", i, err)
		}
	}

	// All should be unprocessed
	unprocessed, err := testDB.ListUnprocessedDocuments(ctx, 100)
	if err != nil {
		t.Fatalf("list unprocessed: %v", err)
	}
	if len(unprocessed) < 5 {
		t.Fatalf("expected at least 5 unprocessed, got %d", len(unprocessed))
	}

	// Process all
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// None should be unprocessed now
	unprocessed, err = testDB.ListUnprocessedDocuments(ctx, 100)
	if err != nil {
		t.Fatalf("list unprocessed after: %v", err)
	}
	if len(unprocessed) != 0 {
		t.Errorf("expected 0 unprocessed after ProcessAllPending, got %d", len(unprocessed))
	}
}

// TestUpdate_ResetsProcessedFlag verifies that updating a document's content
// resets the processed flag, causing it to be reprocessed.
func TestUpdate_ResetsProcessedFlag(t *testing.T) {
	ctx := context.Background()
	vaultID, docSvc := setupProcessingTest(t, ctx)

	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/reset-test.md",
		Content: "# Original\n\nOriginal content.",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	// Process it
	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	// Verify it's processed
	processed, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if !processed.Processed {
		t.Fatal("expected processed=true after ProcessAllPending")
	}

	// Update with new content (re-creates via upsert)
	_, err = docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/reset-test.md",
		Content: "# Updated\n\nNew content after update.",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Should be unprocessed again
	updated, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("get doc after update: %v", err)
	}
	if updated.Processed {
		t.Error("expected processed=false after content update")
	}
}

// TestProcessDocument_Idempotent verifies that processing an already-processed
// document a second time doesn't create duplicate chunks or wiki-links.
func TestProcessDocument_Idempotent(t *testing.T) {
	ctx := context.Background()
	vaultID, docSvc := setupProcessingTest(t, ctx)

	longContent := "# Idempotent Test\n\n" + strings.Repeat("Content that will produce chunks. ", 200)
	longContent += "\n\nSee also [[Some Link]]."

	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/idempotent-test.md",
		Content: longContent,
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	// Process once
	if err := docSvc.ProcessAllPending(ctx); err != nil {
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

	// Re-fetch and process again manually (simulating a re-run)
	latest, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	if err := docSvc.ProcessDocument(ctx, latest); err != nil {
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
