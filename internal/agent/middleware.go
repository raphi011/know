package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/diff"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/tools"
)

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

// --- tokenTrackingMiddleware: accumulate token usage in session values ---

type tokenTrackingMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

func (m *tokenTrackingMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if len(state.Messages) == 0 {
		return ctx, state, nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last.ResponseMeta == nil || last.ResponseMeta.Usage == nil {
		return ctx, state, nil
	}

	u := last.ResponseMeta.Usage

	// Retrieve current totals from session values
	var usage TokenUsage
	if v, ok := adk.GetSessionValue(ctx, sessionKeyTokenUsage); ok {
		if existing, ok := v.(*TokenUsage); ok {
			usage = *existing
		}
	}

	usage.InputTokens += int64(u.PromptTokens)
	usage.OutputTokens += int64(u.CompletionTokens)
	usage.FinalPromptTokens = int64(u.PromptTokens)

	adk.AddSessionValue(ctx, sessionKeyTokenUsage, &usage)

	return ctx, state, nil
}

// --- toolExecutionMiddleware: approval gating + SSE tool events ---

type toolExecutionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	service *Service
	req     *ChatRequest
	mu      sync.Mutex // protects recordTool's read-modify-write on session values
}

func (m *toolExecutionMiddleware) WrapInvokableToolCall(ctx context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		logger := logutil.FromCtx(ctx)
		toolName := tCtx.Name
		callID := tCtx.CallID

		// Parse input for display
		var inputMap map[string]any
		if jsonErr := json.Unmarshal([]byte(argumentsInJSON), &inputMap); jsonErr != nil {
			logger.Debug("tool arguments are not valid JSON object", "tool", toolName, "error", jsonErr)
			inputMap = map[string]any{"raw": argumentsInJSON}
		}

		if err := adk.SendEvent(ctx, &adk.AgentEvent{
			Output: &adk.AgentOutput{CustomizedOutput: &ToolStartEvent{
				CallID: callID,
				Tool:   toolName,
				Input:  inputMap,
			}},
		}); err != nil {
			logger.Debug("failed to send tool start event", "tool", toolName, "error", err)
		}

		// Reconstruct a schema.ToolCall for approval logic and persistence.
		call := schema.ToolCall{
			ID:       callID,
			Function: schema.FunctionCall{Name: toolName, Arguments: argumentsInJSON},
		}

		// Gate write tools on user approval
		if isWriteTool(toolName) && !m.req.AutoApprove && m.req.Approvals != nil {
			approvalReq, buildErr := m.service.buildApprovalRequest(ctx, m.req.VaultID, call)
			if buildErr != nil {
				return m.failTool(ctx, call, callID, toolName, fmt.Sprintf("error: %v", buildErr), nil)
			}

			if err := adk.SendEvent(ctx, &adk.AgentEvent{
				Output: &adk.AgentOutput{CustomizedOutput: &ApprovalRequiredEvent{
					CallID:  callID,
					Tool:    toolName,
					Request: approvalReq,
				}},
			}); err != nil {
				logger.Debug("failed to send approval required event", "tool", toolName, "error", err)
			}

			ch := m.req.Approvals.register(callID)

			var resp ApprovalResponse
			var ok bool
			select {
			case resp, ok = <-ch:
				if !ok {
					return m.failTool(ctx, call, callID, toolName, "error: approval cancelled", nil)
				}
			case <-ctx.Done():
				return m.failTool(ctx, call, callID, toolName, "error: request cancelled", nil)
			}

			if resp.Action == ApprovalReject {
				return m.failTool(ctx, call, callID, toolName, "User rejected the proposed changes.", nil)
			}

			if resp.Action == ApprovalApproveHunks {
				if approvalReq.Diff == nil {
					logger.Warn("approve_hunks received but no diff available", "tool", toolName, "call_id", callID)
					return m.failTool(ctx, call, callID, toolName, "error: partial approval not available for this operation", nil)
				}
				doc, docErr := m.service.db.GetDocumentByPath(ctx, m.req.VaultID, approvalReq.Path)
				if docErr != nil {
					logger.Warn("failed to retrieve document for partial approval", "path", approvalReq.Path, "error", docErr)
					return m.failTool(ctx, call, callID, toolName, fmt.Sprintf("error: retrieve document for partial approval: %v", docErr), nil)
				}
				if doc == nil {
					return m.failTool(ctx, call, callID, toolName, "error: document no longer exists at "+approvalReq.Path, nil)
				}
				merged, mergeErr := diff.ApplyHunks(doc.Content, approvalReq.Diff.Hunks, resp.HunkIndexes)
				if mergeErr != nil {
					return m.failTool(ctx, call, callID, toolName, fmt.Sprintf("error applying hunks: %v", mergeErr), nil)
				}
				inputMap["content"] = merged
				newArgs, marshalErr := json.Marshal(inputMap)
				if marshalErr != nil {
					return m.failTool(ctx, call, callID, toolName, fmt.Sprintf("error: marshal args: %v", marshalErr), nil)
				}
				argumentsInJSON = string(newArgs)
				call.Function.Arguments = argumentsInJSON
			} else if resp.Action != ApprovalApproveAll {
				logger.Warn("unexpected approval action, treating as rejection", "action", resp.Action, "tool", toolName, "call_id", callID)
				return m.failTool(ctx, call, callID, toolName, fmt.Sprintf("error: unexpected approval action: %s", resp.Action), nil)
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
			return m.failTool(ctx, call, callID, toolName, fmt.Sprintf("error: %v", err), meta)
		}

		m.sendToolEnd(ctx, callID, toolName, "", meta)
		m.recordTool(ctx, call, result, meta)
		return result, nil
	}, nil
}

