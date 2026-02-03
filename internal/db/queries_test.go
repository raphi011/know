// Package db_test contains integration tests for query functions.
package db_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: getTestConfig() and getEnv() are defined in client_test.go
// Both files are in package db_test, so these helpers are shared.

// testClient creates a connected client for testing.
// Skips test in short mode.
func testClient(t *testing.T) (*db.Client, context.Context) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(func() { cancel() })

	cfg := getTestConfig() // from client_test.go
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := db.NewClient(ctx, cfg, logger)
	require.NoError(t, err, "should connect to SurrealDB")
	t.Cleanup(func() { client.Close(ctx) })

	err = client.InitSchema(ctx)
	require.NoError(t, err, "should initialize schema")

	return client, ctx
}

// cleanupEntities removes test entities by ID prefix.
// Uses <string> cast for v3 compatibility where id is a record type.
func cleanupEntities(t *testing.T, client *db.Client, ctx context.Context, prefix string) {
	_, err := client.Query(ctx, `DELETE entity WHERE string::starts_with(<string>id, $prefix)`, map[string]any{"prefix": "entity:" + prefix})
	require.NoError(t, err, "cleanup entities")
}

// cleanupEpisodes removes test episodes by ID prefix.
func cleanupEpisodes(t *testing.T, client *db.Client, ctx context.Context, prefix string) {
	_, err := client.Query(ctx, `DELETE episode WHERE string::starts_with(<string>id, $prefix)`, map[string]any{"prefix": "episode:" + prefix})
	require.NoError(t, err, "cleanup episodes")
}

// cleanupProcedures removes test procedures by ID prefix.
func cleanupProcedures(t *testing.T, client *db.Client, ctx context.Context, prefix string) {
	_, err := client.Query(ctx, `DELETE procedure WHERE string::starts_with(<string>id, $prefix)`, map[string]any{"prefix": "procedure:" + prefix})
	require.NoError(t, err, "cleanup procedures")
}

// testEmbedding returns a dummy 384-dim embedding for testing.
func testEmbedding() []float32 {
	emb := make([]float32, 384)
	for i := range emb {
		emb[i] = float32(i) / 384.0
	}
	return emb
}

// similarEmbedding returns an embedding similar to testEmbedding (high cosine similarity).
func similarEmbedding() []float32 {
	emb := make([]float32, 384)
	for i := range emb {
		emb[i] = float32(i)/384.0 + 0.01 // Small perturbation
	}
	return emb
}

// differentEmbedding returns an embedding different from testEmbedding (low cosine similarity).
func differentEmbedding() []float32 {
	emb := make([]float32, 384)
	for i := range emb {
		emb[i] = float32(384-i) / 384.0 // Reversed
	}
	return emb
}

func TestQueryUpsertEntity(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_upsert_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	id := prefix + "_entity1"
	testCtx := "test-context"

	// Create new entity
	entity, wasCreated, err := client.QueryUpsertEntity(
		ctx, id, "concept", []string{"test", "unit"}, "Test content",
		testEmbedding(), 0.9, nil, &testCtx,
	)
	require.NoError(t, err)
	assert.True(t, wasCreated, "should be created")
	assert.Equal(t, "entity:"+id, entity.ID.String())
	assert.Equal(t, "concept", entity.Type)
	assert.Contains(t, entity.Labels, "test")

	// Update existing entity
	entity2, wasCreated2, err := client.QueryUpsertEntity(
		ctx, id, "concept", []string{"new-label"}, "Updated content",
		testEmbedding(), 0.95, nil, &testCtx,
	)
	require.NoError(t, err)
	assert.False(t, wasCreated2, "should be updated")
	assert.Equal(t, "Updated content", entity2.Content)
	// Labels should be merged
	assert.Contains(t, entity2.Labels, "test")
	assert.Contains(t, entity2.Labels, "new-label")
}

