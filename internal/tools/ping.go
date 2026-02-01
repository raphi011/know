package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PingInput defines the input schema for the ping tool.
type PingInput struct {
	Echo string `json:"echo,omitempty" jsonschema:"Text to echo back"`
}

// NewPingHandler creates a ping tool handler with injected dependencies.
// This is a simple test tool that responds with "pong" or echoes input.
func NewPingHandler(deps *Dependencies) mcp.ToolHandlerFor[PingInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input PingInput) (*mcp.CallToolResult, any, error) {
		// Log the call if logger is available
		if deps != nil && deps.Logger != nil {
			deps.Logger.Debug("ping tool called", "echo", input.Echo)
		}

		// Return echo text if provided, otherwise "pong"
		if input.Echo != "" {
			return TextResult(input.Echo), nil, nil
		}
		return TextResult("pong"), nil, nil
	}
}
