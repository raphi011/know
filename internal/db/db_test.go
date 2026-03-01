package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *Client
var testContainer testcontainers.Container

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
		log.Fatalf("Failed to start SurrealDB container: %v", err)
	}

	host, err := testContainer.Host(ctx)
	if err != nil {
		log.Fatalf("Failed to get container host: %v", err)
	}
	// Colima VM IPs are not reachable; ports are forwarded to localhost
	if host == "" || host == "null" || host == "192.168.64.2" {
		host = "localhost"
	}
	mappedPort, err := testContainer.MappedPort(ctx, "8000")
	if err != nil {
		log.Fatalf("Failed to get mapped port: %v", err)
	}

	testDB, err = NewClient(ctx, Config{
		URL:       fmt.Sprintf("ws://%s:%s/rpc", host, mappedPort.Port()),
		Namespace: "test_v2",
		Database:  "test_v2",
		Username:  "root",
		Password:  "root",
		AuthLevel: "root",
	}, nil, nil)
	if err != nil {
		log.Fatalf("Failed to connect to test database: %v", err)
	}

	if err := testDB.InitSchema(ctx, 384); err != nil {
		log.Fatalf("Failed to initialize v2 schema: %v", err)
	}

	code := m.Run()

	// Cleanup: best-effort, process is exiting
	if err := testDB.Close(ctx); err != nil {
		log.Printf("warning: failed to close test DB: %v", err)
	}
	if err := testContainer.Terminate(ctx); err != nil {
		log.Printf("warning: failed to terminate container: %v", err)
	}

	os.Exit(code)
}

func dummyEmbedding() []float32 {
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = float32(i) / 384.0
	}
	return embedding
}

func createTestUser(t *testing.T, ctx context.Context) *models.User {
	t.Helper()
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "testuser-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return user
}

func createTestVault(t *testing.T, ctx context.Context, userID string) *models.Vault {
	t.Helper()
	vault, err := testDB.CreateVault(ctx, userID, models.VaultInput{
		Name: "test-vault-" + fmt.Sprint(time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("Failed to create test vault: %v", err)
	}
	return vault
}

// =============================================================================
// VAULT TESTS
// =============================================================================

func TestCreateVault(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	desc := "A test vault"
	vault, err := testDB.CreateVault(ctx, userID, models.VaultInput{
		Name:        "My Vault",
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("CreateVault failed: %v", err)
	}
	if vault.Name != "My Vault" {
		t.Errorf("Expected name 'My Vault', got %q", vault.Name)
	}
	if vault.Description == nil || *vault.Description != "A test vault" {
		t.Errorf("Expected description 'A test vault', got %v", vault.Description)
	}
}

func TestGetVault(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	retrieved, err := testDB.GetVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVault failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetVault returned nil")
	}
	if retrieved.Name != vault.Name {
		t.Errorf("Expected name %q, got %q", vault.Name, retrieved.Name)
	}

	nonExistent, err := testDB.GetVault(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetVault non-existent should not error: %v", err)
	}
	if nonExistent != nil {
		t.Error("GetVault non-existent should return nil")
	}
}

func TestListVaults(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	_ = createTestVault(t, ctx, userID)

	vaults, err := testDB.ListVaults(ctx)
	if err != nil {
		t.Fatalf("ListVaults failed: %v", err)
	}
	if len(vaults) == 0 {
		t.Error("ListVaults should return at least one vault")
	}
}

func TestDeleteVault(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create a document in the vault to test cascade
	hash := "testhash"
	_, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/test.md",
		Title:       "Test",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		ContentHash: &hash,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	err = testDB.DeleteVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("DeleteVault failed: %v", err)
	}

	deleted, err := testDB.GetVault(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetVault after delete failed: %v", err)
	}
	if deleted != nil {
		t.Error("Vault should be nil after delete")
	}
}

// =============================================================================
// DOCUMENT TESTS
// =============================================================================

func TestCreateDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	hash := "abc123"
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/docs/hello.md",
		Title:       "Hello World",
		Content:     "---\ntitle: Hello\n---\nHello world content",
		ContentBody: "Hello world content",
		Source:      models.SourceManual,
		Labels:      []string{"test", "greeting"},
		ContentHash: &hash,
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	if doc.Title != "Hello World" {
		t.Errorf("Expected title 'Hello World', got %q", doc.Title)
	}
	if doc.Path != "/docs/hello.md" {
		t.Errorf("Expected path '/docs/hello.md', got %q", doc.Path)
	}
	if len(doc.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(doc.Labels))
	}
}