func TestQueryGetEntity(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_get_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	id := prefix + "_entity1"

	// Get non-existent
	entity, err := client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, entity, "should return nil for non-existent")

	// Create and get
	_, _, err = client.QueryUpsertEntity(ctx, id, "test", nil, "Content", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	entity, err = client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, entity)
	assert.Equal(t, "Content", entity.Content)
}

func TestQueryDeleteEntity(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_del_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	id := prefix + "_entity1"

	// Delete non-existent (idempotent)
	count, err := client.QueryDeleteEntity(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Create and delete
	_, _, err = client.QueryUpsertEntity(ctx, id, "test", nil, "Content", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	count, err = client.QueryDeleteEntity(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify deleted
	entity, err := client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, entity)
}

func TestQueryHybridSearch(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_search_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "search-test-ctx"

	// Create test entities
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_go", "lang", []string{"programming"}, "Go is a programming language", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_rust", "lang", []string{"programming"}, "Rust is a systems language", differentEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// Search
	results, err := client.QueryHybridSearch(ctx, "programming", testEmbedding(), nil, 10, &testCtx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1, "should find results")

	// Search with label filter
	results, err = client.QueryHybridSearch(ctx, "programming", testEmbedding(), []string{"programming"}, 10, &testCtx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestQueryListLabels(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_labels_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "labels-test-ctx"

	// Create entities with labels
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_1", "test", []string{"alpha", "beta"}, "Content 1", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_2", "test", []string{"alpha", "gamma"}, "Content 2", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// List labels
	labels, err := client.QueryListLabels(ctx, &testCtx)
	require.NoError(t, err)

	// Find alpha (should have count 2)
	var alphaCount int
	for _, l := range labels {
		if l.Label == "alpha" {
			alphaCount = l.Count
			break
		}
	}
	assert.Equal(t, 2, alphaCount, "alpha should appear twice")
}

func TestQueryListTypes(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_types_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "types-test-ctx"

	// Create entities with types
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_1", "concept", nil, "Content 1", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_2", "concept", nil, "Content 2", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_3", "person", nil, "Content 3", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// List types
	types, err := client.QueryListTypes(ctx, &testCtx)
	require.NoError(t, err)

	typeMap := make(map[string]int)
	for _, tc := range types {
		typeMap[tc.Type] = tc.Count
	}
	assert.Equal(t, 2, typeMap["concept"])
	assert.Equal(t, 1, typeMap["person"])
}

// ========== Episode Query Tests ==========

func TestQueryCreateEpisode(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_episode_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEpisodes(t, client, ctx, prefix) })

	id := prefix + "_ep1"
	testCtx := "episode-test-ctx"
	timestamp := time.Now().Format(time.RFC3339)

	episode, err := client.QueryCreateEpisode(ctx, id, "Episode content", testEmbedding(), timestamp, nil, nil, &testCtx)
	require.NoError(t, err)
	assert.Equal(t, "episode:"+id, episode.ID)
	assert.Equal(t, "Episode content", episode.Content)
}

func TestQueryGetEpisode(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_get_ep_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEpisodes(t, client, ctx, prefix) })

	id := prefix + "_ep1"

	// Get non-existent
	episode, err := client.QueryGetEpisode(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, episode)

	// Create and get
	_, err = client.QueryCreateEpisode(ctx, id, "Content", testEmbedding(), time.Now().Format(time.RFC3339), nil, nil, nil)
	require.NoError(t, err)

	episode, err = client.QueryGetEpisode(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, episode)
}

