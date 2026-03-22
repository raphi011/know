package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/apify"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/diff"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/tools"
)

// StreamEvent is sent to the client via SSE.
type StreamEvent struct {
	Type              string                `json:"type"` // "text" | "tool_start" | "tool_end" | "interrupted" | "msg_start" | "msg_end" | "conv_id" | "error"
	Content           string                `json:"content,omitempty"`
	ConvID            string                `json:"convId,omitempty"`
	MsgID             string                `json:"msgId,omitempty"`
	CallID            string                `json:"callId,omitempty"`
	InterruptID       string                `json:"interruptId,omitempty"`
	Tool              string                `json:"tool,omitempty"`
	Input             map[string]any        `json:"input,omitempty"`
	Meta              *tools.ToolResultMeta `json:"meta,omitempty"`
	Approval          *ApprovalRequest      `json:"approval,omitempty"`
	InputTokens       int64                 `json:"inputTokens,omitempty"`
	OutputTokens      int64                 `json:"outputTokens,omitempty"`
	ContextWindowMax  int                   `json:"contextWindowMax,omitempty"`
	ContextWindowUsed int64                 `json:"contextWindowUsed,omitempty"`
}

const defaultAgentTimeout = 10 * time.Minute

// Service orchestrates the agent loop: native tool calling → streaming answer.
type Service struct {
	db              *db.Client
	fileSvc         *file.Service
	model           atomic.Pointer[llm.Model]
	tools           []tool.BaseTool
	tavily          *tavilyClient
	apifyClient     *apify.Client
	checkpointStore *SurrealCheckPointStore
}

// SetModel atomically replaces the LLM model (used by SIGHUP reload).
func (s *Service) SetModel(m *llm.Model) {
	s.model.Store(m)
}

// getModel returns the current model via an atomic load.
func (s *Service) getModel() *llm.Model {
	return s.model.Load()
}

// NewService creates a new agent service. The tools slice should contain
// multi-vault tool wrappers built by the bootstrap layer.
func NewService(db *db.Client, fileSvc *file.Service, model *llm.Model, agentTools []tool.BaseTool, tavilyAPIKey string, apifyClient *apify.Client) *Service {
	s := &Service{
		db:              db,
		fileSvc:         fileSvc,
		tools:           agentTools,
		apifyClient:     apifyClient,
		checkpointStore: NewCheckPointStore(db),
	}
	s.model.Store(model)
	if tavilyAPIKey != "" {
		s.tavily = newTavilyClient(tavilyAPIKey)
	}
	return s
}

// Available returns true if the agent has an LLM model configured.
func (s *Service) Available() bool {
	return s.getModel() != nil
}

// instructionTemplate is the system prompt template for the agent. {FolderTree},
// {Labels}, {Templates}, and {CurrentDate} are hydrated via session values in contextInjectionMiddleware.BeforeAgent.
const instructionTemplate = `You are a helpful knowledge assistant for the Know knowledge base. You help users find and understand information stored in their documents.

Today's date is {CurrentDate}.

- Use search to find relevant documents in the knowledge base
- Use read_document to read the full content of a specific document by path
- Use list_labels to discover available labels/categories
- Use list_folders to browse the folder structure
- Use list_folder_contents to see documents in a specific folder
- Use get_document_versions to see version history for a document
- Use create_memory to save important information for later recall (e.g. decisions, insights, project context)
- If the knowledge base has no results, tell the user and offer to search the web
- NEVER call web_search without the user's explicit permission
- Always cite document paths when referencing information from the knowledge base
- Do not include a sources section at the end of your response — sources are shown separately in the UI
- You can access multiple vaults including remote ones. Read tools (search, read_document, etc.) automatically query all accessible vaults. Results from remote vaults are prefixed with [namespace].
- For write tools (create_document, edit_document, etc.), you can target a specific vault by setting the "vault" field. Use "remote-name/vault-name" format for remote vaults. If omitted, the first local vault is used.
- When asked to use a template, read it with read_document and structure your output accordingly{FolderTree}{Labels}{Templates}{VaultInstructions}`

