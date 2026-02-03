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
func cleanupEntities(t *testing.T, client *db.Client, ctx context.Context, prefix string) {
	_, err := client.Query(ctx, `DELETE entity WHERE string::starts_with(id, $prefix)`, map[string]any{"prefix": "entity:" + prefix})
	require.NoError(t, err, "cleanup entities")
}

// cleanupEpisodes removes test episodes by ID prefix.
func cleanupEpisodes(t *testing.T, client *db.Client, ctx context.Context, prefix string) {
	_, err := client.Query(ctx, `DELETE episode WHERE string::starts_with(id, $prefix)`, map[string]any{"prefix": "episode:" + prefix})
	require.NoError(t, err, "cleanup episodes")
}

// cleanupProcedures removes test procedures by ID prefix.
func cleanupProcedures(t *testing.T, client *db.Client, ctx context.Context, prefix string) {
	_, err := client.Query(ctx, `DELETE procedure WHERE string::starts_with(id, $prefix)`, map[string]any{"prefix": "procedure:" + prefix})
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
	assert.Equal(t, "entity:"+id, entity.ID)
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
