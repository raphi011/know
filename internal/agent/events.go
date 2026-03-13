package agent

import (
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/diff"
	"github.com/raphi011/knowhow/internal/tools"
)

func init() {
	// Register types for gob serialization so eino's checkpoint system can
	// persist/restore interrupt state and resume data.
	schema.RegisterName[*ApprovalRequest]("knowhow_approval_request")
	schema.RegisterName[*ApprovalResponse]("knowhow_approval_response")
	schema.RegisterName[*DiffPayload]("knowhow_diff_payload")
	schema.RegisterName[*approvalToolState]("knowhow_approval_tool_state")
	schema.RegisterName[diff.Hunk]("knowhow_diff_hunk")
	schema.RegisterName[diff.DiffLine]("knowhow_diff_line")
	schema.RegisterName[diff.DiffStats]("knowhow_diff_stats")
	schema.RegisterName[diff.DiffLineType]("knowhow_diff_line_type")
}

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
	Interrupted bool // true if the run was interrupted (pending approval)
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

// RunCompleteEvent is emitted by the sessionDumpAgent wrapper after the inner
// agent finishes. It carries accumulated token usage and tool records extracted
// from session values.
type RunCompleteEvent struct {
	TokenUsage  TokenUsage
	ToolRecords []ToolRecord
}
