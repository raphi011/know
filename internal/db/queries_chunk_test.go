package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func TestCreateAndGetChunks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/chunk-test.md",
		Title:       "Chunk Test",
		Content:     "long content",
		ContentBody: "long content",
		Source:      models.SourceManual,
		Labels:      []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	heading := "## Section 1"
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Chunk 1", Position: 0, HeadingPath: &heading, Labels: []string{"test"}, Embedding: dummyEmbedding()},
		{DocumentID: docID, Content: "Chunk 2", Position: 1, Labels: []string{"test"}, Embedding: dummyEmbedding()},
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

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/chunk-delete-test.md",
		Title:       "Chunk Delete",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "To delete", Position: 0, Embedding: dummyEmbedding()},
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

func TestCascadeDeleteChunksOnDocumentDelete(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/cascade-test.md",
		Title:       "Cascade",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Cascade chunk", Position: 0, Embedding: dummyEmbedding()},
	}); err != nil {
		t.Fatalf("CreateChunks setup failed: %v", err)
	}

	err = testDB.DeleteDocument(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
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

func TestCreateChunks_WithEmbedAt(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/embed-at-test.md", Title: "Embed At Test",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	now := time.Now().UTC()
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Chunk with embed_at", Position: 0, Embedding: nil, EmbedAt: &now},
		{DocumentID: docID, Content: "Chunk without embed_at", Position: 1, Embedding: dummyEmbedding()},
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
	if chunks[0].EmbedAt == nil {
		t.Error("First chunk should have embed_at set")
	}
	if chunks[1].EmbedAt != nil {
		t.Error("Second chunk should not have embed_at set")
	}
}

func TestClaimChunksForEmbedding(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/claim-test.md", Title: "Claim Test",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	past := time.Now().UTC().Add(-time.Minute)
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Ready chunk 1", Position: 0, Embedding: nil, EmbedAt: &past},
		{DocumentID: docID, Content: "Ready chunk 2", Position: 1, Embedding: nil, EmbedAt: &past},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	claimed, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimChunksForEmbedding failed: %v", err)
	}

	var ourClaimed []models.Chunk
	for _, c := range claimed {
		cDocID := models.MustRecordIDString(c.Document)
		if cDocID == docID {
			ourClaimed = append(ourClaimed, c)
		}
	}
	if len(ourClaimed) != 2 {
		t.Fatalf("Expected 2 claimed chunks, got %d", len(ourClaimed))
	}

	for _, c := range ourClaimed {
		if c.EmbedAt == nil {
			t.Error("Claimed chunk BEFORE state should have embed_at set")
		}
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after claim failed: %v", err)
	}
	for _, c := range chunks {
		if c.EmbedAt != nil {
			t.Error("After claim, embed_at should be NONE in DB")
		}
	}

	claimed2, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("Second ClaimChunksForEmbedding failed: %v", err)
	}
	for _, c := range claimed2 {
		cDocID := models.MustRecordIDString(c.Document)
		if cDocID == docID {
			t.Error("Should not re-claim already-claimed chunks")
		}
	}
}

func TestClaimChunksForEmbedding_SkipsFutureEmbedAt(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/future-embed-test.md", Title: "Future Embed",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	future := time.Now().UTC().Add(time.Hour)
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Future chunk", Position: 0, Embedding: nil, EmbedAt: &future},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	claimed, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimChunksForEmbedding failed: %v", err)
	}

	for _, c := range claimed {
		cDocID := models.MustRecordIDString(c.Document)
		if cDocID == docID {
			t.Error("Should not claim chunks with future embed_at")
		}
	}
}

func TestUpdateChunkEmbedding(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/update-embed-test.md", Title: "Update Embed",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "To embed", Position: 0, Embedding: nil},
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

func TestRescheduleChunkEmbedding(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/reschedule-test.md", Title: "Reschedule",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Reschedule me", Position: 0, Embedding: nil},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	chunkID := models.MustRecordIDString(chunks[0].ID)

	if chunks[0].EmbedAt != nil {
		t.Error("embed_at should be nil initially")
	}

	err = testDB.RescheduleChunkEmbedding(ctx, chunkID)
	if err != nil {
		t.Fatalf("RescheduleChunkEmbedding failed: %v", err)
	}

	chunks, err = testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after reschedule failed: %v", err)
	}
	if chunks[0].EmbedAt == nil {
		t.Error("embed_at should be set after reschedule")
	}
}

