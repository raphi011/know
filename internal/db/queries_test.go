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

// Silence unused import warnings - these will be used in subsequent tasks
var (
	_ = fmt.Sprintf
	_ = models.Entity{}
	_ = assert.True
)