// buildMessages converts DB messages to eino schema messages.
func buildMessages(ctx context.Context, dbMsgs []models.Message) []*schema.Message {
	logger := logutil.FromCtx(ctx)
	var out []*schema.Message
	for _, msg := range dbMsgs {
		switch msg.Role {
		case models.RoleUser:
			out = append(out, &schema.Message{Role: schema.User, Content: msg.Content})
		case models.RoleAssistant:
			m := &schema.Message{Role: schema.Assistant, Content: msg.Content}
			if msg.ToolCalls != nil && *msg.ToolCalls != "" {
				var toolCalls []schema.ToolCall
				if err := json.Unmarshal([]byte(*msg.ToolCalls), &toolCalls); err != nil {
					logger.Warn("failed to deserialize tool calls from history", "message_id", msg.ID, "error", err)
				} else {
					m.ToolCalls = toolCalls
				}
			}
			out = append(out, m)
		case models.RoleToolResult:
			callID := ""
			if msg.ToolCallID != nil {
				callID = *msg.ToolCallID
			}
			out = append(out, schema.ToolMessage(msg.Content, callID))
		}
	}
	return out
}

// ChatRequest contains the parameters for a chat request.
type ChatRequest struct {
	ConversationID string
	VaultID        string
	UserID         string
	Content        string
	DocRefs        []string
	Attachments    []models.ChatAttachment
	AutoApprove    bool // true = skip approval for write tools
}

// Chat runs the agent loop using ADK ChatModelAgent and emits SSE events via the callback.
func (s *Service) Chat(ctx context.Context, req ChatRequest, emit func(StreamEvent)) error {
	model := s.getModel()
	if model == nil {
		return fmt.Errorf("agent not available: no LLM configured")
	}

	// 1. Create conversation if needed
	if req.ConversationID == "" {
		conv, err := s.db.CreateConversation(ctx, req.VaultID, req.UserID)
		if err != nil {
			return fmt.Errorf("create conversation: %w", err)
		}
		convID, err := models.RecordIDString(conv.ID)
		if err != nil {
			return fmt.Errorf("extract conversation ID: %w", err)
		}
		req.ConversationID = convID
		emit(StreamEvent{Type: "conv_id", ConvID: convID})
	}

	// 2. Store user message
	if _, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleUser, req.Content, req.DocRefs, nil, nil, nil, nil, nil); err != nil {
		return fmt.Errorf("create user message: %w", err)
	}

	// 3. Load history (all messages); the last one is the user message we just stored — exclude it
	allMessages, err := s.db.ListMessages(ctx, req.ConversationID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}
	history := allMessages
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	// 4. Build message list: history + current user message (system prompt handled by GenModelInput)
	messages := make([]*schema.Message, 0, len(history)+1)
	messages = append(messages, buildMessages(ctx, history)...)
	messages = append(messages, buildUserMessage(req.Content, splitAttachments(req.Attachments, emit)))

	// 5. Build agent + runner
	runner, tokenMW, toolMW, err := s.buildAgent(ctx, model, &req, emit)
	if err != nil {
		return fmt.Errorf("build agent: %w", err)
	}

	// 6. Run + consume (pass checkpoint ID for interrupt/resume)
	ctx, cancel := context.WithTimeout(ctx, defaultAgentTimeout)
	defer cancel()

	iter := runner.Run(ctx, messages, adk.WithCheckPointID(req.ConversationID))
	result := consumeAgentEvents(ctx, iter, emit)

	// 7. Populate result from middleware state
	tokenMW.mu.Lock()
	result.TokenUsage = tokenMW.usage
	tokenMW.mu.Unlock()
	toolMW.mu.Lock()
	result.ToolRecords = toolMW.records
	toolMW.mu.Unlock()

	// 8. Persist, update tokens, emit msg_end (or skip if interrupted)
	if err := s.finalizeRun(ctx, req.ConversationID, model, &result, emit); err != nil {
		return fmt.Errorf("finalize run: %w", err)
	}

	// 9. Auto-title if first message
	if len(history) == 0 {
		go s.autoTitle(context.Background(), req.ConversationID, req.Content)
	}

	return nil
}