// failTool sends a ToolEndEvent with an error, records the failure in session
// values, and returns the error as a string so the ReAct loop continues.
func (m *toolExecutionMiddleware) failTool(ctx context.Context, call schema.ToolCall, callID, toolName, errMsg string, meta *tools.ToolResultMeta) (string, error) {
	m.sendToolEnd(ctx, callID, toolName, errMsg, meta)
	m.recordTool(ctx, call, errMsg, meta)
	return errMsg, nil
}

// sendToolEnd emits a ToolEndEvent via adk.SendEvent.
func (m *toolExecutionMiddleware) sendToolEnd(ctx context.Context, callID, toolName, errMsg string, meta *tools.ToolResultMeta) {
	if err := adk.SendEvent(ctx, &adk.AgentEvent{
		Output: &adk.AgentOutput{CustomizedOutput: &ToolEndEvent{
			CallID: callID,
			Tool:   toolName,
			Meta:   meta,
			Error:  errMsg,
		}},
	}); err != nil {
		logutil.FromCtx(ctx).Debug("failed to send tool end event", "tool", toolName, "error", err)
	}
}

// recordTool appends a ToolRecord to the session values.
// Protected by mu because tools may execute in parallel.
func (m *toolExecutionMiddleware) recordTool(ctx context.Context, call schema.ToolCall, result string, meta *tools.ToolResultMeta) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record := ToolRecord{Call: call, Result: result, Meta: meta}

	var records []ToolRecord
	if v, ok := adk.GetSessionValue(ctx, sessionKeyToolRecords); ok {
		if existing, ok := v.([]ToolRecord); ok {
			records = existing
		}
	}
	records = append(records, record)
	adk.AddSessionValue(ctx, sessionKeyToolRecords, records)
}

// --- sessionDumpAgent: wraps an agent to emit RunCompleteEvent with session values ---

type sessionDumpAgent struct {
	adk.Agent
}

func (a *sessionDumpAgent) Run(ctx context.Context, input *adk.AgentInput,
	opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {

	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	inner := a.Agent.Run(ctx, input, opts...)

	go func() {
		defer gen.Close()
		defer func() {
			if r := recover(); r != nil {
				logutil.FromCtx(ctx).Error("panic in agent run", "recover", r)
				gen.Send(&adk.AgentEvent{Err: fmt.Errorf("agent panic: %v", r)})
			}
			// Always emit RunCompleteEvent so token usage and tool records are captured
			kvs := adk.GetSessionValues(ctx)
			gen.Send(&adk.AgentEvent{
				Output: &adk.AgentOutput{CustomizedOutput: &RunCompleteEvent{
					TokenUsage:  extractTokenUsage(kvs),
					ToolRecords: extractToolRecords(kvs),
				}},
			})
		}()

		for {
			event, ok := inner.Next()
			if !ok {
				break
			}
			gen.Send(event)
		}
	}()

	return iter
}

func extractTokenUsage(kvs map[string]any) TokenUsage {
	if v, ok := kvs[sessionKeyTokenUsage]; ok {
		if usage, ok := v.(*TokenUsage); ok {
			return *usage
		}
	}
	return TokenUsage{}
}

func extractToolRecords(kvs map[string]any) []ToolRecord {
	if v, ok := kvs[sessionKeyToolRecords]; ok {
		if records, ok := v.([]ToolRecord); ok {
			return records
		}
	}
	return nil
}
