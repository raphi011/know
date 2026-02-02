//go:build integration

// Package tools_test contains tests for MCP tools.
// Integration tests requiring SurrealDB/Ollama are in test_integration.go
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

func TestRememberToolRegistered(t *testing.T) {
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

	// Test: List tools - verify remember tool is registered
	t.Run("remember tool appears in tools/list", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, result.Tools, 7) // ping + search + get_entity + list_labels + list_types + remember + forget

		toolNames := make([]string, len(result.Tools))
		for i, tool := range result.Tools {
			toolNames[i] = tool.Name
		}
		assert.Contains(t, toolNames, "remember")
	})

	// Test: Verify remember tool description
	t.Run("remember has correct description", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)

		var rememberTool *mcp.Tool
		for _, tool := range result.Tools {
			if tool.Name == "remember" {
				rememberTool = tool
				break
			}
		}
		require.NotNil(t, rememberTool, "remember tool should exist")
		assert.Equal(t, "Store entities in the knowledge graph with auto-generated embeddings", rememberTool.Description)
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

func TestRememberToolValidation(t *testing.T) {
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

	// Test: Empty entities returns error
	t.Run("empty entities returns error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name:      "remember",
			Arguments: map[string]any{"entities": []any{}},
		}
		result, err := session.CallTool(ctx, params)
		require.NoError(t, err)
		assert.True(t, result.IsError, "empty entities should return error")

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "At least one entity is required")
	})

	// Test: Entity without name returns error (schema validation at SDK level)
	t.Run("entity without name returns error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name: "remember",
			Arguments: map[string]any{
				"entities": []any{
					map[string]any{"content": "test content"},
				},
			},
		}
		// SDK validates schema and returns error before handler is called
		_, err := session.CallTool(ctx, params)
		require.Error(t, err, "entity without name should return error")
		assert.Contains(t, err.Error(), "name")
	})

	// Test: Entity without content returns error (schema validation at SDK level)
	t.Run("entity without content returns error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name: "remember",
			Arguments: map[string]any{
				"entities": []any{
					map[string]any{"name": "test-entity"},
				},
			},
		}
		// SDK validates schema and returns error before handler is called
		_, err := session.CallTool(ctx, params)
		require.Error(t, err, "entity without content should return error")
		assert.Contains(t, err.Error(), "content")
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

// TestSlugify tests the slugify function via composite ID behavior
func TestGenerateEntityID(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		expected string
	}{
		{
			name:     "Simple Name",
			context:  "",
			expected: "simple-name",
		},
		{
			name:     "user preferences",
			context:  "myproject",
			expected: "myproject:user-preferences",
		},
		{
			name:     "Test With  Spaces",
			context:  "",
			expected: "test-with--spaces",
		},
		{
			name:     "UPPERCASE",
			context:  "ctx",
			expected: "ctx:uppercase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test via RememberInput handling
			// The generateEntityID function is private, so we test its behavior
			// through integration or by examining response IDs
			// This test documents expected ID format
			t.Logf("Expected ID for name=%q context=%q: %s", tt.name, tt.context, tt.expected)
		})
	}
}

func TestRememberToolRelationValidation(t *testing.T) {
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

	// Test: Relation without from/to/type returns error (schema validation)
	t.Run("relation without from returns schema error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name: "remember",
			Arguments: map[string]any{
				"entities": []any{
					map[string]any{"name": "entity1", "content": "test content"},
				},
				"relations": []any{
					map[string]any{"to": "entity2", "type": "relates_to"},
				},
			},
		}
		// SDK validates schema and returns error for missing required field
		_, err := session.CallTool(ctx, params)
		require.Error(t, err, "relation without from should return error")
		assert.Contains(t, err.Error(), "from")
	})

	t.Run("relation without to returns schema error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name: "remember",
			Arguments: map[string]any{
				"entities": []any{
					map[string]any{"name": "entity1", "content": "test content"},
				},
				"relations": []any{
					map[string]any{"from": "entity1", "type": "relates_to"},
				},
			},
		}
		_, err := session.CallTool(ctx, params)
		require.Error(t, err, "relation without to should return error")
		assert.Contains(t, err.Error(), "to")
	})

	t.Run("relation without type returns schema error", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name: "remember",
			Arguments: map[string]any{
				"entities": []any{
					map[string]any{"name": "entity1", "content": "test content"},
				},
				"relations": []any{
					map[string]any{"from": "entity1", "to": "entity2"},
				},
			},
		}
		_, err := session.CallTool(ctx, params)
		require.Error(t, err, "relation without type should return error")
		assert.Contains(t, err.Error(), "type")
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
