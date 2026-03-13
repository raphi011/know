package agent

import "github.com/raphi011/knowhow/internal/diff"

// ApprovalAction is the user's decision on a tool approval request.
type ApprovalAction string

const (
	ApprovalApproveAll   ApprovalAction = "approve_all"
	ApprovalApproveHunks ApprovalAction = "approve_hunks"
	ApprovalReject       ApprovalAction = "reject"
)

// ApprovalRequest is emitted via SSE when a write tool needs user approval.
type ApprovalRequest struct {
	CallID  string       `json:"callId"`
	Tool    string       `json:"tool"`
	Path    string       `json:"path"`
	IsNew   bool         `json:"isNew"`
	Diff    *DiffPayload `json:"diff,omitempty"`
	Content string       `json:"content,omitempty"` // full content for new documents
}

// DiffPayload contains the computed diff for an edit.
type DiffPayload struct {
	Hunks []diff.Hunk    `json:"hunks"`
	Stats diff.DiffStats `json:"stats"`
}

// ApprovalResponse is sent by the user to approve or reject a tool call.
type ApprovalResponse struct {
	Action      ApprovalAction `json:"action"`
	HunkIndexes []int          `json:"hunkIndexes,omitempty"`
}
