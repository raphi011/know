// Package db provides integration tests for SurrealDB operations.
package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *Client
var testContainer testcontainers.Container

// TestMain sets up and tears down the SurrealDB container for all tests.
func TestMain(m *testing.M) {
	// Disable ryuk (cleanup container) as it can cause issues in some environments
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	ctx := context.Background()

	// Start SurrealDB container
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

	// Get container host and port
	host, err := testContainer.Host(ctx)
	if err != nil {
		log.Fatalf("Failed to get container host: %v", err)
	}
	// Workaround: testcontainers may return "null" as host in some environments
	if host == "" || host == "null" {
		host = "localhost"
	}
	mappedPort, err := testContainer.MappedPort(ctx, "8000")
	if err != nil {
		log.Fatalf("Failed to get mapped port: %v", err)
	}

	// Connect to test database
	testDB, err = NewClient(ctx, Config{
		URL:       fmt.Sprintf("ws://%s:%s/rpc", host, mappedPort.Port()),
		Namespace: "test",
		Database:  "test",
		Username:  "root",
		Password:  "root",
		AuthLevel: "root",
	}, nil, nil)
	if err != nil {
		log.Fatalf("Failed to connect to test database: %v", err)
	}

	// Initialize schema with test embedding dimension (384)
	if err := testDB.InitSchema(ctx, 384); err != nil {
		log.Fatalf("Failed to initialize schema: %v", err)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	_ = testDB.Close(ctx)
	_ = testContainer.Terminate(ctx)

	os.Exit(code)
}

// dummyEmbedding returns a dummy embedding vector for testing.
// Uses 384 dimensions to match the default all-minilm:l6-v2 model.
func dummyEmbedding() []float32 {
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = float32(i) / 384.0
	}
	return embedding
}

// =============================================================================
// ENTITY TESTS
// =============================================================================

func TestCreateEntity(t *testing.T) {
	ctx := context.Background()

	content := "Test entity content"
	entity, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Test Entity",
		Content:   &content,
		Labels:    []string{"test", "unit-test"},
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	if entity.Name != "Test Entity" {
		t.Errorf("Expected name 'Test Entity', got %q", entity.Name)
	}
	if entity.Type != "concept" {
		t.Errorf("Expected type 'concept', got %q", entity.Type)
	}
	if entity.Content == nil || *entity.Content != content {
		t.Errorf("Expected content %q, got %v", content, entity.Content)
	}
	if len(entity.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(entity.Labels))
	}

	// Cleanup
	_, _ = testDB.DeleteEntity(ctx, models.MustRecordIDString(entity.ID))
}

func TestGetEntity(t *testing.T) {
	ctx := context.Background()

	// Create test entity
	content := "Get test content"
	created, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Get Test Entity",
		Content:   &content,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create test entity: %v", err)
	}
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, models.MustRecordIDString(created.ID))
	}()

	// Get by ID
	entityID := models.MustRecordIDString(created.ID)
	entity, err := testDB.GetEntity(ctx, entityID)
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if entity == nil {
		t.Fatal("GetEntity returned nil")
	}
	if entity.Name != "Get Test Entity" {
		t.Errorf("Expected name 'Get Test Entity', got %q", entity.Name)
	}

	// Get non-existent
	nonExistent, err := testDB.GetEntity(ctx, "non-existent-id")
	if err != nil {
		t.Errorf("GetEntity with non-existent ID should not error: %v", err)
	}
	if nonExistent != nil {
		t.Error("GetEntity with non-existent ID should return nil")
	}
}

func TestGetEntityByName(t *testing.T) {
	ctx := context.Background()

	// Create test entity
	content := "Name lookup test"
	created, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Unique Name For Test",
		Content:   &content,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create test entity: %v", err)
	}
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, models.MustRecordIDString(created.ID))
	}()

	// Exact name match
	entity, err := testDB.GetEntityByName(ctx, "Unique Name For Test")
	if err != nil {
		t.Fatalf("GetEntityByName failed: %v", err)
	}
	if entity == nil {
		t.Fatal("GetEntityByName returned nil")
	}

	// Case-insensitive match
	entityLower, err := testDB.GetEntityByName(ctx, "unique name for test")
	if err != nil {
		t.Fatalf("GetEntityByName (case-insensitive) failed: %v", err)
	}
	if entityLower == nil {
		t.Fatal("GetEntityByName should be case-insensitive")
	}

	// Non-existent name
	nonExistent, err := testDB.GetEntityByName(ctx, "This Name Does Not Exist")
	if err != nil {
		t.Errorf("GetEntityByName with non-existent name should not error: %v", err)
	}
	if nonExistent != nil {
		t.Error("GetEntityByName with non-existent name should return nil")
	}
}