func TestDeleteChunkByID(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/delete-chunk-byid.md", Title: "Delete Chunk",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Keep", Position: 0, Embedding: dummyEmbedding()},
		{DocumentID: docID, Content: "Delete", Position: 1, Embedding: dummyEmbedding()},
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

	deleteID := models.MustRecordIDString(chunks[1].ID)
	err = testDB.DeleteChunkByID(ctx, deleteID)
	if err != nil {
		t.Fatalf("DeleteChunkByID failed: %v", err)
	}

	remaining, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after delete failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("Expected 1 chunk after delete, got %d", len(remaining))
	}
	if remaining[0].Content != "Keep" {
		t.Errorf("Wrong chunk remaining: got %q", remaining[0].Content)
	}
}

func TestUpdateChunkPosition(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/update-pos-test.md", Title: "Update Pos",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Move me", Position: 0, Embedding: dummyEmbedding()},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	chunkID := models.MustRecordIDString(chunks[0].ID)

	err = testDB.UpdateChunkPosition(ctx, chunkID, 5)
	if err != nil {
		t.Fatalf("UpdateChunkPosition failed: %v", err)
	}

	updated, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after update failed: %v", err)
	}
	if updated[0].Position != 5 {
		t.Errorf("Expected position 5, got %d", updated[0].Position)
	}
}

func TestClaimChunksForEmbedding_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/limit-test.md", Title: "Limit Test",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	past := time.Now().UTC().Add(-time.Minute)
	for i := range 5 {
		err := testDB.CreateChunks(ctx, []models.ChunkInput{
			{DocumentID: docID, Content: fmt.Sprintf("Limit chunk %d", i), Position: i, Embedding: nil, EmbedAt: &past},
		})
		if err != nil {
			t.Fatalf("CreateChunks %d failed: %v", i, err)
		}
	}

	claimed, err := testDB.ClaimChunksForEmbedding(ctx, 3)
	if err != nil {
		t.Fatalf("ClaimChunksForEmbedding failed: %v", err)
	}

	var ours int
	for _, c := range claimed {
		if models.MustRecordIDString(c.Document) == docID {
			ours++
		}
	}
	if ours != 3 {
		t.Errorf("Expected exactly 3 claimed chunks (limit), got %d", ours)
	}

	claimed2, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("second ClaimChunksForEmbedding failed: %v", err)
	}
	var ours2 int
	for _, c := range claimed2 {
		if models.MustRecordIDString(c.Document) == docID {
			ours2++
		}
	}
	if ours2 != 2 {
		t.Errorf("Expected 2 remaining chunks, got %d", ours2)
	}
}

func TestRescheduleAndReclaimRoundTrip(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/roundtrip-test.md", Title: "Roundtrip",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	past := time.Now().UTC().Add(-time.Minute)
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Roundtrip chunk", Position: 0, Embedding: nil, EmbedAt: &past},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	claimed, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimChunksForEmbedding failed: %v", err)
	}
	var claimedChunkID string
	for _, c := range claimed {
		if models.MustRecordIDString(c.Document) == docID {
			claimedChunkID = models.MustRecordIDString(c.ID)
		}
	}
	if claimedChunkID == "" {
		t.Fatal("Expected to claim our chunk")
	}

	claimed2, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	for _, c := range claimed2 {
		if models.MustRecordIDString(c.ID) == claimedChunkID {
			t.Fatal("Chunk should not be claimable after first claim")
		}
	}

	err = testDB.RescheduleChunkEmbedding(ctx, claimedChunkID)
	if err != nil {
		t.Fatalf("RescheduleChunkEmbedding failed: %v", err)
	}

	claimed3, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("third claim failed: %v", err)
	}
	for _, c := range claimed3 {
		if models.MustRecordIDString(c.ID) == claimedChunkID {
			t.Fatal("Rescheduled chunk should not be immediately claimable (30s backoff)")
		}
	}

	pastAgain := time.Now().UTC().Add(-time.Minute)
	sql := `UPDATE type::record("chunk", $id) SET embed_at = $embed_at`
	if _, err := surrealdb.Query[any](ctx, testDB.DB(), sql, map[string]any{
		"id":       claimedChunkID,
		"embed_at": pastAgain,
	}); err != nil {
		t.Fatalf("manual embed_at update failed: %v", err)
	}

	claimed4, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("fourth claim failed: %v", err)
	}
	var reclaimed bool
	for _, c := range claimed4 {
		if models.MustRecordIDString(c.ID) == claimedChunkID {
			reclaimed = true
		}
	}
	if !reclaimed {
		t.Fatal("Rescheduled chunk should be claimable after backoff elapsed")
	}
}