func TestQueryDeleteEpisode(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_del_ep_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEpisodes(t, client, ctx, prefix) })

	id := prefix + "_ep1"

	// Delete non-existent
	count, err := client.QueryDeleteEpisode(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Create and delete
	_, err = client.QueryCreateEpisode(ctx, id, "Content", testEmbedding(), time.Now().Format(time.RFC3339), nil, nil, nil)
	require.NoError(t, err)

	count, err = client.QueryDeleteEpisode(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// ========== Procedure Query Tests ==========

func TestQueryCreateProcedure(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_proc_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupProcedures(t, client, ctx, prefix) })

	id := prefix + "_proc1"
	testCtx := "proc-test-ctx"
	steps := []models.ProcedureStep{
		{Order: 1, Content: "Step 1", Optional: false},
		{Order: 2, Content: "Step 2", Optional: true},
	}

	proc, err := client.QueryCreateProcedure(ctx, id, "Test Procedure", "A test", steps, testEmbedding(), []string{"test"}, &testCtx)
	require.NoError(t, err)
	assert.Equal(t, "procedure:"+id, proc.ID)
	assert.Equal(t, "Test Procedure", proc.Name)
	assert.Len(t, proc.Steps, 2)
}

func TestQueryGetProcedure(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_get_proc_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupProcedures(t, client, ctx, prefix) })

	id := prefix + "_proc1"

	// Get non-existent
	proc, err := client.QueryGetProcedure(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, proc)

	// Create and get
	_, err = client.QueryCreateProcedure(ctx, id, "Test", "Desc", nil, testEmbedding(), nil, nil)
	require.NoError(t, err)

	proc, err = client.QueryGetProcedure(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, proc)
}

func TestQueryDeleteProcedure(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_del_proc_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupProcedures(t, client, ctx, prefix) })

	id := prefix + "_proc1"

	// Delete non-existent
	count, err := client.QueryDeleteProcedure(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Create and delete
	_, err = client.QueryCreateProcedure(ctx, id, "Test", "Desc", nil, testEmbedding(), nil, nil)
	require.NoError(t, err)

	count, err = client.QueryDeleteProcedure(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// ========== Maintenance Query Tests ==========

func TestQueryApplyDecay(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_decay_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "decay-test-ctx"

	// Create entity with old accessed timestamp
	id := prefix + "_stale"
	_, _, err := client.QueryUpsertEntity(ctx, id, "test", nil, "Stale content", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// Manually set accessed to 60 days ago
	_, err = client.Query(ctx, `UPDATE type::record("entity", $id) SET accessed = time::now() - 60d`, map[string]any{"id": id})
	require.NoError(t, err)

	// Dry run - should find the stale entity
	entities, err := client.QueryApplyDecay(ctx, 30, &testCtx, false, true)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entities), 1, "should find stale entity in dry run")

	// Apply decay
	entities, err = client.QueryApplyDecay(ctx, 30, &testCtx, false, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entities), 1, "should apply decay")

	// Verify decay applied
	entity, err := client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	assert.Less(t, entity.DecayWeight, 1.0, "decay_weight should be reduced")
}

func TestQueryApplyDecay_Floor(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_decay_floor_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "decay-floor-ctx"
	id := prefix + "_floor"

	// Create entity with low decay_weight
	_, _, err := client.QueryUpsertEntity(ctx, id, "test", nil, "Content", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// Set decay_weight to 0.15 and accessed to old
	_, err = client.Query(ctx, `UPDATE type::record("entity", $id) SET decay_weight = 0.15, accessed = time::now() - 60d`, map[string]any{"id": id})
	require.NoError(t, err)

	// Apply decay
	_, err = client.QueryApplyDecay(ctx, 30, &testCtx, false, false)
	require.NoError(t, err)

	// Verify floor at 0.1
	entity, err := client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, entity.DecayWeight, 0.1, "decay_weight should not go below 0.1")
}

func TestQueryFindSimilarPairs(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_similar_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "similar-test-ctx"

	// Create two similar entities
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_a", "test", nil, "Content A", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_b", "test", nil, "Content B", similarEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// Create one different entity
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_c", "test", nil, "Content C", differentEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	// Find similar pairs with low threshold
	pairs, err := client.QueryFindSimilarPairs(ctx, 0.5, 10, &testCtx, false)
	require.NoError(t, err)

	// Should find at least the A-B pair (similar embeddings)
	found := false
	for _, p := range pairs {
		if (p.Entity1ID == "entity:"+prefix+"_a" && p.Entity2ID == "entity:"+prefix+"_b") ||
			(p.Entity1ID == "entity:"+prefix+"_b" && p.Entity2ID == "entity:"+prefix+"_a") {
			found = true
			assert.Greater(t, p.Similarity, 0.9, "A-B pair should have high similarity")
			break
		}
	}
	assert.True(t, found, "should find similar pair A-B")
}