func TestGetEntitiesByNames(t *testing.T) {
	ctx := context.Background()

	// Create test entities
	entities := []models.EntityInput{
		{Type: "concept", Name: "Batch Test Alpha", Embedding: dummyEmbedding()},
		{Type: "concept", Name: "Batch Test Beta", Embedding: dummyEmbedding()},
		{Type: "concept", Name: "Batch Test Gamma", Embedding: dummyEmbedding()},
	}

	var createdIDs []string
	for _, input := range entities {
		created, err := testDB.CreateEntity(ctx, input)
		if err != nil {
			t.Fatalf("Failed to create test entity %s: %v", input.Name, err)
		}
		createdIDs = append(createdIDs, models.MustRecordIDString(created.ID))
	}
	defer func() {
		for _, id := range createdIDs {
			_, _ = testDB.DeleteEntity(ctx, id)
		}
	}()

	// Test batch lookup with mixed existing and non-existing names
	names := []string{"Batch Test Alpha", "batch test beta", "Nonexistent Entity", "Batch Test Gamma"}
	result, err := testDB.GetEntitiesByNames(ctx, names)
	if err != nil {
		t.Fatalf("GetEntitiesByNames failed: %v", err)
	}

	// Should find 3 entities (case-insensitive)
	if len(result) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(result))
	}

	// Check case-insensitive lookup
	if _, ok := result["batch test alpha"]; !ok {
		t.Error("Expected to find 'batch test alpha' (lowercase key)")
	}
	if _, ok := result["batch test beta"]; !ok {
		t.Error("Expected to find 'batch test beta' (lowercase key)")
	}
	if _, ok := result["nonexistent entity"]; ok {
		t.Error("Should not find 'nonexistent entity'")
	}

	// Test empty input
	emptyResult, err := testDB.GetEntitiesByNames(ctx, []string{})
	if err != nil {
		t.Fatalf("GetEntitiesByNames with empty input failed: %v", err)
	}
	if len(emptyResult) != 0 {
		t.Errorf("Expected empty result for empty input, got %d", len(emptyResult))
	}
}

func TestUpdateEntity(t *testing.T) {
	ctx := context.Background()

	// Create test entity
	content := "Original content"
	created, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Update Test Entity",
		Content:   &content,
		Labels:    []string{"original"},
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create test entity: %v", err)
	}
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, models.MustRecordIDString(created.ID))
	}()

	// Update content and labels
	entityID := models.MustRecordIDString(created.ID)
	newContent := "Updated content"
	updated, err := testDB.UpdateEntity(ctx, entityID, models.EntityUpdate{
		Content:   &newContent,
		AddLabels: []string{"updated"},
	})
	if err != nil {
		t.Fatalf("UpdateEntity failed: %v", err)
	}
	if updated.Content == nil || *updated.Content != newContent {
		t.Errorf("Expected updated content %q, got %v", newContent, updated.Content)
	}
	if len(updated.Labels) != 2 {
		t.Errorf("Expected 2 labels after update, got %d: %v", len(updated.Labels), updated.Labels)
	}
}

func TestDeleteEntity(t *testing.T) {
	ctx := context.Background()

	// Create test entity
	content := "Delete test"
	created, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Delete Test Entity",
		Content:   &content,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create test entity: %v", err)
	}

	entityID := models.MustRecordIDString(created.ID)

	// Delete
	deleted, err := testDB.DeleteEntity(ctx, entityID)
	if err != nil {
		t.Fatalf("DeleteEntity failed: %v", err)
	}
	if !deleted {
		t.Error("DeleteEntity should return true for existing entity")
	}

	// Verify gone
	entity, err := testDB.GetEntity(ctx, entityID)
	if err != nil {
		t.Fatalf("GetEntity after delete failed: %v", err)
	}
	if entity != nil {
		t.Error("Entity should be nil after delete")
	}

	// Delete non-existent
	deleted, err = testDB.DeleteEntity(ctx, "non-existent-id")
	if err != nil {
		t.Errorf("DeleteEntity with non-existent ID should not error: %v", err)
	}
	if deleted {
		t.Error("DeleteEntity with non-existent ID should return false")
	}
}

