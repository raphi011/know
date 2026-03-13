package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/diff"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/tools"
)

// toolCallRecord captures a tool call and its result for later persistence.
type toolCallRecord struct {
	call   schema.ToolCall
	result string
	meta   *tools.ToolResultMeta
}

// --- contextInjectionMiddleware: inject doc refs + text attachments ---

type contextInjectionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	docRefs  []string
	textAtts []models.ChatAttachment
	vaultID  string
	db       *db.Client
	emit     func(StreamEvent)
	injected bool
}

func (m *contextInjectionMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if m.injected {
		return ctx, state, nil
	}
	m.injected = true

	var contextMsgs []*schema.Message

	if len(m.docRefs) > 0 {
		var refContext strings.Builder
		for _, ref := range m.docRefs {
			doc, docErr := m.db.GetDocumentByPath(ctx, m.vaultID, ref)
			if docErr != nil {
				logutil.FromCtx(ctx).Warn("failed to read referenced doc", "path", ref, "error", docErr)
				m.emit(StreamEvent{Type: "error", Content: fmt.Sprintf("could not read referenced document: %s", ref)})
				continue
			}
			if doc != nil {
				fmt.Fprintf(&refContext, "\n--- Document: %s ---\n%s\n", doc.Path, doc.ContentBody)
			}
		}
		if refContext.Len() > 0 {
			contextMsgs = append(contextMsgs,
				&schema.Message{Role: schema.User, Content: "Referenced documents:\n" + refContext.String()},
				&schema.Message{Role: schema.Assistant, Content: "I'll use these referenced documents to help answer your question."},
			)
		}
	}

	if len(m.textAtts) > 0 {
		var fileContext strings.Builder
		for _, att := range m.textAtts {
			fmt.Fprintf(&fileContext, "\n--- File: %s ---\n```%s\n%s\n```\n", att.Path, att.Language, att.Content)
		}
		contextMsgs = append(contextMsgs,
			&schema.Message{Role: schema.User, Content: "Attached local files:\n" + fileContext.String()},
			&schema.Message{Role: schema.Assistant, Content: "I'll use these attached files to help answer your question."},
		)
	}

	if len(contextMsgs) == 0 {
		return ctx, state, nil
	}

	// Insert context messages before the last user message.
	msgs := state.Messages
	if len(msgs) < 2 {
		// Not enough messages — prepend context at start of message list.
		state.Messages = append(contextMsgs, msgs...)
		return ctx, state, nil
	}
	insertAt := len(msgs) - 1
	result := make([]*schema.Message, 0, len(msgs)+len(contextMsgs))
	result = append(result, msgs[:insertAt]...)
	result = append(result, contextMsgs...)
	result = append(result, msgs[insertAt:]...)
	state.Messages = result

	return ctx, state, nil
}

// --- tokenTrackingMiddleware: accumulate token usage ---

type tokenTrackingMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	usage *llm.TokenUsage
}

func (m *tokenTrackingMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if len(state.Messages) == 0 {
		return ctx, state, nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last.ResponseMeta != nil && last.ResponseMeta.Usage != nil {
		u := last.ResponseMeta.Usage
		m.usage.InputTokens += int64(u.PromptTokens)
		m.usage.OutputTokens += int64(u.CompletionTokens)
		m.usage.FinalPromptTokens = int64(u.PromptTokens)
	}
	return ctx, state, nil
}

// --- toolExecutionMiddleware: approval gating + SSE tool events ---

type toolExecutionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	service     *Service
	req         *ChatRequest
	emit        func(StreamEvent)
	toolRecords *[]toolCallRecord
}

