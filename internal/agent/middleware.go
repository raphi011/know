package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
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

// --- toolExecutionMiddleware: SSE tool events + recording ---

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

		call := schema.ToolCall{
			ID:       callID,
			Function: schema.FunctionCall{Name: toolName, Arguments: argumentsInJSON},
		}

		// Inject context for result metadata collection
		ctx = tools.WithResultMeta(ctx)
		opts = append(opts, tools.WithVaultID(m.req.VaultID))

		result, err := endpoint(ctx, argumentsInJSON, opts...)

		// If the tool returned an interrupt error (from approvalToolWrapper), emit
		// a tool_end so clients see a paired start/end, then let the error
		// propagate up to the compose engine.
		if _, isInterrupt := compose.IsInterruptRerunError(err); isInterrupt {
			m.sendToolEnd(ctx, callID, toolName, "interrupted", nil)
			return "", err
		}

		meta := tools.ResultMeta(ctx)
		if err != nil {
			logger.Warn("tool execution failed", "tool", toolName, "error", err)
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
