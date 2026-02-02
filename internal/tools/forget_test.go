//go:build integration

// Package tools_test contains tests for MCP tools.
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

func TestForgetToolRegistered(t *testing.T) {
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

	// Test: List tools - verify forget tool is registered
	t.Run("forget tool appears in tools/list", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, result.Tools, 7) // ping + search + get_entity + list_labels + list_types + remember + forget

		toolNames := make([]string, len(result.Tools))
		for i, tool := range result.Tools {
			toolNames[i] = tool.Name
		}
		assert.Contains(t, toolNames, "forget")
	})

	// Test: Verify forget tool description
	t.Run("forget has correct description", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)

		var forgetTool *mcp.Tool
		for _, tool := range result.Tools {
			if tool.Name == "forget" {
				forgetTool = tool
				break
			}
		}
		require.NotNil(t, forgetTool, "forget tool should exist")
		assert.Equal(t, "Delete entities from the knowledge graph by ID", forgetTool.Description)
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

func TestForgetToolValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create server
	impl := &mcp.Implementation{
		Name:    "test-memcp",
		Version: "0.0.1-test",
	}
	server := mcp.NewServer(impl, nil)

	// Register tools with nil deps - validation happens before DB calls
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

	// Test: Empty IDs returns error
	t.Run("empty ids returns error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name:      "forget",
			Arguments: map[string]any{"ids": []any{}},
		}
		result, err := session.CallTool(ctx, params)
		require.NoError(t, err)
		assert.True(t, result.IsError, "empty ids should return error")

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "At least one ID is required")
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
