package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

// SearchInput defines the input schema for the search tool.
type SearchInput struct {
	Query   string   `json:"query" jsonschema:"required,The search query text"`
	Labels  []string `json:"labels,omitempty" jsonschema:"Optional label filter (entities must have at least one)"`
	Limit   int      `json:"limit,omitempty" jsonschema:"Max results 1-100, default 10"`
	Context string   `json:"context,omitempty" jsonschema:"Project namespace filter"`
}

// NewSearchHandler creates the search tool handler.
// Uses hybrid BM25 + vector search with RRF fusion.
func NewSearchHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[SearchInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Input validation
		if input.Query == "" {
			return ErrorResult("Query cannot be empty", "Provide a search query"), nil, nil
		}

		// Set defaults and validate limit
		limit := input.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			return ErrorResult("Limit must be 1-100", "Reduce limit value"), nil, nil
		}

		// Generate embedding for query
		embedding, err := deps.Embedder.Embed(ctx, input.Query)
		if err != nil {
			deps.Logger.Error("embedding failed", "error", err)
			return ErrorResult("Failed to generate query embedding", "Check Ollama connection"), nil, nil
		}

		// Detect context: explicit > config > nil
		var contextPtr *string
		if input.Context != "" {
			contextPtr = &input.Context
		} else {
			contextPtr = DetectContext(cfg)
		}

		// Execute hybrid search
		entities, err := deps.DB.QueryHybridSearch(ctx, input.Query, embedding, input.Labels, limit, contextPtr)
		if err != nil {
			deps.Logger.Error("search failed", "error", err)
			return ErrorResult("Search failed", "Database may be unavailable"), nil, nil
		}

		// Update access tracking for each result
		for _, e := range entities {
			if updateErr := deps.DB.QueryUpdateAccess(ctx, extractID(e.ID)); updateErr != nil {
				deps.Logger.Warn("failed to update access", "id", e.ID, "error", updateErr)
			}
		}

		// Format response as JSON
		result := models.SearchResult{
			Entities: entities,
			Count:    len(entities),
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		// Log completion (truncate query to 30 chars)
		queryLog := input.Query
		if len(queryLog) > 30 {
			queryLog = queryLog[:30] + "..."
		}
		deps.Logger.Info("search completed", "query", queryLog, "results", len(entities))

		return TextResult(string(jsonBytes)), nil, nil
	}
}