func (m *toolExecutionMiddleware) WrapInvokableToolCall(ctx context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		logger := logutil.FromCtx(ctx)
		toolName := tCtx.Name
		callID := tCtx.CallID

		// Parse input for SSE event
		var inputMap map[string]any
		if jsonErr := json.Unmarshal([]byte(argumentsInJSON), &inputMap); jsonErr != nil {
			logger.Debug("tool arguments are not valid JSON object", "tool", toolName, "error", jsonErr)
			inputMap = map[string]any{"raw": argumentsInJSON}
		}
		m.emit(StreamEvent{Type: "tool_start", CallID: callID, Tool: toolName, Input: inputMap})

		// Reconstruct a schema.ToolCall for approval logic and persistence.
		call := schema.ToolCall{
			ID:       callID,
			Function: schema.FunctionCall{Name: toolName, Arguments: argumentsInJSON},
		}

		// Gate write tools on user approval
		if isWriteTool(toolName) && !m.req.AutoApprove && m.req.Approvals != nil {
			approvalReq, buildErr := m.service.buildApprovalRequest(ctx, m.req.VaultID, call)
			if buildErr != nil {
				return m.failTool(call, callID, toolName, fmt.Sprintf("error: %v", buildErr), nil)
			}

			m.emit(StreamEvent{Type: "tool_approval_required", CallID: callID, Tool: toolName, Approval: approvalReq})
			ch := m.req.Approvals.register(callID)

			var resp ApprovalResponse
			var ok bool
			select {
			case resp, ok = <-ch:
				if !ok {
					return m.failTool(call, callID, toolName, "error: approval cancelled", nil)
				}
			case <-ctx.Done():
				return m.failTool(call, callID, toolName, "error: request cancelled", nil)
			}

			if resp.Action == ApprovalReject {
				return m.failTool(call, callID, toolName, "User rejected the proposed changes.", nil)
			}

			if resp.Action == ApprovalApproveHunks {
				if approvalReq.Diff == nil {
					logger.Warn("approve_hunks received but no diff available", "tool", toolName, "call_id", callID)
					return m.failTool(call, callID, toolName, "error: partial approval not available for this operation", nil)
				}
				doc, docErr := m.service.db.GetDocumentByPath(ctx, m.req.VaultID, approvalReq.Path)
				if docErr != nil {
					logger.Warn("failed to retrieve document for partial approval", "path", approvalReq.Path, "error", docErr)
					return m.failTool(call, callID, toolName, fmt.Sprintf("error: retrieve document for partial approval: %v", docErr), nil)
				}
				if doc == nil {
					return m.failTool(call, callID, toolName, "error: document no longer exists at "+approvalReq.Path, nil)
				}
				merged, mergeErr := diff.ApplyHunks(doc.Content, approvalReq.Diff.Hunks, resp.HunkIndexes)
				if mergeErr != nil {
					return m.failTool(call, callID, toolName, fmt.Sprintf("error applying hunks: %v", mergeErr), nil)
				}
				inputMap["content"] = merged
				newArgs, marshalErr := json.Marshal(inputMap)
				if marshalErr != nil {
					return m.failTool(call, callID, toolName, fmt.Sprintf("error: marshal args: %v", marshalErr), nil)
				}
				argumentsInJSON = string(newArgs)
				call.Function.Arguments = argumentsInJSON
			} else if resp.Action != ApprovalApproveAll {
				logger.Warn("unexpected approval action, treating as rejection", "action", resp.Action, "tool", toolName, "call_id", callID)
				return m.failTool(call, callID, toolName, fmt.Sprintf("error: unexpected approval action: %s", resp.Action), nil)
			}
			// ApprovalApproveAll falls through to normal execution
		}

		// Inject context for result metadata collection
		ctx = tools.WithResultMeta(ctx)
		opts = append(opts, tools.WithVaultID(m.req.VaultID))

		result, err := endpoint(ctx, argumentsInJSON, opts...)

		meta := tools.ResultMeta(ctx)
		if err != nil {
			logger.Warn("tool execution failed", "tool", toolName, "error", err)
			// Return error as string so ReAct loop continues
			return m.failTool(call, callID, toolName, fmt.Sprintf("error: %v", err), meta)
		}

		m.emit(StreamEvent{Type: "tool_end", CallID: callID, Tool: toolName, Meta: meta})
		*m.toolRecords = append(*m.toolRecords, toolCallRecord{call: call, result: result, meta: meta})
		return result, nil
	}, nil
}

// failTool emits a tool_end event, records the failure, and returns the error
// message as a string result so the ReAct loop can continue.
func (m *toolExecutionMiddleware) failTool(call schema.ToolCall, callID, toolName, errMsg string, meta *tools.ToolResultMeta) (string, error) {
	m.emit(StreamEvent{Type: "tool_end", CallID: callID, Tool: toolName, Content: errMsg})
	*m.toolRecords = append(*m.toolRecords, toolCallRecord{call: call, result: errMsg, meta: meta})
	return errMsg, nil
}
