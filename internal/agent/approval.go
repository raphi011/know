package agent

import (
	"fmt"
	"sync"

	"github.com/raphi011/knowhow/internal/diff"
)

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
	CallID      string         `json:"callId"`
	Action      ApprovalAction `json:"action"`
	HunkIndexes []int          `json:"hunkIndexes,omitempty"`
}

// approvalRegistry manages pending approval channels keyed by call ID.
type approvalRegistry struct {
	mu      sync.Mutex
	pending map[string]chan ApprovalResponse
}

func newApprovalRegistry() *approvalRegistry {
	return &approvalRegistry{pending: make(map[string]chan ApprovalResponse)}
}

// register creates a channel for the given call ID and returns it.
func (r *approvalRegistry) register(callID string) <-chan ApprovalResponse {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan ApprovalResponse, 1)
	r.pending[callID] = ch
	return ch
}

// resolve sends a response to the waiting goroutine and removes the entry.
func (r *approvalRegistry) resolve(resp ApprovalResponse) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.pending[resp.CallID]
	if !ok {
		return fmt.Errorf("no pending approval for call %q", resp.CallID)
	}
	ch <- resp
	delete(r.pending, resp.CallID)
	return nil
}

// cancel removes all pending approvals (e.g. on context cancellation).
func (r *approvalRegistry) cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, ch := range r.pending {
		close(ch)
		delete(r.pending, id)
	}
}
