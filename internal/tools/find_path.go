package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FindPathInput defines the input schema for the find_path tool.
type FindPathInput struct {
	From     string `json:"from" jsonschema:"required,Starting entity ID"`
	To       string `json:"to" jsonschema:"required,Target entity ID"`
	MaxDepth int    `json:"max_depth,omitempty" jsonschema:"Maximum path length 1-20 (default 5)"`
}

// FindPathResult is the response from the find_path tool.
type FindPathResult struct {
	PathFound bool     `json:"path_found"`
	Path      []string `json:"path,omitempty"`
	Length    int      `json:"length,omitempty"`
	Message   string   `json:"message,omitempty"`
}

// NewFindPathHandler creates the find_path tool handler.
// Finds shortest path between two entities.
func NewFindPathHandler(deps *Dependencies) mcp.ToolHandlerFor[FindPathInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input FindPathInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate from
		if input.From == "" {
			return ErrorResult("from cannot be empty", "Provide starting entity ID"), nil, nil
		}

		// Validate to
		if input.To == "" {
			return ErrorResult("to cannot be empty", "Provide target entity ID"), nil, nil
		}

		// Set and validate max_depth
		maxDepth := input.MaxDepth
		if maxDepth <= 0 {
			maxDepth = 5
		}
		if maxDepth > 20 {
			return ErrorResult("max_depth must be between 1 and 20", "Reduce max_depth value"), nil, nil
		}

		// Execute path finding
		path, err := deps.DB.QueryFindPath(ctx, input.From, input.To, maxDepth)
		if err != nil {
			deps.Logger.Error("find_path failed", "from", input.From, "to", input.To, "error", err)
			return ErrorResult("Path finding failed", "Database may be unavailable"), nil, nil
		}

		// Handle no path found
		if path == nil || len(path) == 0 {
			result := FindPathResult{
				PathFound: false,
				Message:   fmt.Sprintf("No path found between %s and %s within %d hops", input.From, input.To, maxDepth),
			}
			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			deps.Logger.Info("find_path: no path", "from", input.From, "to", input.To)
			return TextResult(string(jsonBytes)), nil, nil
		}

		// Build path IDs from entities
		pathIDs := make([]string, len(path))
		for i, e := range path {
			pathIDs[i] = e.ID.String()
		}

		result := FindPathResult{
			PathFound: true,
			Path:      pathIDs,
			Length:    len(path),
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		deps.Logger.Info("find_path completed", "from", input.From, "to", input.To, "length", len(path))
		return TextResult(string(jsonBytes)), nil, nil
	}
}
