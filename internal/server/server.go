// Package server provides the MCP server wrapper with lifecycle management.
package server

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with dependencies and lifecycle management.
type Server struct {
	mcp    *mcp.Server
	logger *slog.Logger
}

// New creates a new MCP server with the given version and logger.
func New(version string, logger *slog.Logger) *Server {
	impl := &mcp.Implementation{
		Name:    "memcp",
		Version: version,
	}

	mcpServer := mcp.NewServer(impl, nil)

	return &Server{
		mcp:    mcpServer,
		logger: logger,
	}
}

// Run starts the server on stdio transport and blocks until disconnect or context cancellation.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting MCP server", "transport", "stdio")
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// MCPServer returns the underlying MCP server for tool registration.
func (s *Server) MCPServer() *mcp.Server {
	return s.mcp
}

// Setup adds middleware to the server (logging, error handling).
func (s *Server) Setup() {
	s.mcp.AddReceivingMiddleware(LoggingMiddleware(s.logger))
}