// finalizeRun handles the shared post-run logic: persist results, update token
// counts, and emit msg_end (skipped if the run was interrupted).
func (s *Service) finalizeRun(ctx context.Context, convID string, model *llm.Model, result *AgentResult, emit func(StreamEvent)) error {
	assistantMsgID, err := s.persistResults(ctx, convID, result)

	// Update conversation token usage (even on persist error — tokens were consumed)
	if result.TokenUsage.InputTokens > 0 || result.TokenUsage.OutputTokens > 0 {
		if tokenErr := s.db.UpdateConversationTokens(ctx, convID, result.TokenUsage.InputTokens, result.TokenUsage.OutputTokens); tokenErr != nil {
			logutil.FromCtx(ctx).Warn("failed to update conversation tokens", "conversation_id", convID, "error", tokenErr)
		}
	}

	if err != nil {
		logutil.FromCtx(ctx).Error("failed to persist agent results", "conversation_id", convID, "error", err)
		emit(StreamEvent{Type: "error", Content: "warning: response may not be saved to conversation history"})
	}

	// If interrupted, skip msg_end — it will be emitted after resume completes
	if result.Interrupted {
		return nil
	}

	emit(StreamEvent{
		Type:              "msg_end",
		MsgID:             assistantMsgID,
		InputTokens:       result.TokenUsage.InputTokens,
		OutputTokens:      result.TokenUsage.OutputTokens,
		ContextWindowMax:  model.ContextWindow(),
		ContextWindowUsed: result.TokenUsage.FinalPromptTokens,
	})

	// Clean up checkpoint after successful completion to prevent unbounded growth.
	if delErr := s.db.DeleteCheckpoint(ctx, convID); delErr != nil {
		logutil.FromCtx(ctx).Warn("failed to delete checkpoint", "conversation_id", convID, "error", delErr)
	}

	return nil
}

// buildAgent constructs a ChatModelAgent + Runner per request.
func (s *Service) buildAgent(ctx context.Context, model *llm.Model, req *ChatRequest, emit func(StreamEvent)) (*adk.Runner, *tokenTrackingMiddleware, *toolExecutionMiddleware, error) {
	// Collect tools — wrap write tools for interrupt/resume approval if needed
	agentTools := make([]tool.BaseTool, len(s.tools))
	copy(agentTools, s.tools)
	if s.tavily != nil {
		agentTools = append(agentTools, &WebSearchTool{tavily: s.tavily})
	}
	if s.apifyClient != nil {
		agentTools = append(agentTools, &YouTubeTranscriptTool{client: s.apifyClient})
	}
	if !req.AutoApprove {
		agentTools = wrapWriteToolsForApproval(ctx, agentTools, s, req.VaultID)
	}

	// Separate text/image attachments for context injection
	var textAtts []models.ChatAttachment
	for _, att := range req.Attachments {
		if att.Type == models.AttachmentTypeText {
			textAtts = append(textAtts, att)
		}
	}

	// Create middleware instances with per-request state
	contextMW := &contextInjectionMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		docRefs:                      req.DocRefs,
		textAtts:                     textAtts,
		vaultID:                      req.VaultID,
		db:                           s.db,
		fileSvc:                      s.fileSvc,
		emit:                         emit,
	}
	tokenMW := &tokenTrackingMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
	}
	toolMW := &toolExecutionMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		service:                      s,
		req:                          req,
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "know",
		Description: "Knowledge base assistant",
		Model:       model.BaseChatModel(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: agentTools,
			},
		},
		Instruction: instructionTemplate,
		Handlers:    []adk.ChatModelAgentMiddleware{contextMW, tokenMW, toolMW},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create chat model agent: %w", err)
	}

	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: s.checkpointStore,
	}), tokenMW, toolMW, nil
}