func TestQueryFindSimilarPairs_NoDuplicates(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_nodup_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	testCtx := "nodup-test-ctx"

	// Create similar entities
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_x", "test", nil, "X", testEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_y", "test", nil, "Y", similarEmbedding(), 1.0, nil, &testCtx)
	require.NoError(t, err)

	pairs, err := client.QueryFindSimilarPairs(ctx, 0.5, 10, &testCtx, false)
	require.NoError(t, err)

	// Should not have both X-Y and Y-X
	pairCount := 0
	for _, p := range pairs {
		isXY := (p.Entity1ID == "entity:"+prefix+"_x" && p.Entity2ID == "entity:"+prefix+"_y")
		isYX := (p.Entity1ID == "entity:"+prefix+"_y" && p.Entity2ID == "entity:"+prefix+"_x")
		if isXY || isYX {
			pairCount++
		}
	}
	assert.LessOrEqual(t, pairCount, 1, "should not have duplicate pairs")
}

// ========== Relation Query Tests ==========

func TestQueryCreateRelation(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_rel_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupEntities(t, client, ctx, prefix)
		// Also cleanup relations - use string cast for v3 compatibility
		_, _ = client.Query(ctx, `DELETE relates WHERE string::contains(<string>in, $prefix) OR string::contains(<string>out, $prefix)`, map[string]any{"prefix": prefix})
	})

	// Create two entities
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_from", "concept", nil, "From entity", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_to", "concept", nil, "To entity", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	// Create relation
	err = client.QueryCreateRelation(ctx, prefix+"_from", "relates_to", prefix+"_to", 0.8)
	require.NoError(t, err, "should create relation")

	// Create relation again (upsert - should not error)
	err = client.QueryCreateRelation(ctx, prefix+"_from", "relates_to", prefix+"_to", 0.9)
	require.NoError(t, err, "should upsert relation")

	// Create relation to non-existent entity
	err = client.QueryCreateRelation(ctx, prefix+"_from", "relates_to", "nonexistent_xyz", 0.5)
	assert.Error(t, err, "should fail for non-existent target")
}

func TestQueryGetLinkedEntities(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_linked_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupEntities(t, client, ctx, prefix)
		cleanupEpisodes(t, client, ctx, prefix)
		// Cleanup extracted_from relations - use string cast for v3 compatibility
		_, _ = client.Query(ctx, `DELETE extracted_from WHERE string::contains(<string>out, $prefix)`, map[string]any{"prefix": prefix})
	})

	// Create episode
	timestamp := time.Now().Format(time.RFC3339)
	_, err := client.QueryCreateEpisode(ctx, prefix+"_ep", "Episode content", testEmbedding(), timestamp, nil, nil, nil)
	require.NoError(t, err)

	// Create entities and link to episode
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_e1", "concept", nil, "Entity 1", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_e2", "concept", nil, "Entity 2", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	err = client.QueryLinkEntityToEpisode(ctx, prefix+"_e1", prefix+"_ep", 0, 0.9)
	require.NoError(t, err)
	err = client.QueryLinkEntityToEpisode(ctx, prefix+"_e2", prefix+"_ep", 1, 0.8)
	require.NoError(t, err)

	// Get linked entities
	entities, err := client.QueryGetLinkedEntities(ctx, prefix+"_ep")
	require.NoError(t, err)
	assert.Len(t, entities, 2, "should find 2 linked entities")

	// Get linked for episode with no links
	entities, err = client.QueryGetLinkedEntities(ctx, "nonexistent_ep_xyz")
	require.NoError(t, err)
	assert.Empty(t, entities, "should return empty for non-existent episode")
}

// ========== Graph Traversal Query Tests ==========

