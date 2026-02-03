// Package db_test contains integration tests for SurrealDB client.
package db_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestConfig returns config from environment or defaults for local testing.
func getTestConfig() db.Config {
	return db.Config{
		URL:       getEnv("SURREALDB_URL", "ws://localhost:8000/rpc"),
		Namespace: getEnv("SURREALDB_NAMESPACE", "test_knowledge"),
		Database:  getEnv("SURREALDB_DATABASE", "test_graph"),
		Username:  getEnv("SURREALDB_USER", "root"),
		Password:  getEnv("SURREALDB_PASS", "root"),
		AuthLevel: getEnv("SURREALDB_AUTH_LEVEL", "root"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func TestClientConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := getTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := db.NewClient(ctx, cfg, logger)
	require.NoError(t, err, "should connect to SurrealDB")
	defer client.Close(ctx)

	assert.NotNil(t, client.DB(), "should have valid DB reference")
}

func TestClientInitSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := getTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := db.NewClient(ctx, cfg, logger)
	require.NoError(t, err, "should connect to SurrealDB")
	defer client.Close(ctx)

	err = client.InitSchema(ctx)
	require.NoError(t, err, "should initialize schema without error")

	// Verify tables exist by querying INFO FOR DB
	result, err := client.Query(ctx, "INFO FOR DB", nil)
	require.NoError(t, err, "should query database info")
	assert.NotNil(t, result, "should return database info")
}

func TestClientQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := getTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := db.NewClient(ctx, cfg, logger)
	require.NoError(t, err, "should connect to SurrealDB")
	defer client.Close(ctx)

	err = client.InitSchema(ctx)
	require.NoError(t, err, "should initialize schema")

	// Test simple query
	result, err := client.Query(ctx, "SELECT count() FROM entity GROUP ALL", nil)
	require.NoError(t, err, "should execute count query")
	assert.NotNil(t, result, "should return result")
}

func TestClientReconnection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := getTestConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := db.NewClient(ctx, cfg, logger)
	require.NoError(t, err, "should connect to SurrealDB")
	defer client.Close(ctx)

	// Execute query before and after a short wait to verify connection stays alive
	_, err = client.Query(ctx, "RETURN 1", nil)
	require.NoError(t, err, "should execute query before wait")

	time.Sleep(2 * time.Second)

	_, err = client.Query(ctx, "RETURN 2", nil)
	require.NoError(t, err, "should execute query after wait (connection maintained)")
}