func TestGetDocumentByPath(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	_, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/unique-path.md",
		Title:       "Unique Path Doc",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	doc, err := testDB.GetDocumentByPath(ctx, vaultID, "/unique-path.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath failed: %v", err)
	}
	if doc == nil {
		t.Fatal("GetDocumentByPath returned nil")
	}
	if doc.Title != "Unique Path Doc" {
		t.Errorf("Expected title 'Unique Path Doc', got %q", doc.Title)
	}

	notFound, err := testDB.GetDocumentByPath(ctx, vaultID, "/nonexistent.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent path")
	}
}

func TestListDocuments(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	for _, path := range []string{"/a/doc1.md", "/a/doc2.md", "/b/doc3.md"} {
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        path,
			Title:       "Doc " + path,
			Content:     "content of " + path,
			ContentBody: "content of " + path,
			Source:      models.SourceManual,
			Labels:      []string{"test"},
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", path, err)
		}
	}

	// List all
	docs, err := testDB.ListDocuments(ctx, ListDocumentsFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("Expected 3 docs, got %d", len(docs))
	}

	// List by folder
	folder := "/a/"
	docs, err = testDB.ListDocuments(ctx, ListDocumentsFilter{VaultID: vaultID, Folder: &folder})
	if err != nil {
		t.Fatalf("ListDocuments with folder failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("Expected 2 docs in /a/, got %d", len(docs))
	}
}

func TestUpdateDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/update-test.md",
		Title:       "Original",
		Content:     "original",
		ContentBody: "original",
		Source:      models.SourceManual,
		Labels:      []string{"old"},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	updated, err := testDB.UpdateDocument(ctx, docID, "new content", "new content", "Updated Title", []string{"new"}, nil, nil)
	if err != nil {
		t.Fatalf("UpdateDocument failed: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got %q", updated.Title)
	}
	if updated.ContentBody != "new content" {
		t.Errorf("Expected content_body 'new content', got %q", updated.ContentBody)
	}
}

func TestDeleteDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/delete-test.md",
		Title:       "Delete Me",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.DeleteDocument(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	gone, err := testDB.GetDocumentByID(ctx, docID)
	if err != nil {
		t.Fatalf("GetDocumentByID after delete error: %v", err)
	}
	if gone != nil {
		t.Error("Document should be nil after delete")
	}
}

func TestMoveDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/old-path.md",
		Title:       "Move Test",
		Content:     "content",
		ContentBody: "content",
		Source:      models.SourceManual,
		Labels:      []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	moved, err := testDB.MoveDocument(ctx, docID, "/new-path.md")
	if err != nil {
		t.Fatalf("MoveDocument failed: %v", err)
	}
	if moved.Path != "/new-path.md" {
		t.Errorf("Expected path '/new-path.md', got %q", moved.Path)
	}
}

// =============================================================================
// CHUNK TESTS
// =============================================================================

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

	// Delete the document — should cascade to chunks
	err = testDB.DeleteDocument(ctx, docID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	// SurrealDB cascade events are async, give it a moment
	time.Sleep(100 * time.Millisecond)

	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after cascade failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("Expected chunks to be cascade-deleted, got %d", len(chunks))
	}
}

