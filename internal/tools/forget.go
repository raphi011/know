package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
)

// ForgetInput defines the input schema for the forget tool.
type ForgetInput struct {
	IDs     []string `json:"ids" jsonschema:"required,Entity IDs to delete"`
	Context string   `json:"context,omitempty" jsonschema:"Project namespace for name resolution"`
}

// ForgetResult is the response from the forget tool.
type ForgetResult struct {
	Deleted int    `json:"deleted"`
	Message string `json:"message"`
}

// NewForgetHandler creates the forget tool handler.
// Deletes entities by ID. Idempotent - non-existent IDs silently succeed.
func NewForgetHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[ForgetInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ForgetInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate: at least one ID required
		if len(input.IDs) == 0 {
			return ErrorResult("At least one ID is required", "Provide ids array with entity IDs to delete"), nil, nil
		}

		// Detect context for name resolution
		var entityContext *string
		if input.Context != "" {
			entityContext = &input.Context
		} else {
			entityContext = DetectContext(cfg)
		}

		ctxStr := ""
		if entityContext != nil {
			ctxStr = *entityContext
		}

		// Resolve IDs - normalize names to composite IDs
		resolvedIDs := make([]string, 0, len(input.IDs))
		for _, id := range input.IDs {
			// Strip "entity:" prefix if present
			id = strings.TrimPrefix(id, "entity:")

			// If looks like a name (no colon), resolve via context
			if !strings.Contains(id, ":") && ctxStr != "" {
				// Treat as entity name, generate composite ID
				id = generateEntityID(id, ctxStr)
			}

			resolvedIDs = append(resolvedIDs, id)
		}

		// Delete entities
		deleted, err := deps.DB.QueryDeleteEntity(ctx, resolvedIDs...)
		if err != nil {
			deps.Logger.Error("delete failed", "ids", resolvedIDs, "error", err)
			return ErrorResult("Failed to delete entities", "Database may be unavailable"), nil, nil
		}

		// Build result
		result := ForgetResult{
			Deleted: deleted,
			Message: fmt.Sprintf("Deleted %d entities", deleted),
		}

		// Format response as JSON
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("forget completed", "deleted", deleted, "requested", len(input.IDs))
		return TextResult(string(jsonBytes)), nil, nil
	}
}
