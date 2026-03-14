package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tools"
)

// --- contextInjectionMiddleware: inject doc refs + text attachments + session values ---

type contextInjectionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	docRefs  []string
	textAtts []models.ChatAttachment
	vaultID  string
	db       *db.Client
	emit     func(StreamEvent)
	injected bool
}

// BeforeAgent hydrates session values for FString interpolation in the instruction template.
func (m *contextInjectionMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	logger := logutil.FromCtx(ctx)

	var folderTree string
	folders, err := m.db.ListFolders(ctx, m.vaultID)
	if err != nil {
		logger.Warn("failed to list folders for system prompt", "vault_id", m.vaultID, "error", err)
	} else {
		folderTree = formatFolderTree(folders)
	}

	var labels string
	labelCounts, err := m.db.ListLabelsWithCounts(ctx, m.vaultID)
	if err != nil {
		logger.Warn("failed to list labels for system prompt", "vault_id", m.vaultID, "error", err)
	} else {
		labels = formatLabels(labelCounts)
	}

	vals := map[string]any{
		"FolderTree": folderTree,
		"Labels":     labels,
	}
	adk.AddSessionValues(ctx, vals)

	// Verify session values were stored — AddSessionValues silently no-ops if
	// the session context is nil (e.g. ADK session not initialized).
	if stored := adk.GetSessionValues(ctx); stored == nil {
		logger.Warn("session values not stored — ADK session may not be initialized; system prompt will contain raw template placeholders")
	}

	return ctx, runCtx, nil
}

// formatFolderTree renders a vault's folder structure as a code block, or "" if empty.
func formatFolderTree(folders []models.Folder) string {
	if len(folders) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nVault folder structure:\n```\n/\n")
	for _, f := range folders {
		depth := strings.Count(strings.Trim(f.Path, "/"), "/")
		indent := strings.Repeat("  ", depth)
		sb.WriteString(indent)
		sb.WriteString("├── ")
		// Escape curly braces to prevent FString interpolation of user-controlled folder names.
		sb.WriteString(strings.NewReplacer("{", "{{", "}", "}}").Replace(f.Name))
		sb.WriteString("/\n")
	}
	sb.WriteString("```")
	return sb.String()
}

// formatLabels renders vault labels with counts, or "" if empty.
func formatLabels(labelCounts []models.LabelCount) string {
	if len(labelCounts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nVault labels:\n")
	for _, lc := range labelCounts {
		// Escape curly braces to prevent FString interpolation of user-controlled label names.
		escaped := strings.NewReplacer("{", "{{", "}", "}}").Replace(lc.Label)
		fmt.Fprintf(&sb, "- %s (%d)\n", escaped, lc.Count)
	}
	return sb.String()
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

// --- tokenTrackingMiddleware: accumulate token usage across ReAct iterations ---

type tokenTrackingMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	mu    sync.Mutex
	usage TokenUsage
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

	m.mu.Lock()
	m.usage.InputTokens += int64(u.PromptTokens)
	m.usage.OutputTokens += int64(u.CompletionTokens)
	m.usage.FinalPromptTokens = int64(u.PromptTokens)
	m.mu.Unlock()

	return ctx, state, nil
}

// --- toolExecutionMiddleware: SSE tool events + recording ---

type toolExecutionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	service *Service
	req     *ChatRequest
	mu      sync.Mutex
	records []ToolRecord
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
			logger.Warn("failed to send tool start event", "tool", toolName, "error", err)
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

// failTool sends a ToolEndEvent with an error, records the failure, and
// returns the error as a string so the ReAct loop continues.
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
		logutil.FromCtx(ctx).Warn("failed to send tool end event", "tool", toolName, "error", err)
	}
}

// recordTool appends a ToolRecord to the middleware's records slice.
// Protected by mu because tools may execute in parallel.
func (m *toolExecutionMiddleware) recordTool(_ context.Context, call schema.ToolCall, result string, meta *tools.ToolResultMeta) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, ToolRecord{Call: call, Result: result, Meta: meta})
}
