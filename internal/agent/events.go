package agent

import (
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/tools"
)

// Session value keys for accumulating state across ReAct iterations.
const (
	sessionKeyTokenUsage  = "token_usage"
	sessionKeyToolRecords = "tool_records"
)

// TokenUsage holds cumulative token counts from an agentic generation run.
type TokenUsage struct {
	InputTokens       int64
	OutputTokens      int64
	FinalPromptTokens int64 // prompt tokens from the final iteration (context fill level)
}

// ToolRecord captures a tool call and its result for persistence.
type ToolRecord struct {
	Call   schema.ToolCall
	Result string
	Meta   *tools.ToolResultMeta
}

// AgentResult is the accumulated output from a single agent run.
type AgentResult struct {
	Answer      string
	TokenUsage  TokenUsage
	ToolRecords []ToolRecord
}

// --- Custom events emitted via adk.SendEvent ---

// ToolStartEvent is emitted when a tool call begins.
type ToolStartEvent struct {
	CallID string
	Tool   string
	Input  map[string]any
}

// ToolEndEvent is emitted when a tool call completes.
type ToolEndEvent struct {
	CallID string
	Tool   string
	Meta   *tools.ToolResultMeta
	Error  string // empty on success
}

// ApprovalRequiredEvent is emitted when a write tool needs user approval.
type ApprovalRequiredEvent struct {
	CallID  string
	Tool    string
	Request *ApprovalRequest
}

// RunCompleteEvent is emitted by the sessionDumpAgent wrapper after the inner
// agent finishes. It carries accumulated token usage and tool records extracted
// from session values.
type RunCompleteEvent struct {
	TokenUsage  TokenUsage
	ToolRecords []ToolRecord
}