// =============================================================================
// CHUNK EMBEDDING TESTS
// =============================================================================

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

	// Create chunks with embed_at in the past (ready to claim)
	past := time.Now().UTC().Add(-time.Minute)
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Ready chunk 1", Position: 0, Embedding: nil, EmbedAt: &past},
		{DocumentID: docID, Content: "Ready chunk 2", Position: 1, Embedding: nil, EmbedAt: &past},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	// Claim the chunks
	claimed, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimChunksForEmbedding failed: %v", err)
	}

	// Filter to chunks from our document (other tests may create chunks too)
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

	// Claimed chunks should have embed_at set (RETURN BEFORE)
	for _, c := range ourClaimed {
		if c.EmbedAt == nil {
			t.Error("Claimed chunk BEFORE state should have embed_at set")
		}
	}

	// After claim, embed_at should be cleared in the DB
	chunks, err := testDB.GetChunks(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunks after claim failed: %v", err)
	}
	for _, c := range chunks {
		if c.EmbedAt != nil {
			t.Error("After claim, embed_at should be NONE in DB")
		}
	}

	// Claim again — should get nothing (already claimed)
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

	// Create chunk with embed_at far in the future
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

	// Create chunk without embedding
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

	// Update with embedding
	emb := dummyEmbedding()
	err = testDB.UpdateChunkEmbedding(ctx, chunkID, emb)
	if err != nil {
		t.Fatalf("UpdateChunkEmbedding failed: %v", err)
	}

	// Verify
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

	// Create chunk without embed_at
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

	// Verify embed_at is nil initially
	if chunks[0].EmbedAt != nil {
		t.Error("embed_at should be nil initially")
	}

	// Reschedule
	err = testDB.RescheduleChunkEmbedding(ctx, chunkID)
	if err != nil {
		t.Fatalf("RescheduleChunkEmbedding failed: %v", err)
	}

	// Verify embed_at is now set
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

	// Delete the second chunk
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

// =============================================================================
// WIKI-LINK TESTS
// =============================================================================

func TestCreateWikiLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/link-source.md", Title: "Link Source",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	err = testDB.CreateWikiLinks(ctx, docID, vaultID, []WikiLinkInput{
		{RawTarget: "Some Target"},
		{RawTarget: "Another Target"},
	})
	if err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	links, err := testDB.GetWikiLinks(ctx, docID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("Expected 2 wiki links, got %d", len(links))
	}
}

func TestGetBacklinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	docA, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/backlink-a.md", Title: "Doc A",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument docA failed: %v", err)
	}
	docB, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/backlink-b.md", Title: "Doc B",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument docB failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	// A links to B
	if err := testDB.CreateWikiLinks(ctx, docAID, vaultID, []WikiLinkInput{
		{RawTarget: "Doc B", ToDocID: &docBID},
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	backlinks, err := testDB.GetBacklinks(ctx, docBID)
	if err != nil {
		t.Fatalf("GetBacklinks failed: %v", err)
	}
	if len(backlinks) != 1 {
		t.Errorf("Expected 1 backlink, got %d", len(backlinks))
	}
}

func TestResolveDanglingLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create doc with dangling link to "Future Doc"
	docA, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/dangling-source.md", Title: "Source",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument docA failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)

	if err := testDB.CreateWikiLinks(ctx, docAID, vaultID, []WikiLinkInput{
		{RawTarget: "Future Doc"}, // No ToDocID — dangling
	}); err != nil {
		t.Fatalf("CreateWikiLinks failed: %v", err)
	}

	// Verify it's dangling
	links, err := testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("GetWikiLinks failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0].ToDoc != nil {
		t.Fatal("Link should be dangling (to_doc nil)")
	}

	// Create the target document
	docB, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/future-doc.md", Title: "Future Doc",
		Content: "arrived", ContentBody: "arrived", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument docB failed: %v", err)
	}
	docBID := models.MustRecordIDString(docB.ID)

	// Resolve dangling links
	resolved, err := testDB.ResolveDanglingLinks(ctx, vaultID, "Future Doc", docBID)
	if err != nil {
		t.Fatalf("ResolveDanglingLinks failed: %v", err)
	}
	if resolved != 1 {
		t.Errorf("Expected 1 resolved link, got %d", resolved)
	}

	// Verify it's resolved
	links, err = testDB.GetWikiLinks(ctx, docAID)
	if err != nil {
		t.Fatalf("GetWikiLinks after resolve failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0].ToDoc == nil {
		t.Error("Link should now be resolved")
	}
}

// =============================================================================
// SEARCH TESTS
// =============================================================================

