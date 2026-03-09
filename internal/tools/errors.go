package tools

// ToolError represents a user-correctable error from a tool execution,
// such as "document not found" or "document already exists". These should
// be returned to the MCP client as tool-level errors (IsError=true) rather
// than treated as infrastructure failures.
type ToolError struct {
	Message string
}

func (e *ToolError) Error() string { return e.Message }
