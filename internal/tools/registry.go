package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
)

// RegisterAll registers all tools with the MCP server.
// This is called from main after server creation but before Run().
func RegisterAll(server *mcp.Server, deps *Dependencies, cfg *config.Config) {
	// Ping tool - test/placeholder
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "Test tool - responds with pong or echoes input",
	}, NewPingHandler(deps))

	// Search tool - hybrid BM25 + vector search with RRF fusion
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the knowledge graph using hybrid BM25 + vector search with RRF fusion",
	}, NewSearchHandler(deps, cfg))
}