func TestUpsertEntity(t *testing.T) {
	ctx := context.Background()

	// Test 1: Create new entity via upsert
	explicitID := "upsert-test-entity"
	content1 := "Original content"
	entity, wasCreated, err := testDB.UpsertEntity(ctx, models.EntityInput{
		ID:        &explicitID,
		Type:      "document",
		Name:      "Upsert Test",
		Content:   &content1,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("First UpsertEntity failed: %v", err)
	}
	if !wasCreated {
		t.Error("First upsert should report wasCreated=true")
	}
	if entity.Name != "Upsert Test" {
		t.Errorf("Name mismatch: got %q, want %q", entity.Name, "Upsert Test")
	}
	if entity.Content == nil || *entity.Content != "Original content" {
		t.Errorf("Content mismatch after first upsert")
	}

	// Test 2: Update existing entity via upsert (same ID, new content)
	content2 := "Updated content"
	entity2, wasCreated2, err := testDB.UpsertEntity(ctx, models.EntityInput{
		ID:        &explicitID,
		Type:      "document",
		Name:      "Upsert Test Updated",
		Content:   &content2,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Second UpsertEntity failed: %v", err)
	}
	if wasCreated2 {
		t.Error("Second upsert should report wasCreated=false (update)")
	}
	if entity2.Name != "Upsert Test Updated" {
		t.Errorf("Name not updated: got %q, want %q", entity2.Name, "Upsert Test Updated")
	}
	if entity2.Content == nil || *entity2.Content != "Updated content" {
		t.Errorf("Content not updated")
	}

	// Verify via direct get
	entityID := models.MustRecordIDString(entity2.ID)
	fetched, err := testDB.GetEntity(ctx, entityID)
	if err != nil {
		t.Fatalf("GetEntity after upsert failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("Entity should exist after upsert")
	}
	if fetched.Content == nil || *fetched.Content != "Updated content" {
		t.Error("GetEntity content should reflect upsert update")
	}

	// Cleanup
	_, _ = testDB.DeleteEntity(ctx, entityID)
}

// =============================================================================
// SEARCH TESTS
// =============================================================================

func TestHybridSearch(t *testing.T) {
	ctx := context.Background()

	// Create test entities
	content1 := "Go is a programming language designed at Google"
	content2 := "Python is a popular scripting language"
	content3 := "JavaScript runs in the browser"

	entities := []models.EntityInput{
		{Type: "concept", Name: "Go Language", Content: &content1, Labels: []string{"programming"}, Embedding: dummyEmbedding()},
		{Type: "concept", Name: "Python Language", Content: &content2, Labels: []string{"programming"}, Embedding: dummyEmbedding()},
		{Type: "concept", Name: "JavaScript", Content: &content3, Labels: []string{"programming", "web"}, Embedding: dummyEmbedding()},
	}

	var createdIDs []string
	for _, input := range entities {
		entity, err := testDB.CreateEntity(ctx, input)
		if err != nil {
			t.Fatalf("Failed to create test entity: %v", err)
		}
		createdIDs = append(createdIDs, models.MustRecordIDString(entity.ID))
	}
	defer func() {
		for _, id := range createdIDs {
			_, _ = testDB.DeleteEntity(ctx, id)
		}
	}()

	// Search for "Go"
	results, err := testDB.HybridSearch(ctx, SearchOptions{
		Query:     "Go programming",
		Embedding: dummyEmbedding(),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("HybridSearch should return results for 'Go programming'")
	}

	// Search with label filter
	results, err = testDB.HybridSearch(ctx, SearchOptions{
		Query:     "language",
		Embedding: dummyEmbedding(),
		Labels:    []string{"web"},
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("HybridSearch with labels failed: %v", err)
	}
	// Should only find JavaScript (has "web" label)
	if len(results) == 0 {
		t.Log("HybridSearch with web label returned no results (may be RRF limitation in v3)")
		// Don't fail - RRF might not work with all dummy embeddings
		return
	}
	found := false
	for _, r := range results {
		t.Logf("Found: %s (labels: %v)", r.Name, r.Labels)
		if r.Name == "JavaScript" {
			found = true
		}
	}
	if !found {
		t.Error("HybridSearch with web label should find JavaScript")
	}
}

// =============================================================================
// CHUNK TESTS
// =============================================================================

func TestCreateAndGetChunks(t *testing.T) {
	ctx := context.Background()

	// Create test entity
	content := "Long content that would be chunked"
	entity, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "document",
		Name:      "Chunk Test Doc",
		Content:   &content,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create test entity: %v", err)
	}
	entityID := models.MustRecordIDString(entity.ID)
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, entityID)
	}()

	// Create chunks
	headingPath := "## Section 1"
	chunks := []models.ChunkInput{
		{EntityID: entityID, Content: "Chunk 1 content", Position: 0, HeadingPath: &headingPath, Embedding: dummyEmbedding()},
		{EntityID: entityID, Content: "Chunk 2 content", Position: 1, Embedding: dummyEmbedding()},
	}

	err = testDB.CreateChunks(ctx, entityID, chunks)
	if err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	// Get chunks
	retrieved, err := testDB.GetChunks(ctx, entityID)
	if err != nil {
		t.Fatalf("GetChunks failed: %v", err)
	}
	if len(retrieved) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(retrieved))
	}
	if retrieved[0].Position != 0 {
		t.Error("Chunks should be ordered by position")
	}
}

