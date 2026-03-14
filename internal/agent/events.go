package agent

import (
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/diff"
	"github.com/raphi011/know/internal/tools"
)

func init() {
	// Register types for gob serialization so eino's checkpoint system can
	// persist/restore interrupt state and resume data.
	schema.RegisterName[*ApprovalRequest]("know_approval_request")
	schema.RegisterName[*ApprovalResponse]("know_approval_response")
	schema.RegisterName[*DiffPayload]("know_diff_payload")
	schema.RegisterName[*approvalToolState]("know_approval_tool_state")
	schema.RegisterName[diff.Hunk]("know_diff_hunk")
	schema.RegisterName[diff.DiffLine]("know_diff_line")
	schema.RegisterName[diff.DiffStats]("know_diff_stats")
	schema.RegisterName[diff.DiffLineType]("know_diff_line_type")
}

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