func TestQueryTraverse(t *testing.T) {
	t.Skip("SKIP: Go SDK v1.2.0 cannot decode SurrealDB v3 graph traversal results due to CBOR range type incompatibility. " +
		"The query syntax '->relates..{depth}->entity' returns range bounds that cause CBOR decode panics. " +
		"This will be fixed in a future SDK release.")

	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_traverse_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupEntities(t, client, ctx, prefix)
		_, _ = client.Query(ctx, `DELETE relates WHERE string::contains(<string>in, $prefix) OR string::contains(<string>out, $prefix)`, map[string]any{"prefix": prefix})
	})

	// Create chain: A -> B -> C
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_a", "concept", nil, "A", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_b", "concept", nil, "B", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_c", "concept", nil, "C", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	err = client.QueryCreateRelation(ctx, prefix+"_a", "connects", prefix+"_b", 1.0)
	require.NoError(t, err)
	err = client.QueryCreateRelation(ctx, prefix+"_b", "connects", prefix+"_c", 1.0)
	require.NoError(t, err)

	// Traverse from A with depth 1
	results, err := client.QueryTraverse(ctx, prefix+"_a", 1, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "should find traverse results")

	// Traverse from A with depth 2 (should reach C)
	results, err = client.QueryTraverse(ctx, prefix+"_a", 2, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "should find deeper traverse results")

	// Traverse with relation type filter
	results, err = client.QueryTraverse(ctx, prefix+"_a", 2, []string{"connects"})
	require.NoError(t, err)
	assert.NotEmpty(t, results, "should find results with type filter")

	// Traverse with non-matching relation type
	results, err = client.QueryTraverse(ctx, prefix+"_a", 2, []string{"unrelated_type"})
	require.NoError(t, err)
	// May be empty when filtering by non-existent type
}

func TestQueryFindPath(t *testing.T) {
	t.Skip("SKIP: Go SDK v1.2.0 cannot decode SurrealDB v3 graph traversal results due to CBOR range type incompatibility. " +
		"The query syntax '->relates..{depth}->entity' returns range bounds that cause CBOR decode panics. " +
		"This will be fixed in a future SDK release.")

	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_path_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupEntities(t, client, ctx, prefix)
		_, _ = client.Query(ctx, `DELETE relates WHERE string::contains(<string>in, $prefix) OR string::contains(<string>out, $prefix)`, map[string]any{"prefix": prefix})
	})

	// Create path: X -> Y -> Z
	_, _, err := client.QueryUpsertEntity(ctx, prefix+"_x", "concept", nil, "X", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_y", "concept", nil, "Y", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_z", "concept", nil, "Z", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	err = client.QueryCreateRelation(ctx, prefix+"_x", "links", prefix+"_y", 1.0)
	require.NoError(t, err)
	err = client.QueryCreateRelation(ctx, prefix+"_y", "links", prefix+"_z", 1.0)
	require.NoError(t, err)

	// Find path from X to Z (depth 2)
	path, err := client.QueryFindPath(ctx, prefix+"_x", prefix+"_z", 2)
	require.NoError(t, err)
	// Path traversal returns intermediate nodes or target

	// Find path with insufficient depth
	path, err = client.QueryFindPath(ctx, prefix+"_x", prefix+"_z", 1)
	require.NoError(t, err)
	// May be nil or empty if path requires depth 2

	// Find path to non-existent entity
	path, err = client.QueryFindPath(ctx, prefix+"_x", "nonexistent_xyz", 5)
	require.NoError(t, err)
	assert.Nil(t, path, "should return nil for no path found")

	_ = path // Use variable
}

// ========== Search Query Tests ==========

