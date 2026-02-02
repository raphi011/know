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

	// Get entity tool - retrieve by ID
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_entity",
		Description: "Retrieve an entity by its ID with full details",
	}, NewGetEntityHandler(deps))

	// List labels tool - taxonomy navigation
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_labels",
		Description: "List all unique labels with entity counts",
	}, NewListLabelsHandler(deps, cfg))

	// List types tool - entity type counts
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_types",
		Description: "List all entity types with counts",
	}, NewListTypesHandler(deps, cfg))

	// Remember tool - store entities with auto-generated embeddings
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remember",
		Description: "Store entities in the knowledge graph with auto-generated embeddings",
	}, NewRememberHandler(deps, cfg))

	// Forget tool - delete entities from the knowledge graph
	mcp.AddTool(server, &mcp.Tool{
		Name:        "forget",
		Description: "Delete entities from the knowledge graph by ID",
	}, NewForgetHandler(deps, cfg))

	// Traverse tool - explore graph neighbors
	mcp.AddTool(server, &mcp.Tool{
		Name:        "traverse",
		Description: "Explore how stored knowledge connects to other knowledge. Use when the user asks 'what's related to...', 'how does X connect to Y', or wants to understand context around a topic.",
	}, NewTraverseHandler(deps))

	// Find path tool - shortest path between entities
	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_path",
		Description: "Find how two pieces of knowledge are connected through intermediate relationships. Use when the user asks 'how is X related to Y' or wants to trace connections between concepts.",
	}, NewFindPathHandler(deps))

	// Add episode tool - store episodic memories
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_episode",
		Description: "Store a conversation or experience as an episodic memory with auto-generated timestamp and embedding",
	}, NewAddEpisodeHandler(deps, cfg))

	// Get episode tool - retrieve by ID
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_episode",
		Description: "Retrieve an episodic memory by its ID with full content",
	}, NewGetEpisodeHandler(deps))

	// Delete episode tool - remove by ID
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_episode",
		Description: "Delete an episodic memory by its ID",
	}, NewDeleteEpisodeHandler(deps))
}
