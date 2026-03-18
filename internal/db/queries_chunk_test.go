package db

import (
	"context"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

// Tests for ClaimChunksForEmbedding, RescheduleChunkEmbedding, and embed_at were
// removed when embed_at was replaced by the pipeline_job table.

func TestCreateAndGetChunks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/chunk-test.md",
		Title:   "Chunk Test",
		Content: "long content",
		Labels:  []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	heading := "## Section 1"
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Chunk 1", Position: 0, SourceLoc: &heading, Labels: []string{"test"}, Embedding: dummyEmbedding()},
		{FileID: docID, Text: "Chunk 2", Position: 1, Labels: []string{"test"}, Embedding: dummyEmbedding()},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Position != 0 {
		t.Error("Chunks should be ordered by position")
	}
}

func TestCreateChunksWithDataHash(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/chunk-datahash-test.pdf",
		Title:   "DataHash Test",
		Content: "",
		Labels:  []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	hash := "abc123def456"
	sourceLoc := "Page 1"
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{
			FileID:    docID,
			Text:      "PDF page text",
			MimeType:  "image/png",
			Position:  1,
			SourceLoc: &sourceLoc,
			DataHash:  &hash,
			Labels:    []string{"test"},
		},
		{
			FileID:   docID,
			Text:     "Text-only chunk",
			MimeType: "text/plain",
			Position: 2,
			Labels:   []string{"test"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks, got %d", len(chunks))
	}

	// Chunk with DataHash should round-trip.
	if chunks[0].DataHash == nil {
		t.Fatal("Expected DataHash to be set on chunk 0")
	}
	if *chunks[0].DataHash != hash {
		t.Errorf("Expected DataHash %q, got %q", hash, *chunks[0].DataHash)
	}
	if chunks[0].MimeType != "image/png" {
		t.Errorf("Expected MimeType 'image/png', got %q", chunks[0].MimeType)
	}
	if !chunks[0].IsMultimodal() {
		t.Error("Expected chunk with DataHash+image/png to be multimodal")
	}

	// Chunk without DataHash.
	if chunks[1].DataHash != nil {
		t.Errorf("Expected DataHash to be nil on chunk 1, got %v", *chunks[1].DataHash)
	}
	if chunks[1].IsMultimodal() {
		t.Error("Expected text-only chunk to not be multimodal")
	}
}

func TestDeleteChunks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/chunk-delete-test.md",
		Title:   "Chunk Delete",
		Content: "content",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "To delete", Position: 0, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks setup failed: %v", err)
	}

	err = testDB.DeleteChunks(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after delete failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks after delete, got %d", len(chunks))
	}
}

func TestCascadeDeleteChunksOnFileDelete(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/cascade-test.md",
		Title:   "Cascade",
		Content: "content",
		Labels:  []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Cascade chunk", Position: 0, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks setup failed: %v", err)
	}

	err = testDB.DeleteFile(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after cascade failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("Expected chunks to be cascade-deleted, got %d", len(chunks))
	}
}

func TestUpdateChunkEmbedding(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/update-embed-test.md", Title: "Update Embed",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "To embed", Position: 0, Embedding: nil},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}
	chunkID := models.MustRecordIDString(chunks[0].ID)

	emb := dummyEmbedding()
	err = testDB.UpdateChunkEmbedding(ctx, chunkID, emb)
	if err != nil {
		t.Fatalf("UpdateChunkEmbedding failed: %v", err)
	}

	chunks, err = testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after update failed: %v", err)
	}
	if len(chunks[0].Embedding) != 384 {
		t.Errorf("Expected embedding of length 384, got %d", len(chunks[0].Embedding))
	}
}

func TestDeleteChunksByIDs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/delete-chunk-byids.md", Title: "Delete Chunks",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Keep", Position: 0, Embedding: dummyEmbedding()},
		{FileID: docID, Text: "Delete1", Position: 1, Embedding: dummyEmbedding()},
		{FileID: docID, Text: "Delete2", Position: 2, Embedding: dummyEmbedding()},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("Expected 3 chunks, got %d", len(chunks))
	}

	deleteIDs := []string{
		models.MustRecordIDString(chunks[1].ID),
		models.MustRecordIDString(chunks[2].ID),
	}
	err = testDB.DeleteChunksByIDs(ctx, deleteIDs)
	if err != nil {
		t.Fatalf("DeleteChunksByIDs failed: %v", err)
	}

	remaining, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after delete failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("Expected 1 chunk after delete, got %d", len(remaining))
	}
	if remaining[0].Text != "Keep" {
		t.Errorf("Wrong chunk remaining: got %q", remaining[0].Text)
	}
}

func TestBatchUpdateChunkPositions(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/batch-update-pos-test.md", Title: "Batch Update Pos",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "First", Position: 0, Embedding: dummyEmbedding()},
		{FileID: docID, Text: "Second", Position: 1, Embedding: dummyEmbedding()},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}

	err = testDB.BatchUpdateChunkPositions(ctx, []ChunkPositionUpdate{
		{ID: models.MustRecordIDString(chunks[0].ID), Position: 5},
		{ID: models.MustRecordIDString(chunks[1].ID), Position: 10},
	})
	if err != nil {
		t.Fatalf("BatchUpdateChunkPositions failed: %v", err)
	}

	updated, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after update failed: %v", err)
	}
	if updated[0].Position != 5 {
		t.Errorf("Expected position 5 for first chunk, got %d", updated[0].Position)
	}
	if updated[1].Position != 10 {
		t.Errorf("Expected position 10 for second chunk, got %d", updated[1].Position)
	}
}