func TestBM25Search(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	if _, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/search-go.md", Title: "Go Programming",
		Content: "---\ntitle: Go\n---\nGo is a statically typed language", ContentBody: "Go is a statically typed language",
		Source: models.SourceManual, Labels: []string{"programming"},
	}); err != nil {
		t.Fatalf("CreateDocument go failed: %v", err)
	}
	if _, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/search-python.md", Title: "Python Programming",
		Content: "---\ntitle: Python\n---\nPython is a dynamic language", ContentBody: "Python is a dynamic language",
		Source: models.SourceManual, Labels: []string{"programming"},
	}); err != nil {
		t.Fatalf("CreateDocument python failed: %v", err)
	}

	results, err := testDB.BM25Search(ctx, "Go statically typed", SearchFilter{
		VaultID: vaultID,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("BM25Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("BM25Search should return results for 'Go statically typed'")
	}
}

func TestSearchWithLabelFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	if _, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/label-a.md", Title: "Web Doc",
		Content: "Web frameworks are great", ContentBody: "Web frameworks are great",
		Source: models.SourceManual, Labels: []string{"web"},
	}); err != nil {
		t.Fatalf("CreateDocument web failed: %v", err)
	}
	if _, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/label-b.md", Title: "CLI Doc",
		Content: "CLI tools are useful frameworks", ContentBody: "CLI tools are useful frameworks",
		Source: models.SourceManual, Labels: []string{"cli"},
	}); err != nil {
		t.Fatalf("CreateDocument cli failed: %v", err)
	}

	results, err := testDB.BM25Search(ctx, "frameworks", SearchFilter{
		VaultID: vaultID,
		Labels:  []string{"web"},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("BM25Search with labels failed: %v", err)
	}
	for _, r := range results {
		foundLabel := false
		for _, l := range r.Document.Labels {
			if l == "web" {
				foundLabel = true
			}
		}
		if !foundLabel {
			t.Errorf("Result %q should have 'web' label", r.Document.Title)
		}
	}
}

func TestSearchWithFolderFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	if _, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/guides/setup.md", Title: "Setup Guide",
		Content: "Install the software first", ContentBody: "Install the software first",
		Source: models.SourceManual, Labels: []string{},
	}); err != nil {
		t.Fatalf("CreateDocument guides failed: %v", err)
	}
	if _, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/notes/install.md", Title: "Install Notes",
		Content: "Notes about installing software", ContentBody: "Notes about installing software",
		Source: models.SourceManual, Labels: []string{},
	}); err != nil {
		t.Fatalf("CreateDocument notes failed: %v", err)
	}

	folder := "/guides/"
	results, err := testDB.BM25Search(ctx, "software", SearchFilter{
		VaultID: vaultID,
		Folder:  &folder,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("BM25Search with folder failed: %v", err)
	}
	for _, r := range results {
		if r.Document.Path[:8] != "/guides/" {
			t.Errorf("Result %q path %q should start with /guides/", r.Document.Title, r.Document.Path)
		}
	}
}

// =============================================================================
// TOKEN TESTS
// =============================================================================

func TestCreateAndLookupToken(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	token, err := testDB.CreateToken(ctx, userID, "hash123unique"+fmt.Sprint(time.Now().UnixNano()), "test-token", nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if token.Name != "test-token" {
		t.Errorf("Expected name 'test-token', got %q", token.Name)
	}

	found, err := testDB.GetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetTokenByHash failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetTokenByHash returned nil")
	}

	notFound, err := testDB.GetTokenByHash(ctx, "nonexistenthash")
	if err != nil {
		t.Fatalf("GetTokenByHash nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent hash")
	}
}