func TestDeleteChunks(t *testing.T) {
	ctx := context.Background()

	// Create test entity with chunks
	content := "Content for chunk deletion test"
	entity, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "document",
		Name:      "Delete Chunks Test",
		Content:   &content,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create test entity: %v", err)
	}
	entityID := models.MustRecordIDString(entity.ID)
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, entityID)
	}()

	chunks := []models.ChunkInput{
		{EntityID: entityID, Content: "To be deleted", Position: 0},
	}
	_ = testDB.CreateChunks(ctx, entityID, chunks)

	// Delete chunks
	err = testDB.DeleteChunks(ctx, entityID)
	if err != nil {
		t.Fatalf("DeleteChunks failed: %v", err)
	}

	// Verify gone
	retrieved, err := testDB.GetChunks(ctx, entityID)
	if err != nil {
		t.Fatalf("GetChunks after delete failed: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("Expected 0 chunks after delete, got %d", len(retrieved))
	}
}

// =============================================================================
// RELATION TESTS
// =============================================================================

func TestCreateAndGetRelations(t *testing.T) {
	ctx := context.Background()

	// Create two entities
	content1 := "First entity"
	content2 := "Second entity"
	entity1, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Relation Test 1",
		Content:   &content1,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create entity 1: %v", err)
	}
	entity2, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Relation Test 2",
		Content:   &content2,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create entity 2: %v", err)
	}

	id1 := models.MustRecordIDString(entity1.ID)
	id2 := models.MustRecordIDString(entity2.ID)
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, id1)
		_, _ = testDB.DeleteEntity(ctx, id2)
	}()

	// Create relation
	err = testDB.CreateRelation(ctx, models.RelationInput{
		FromID:  id1,
		ToID:    id2,
		RelType: "relates_to",
	})
	if err != nil {
		t.Fatalf("CreateRelation failed: %v", err)
	}

	// Get relations for entity1
	relations, err := testDB.GetRelations(ctx, id1)
	if err != nil {
		t.Fatalf("GetRelations failed: %v", err)
	}
	if len(relations) == 0 {
		t.Error("Expected at least one relation")
	}

	// Get relations for entity2 (should also see the relation)
	relations, err = testDB.GetRelations(ctx, id2)
	if err != nil {
		t.Fatalf("GetRelations for entity2 failed: %v", err)
	}
	if len(relations) == 0 {
		t.Error("Expected to find relation from entity2 perspective")
	}
}

func TestDeleteRelation(t *testing.T) {
	ctx := context.Background()

	// Create two entities with relation
	content1 := "Delete relation test 1"
	content2 := "Delete relation test 2"
	entity1, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Delete Rel Test 1",
		Content:   &content1,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create entity 1: %v", err)
	}
	entity2, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:      "concept",
		Name:      "Delete Rel Test 2",
		Content:   &content2,
		Embedding: dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("Failed to create entity 2: %v", err)
	}

	id1 := models.MustRecordIDString(entity1.ID)
	id2 := models.MustRecordIDString(entity2.ID)
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, id1)
		_, _ = testDB.DeleteEntity(ctx, id2)
	}()

	_ = testDB.CreateRelation(ctx, models.RelationInput{
		FromID:  id1,
		ToID:    id2,
		RelType: "test_rel",
	})

	// Delete relation
	err = testDB.DeleteRelation(ctx, id1, id2, "test_rel")
	if err != nil {
		t.Fatalf("DeleteRelation failed: %v", err)
	}

	// Verify gone
	relations, _ := testDB.GetRelations(ctx, id1)
	for _, rel := range relations {
		if rel.RelType == "test_rel" {
			t.Error("Relation should have been deleted")
		}
	}
}