// consumeAgentEvents drains the AsyncIterator, translating AgentEvents to SSE
// StreamEvents. Returns the accumulated AgentResult.
func consumeAgentEvents(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent], emit func(StreamEvent)) AgentResult {
	logger := logutil.FromCtx(ctx)
	var result AgentResult
	var answer strings.Builder
	var hitError bool

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		// Error event — stop processing messages but keep draining for RunCompleteEvent
		if event.Err != nil {
			logger.Error("agent error", "error", event.Err)
			emit(StreamEvent{Type: "error", Content: fmt.Sprintf("generation failed: %v", event.Err)})
			hitError = true
			continue
		}

		// Interrupt action — eino's Runner emits this when a tool calls StatefulInterrupt.
		// The checkpoint is already persisted by the Runner at this point.
		if event.Action != nil && event.Action.Interrupted != nil {
			for _, ictx := range event.Action.Interrupted.InterruptContexts {
				if ictx.IsRootCause {
					if req, ok := ictx.Info.(*ApprovalRequest); ok {
						emit(StreamEvent{
							Type:        "interrupted",
							InterruptID: ictx.ID,
							CallID:      req.CallID,
							Tool:        req.Tool,
							Approval:    req,
						})
					} else {
						logger.Error("interrupt has unexpected info type", "id", ictx.ID, "type", fmt.Sprintf("%T", ictx.Info))
					}
				}
			}
			result.Interrupted = true
			continue
		}

		if event.Output == nil {
			continue
		}

		// Custom events from middleware — always process (including after errors)
		if event.Output.CustomizedOutput != nil {
			switch e := event.Output.CustomizedOutput.(type) {
			case *ToolStartEvent:
				if !hitError {
					emit(StreamEvent{Type: "tool_start", CallID: e.CallID, Tool: e.Tool, Input: e.Input})
				}

			case *ToolEndEvent:
				if !hitError {
					if e.Error != "" {
						emit(StreamEvent{Type: "tool_end", CallID: e.CallID, Tool: e.Tool, Content: e.Error})
					} else {
						emit(StreamEvent{Type: "tool_end", CallID: e.CallID, Tool: e.Tool, Meta: e.Meta})
					}
				}

			default:
				logger.Debug("unknown customized output type", "type", fmt.Sprintf("%T", e))
			}
			continue
		}

		// Skip message events after an error
		if hitError {
			continue
		}

		// Message events
		if event.Output.MessageOutput == nil {
			continue
		}

		mv := event.Output.MessageOutput

		// Skip native Role:Tool events — our middleware already emitted tool_end
		if mv.Role == schema.Tool {
			continue
		}

		// Role:Assistant — streaming text
		if mv.Role == schema.Assistant {
			if mv.IsStreaming && mv.MessageStream != nil {
				for {
					chunk, recvErr := mv.MessageStream.Recv()
					if errors.Is(recvErr, io.EOF) {
						break
					}
					if recvErr != nil {
						logger.Error("stream recv error", "error", recvErr)
						emit(StreamEvent{Type: "error", Content: fmt.Sprintf("response was truncated: %v", recvErr)})
						hitError = true
						break
					}
					if chunk.Content != "" {
						answer.WriteString(chunk.Content)
						emit(StreamEvent{Type: "text", Content: chunk.Content})
					}
				}
			} else if mv.Message != nil && mv.Message.Content != "" {
				answer.WriteString(mv.Message.Content)
				emit(StreamEvent{Type: "text", Content: mv.Message.Content})
			}
		}
	}

	result.Answer = answer.String()
	return result
}