func TestTokenLastUsed(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	token, err := testDB.CreateToken(ctx, userID, "lastusedhash"+fmt.Sprint(time.Now().UnixNano()), "last-used-token", nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	tokenID := models.MustRecordIDString(token.ID)

	if token.LastUsed != nil {
		t.Error("LastUsed should be nil initially")
	}

	err = testDB.UpdateTokenLastUsed(ctx, tokenID)
	if err != nil {
		t.Fatalf("UpdateTokenLastUsed failed: %v", err)
	}

	updated, err := testDB.GetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetTokenByHash after update failed: %v", err)
	}
	if updated.LastUsed == nil {
		t.Error("LastUsed should be set after update")
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

	// Create 5 chunks ready for embedding
	past := time.Now().UTC().Add(-time.Minute)
	for i := range 5 {
		err := testDB.CreateChunks(ctx, []models.ChunkInput{
			{DocumentID: docID, Content: fmt.Sprintf("Limit chunk %d", i), Position: i, Embedding: nil, EmbedAt: &past},
		})
		if err != nil {
			t.Fatalf("CreateChunks %d failed: %v", i, err)
		}
	}

	// Claim with limit 3
	claimed, err := testDB.ClaimChunksForEmbedding(ctx, 3)
	if err != nil {
		t.Fatalf("ClaimChunksForEmbedding failed: %v", err)
	}

	// Filter to our doc's chunks
	var ours int
	for _, c := range claimed {
		if models.MustRecordIDString(c.Document) == docID {
			ours++
		}
	}
	if ours != 3 {
		t.Errorf("Expected exactly 3 claimed chunks (limit), got %d", ours)
	}

	// Claim again with limit 10 — should get the remaining 2
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

	// Create chunk with embed_at in the past
	past := time.Now().UTC().Add(-time.Minute)
	err = testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "Roundtrip chunk", Position: 0, Embedding: nil, EmbedAt: &past},
	})
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	// Claim it (clears embed_at)
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

	// Verify it's no longer claimable
	claimed2, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	for _, c := range claimed2 {
		if models.MustRecordIDString(c.ID) == claimedChunkID {
			t.Fatal("Chunk should not be claimable after first claim")
		}
	}

	// Reschedule it (simulating a failure recovery)
	err = testDB.RescheduleChunkEmbedding(ctx, claimedChunkID)
	if err != nil {
		t.Fatalf("RescheduleChunkEmbedding failed: %v", err)
	}

	// The rescheduled chunk has a 30s backoff, so it should NOT be claimable yet
	claimed3, err := testDB.ClaimChunksForEmbedding(ctx, 10)
	if err != nil {
		t.Fatalf("third claim failed: %v", err)
	}
	for _, c := range claimed3 {
		if models.MustRecordIDString(c.ID) == claimedChunkID {
			t.Fatal("Rescheduled chunk should not be immediately claimable (30s backoff)")
		}
	}

	// Manually set embed_at to the past to simulate backoff elapsed
	pastAgain := time.Now().UTC().Add(-time.Minute)
	sql := `UPDATE type::record("chunk", $id) SET embed_at = $embed_at`
	if _, err := surrealdb.Query[any](ctx, testDB.DB(), sql, map[string]any{
		"id":       claimedChunkID,
		"embed_at": pastAgain,
	}); err != nil {
		t.Fatalf("manual embed_at update failed: %v", err)
	}

	// Now it should be claimable again
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

func TestListDocuments_LabelFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	// Create documents with different labels and paths
	for _, doc := range []struct {
		path   string
		title  string
		labels []string
	}{
		{"/projects/go-app.md", "Go App", []string{"go", "project"}},
		{"/projects/rust-app.md", "Rust App", []string{"rust", "project"}},
		{"/notes/setup.md", "Setup Guide", []string{"guide"}},
	} {
		_, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        doc.path,
			Title:       doc.title,
			Content:     "# " + doc.title,
			ContentBody: doc.title,
			Labels:      doc.labels,
			Source:      models.SourceManual,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", doc.path, err)
		}
	}

	// Query by label
	docs, err := testDB.ListDocuments(ctx, ListDocumentsFilter{
		VaultID: vaultID,
		Labels:  []string{"go"},
	})
	if err != nil {
		t.Fatalf("list by label: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with label 'go', got %d", len(docs))
	}

	// Query by folder + label
	folder := "/projects/"
	docs, err = testDB.ListDocuments(ctx, ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  &folder,
		Labels:  []string{"project"},
	})
	if err != nil {
		t.Fatalf("list by folder+label: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs in /projects/ with label 'project', got %d", len(docs))
	}
}