func TestGetFirstChunksByFileIDs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc1, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/first-chunk-1.md", Title: "First Chunk 1",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile 1 failed: %v", err)
	}
	doc2, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/first-chunk-2.md", Title: "First Chunk 2",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile 2 failed: %v", err)
	}
	doc1ID := models.MustRecordIDString(doc1.ID)
	doc2ID := models.MustRecordIDString(doc2.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: doc1ID, Text: "Doc1 Chunk0", Position: 0, Embedding: dummyEmbedding()},
		{FileID: doc1ID, Text: "Doc1 Chunk1", Position: 1, Embedding: dummyEmbedding()},
		{FileID: doc2ID, Text: "Doc2 Chunk0", Position: 0, Embedding: dummyEmbedding()},
		{FileID: doc2ID, Text: "Doc2 Chunk1", Position: 1, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	result, err := testDB.GetFirstChunksByFileIDs(ctx, []string{doc1ID, doc2ID})
	if err != nil {
		t.Fatalf("GetFirstChunksByFileIDs failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 entries in map, got %d", len(result))
	}
	if ch, ok := result[doc1ID]; !ok || ch.Position != 0 {
		t.Errorf("Expected first chunk of doc1 with position 0, got %+v (ok=%v)", result[doc1ID], ok)
	}
	if ch, ok := result[doc2ID]; !ok || ch.Position != 0 {
		t.Errorf("Expected first chunk of doc2 with position 0, got %+v (ok=%v)", result[doc2ID], ok)
	}
}

func TestGetUnembeddedChunks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/unembedded-chunks.md", Title: "Unembedded Chunks",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Has embedding", Position: 0, Embedding: dummyEmbedding()},
		{FileID: docID, Text: "No embedding", Position: 1, Embedding: nil},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	unembedded, err := testDB.GetUnembeddedChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetUnembeddedChunks failed: %v", err)
	}
	if len(unembedded) != 1 {
		t.Fatalf("Expected 1 unembedded chunk, got %d", len(unembedded))
	}
	if unembedded[0].Text != "No embedding" {
		t.Errorf("Expected unembedded chunk text 'No embedding', got %q", unembedded[0].Text)
	}
}

func TestStripEmbeddingsByFolder(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	prefix := "/strip-embed-" + t.Name()

	// Create folders
	if err := testDB.EnsureFolderPath(ctx, vaultID, prefix+"/sub"); err != nil {
		t.Fatalf("EnsureFolderPath failed: %v", err)
	}

	// Create a file inside the folder and one outside
	inside, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: prefix + "/sub/doc.md", Title: "Inside",
		Content: "inside content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile inside failed: %v", err)
	}
	outside, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/outside-strip-test.md", Title: "Outside",
		Content: "outside content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile outside failed: %v", err)
	}
	insideID := models.MustRecordIDString(inside.ID)
	outsideID := models.MustRecordIDString(outside.ID)

	// Create chunks with embeddings for both files
	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: insideID, Text: "Inside chunk", Position: 0, Embedding: dummyEmbedding()},
		{FileID: outsideID, Text: "Outside chunk", Position: 0, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	// Strip embeddings for the folder
	stripped, err := testDB.StripEmbeddingsByFolder(ctx, vaultID, prefix)
	if err != nil {
		t.Fatalf("StripEmbeddingsByFolder failed: %v", err)
	}
	if stripped != 1 {
		t.Errorf("Expected 1 stripped chunk, got %d", stripped)
	}

	// Inside file's chunks should have no embedding
	insideChunks, err := testDB.GetChunks(ctx, insideID)
	if err != nil {
		t.Fatalf("GetChunks inside failed: %v", err)
	}
	if len(insideChunks) != 1 {
		t.Fatalf("Expected 1 inside chunk, got %d", len(insideChunks))
	}
	if len(insideChunks[0].Embedding) != 0 {
		t.Errorf("Expected embedding to be stripped from inside chunk, got length %d", len(insideChunks[0].Embedding))
	}

	// Outside file's chunks should still have embeddings
	outsideChunks, err := testDB.GetChunks(ctx, outsideID)
	if err != nil {
		t.Fatalf("GetChunks outside failed: %v", err)
	}
	if len(outsideChunks) != 1 {
		t.Fatalf("Expected 1 outside chunk, got %d", len(outsideChunks))
	}
	if len(outsideChunks[0].Embedding) != 384 {
		t.Errorf("Expected outside chunk embedding to be preserved (384), got %d", len(outsideChunks[0].Embedding))
	}
}

func TestBatchUpdateChunkEmbeddings(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/batch-embed-test.md", Title: "Batch Embed",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{FileID: docID, Text: "Chunk A", Position: 0, Embedding: nil},
		{FileID: docID, Text: "Chunk B", Position: 1, Embedding: nil},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks, got %d", len(chunks))
	}

	updates := []ChunkEmbeddingUpdate{
		{ID: models.MustRecordIDString(chunks[0].ID), Embedding: dummyEmbedding()},
		{ID: models.MustRecordIDString(chunks[1].ID), Embedding: dummyEmbedding()},
	}
	if err := testDB.BatchUpdateChunkEmbeddings(ctx, updates); err != nil {
		t.Fatalf("BatchUpdateChunkEmbeddings failed: %v", err)
	}

	updated, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after batch update failed: %v", err)
	}
	for _, ch := range updated {
		if len(ch.Embedding) != 384 {
			t.Errorf("Expected embedding length 384, got %d for chunk %v", len(ch.Embedding), ch.Text)
		}
	}
}