// persistResults batch-writes the agent's output to the database.
// Returns the assistant message ID (empty on error) and any error.
func (s *Service) persistResults(ctx context.Context, convID string, result *AgentResult) (string, error) {
	logger := logutil.FromCtx(ctx)

	// Build tool calls JSON for the assistant message
	var toolCallsJSON *string
	if len(result.ToolRecords) > 0 {
		var tcs []schema.ToolCall
		for _, r := range result.ToolRecords {
			tcs = append(tcs, r.Call)
		}
		b, marshalErr := json.Marshal(tcs)
		if marshalErr != nil {
			logger.Error("failed to marshal tool calls", "error", marshalErr)
		} else {
			s := string(b)
			toolCallsJSON = &s
		}
	}

	// Store assistant message
	assistantMsg, err := s.db.CreateMessage(ctx, convID, models.RoleAssistant, result.Answer, nil, nil, nil, nil, nil, toolCallsJSON)
	if err != nil {
		return "", fmt.Errorf("create assistant message: %w", err)
	}

	assistantMsgID, err := models.RecordIDString(assistantMsg.ID)
	if err != nil {
		logger.Warn("unexpected assistant message ID format", "conversation_id", convID, "error", err)
	}

	// Store tool result messages
	var errs []error
	for _, r := range result.ToolRecords {
		toolName := r.Call.Function.Name
		toolInput := r.Call.Function.Arguments
		callID := r.Call.ID

		var toolMetaStr *string
		if r.Meta != nil {
			metaJSON, metaErr := json.Marshal(r.Meta)
			if metaErr != nil {
				logger.Error("failed to marshal tool meta", "tool", toolName, "error", metaErr)
			} else {
				ms := string(metaJSON)
				toolMetaStr = &ms
			}
		}

		if _, storeErr := s.db.CreateMessage(ctx, convID, models.RoleToolResult, r.Result, nil, &toolName, &toolInput, toolMetaStr, &callID, nil); storeErr != nil {
			logger.Error("failed to store tool result message", "conversation_id", convID, "tool", toolName, "error", storeErr)
			errs = append(errs, storeErr)
		}
	}

	return assistantMsgID, errors.Join(errs...)
}

// buildUserMessage constructs the final user message. If image attachments are
// present, it returns a multimodal message with text + image parts; otherwise a
// plain text message.
func buildUserMessage(content string, imageAtts []models.ChatAttachment) *schema.Message {
	if len(imageAtts) == 0 {
		return &schema.Message{Role: schema.User, Content: content}
	}

	parts := []schema.MessageInputPart{
		{Type: schema.ChatMessagePartTypeText, Text: content},
	}
	for _, img := range imageAtts {
		// Copy to a loop-local variable so each part gets its own pointer.
		b64 := img.Content
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					Base64Data: &b64,
					MIMEType:   img.MimeType,
				},
			},
		})
	}
	return &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	}
}

// splitAttachments separates text and image attachments, emitting warnings for unsupported types.
// Returns only image attachments (text attachments are injected via contextInjectionMiddleware).
func splitAttachments(attachments []models.ChatAttachment, emit func(StreamEvent)) []models.ChatAttachment {
	var imageAtts []models.ChatAttachment
	for _, att := range attachments {
		switch att.Type {
		case models.AttachmentTypeText:
			// handled by contextInjectionMiddleware
		case models.AttachmentTypeImage:
			imageAtts = append(imageAtts, att)
		default:
			emit(StreamEvent{Type: "error", Content: fmt.Sprintf("unsupported attachment type %q for %s, skipping", att.Type, att.Path)})
		}
	}
	return imageAtts
}

// isWriteTool returns true for tools that modify documents.
func isWriteTool(name string) bool {
	return name == "create_document" || name == "edit_document" || name == "edit_document_section" || name == "create_memory"
}