func TestQuerySearchEpisodes(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_search_ep_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEpisodes(t, client, ctx, prefix) })

	testCtx := "search-ep-test-ctx"
	now := time.Now()

	// Create episodes at different times
	_, err := client.QueryCreateEpisode(ctx, prefix+"_recent", "Recent episode about coding", testEmbedding(), now.Format(time.RFC3339), nil, nil, &testCtx)
	require.NoError(t, err)
	_, err = client.QueryCreateEpisode(ctx, prefix+"_old", "Old episode about testing", differentEmbedding(), now.Add(-48*time.Hour).Format(time.RFC3339), nil, nil, &testCtx)
	require.NoError(t, err)

	// Basic search
	results, err := client.QuerySearchEpisodes(ctx, "coding", testEmbedding(), nil, nil, &testCtx, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "should find episodes")

	// Search with time range (last 24 hours)
	timeStart := now.Add(-24 * time.Hour).Format(time.RFC3339)
	results, err = client.QuerySearchEpisodes(ctx, "episode", testEmbedding(), &timeStart, nil, &testCtx, 10)
	require.NoError(t, err)
	// Should find recent episode only

	// Search with context filter
	otherCtx := "other-context"
	results, err = client.QuerySearchEpisodes(ctx, "episode", testEmbedding(), nil, nil, &otherCtx, 10)
	require.NoError(t, err)
	assert.Empty(t, results, "should find nothing in different context")
}

func TestQuerySearchProcedures(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_search_proc_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupProcedures(t, client, ctx, prefix) })

	testCtx := "search-proc-test-ctx"

	// Create procedures with labels
	_, err := client.QueryCreateProcedure(ctx, prefix+"_deploy", "Deploy Application", "Steps to deploy", nil, testEmbedding(), []string{"devops", "prod"}, &testCtx)
	require.NoError(t, err)
	_, err = client.QueryCreateProcedure(ctx, prefix+"_test", "Run Tests", "Steps to run tests", nil, differentEmbedding(), []string{"devops", "ci"}, &testCtx)
	require.NoError(t, err)

	// Basic search
	results, err := client.QuerySearchProcedures(ctx, "deploy", testEmbedding(), nil, &testCtx, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "should find procedures")

	// Search with label filter
	results, err = client.QuerySearchProcedures(ctx, "steps", testEmbedding(), []string{"prod"}, &testCtx, 10)
	require.NoError(t, err)
	// Should find deploy procedure (has prod label)

	// Search with non-matching label
	results, err = client.QuerySearchProcedures(ctx, "steps", testEmbedding(), []string{"nonexistent_label"}, &testCtx, 10)
	require.NoError(t, err)
	assert.Empty(t, results, "should find nothing with non-matching label")
}

func TestQueryListProcedures(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_list_proc_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupProcedures(t, client, ctx, prefix) })

	testCtx := "list-proc-test-ctx"

	// Create procedures
	_, err := client.QueryCreateProcedure(ctx, prefix+"_a", "Procedure A", "Desc A", nil, testEmbedding(), nil, &testCtx)
	require.NoError(t, err)
	_, err = client.QueryCreateProcedure(ctx, prefix+"_b", "Procedure B", "Desc B", nil, testEmbedding(), nil, &testCtx)
	require.NoError(t, err)

	// List with context filter
	results, err := client.QueryListProcedures(ctx, &testCtx, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "should find at least 2 procedures")

	// List with limit
	results, err = client.QueryListProcedures(ctx, &testCtx, 1)
	require.NoError(t, err)
	assert.Len(t, results, 1, "should respect limit")

	// List with different context (empty result)
	otherCtx := "other-context-xyz"
	results, err = client.QueryListProcedures(ctx, &otherCtx, 100)
	require.NoError(t, err)
	assert.Empty(t, results, "should find nothing in different context")

	// List all (no context filter)
	results, err = client.QueryListProcedures(ctx, nil, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "should find procedures without context filter")
}

// ========== Link Query Tests ==========

