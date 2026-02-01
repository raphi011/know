package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAll registers all tools with the MCP server.
// This is called from main after server creation but before Run().
func RegisterAll(server *mcp.Server, deps *Dependencies) {
	// Ping tool - test/placeholder
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "Test tool - responds with pong or echoes input",
	}, NewPingHandler(deps))
}
