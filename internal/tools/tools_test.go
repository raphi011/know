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

// testLogger creates a logger for test visibility.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestPingTool(t *testing.T) {
	logger := testLogger()

	// Create server
	impl := &mcp.Implementation{
		Name:    "test-memcp",
		Version: "0.0.1-test",
	}
	server := mcp.NewServer(impl, nil)

	// Register tools with nil deps (ping doesn't need them for basic operation)
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

	// Test 1: List tools - verify "ping" is registered
	t.Run("tools/list returns ping", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, result.Tools, 6) // ping + search + get_entity + list_labels + list_types + remember

		// Find ping tool
		var pingTool *mcp.Tool
		for _, tool := range result.Tools {
			if tool.Name == "ping" {
				pingTool = tool
				break
			}
		}
		require.NotNil(t, pingTool, "ping tool should exist")
		assert.Equal(t, "Test tool - responds with pong or echoes input", pingTool.Description)
	})

	// Test 2: Call ping without echo - should return "pong"
	t.Run("ping returns pong", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name:      "ping",
			Arguments: map[string]any{},
		}
		result, err := session.CallTool(ctx, params)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok, "content should be TextContent")
		assert.Equal(t, "pong", textContent.Text)
		assert.False(t, result.IsError)
	})

	// Test 3: Call ping with echo - should return echoed text
	t.Run("ping echoes input", func(t *testing.T) {
		params := &mcp.CallToolParams{
			Name:      "ping",
			Arguments: map[string]any{"echo": "hello world"},
		}
		result, err := session.CallTool(ctx, params)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok, "content should be TextContent")
		assert.Equal(t, "hello world", textContent.Text)
		assert.False(t, result.IsError)
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