func TestQueryLinkEntityToEpisode(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_link_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupEntities(t, client, ctx, prefix)
		cleanupEpisodes(t, client, ctx, prefix)
		_, _ = client.Query(ctx, `DELETE extracted_from WHERE string::contains(<string>out, $prefix)`, map[string]any{"prefix": prefix})
	})

	// Create episode and entity
	timestamp := time.Now().Format(time.RFC3339)
	_, err := client.QueryCreateEpisode(ctx, prefix+"_ep", "Episode content", testEmbedding(), timestamp, nil, nil, nil)
	require.NoError(t, err)
	_, _, err = client.QueryUpsertEntity(ctx, prefix+"_entity", "concept", nil, "Entity content", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	// Link entity to episode
	err = client.QueryLinkEntityToEpisode(ctx, prefix+"_entity", prefix+"_ep", 0, 0.95)
	require.NoError(t, err, "should create link")

	// Link again (should upsert or create duplicate - depends on schema)
	err = client.QueryLinkEntityToEpisode(ctx, prefix+"_entity", prefix+"_ep", 1, 0.85)
	require.NoError(t, err, "should handle duplicate link")
}

// ========== Access Update Query Tests ==========

func TestQueryUpdateAccess(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_access_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEntities(t, client, ctx, prefix) })

	id := prefix + "_entity"

	// Create entity
	_, _, err := client.QueryUpsertEntity(ctx, id, "concept", nil, "Content", testEmbedding(), 1.0, nil, nil)
	require.NoError(t, err)

	// Get initial state
	entity, err := client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	initialAccess := entity.AccessCount

	// Update access
	err = client.QueryUpdateAccess(ctx, id)
	require.NoError(t, err, "should update access")

	// Verify access_count incremented
	entity, err = client.QueryGetEntity(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, initialAccess+1, entity.AccessCount, "access_count should increment")
	assert.Equal(t, 1.0, entity.DecayWeight, "decay_weight should reset to 1.0")

	// Update access for non-existent entity (no error, just no-op)
	err = client.QueryUpdateAccess(ctx, "nonexistent_entity_xyz")
	require.NoError(t, err, "should not error for non-existent entity")
}

func TestQueryUpdateEpisodeAccess(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_ep_access_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupEpisodes(t, client, ctx, prefix) })

	id := prefix + "_ep"
	timestamp := time.Now().Format(time.RFC3339)

	// Create episode
	_, err := client.QueryCreateEpisode(ctx, id, "Content", testEmbedding(), timestamp, nil, nil, nil)
	require.NoError(t, err)

	// Get initial state
	episode, err := client.QueryGetEpisode(ctx, id)
	require.NoError(t, err)
	initialAccess := episode.AccessCount

	// Update access
	err = client.QueryUpdateEpisodeAccess(ctx, id)
	require.NoError(t, err, "should update episode access")

	// Verify access_count incremented
	episode, err = client.QueryGetEpisode(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, initialAccess+1, episode.AccessCount, "access_count should increment")

	// Update access for non-existent episode (no error, just no-op)
	err = client.QueryUpdateEpisodeAccess(ctx, "nonexistent_ep_xyz")
	require.NoError(t, err, "should not error for non-existent episode")
}

func TestQueryUpdateProcedureAccess(t *testing.T) {
	client, ctx := testClient(t)
	prefix := fmt.Sprintf("test_proc_access_%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupProcedures(t, client, ctx, prefix) })

	id := prefix + "_proc"

	// Create procedure
	_, err := client.QueryCreateProcedure(ctx, id, "Test Proc", "Description", nil, testEmbedding(), nil, nil)
	require.NoError(t, err)

	// Get initial state
	proc, err := client.QueryGetProcedure(ctx, id)
	require.NoError(t, err)
	initialAccess := proc.AccessCount

	// Update access
	err = client.QueryUpdateProcedureAccess(ctx, id)
	require.NoError(t, err, "should update procedure access")

	// Verify access_count incremented
	proc, err = client.QueryGetProcedure(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, initialAccess+1, proc.AccessCount, "access_count should increment")

	// Update access for non-existent procedure (no error, just no-op)
	err = client.QueryUpdateProcedureAccess(ctx, "nonexistent_proc_xyz")
	require.NoError(t, err, "should not error for non-existent procedure")
}
