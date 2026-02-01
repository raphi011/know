//go:build integration

package tools_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests with real DB in test_integration.go

func TestSearchToolRegistered(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create server
	impl := &mcp.Implementation{
		Name:    "test-memcp",
		Version: "0.0.1-test",
	}
	server := mcp.NewServer(impl, nil)

	// Register tools with nil deps (validation only)
	deps := &tools.Dependencies{
		DB:       nil,
		Embedder: nil,
		Logger:   logger,
	}
	cfg := &config.Config{}
	tools.RegisterAll(server, deps, cfg)

	// Create in-memory transports
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx, serverTransport)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Create client and connect
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err, "client should connect successfully")
	defer session.Close()

	// Test: List tools - verify "search" is registered
	t.Run("tools/list returns search", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, result.Tools, 2) // ping + search

		toolNames := make([]string, len(result.Tools))
		for i, tool := range result.Tools {
			toolNames[i] = tool.Name
		}
		assert.Contains(t, toolNames, "search")
		assert.Contains(t, toolNames, "ping")
	})

	// Test: Verify search tool description
	t.Run("search has correct description", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)

		var searchTool *mcp.Tool
		for _, tool := range result.Tools {
			if tool.Name == "search" {
				searchTool = tool
				break
			}
		}
		require.NotNil(t, searchTool, "search tool should exist")
		assert.Equal(t, "Search the knowledge graph using hybrid BM25 + vector search with RRF fusion", searchTool.Description)
	})

	// Cleanup
	cancel()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Logf("server stopped with: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestSearchToolValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create server
	impl := &mcp.Implementation{
		Name:    "test-memcp",
		Version: "0.0.1-test",
	}
	server := mcp.NewServer(impl, nil)

	// Register tools with nil deps - validation happens before DB/Embedder calls
	deps := &tools.Dependencies{
		DB:       nil,
		Embedder: nil,
		Logger:   logger,
	}
	cfg := &config.Config{}
	tools.RegisterAll(server, deps, cfg)

	// Create in-memory transports
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx, serverTransport)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Create client and connect
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err, "client should connect successfully")
	defer session.Close()

	// Test: Empty query returns error
	t.Run("empty query returns error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name:      "search",
			Arguments: map[string]any{"query": ""},
		}
		result, err := session.CallTool(ctx, params)
		require.NoError(t, err)
		assert.True(t, result.IsError, "empty query should return error")

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "Query cannot be empty")
	})

	// Test: Limit > 100 returns error
	t.Run("limit over 100 returns error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name:      "search",
			Arguments: map[string]any{"query": "test", "limit": 150},
		}
		result, err := session.CallTool(ctx, params)
		require.NoError(t, err)
		assert.True(t, result.IsError, "limit > 100 should return error")

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "Limit must be 1-100")
	})

	// Cleanup
	cancel()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Logf("server stopped with: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop within timeout")
	}
}
