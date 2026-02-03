//go:build integration

package server_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger creates a logger that writes to stderr for test visibility.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestServerCreation(t *testing.T) {
	logger := testLogger()

	srv := server.New("test-version", logger)
	require.NotNil(t, srv, "server should not be nil")

	mcpSrv := srv.MCPServer()
	require.NotNil(t, mcpSrv, "underlying MCP server should not be nil")
}

func TestServerSetup(t *testing.T) {
	logger := testLogger()

	srv := server.New("test-version", logger)
	require.NotNil(t, srv)

	// Setup should not panic
	srv.Setup()
}

func TestServerWithInMemoryTransport(t *testing.T) {
	logger := testLogger()

	// Create server
	srv := server.New("0.1.0-test", logger)
	srv.Setup()

	// Create in-memory transports
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.MCPServer().Run(ctx, serverTransport)
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

	// Verify server info from initialize response
	initResult := session.InitializeResult()
	require.NotNil(t, initResult, "initialize result should not be nil")
	assert.Equal(t, "memcp", initResult.ServerInfo.Name)
	assert.Equal(t, "0.1.0-test", initResult.ServerInfo.Version)

	// List tools (should be empty, no tools registered yet)
	toolsResult, err := session.ListTools(ctx, nil)
	require.NoError(t, err, "ListTools should succeed")
	assert.Empty(t, toolsResult.Tools, "should have no tools registered")

	// Close session
	err = session.Close()
	assert.NoError(t, err, "session close should not error")

	// Cancel context to stop server
	cancel()

	// Wait for server to stop
	select {
	case err := <-serverErr:
		// EOF is expected when client disconnects
		if err != nil {
			t.Logf("server stopped with: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestServerRespondsToMultipleRequests(t *testing.T) {
	logger := testLogger()

	srv := server.New("0.1.0-test", logger)
	srv.Setup()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run server
	go func() {
		_ = srv.MCPServer().Run(ctx, serverTransport)
	}()

	time.Sleep(50 * time.Millisecond)

	// Connect client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	// Make multiple requests
	for i := 0; i < 3; i++ {
		_, err := session.ListTools(ctx, nil)
		require.NoError(t, err, "request %d should succeed", i)
	}

	cancel()
}
