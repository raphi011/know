package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/db"
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

// GetEntityInput defines the input schema for the get_entity tool.
type GetEntityInput struct {
	ID string `json:"id" jsonschema:"required,The entity ID to retrieve"`
}

// NewGetEntityHandler creates the get_entity tool handler.
// Retrieves an entity by its ID with full details.
func NewGetEntityHandler(deps *Dependencies) mcp.ToolHandlerFor[GetEntityInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetEntityInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Input validation
		if input.ID == "" {
			return ErrorResult("ID cannot be empty", "Provide an entity ID"), nil, nil
		}

		// Extract ID (handle "entity:xxx" or "xxx")
		id := extractID(input.ID)

		// Query entity
		entity, err := deps.DB.QueryGetEntity(ctx, id)
		if err != nil {
			deps.Logger.Error("get_entity failed", "id", id, "error", err)
			return ErrorResult("Failed to retrieve entity", "Database may be unavailable"), nil, nil
		}

		// Handle not found
		if entity == nil {
			return ErrorResult(fmt.Sprintf("Entity not found: %s", id), "Use search to find valid IDs"), nil, nil
		}

		// Update access tracking
		if updateErr := deps.DB.QueryUpdateAccess(ctx, id); updateErr != nil {
			deps.Logger.Warn("failed to update access", "id", id, "error", updateErr)
		}

		// Format as JSON
		jsonBytes, _ := json.MarshalIndent(entity, "", "  ")

		deps.Logger.Info("get_entity completed", "id", id)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// ListLabelsInput defines the input schema for the list_labels tool.
type ListLabelsInput struct {
	Context string `json:"context,omitempty" jsonschema:"Optional project namespace filter"`
}

// NewListLabelsHandler creates the list_labels tool handler.
// Returns unique labels with entity counts.
func NewListLabelsHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[ListLabelsInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ListLabelsInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Detect context: explicit > config > nil
		var contextPtr *string
		if input.Context != "" {
			contextPtr = &input.Context
		} else {
			contextPtr = DetectContext(cfg)
		}

		// Query labels
		labels, err := deps.DB.QueryListLabels(ctx, contextPtr)
		if err != nil {
			deps.Logger.Error("list_labels failed", "error", err)
			return ErrorResult("Failed to list labels", "Database may be unavailable"), nil, nil
		}

		// Format as JSON
		result := struct {
			Labels []db.LabelCount `json:"labels"`
			Count  int             `json:"count"`
		}{
			Labels: labels,
			Count:  len(labels),
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("list_labels completed", "count", len(labels))
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// ListTypesInput defines the input schema for the list_types tool.
type ListTypesInput struct {
	Context string `json:"context,omitempty" jsonschema:"Optional project namespace filter"`
}

// NewListTypesHandler creates the list_types tool handler.
// Returns entity types with counts.
func NewListTypesHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[ListTypesInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ListTypesInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Detect context: explicit > config > nil
		var contextPtr *string
		if input.Context != "" {
			contextPtr = &input.Context
		} else {
			contextPtr = DetectContext(cfg)
		}

		// Query types
		types, err := deps.DB.QueryListTypes(ctx, contextPtr)
		if err != nil {
			deps.Logger.Error("list_types failed", "error", err)
			return ErrorResult("Failed to list types", "Database may be unavailable"), nil, nil
		}

		// Format as JSON
		result := struct {
			Types []db.TypeCount `json:"types"`
			Count int            `json:"count"`
		}{
			Types: types,
			Count: len(types),
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("list_types completed", "count", len(types))
		return TextResult(string(jsonBytes)), nil, nil
	}
}
