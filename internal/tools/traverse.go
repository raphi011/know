package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TraverseInput defines the input schema for the traverse tool.
type TraverseInput struct {
	Start         string   `json:"start" jsonschema:"required,Entity ID to start traversal from"`
	Depth         int      `json:"depth,omitempty" jsonschema:"Traversal depth 1-10 (default 2)"`
	RelationTypes []string `json:"relation_types,omitempty" jsonschema:"Filter by relation types"`
}

// NewTraverseHandler creates the traverse tool handler.
// Explores neighbors up to specified depth from starting entity.
func NewTraverseHandler(deps *Dependencies) mcp.ToolHandlerFor[TraverseInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input TraverseInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate start
		if input.Start == "" {
			return ErrorResult("start cannot be empty", "Provide entity ID to start traversal"), nil, nil
		}

		// Set and validate depth
		depth := input.Depth
		if depth <= 0 {
			depth = 2
		}
		if depth > 10 {
			return ErrorResult("depth must be between 1 and 10", "Reduce depth value"), nil, nil
		}

		// Execute traversal
		results, err := deps.DB.QueryTraverse(ctx, input.Start, depth, input.RelationTypes)
		if err != nil {
			deps.Logger.Error("traverse failed", "start", input.Start, "error", err)
			return ErrorResult("Traversal failed", "Database may be unavailable"), nil, nil
		}

		// Handle empty result (entity not found)
		if len(results) == 0 {
			return ErrorResult(
				fmt.Sprintf("Entity not found: %s", input.Start),
				"Check entity ID exists",
			), nil, nil
		}

		// Format response
		jsonBytes, _ := json.MarshalIndent(results[0], "", "  ")
		deps.Logger.Info("traverse completed", "start", input.Start, "depth", depth, "connected", len(results[0].Connected))
		return TextResult(string(jsonBytes)), nil, nil
	}
}
