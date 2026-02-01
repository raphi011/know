package tools

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrorResult creates a tool error result with optional recovery hint.
// If hint is non-empty, formats as "{msg}. {hint}".
// Returns IsError=true so LLM can see the error and self-correct.
func ErrorResult(msg, hint string) *mcp.CallToolResult {
	text := msg
	if hint != "" {
		text = msg + ". " + hint
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
		IsError: true,
	}
}

// TextResult creates a success result with text content.
func TextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// FormatResults joins items with newlines for list output.
func FormatResults(items []string) string {
	return strings.Join(items, "\n")
}
