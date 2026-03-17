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