// buildApprovalRequest computes the diff for a write tool call.
func (s *Service) buildApprovalRequest(ctx context.Context, vaultID string, call schema.ToolCall) (*ApprovalRequest, error) {
	var args struct {
		Path       string  `json:"path"`
		Content    *string `json:"content"`
		Operation  string  `json:"operation"`
		Heading    *string `json:"heading"`
		Position   *int    `json:"position"`
		NewHeading *string `json:"new_heading"`
		NewLevel   *int    `json:"new_level"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	req := &ApprovalRequest{
		CallID: call.ID,
		Tool:   call.Function.Name,
		Path:   args.Path,
	}

	doc, err := s.db.GetFileByPath(ctx, vaultID, args.Path)
	if err != nil {
		return nil, fmt.Errorf("check document: %w", err)
	}

	// Section edits require an existing document
	if call.Function.Name == "edit_document_section" && doc == nil {
		return nil, fmt.Errorf("document not found: %s", args.Path)
	}

	// For section edits, reconstruct the full new content for the diff
	var newContent string
	if args.Content != nil {
		newContent = *args.Content
	}

	// Load existing content once (used for both section edit reconstruction and diff).
	var docContent string
	if doc != nil {
		var contentErr error
		docContent, contentErr = s.fileSvc.ReadFileContent(ctx, doc)
		if contentErr != nil {
			return nil, fmt.Errorf("read content: %w", contentErr)
		}
	}

	if call.Function.Name == "edit_document_section" && doc != nil {
		edit := parser.BuildSectionEdit(parser.SectionEditArgs{
			Operation:  args.Operation,
			Heading:    args.Heading,
			Position:   args.Position,
			Content:    args.Content,
			NewHeading: args.NewHeading,
			NewLevel:   args.NewLevel,
		})
		reconstructed, editErr := parser.ApplySectionEdit(docContent, edit)
		if editErr != nil {
			return nil, fmt.Errorf("reconstruct section edit: %w", editErr)
		}
		newContent = reconstructed
	}

	if doc == nil {
		req.IsNew = true
		req.Content = newContent
		return req, nil
	}

	hunks, err := diff.ComputeHunks(docContent, newContent, 3)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}
	req.Diff = &DiffPayload{
		Hunks: hunks,
		Stats: diff.ComputeStats(hunks),
	}
	return req, nil
}

// buildRemoteApprovalRequest creates a content-only approval request for
// remote vault writes where we can't compute diffs.
func (s *Service) buildRemoteApprovalRequest(call schema.ToolCall) *ApprovalRequest {
	var args struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		// Show raw arguments so the user can still make an informed approval decision.
		return &ApprovalRequest{
			CallID:  call.ID,
			Tool:    call.Function.Name,
			Path:    "(unable to parse path)",
			IsNew:   true,
			Content: call.Function.Arguments,
		}
	}

	req := &ApprovalRequest{
		CallID: call.ID,
		Tool:   call.Function.Name,
		Path:   args.Path,
		IsNew:  true, // treat as new — we can't read remote doc for diff
	}
	if args.Content != nil {
		req.Content = *args.Content
	}
	return req
}

func (s *Service) autoTitle(ctx context.Context, conversationID, firstMessage string) {
	logger := logutil.FromCtx(ctx)
	model := s.getModel()
	if model == nil {
		logger.Debug("skipping auto-title: no LLM model configured", "conversation_id", conversationID)
		return
	}
	prompt := fmt.Sprintf("Generate a very short title (3-6 words, no quotes) for a conversation that starts with this message:\n\n%s", firstMessage)
	title, err := model.GenerateWithSystem(ctx, "You generate short conversation titles. Respond with ONLY the title, nothing else.", prompt)
	if err != nil {
		logger.Warn("auto-title failed", "conversation_id", conversationID, "error", err)
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	if err := s.db.UpdateConversationTitle(ctx, conversationID, title); err != nil {
		logger.Warn("failed to update conversation title", "conversation_id", conversationID, "error", err)
	}
}
