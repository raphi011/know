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

	// Search episodes tool - semantic search with time filtering
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_episodes",
		Description: "Search episodic memories by semantic content with optional time range filtering. Use to find past conversations or experiences.",
	}, NewSearchEpisodesHandler(deps, cfg))

	// Create procedure tool - store procedural memories
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_procedure",
		Description: "Store a step-by-step procedure (workflow, process, how-to) with ordered steps and auto-generated embedding",
	}, NewCreateProcedureHandler(deps, cfg))

	// Get procedure tool - retrieve by ID
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_procedure",
		Description: "Retrieve a procedural memory by its ID with all steps",
	}, NewGetProcedureHandler(deps))

	// Delete procedure tool - remove by ID
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_procedure",
		Description: "Delete a procedural memory by its ID",
	}, NewDeleteProcedureHandler(deps))

	// Search procedures tool - semantic search with filtering
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_procedures",
		Description: "Search procedural memories by semantic content. Use to find relevant workflows, processes, or how-to guides.",
	}, NewSearchProceduresHandler(deps, cfg))

	// List procedures tool - enumerate all procedures
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_procedures",
		Description: "List all stored procedures with optional context filtering. Returns summaries, use get_procedure for full steps.",
	}, NewListProceduresHandler(deps, cfg))

	// Reflect tool - memory maintenance operations
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reflect",
		Description: "Maintain memory store health: 'decay' reduces importance of unused entities, 'similar' identifies potential duplicates for manual review",
	}, NewReflectHandler(deps, cfg))
}