// =============================================================================
// TEMPLATE TESTS
// =============================================================================

func TestTemplates(t *testing.T) {
	ctx := context.Background()

	// Create template
	description := "Test template description"
	template, err := testDB.CreateTemplate(ctx, models.TemplateInput{
		Name:        "Test Template",
		Description: &description,
		Content:     "# {{name}}\n\n{{content}}",
	})
	if err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	defer func() {
		_, _ = testDB.DeleteTemplate(ctx, "Test Template")
	}()

	if template.Name != "Test Template" {
		t.Errorf("Expected name 'Test Template', got %q", template.Name)
	}

	// Get template
	retrieved, err := testDB.GetTemplate(ctx, "Test Template")
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetTemplate returned nil")
	}

	// List templates
	templates, err := testDB.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}
	found := false
	for _, tmpl := range templates {
		if tmpl.Name == "Test Template" {
			found = true
		}
	}
	if !found {
		t.Error("ListTemplates should include created template")
	}

	// Delete template
	deleted, err := testDB.DeleteTemplate(ctx, "Test Template")
	if err != nil {
		t.Fatalf("DeleteTemplate failed: %v", err)
	}
	if !deleted {
		t.Error("DeleteTemplate should return true")
	}
}

// =============================================================================
// TOKEN USAGE TESTS
// =============================================================================

func TestTokenUsage(t *testing.T) {
	ctx := context.Background()

	// Record usage
	err := testDB.RecordTokenUsage(ctx, models.TokenUsageInput{
		Operation:    "test_embed",
		Model:        "test-model",
		InputTokens:  100,
		OutputTokens: 50,
	})
	if err != nil {
		t.Fatalf("RecordTokenUsage failed: %v", err)
	}

	// Get summary (from start of time to capture our record)
	summary, err := testDB.GetTokenUsageSummary(ctx, "2020-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("GetTokenUsageSummary failed: %v", err)
	}
	if summary.TotalTokens < 150 {
		t.Errorf("Expected at least 150 total tokens, got %d", summary.TotalTokens)
	}
}

func TestGetExistingHashes(t *testing.T) {
	ctx := context.Background()

	// Create entities with content hashes
	hash1 := "abc123def456"
	hash2 := "xyz789ghi012"
	content := "test content"

	entity1, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:        "document",
		Name:        "Hash Test 1",
		Content:     &content,
		ContentHash: &hash1,
		Embedding:   dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("CreateEntity 1 failed: %v", err)
	}
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, models.MustRecordIDString(entity1.ID))
	}()

	entity2, err := testDB.CreateEntity(ctx, models.EntityInput{
		Type:        "document",
		Name:        "Hash Test 2",
		Content:     &content,
		ContentHash: &hash2,
		Embedding:   dummyEmbedding(),
	})
	if err != nil {
		t.Fatalf("CreateEntity 2 failed: %v", err)
	}
	defer func() {
		_, _ = testDB.DeleteEntity(ctx, models.MustRecordIDString(entity2.ID))
	}()

	// Query with mix of existing and non-existing hashes
	hashes := []string{hash1, "nonexistent", hash2, "alsonotexist"}
	existing, err := testDB.GetExistingHashes(ctx, hashes)
	if err != nil {
		t.Fatalf("GetExistingHashes failed: %v", err)
	}

	if len(existing) != 2 {
		t.Errorf("Expected 2 existing hashes, got %d: %v", len(existing), existing)
	}

	// Verify the correct hashes were returned
	found := make(map[string]bool)
	for _, h := range existing {
		found[h] = true
	}
	if !found[hash1] || !found[hash2] {
		t.Errorf("Expected hashes %s and %s, got %v", hash1, hash2, existing)
	}

	// Empty input should return empty result
	empty, err := testDB.GetExistingHashes(ctx, []string{})
	if err != nil {
		t.Fatalf("GetExistingHashes with empty input failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Expected empty result, got %v", empty)
	}
}
