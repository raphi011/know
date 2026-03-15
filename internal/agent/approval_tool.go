package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/diff"
	"github.com/raphi011/know/internal/logutil"
)

// approvalToolState is persisted in the checkpoint when a write tool is interrupted.
type approvalToolState struct {
	OriginalArgs string           // original JSON args passed to the tool
	Request      *ApprovalRequest // computed diff/content for display
}

// approvalToolWrapper wraps a write tool to use eino's interrupt/resume for approval.
// On first call it computes a diff, interrupts with the approval request, and waits
// for resume. On resume it either delegates to the inner tool (approved) or returns
// a rejection message.
type approvalToolWrapper struct {
	inner   tool.InvokableTool
	service *Service
	vaultID string
}

func (w *approvalToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.inner.Info(ctx)
}

func (w *approvalToolWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	logger := logutil.FromCtx(ctx)

	// Check if we were previously interrupted (resuming from checkpoint).
	wasInterrupted, hasState, state := tool.GetInterruptState[*approvalToolState](ctx)

	if !wasInterrupted {
		// First call — compute diff and interrupt.
		info, infoErr := w.inner.Info(ctx)
		if infoErr != nil {
			return "", fmt.Errorf("get tool info: %w", infoErr)
		}

		call := schema.ToolCall{
			Function: schema.FunctionCall{
				Name:      info.Name,
				Arguments: argumentsInJSON,
			},
		}

		// Determine the effective vault for approval.
		// If the tool args contain a "vault" field with "/" (remote), show
		// content-only approval since we can't compute diffs on remote vaults.
		isRemoteVault := false
		var vaultArg struct {
			Vault string `json:"vault"`
		}
		if err := json.Unmarshal([]byte(argumentsInJSON), &vaultArg); err != nil {
			logger.Warn("failed to parse vault arg for approval routing", "tool", info.Name, "error", err)
		} else if vaultArg.Vault != "" && strings.Contains(vaultArg.Vault, "/") {
			isRemoteVault = true
		}

		var approvalReq *ApprovalRequest
		if isRemoteVault {
			approvalReq = w.service.buildRemoteApprovalRequest(call)
		} else {
			var buildErr error
			approvalReq, buildErr = w.service.buildApprovalRequest(ctx, w.vaultID, call)
			if buildErr != nil {
				logger.Warn("failed to build approval request", "tool", info.Name, "error", buildErr)
				return fmt.Sprintf("error: %v", buildErr), nil
			}
		}

		saved := &approvalToolState{
			OriginalArgs: argumentsInJSON,
			Request:      approvalReq,
		}

		return "", tool.StatefulInterrupt(ctx, approvalReq, saved)
	}

	// We were interrupted — check if we're the resume target.
	isTarget, hasData, response := tool.GetResumeContext[*ApprovalResponse](ctx)

	if !isTarget {
		// Not our turn — a sibling tool was resumed. Re-interrupt with same state
		// to preserve our checkpoint for future approval.
		if hasState {
			return "", tool.StatefulInterrupt(ctx, state.Request, state)
		}
		logger.Warn("re-interrupting tool with no saved state")
		return "", tool.StatefulInterrupt(ctx, nil, nil)
	}

	if !hasState {
		logger.Warn("resume target but no saved state")
		return "error: approval state lost", nil
	}

	// Handle rejection.
	if hasData && response.Action == ApprovalReject {
		return "User rejected the proposed changes.", nil
	}

	// Handle partial approval (hunk selection).
	args := state.OriginalArgs
	if hasData && response.Action == ApprovalApproveHunks {
		if state.Request.Diff == nil {
			logger.Warn("approve_hunks received but no diff available")
			return "error: partial approval not available for this operation", nil
		}

		doc, docErr := w.service.db.GetFileByPath(ctx, w.vaultID, state.Request.Path)
		if docErr != nil {
			logger.Warn("failed to retrieve document for partial approval", "path", state.Request.Path, "error", docErr)
			return fmt.Sprintf("error: retrieve document for partial approval: %v", docErr), nil
		}
		if doc == nil {
			return "error: document no longer exists at " + state.Request.Path, nil
		}

		merged, mergeErr := diff.ApplyHunks(doc.Content, state.Request.Diff.Hunks, response.HunkIndexes)
		if mergeErr != nil {
			return fmt.Sprintf("error applying hunks: %v", mergeErr), nil
		}

		// Rewrite the content arg in the original JSON.
		var inputMap map[string]any
		if err := json.Unmarshal([]byte(args), &inputMap); err != nil {
			return fmt.Sprintf("error: parse args: %v", err), nil
		}
		inputMap["content"] = merged
		newArgs, err := json.Marshal(inputMap)
		if err != nil {
			return fmt.Sprintf("error: marshal args: %v", err), nil
		}
		args = string(newArgs)
	}

	// Approved (fully or partially) — delegate to inner tool.
	return w.inner.InvokableRun(ctx, args, opts...)
}

// wrapWriteToolsForApproval wraps write tools with approvalToolWrapper, leaving
// read-only tools unwrapped.
func wrapWriteToolsForApproval(ctx context.Context, allTools []tool.BaseTool, svc *Service, vaultID string) []tool.BaseTool {
	out := make([]tool.BaseTool, len(allTools))
	for i, t := range allTools {
		inv, ok := t.(tool.InvokableTool)
		if !ok {
			out[i] = t
			continue
		}
		info, err := inv.Info(ctx)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to get tool info for approval wrapping, leaving unwrapped", "error", err)
			out[i] = t
			continue
		}
		if !isWriteTool(info.Name) {
			out[i] = t
			continue
		}
		out[i] = &approvalToolWrapper{
			inner:   inv,
			service: svc,
			vaultID: vaultID,
		}
	}
	return out
}
