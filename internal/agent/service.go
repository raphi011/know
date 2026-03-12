package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/diff"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/search"
	"github.com/raphi011/knowhow/internal/tools"
)

// StreamEvent is sent to the client via SSE.
type StreamEvent struct {
	Type         string                `json:"type"` // "text" | "tool_start" | "tool_end" | "tool_approval_required" | "msg_start" | "msg_end" | "conv_id" | "error"
	Content      string                `json:"content,omitempty"`
	ConvID       string                `json:"convId,omitempty"`
	MsgID        string                `json:"msgId,omitempty"`
	CallID       string                `json:"callId,omitempty"`
	Tool         string                `json:"tool,omitempty"`
	Input        map[string]any        `json:"input,omitempty"`
	Meta         *tools.ToolResultMeta `json:"meta,omitempty"`
	Approval     *ApprovalRequest      `json:"approval,omitempty"`
	InputTokens  int64                 `json:"inputTokens,omitempty"`
	OutputTokens int64                 `json:"outputTokens,omitempty"`
}

// Service orchestrates the agent loop: native tool calling → streaming answer.
type Service struct {
	db              *db.Client
	model           atomic.Pointer[llm.Model]
	search          *search.Service
	docService      *document.Service
	executor        *tools.Executor
	tavily          *tavilyClient
	activeApprovals sync.Map // map[conversationID]*approvalSession
}

// SetModel atomically replaces the LLM model (used by SIGHUP reload).
func (s *Service) SetModel(m *llm.Model) {
	s.model.Store(m)
}

// getModel returns the current model via an atomic load.
func (s *Service) getModel() *llm.Model {
	return s.model.Load()
}

// approvalSession pairs an approval registry with vault context for access checks.
type approvalSession struct {
	registry *approvalRegistry
	vaultID  string
}

// NewService creates a new agent service.
func NewService(db *db.Client, model *llm.Model, search *search.Service, docService *document.Service, tavilyAPIKey string) *Service {
	s := &Service{
		db:         db,
		search:     search,
		docService: docService,
		executor: &tools.Executor{
			DB:         db,
			Search:     search,
			DocService: docService,
		},
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

// buildSystemPrompt constructs the system prompt, appending the vault's folder tree and label summary when available.
func (s *Service) buildSystemPrompt(ctx context.Context, vaultID string) string {
	base := `You are a helpful knowledge assistant for the Knowhow knowledge base. You help users find and understand information stored in their documents.

- Use kb_search to find relevant documents in the knowledge base
- Use read_document to read the full content of a specific document by path
- Use list_labels to discover available labels/categories
- Use list_folders to browse the folder structure
- Use list_folder_contents to see documents in a specific folder
- If the knowledge base has no results, tell the user and offer to search the web
- NEVER call web_search without the user's explicit permission
- Always cite document paths when referencing information from the knowledge base
- Do not include a sources section at the end of your response — sources are shown separately in the UI`

	var sb strings.Builder
	sb.WriteString(base)

	folders, err := s.db.ListFolders(ctx, vaultID)
	if err != nil {
		slog.Warn("failed to list folders for system prompt", "vault_id", vaultID, "error", err)
	} else if len(folders) > 0 {
		sb.WriteString("\n\nVault folder structure:\n```\n/\n")
		for _, f := range folders {
			depth := strings.Count(strings.Trim(f.Path, "/"), "/")
			indent := strings.Repeat("  ", depth)
			sb.WriteString(indent)
			sb.WriteString("├── ")
			sb.WriteString(f.Name)
			sb.WriteString("/\n")
		}
		sb.WriteString("```")
	}

	labelCounts, err := s.db.ListLabelsWithCounts(ctx, vaultID)
	if err != nil {
		slog.Warn("failed to list labels for system prompt", "vault_id", vaultID, "error", err)
	} else if len(labelCounts) > 0 {
		sb.WriteString("\n\nVault labels:\n")
		for _, lc := range labelCounts {
			fmt.Fprintf(&sb, "- %s (%d)\n", lc.Label, lc.Count)
		}
	}

	return sb.String()
}

// buildTools returns the tool definitions for native tool calling.
func (s *Service) buildTools() []*schema.ToolInfo {
	tools := []*schema.ToolInfo{
		{
			Name: "kb_search",
			Desc: "Search the knowledge base for relevant documents",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {
					Type:     schema.String,
					Desc:     "The search query",
					Required: true,
				},
			}),
		},
		{
			Name: "read_document",
			Desc: "Read the full content of a specific document by its path. Set sections=true to include a section outline for use with edit_document_section.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {
					Type:     schema.String,
					Desc:     "The document path (e.g. /folder/document-name)",
					Required: true,
				},
				"sections": {
					Type: schema.Boolean,
					Desc: "Include section outline for targeted editing",
				},
			}),
		},
		{
			Name:        "list_labels",
			Desc:        "List all labels/categories used across documents in the knowledge base",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
		},
		{
			Name: "list_folders",
			Desc: "List the folder structure of the knowledge base. Optionally filter to immediate children of a parent folder.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"parent": {
					Type: schema.String,
					Desc: "Parent folder path to list children of (e.g. /guides/). Lists all folders if omitted.",
				},
			}),
		},
		{
			Name: "list_folder_contents",
			Desc: "List documents and subfolders in a specific folder. Returns immediate children only.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"folder": {
					Type:     schema.String,
					Desc:     "Folder path (e.g. /guides/)",
					Required: true,
				},
			}),
		},
	}

	if s.docService != nil {
		tools = append(tools,
			&schema.ToolInfo{
				Name: "create_document",
				Desc: "Create a new document in the knowledge base. The content should be markdown. Fails if a document already exists at the given path.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path": {
						Type:     schema.String,
						Desc:     "Document path (e.g. /guides/new-guide.md)",
						Required: true,
					},
					"content": {
						Type:     schema.String,
						Desc:     "Full markdown content for the document",
						Required: true,
					},
				}),
			},
			&schema.ToolInfo{
				Name: "edit_document",
				Desc: "Edit an existing document by replacing its full content. Read the document first to get the current content, then modify and pass the complete new content.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path": {
						Type:     schema.String,
						Desc:     "Document path of the existing document",
						Required: true,
					},
					"content": {
						Type:     schema.String,
						Desc:     "Complete new markdown content (replaces existing content entirely)",
						Required: true,
					},
				}),
			},
			&schema.ToolInfo{
				Name: "edit_document_section",
				Desc: "Edit a specific section of a document by heading, without sending the full content. Use read_document with sections=true to see available sections. Supports replace, insert_after, insert_before, delete, and append operations.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path": {
						Type:     schema.String,
						Desc:     "Document path",
						Required: true,
					},
					"operation": {
						Type:     schema.String,
						Desc:     "One of: replace, insert_after, insert_before, delete, append",
						Required: true,
					},
					"heading": {
						Type: schema.String,
						Desc: "Target section heading (empty string for preamble, omit for append)",
					},
					"position": {
						Type: schema.Integer,
						Desc: "Disambiguation index for duplicate headings (default 0)",
					},
					"content": {
						Type: schema.String,
						Desc: "New section body (required for replace, insert, append)",
					},
					"new_heading": {
						Type: schema.String,
						Desc: "Heading text for insert/append operations",
					},
					"new_level": {
						Type: schema.Integer,
						Desc: "Heading level 1-6 for insert/append operations",
					},
				}),
			},
		)
	}

	if s.tavily != nil {
		tools = append(tools, &schema.ToolInfo{
			Name: "web_search",
			Desc: "Search the web for information not found in the knowledge base. Only call this after the user explicitly asks to search the web.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {
					Type:     schema.String,
					Desc:     "The web search query",
					Required: true,
				},
			}),
		})
	}

	return tools
}

// buildMessages converts DB messages to eino schema messages.
func buildMessages(dbMsgs []models.Message) []*schema.Message {
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
					slog.Warn("failed to deserialize tool calls from history", "message_id", msg.ID, "error", err)
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
	AutoApprove    bool              // true = skip approval for write tools
	Approvals      *approvalRegistry // nil if auto-approve
}

// Chat runs the agent loop using native tool calling and emits SSE events via the callback.
func (s *Service) Chat(ctx context.Context, req ChatRequest, emit func(StreamEvent)) error {
	model := s.getModel()
	if model == nil {
		emit(StreamEvent{Type: "error", Content: "agent not available: no LLM configured"})
		return nil
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

	// Set up approval registry for write tool gating
	if !req.AutoApprove {
		approvals := newApprovalRegistry()
		s.activeApprovals.Store(req.ConversationID, &approvalSession{registry: approvals, vaultID: req.VaultID})
		defer func() {
			approvals.cancel()
			s.activeApprovals.Delete(req.ConversationID)
		}()
		req.Approvals = approvals
	}

	// 2. Store user message
	userMsg, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleUser, req.Content, req.DocRefs, nil, nil, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("create user message: %w", err)
	}
	_, err = models.RecordIDString(userMsg.ID)
	if err != nil {
		return fmt.Errorf("extract user message ID: %w", err)
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

	// 4. Build message list: system + history + doc refs context + current user message
	sysPrompt := s.buildSystemPrompt(ctx, req.VaultID)
	messages := make([]*schema.Message, 0, 2+len(history)+len(req.DocRefs)*2+1)
	messages = append(messages, &schema.Message{Role: schema.System, Content: sysPrompt})
	messages = append(messages, buildMessages(history)...)

	// Inject doc refs as a user+assistant pair
	if len(req.DocRefs) > 0 {
		var refContext strings.Builder
		for _, ref := range req.DocRefs {
			doc, docErr := s.db.GetDocumentByPath(ctx, req.VaultID, ref)
			if docErr != nil {
				slog.Warn("failed to read referenced doc", "path", ref, "error", docErr)
				emit(StreamEvent{Type: "error", Content: fmt.Sprintf("could not read referenced document: %s", ref)})
				continue
			}
			if doc != nil {
				fmt.Fprintf(&refContext, "\n--- Document: %s ---\n%s\n", doc.Path, doc.ContentBody)
			}
		}
		if refContext.Len() > 0 {
			messages = append(messages,
				&schema.Message{Role: schema.User, Content: "Referenced documents:\n" + refContext.String()},
				&schema.Message{Role: schema.Assistant, Content: "I'll use these referenced documents to help answer your question."},
			)
		}
	}

	// Separate text and image attachments
	var textAtts, imageAtts []models.ChatAttachment
	for _, att := range req.Attachments {
		switch att.Type {
		case models.AttachmentTypeText:
			textAtts = append(textAtts, att)
		case models.AttachmentTypeImage:
			imageAtts = append(imageAtts, att)
		default:
			logutil.FromCtx(ctx).Warn("unsupported attachment type, skipping", "path", att.Path, "type", att.Type)
			emit(StreamEvent{Type: "error", Content: fmt.Sprintf("unsupported attachment type %q for %s, skipping", att.Type, att.Path)})
		}
	}

	// Inject text file attachments as fenced code blocks
	if len(textAtts) > 0 {
		var fileContext strings.Builder
		for _, att := range textAtts {
			fmt.Fprintf(&fileContext, "\n--- File: %s ---\n```%s\n%s\n```\n", att.Path, att.Language, att.Content)
		}
		messages = append(messages,
			&schema.Message{Role: schema.User, Content: "Attached local files:\n" + fileContext.String()},
			&schema.Message{Role: schema.Assistant, Content: "I'll use these attached files to help answer your question."},
		)
	}

	// Build the final user message — multimodal if image attachments present
	messages = append(messages, buildUserMessage(req.Content, imageAtts))

	// Track tool calls and results for storage
	type toolCallRecord struct {
		call   schema.ToolCall
		result string
		meta   *tools.ToolResultMeta
	}
	var toolCallRecords []toolCallRecord
	var answer strings.Builder

	tools := s.buildTools()

	// 6. Call GenerateStreamWithTools
	usage, err := model.GenerateStreamWithTools(ctx, messages, tools,
		func(token string) error {
			answer.WriteString(token)
			emit(StreamEvent{Type: "text", Content: token})
			return nil
		},
		func(call schema.ToolCall) (string, error) {
			var inputMap map[string]any
			if jsonErr := json.Unmarshal([]byte(call.Function.Arguments), &inputMap); jsonErr != nil {
				inputMap = map[string]any{"raw": call.Function.Arguments}
			}
			toolName := call.Function.Name
			emit(StreamEvent{Type: "tool_start", CallID: call.ID, Tool: toolName, Input: inputMap})

			// Gate write tools on user approval
			if isWriteTool(toolName) && !req.AutoApprove && req.Approvals != nil {
				approvalReq, buildErr := s.buildApprovalRequest(ctx, req.VaultID, call)
				if buildErr != nil {
					errMsg := fmt.Sprintf("error: %v", buildErr)
					emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: errMsg})
					toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: errMsg})
					return errMsg, nil
				}

				emit(StreamEvent{Type: "tool_approval_required", CallID: call.ID, Tool: toolName, Approval: approvalReq})
				ch := req.Approvals.register(call.ID)

				var resp ApprovalResponse
				var ok bool
				select {
				case resp, ok = <-ch:
					if !ok {
						result := "error: approval cancelled"
						emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: result})
						toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: result})
						return result, nil
					}
				case <-ctx.Done():
					result := "error: request cancelled"
					emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: result})
					toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: result})
					return result, nil
				}

				if resp.Action == ApprovalReject {
					result := "User rejected the proposed changes."
					emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: result})
					toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: result})
					return result, nil
				}

				if resp.Action == ApprovalApproveHunks && approvalReq.Diff != nil {
					doc, docErr := s.db.GetDocumentByPath(ctx, req.VaultID, approvalReq.Path)
					if docErr != nil || doc == nil {
						errMsg := "error: could not retrieve document for partial approval"
						emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: errMsg})
						toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: errMsg})
						return errMsg, nil
					}
					merged, mergeErr := diff.ApplyHunks(doc.Content, approvalReq.Diff.Hunks, resp.HunkIndexes)
					if mergeErr != nil {
						errMsg := fmt.Sprintf("error applying hunks: %v", mergeErr)
						emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: errMsg})
						toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: errMsg})
						return errMsg, nil
					}
					inputMap["content"] = merged
					newArgs, marshalErr := json.Marshal(inputMap)
					if marshalErr != nil {
						errMsg := fmt.Sprintf("error: marshal args: %v", marshalErr)
						emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: errMsg})
						toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: errMsg})
						return errMsg, nil
					}
					call.Function.Arguments = string(newArgs)
				}
				// ApprovalApproveAll falls through to normal execution
			}

			result, meta, execErr := s.executeTool(ctx, req.VaultID, toolName, call.Function.Arguments)
			if execErr != nil {
				slog.Warn("tool execution failed", "tool", toolName, "error", execErr)
				emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: fmt.Sprintf("error: %v", execErr)})
				toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: fmt.Sprintf("error: %v", execErr), meta: nil})
				return fmt.Sprintf("error: %v", execErr), nil
			}

			emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Meta: meta})
			toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: result, meta: meta})
			return result, nil
		},
	)
	// Persist cumulative token usage on the conversation (even on error — tokens were consumed).
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		if tokenErr := s.db.UpdateConversationTokens(ctx, req.ConversationID, usage.InputTokens, usage.OutputTokens); tokenErr != nil {
			slog.Warn("failed to update conversation tokens", "conversation_id", req.ConversationID, "error", tokenErr)
		}
	}

	if err != nil {
		slog.Error("LLM streaming generation failed", "conversation_id", req.ConversationID, "error", err)
		emit(StreamEvent{Type: "error", Content: fmt.Sprintf("generation failed: %v", err)})
		emit(StreamEvent{Type: "msg_end", InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens})
		return fmt.Errorf("generate stream: %w", err)
	}

	// 7. Store assistant message with tool calls JSON if applicable
	var toolCallsJSON *string
	if len(toolCallRecords) > 0 {
		// Collect all tool calls
		var tcs []schema.ToolCall
		for _, r := range toolCallRecords {
			tcs = append(tcs, r.call)
		}
		b, marshalErr := json.Marshal(tcs)
		if marshalErr != nil {
			slog.Error("failed to marshal tool calls", "error", marshalErr)
		} else {
			s := string(b)
			toolCallsJSON = &s
		}
	}

	assistantMsg, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleAssistant, answer.String(), nil, nil, nil, nil, nil, toolCallsJSON)
	if err != nil {
		slog.Error("failed to store assistant message", "conversation_id", req.ConversationID, "error", err)
		emit(StreamEvent{Type: "error", Content: "warning: response may not be saved to conversation history"})
	}

	// 8. Store tool result messages
	for _, r := range toolCallRecords {
		toolName := r.call.Function.Name
		toolInput := r.call.Function.Arguments
		callID := r.call.ID

		var toolMetaStr *string
		if r.meta != nil {
			metaJSON, metaErr := json.Marshal(r.meta)
			if metaErr != nil {
				slog.Error("failed to marshal tool meta", "tool", toolName, "error", metaErr)
			} else {
				ms := string(metaJSON)
				toolMetaStr = &ms
			}
		}

		_, storeErr := s.db.CreateMessage(ctx, req.ConversationID, models.RoleToolResult, r.result, nil, &toolName, &toolInput, toolMetaStr, &callID, nil)
		if storeErr != nil {
			slog.Error("failed to store tool result message", "conversation_id", req.ConversationID, "tool", toolName, "error", storeErr)
		}
	}

	// 9. Emit msg_end with assistant message ID and token usage
	if assistantMsg != nil {
		assistantMsgID, idErr := models.RecordIDString(assistantMsg.ID)
		if idErr != nil {
			slog.Warn("unexpected assistant message ID format", "conversation_id", req.ConversationID, "error", idErr)
			emit(StreamEvent{Type: "msg_end", InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens})
		} else {
			emit(StreamEvent{Type: "msg_end", MsgID: assistantMsgID, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens})
		}
	} else {
		emit(StreamEvent{Type: "msg_end", InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens})
	}

	// 10. Auto-title if first message
	if len(history) == 0 {
		go s.autoTitle(context.Background(), req.ConversationID, req.Content)
	}

	return nil
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

// agentToolToCanonical maps agent-specific tool names to canonical executor names.
var agentToolToCanonical = map[string]string{
	"kb_search":             "search",
	"read_document":         "read_document",
	"list_labels":           "list_labels",
	"list_folders":          "list_folders",
	"list_folder_contents":  "list_folder_contents",
	"create_document":       "create_document",
	"edit_document":         "edit_document",
	"edit_document_section": "edit_document_section",
}

// isWriteTool returns true for tools that modify documents.
func isWriteTool(name string) bool {
	return name == "create_document" || name == "edit_document" || name == "edit_document_section"
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

	doc, err := s.db.GetDocumentByPath(ctx, vaultID, args.Path)
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
	if call.Function.Name == "edit_document_section" && doc != nil {
		edit := parser.BuildSectionEdit(parser.SectionEditArgs{
			Operation:  args.Operation,
			Heading:    args.Heading,
			Position:   args.Position,
			Content:    args.Content,
			NewHeading: args.NewHeading,
			NewLevel:   args.NewLevel,
		})
		reconstructed, editErr := parser.ApplySectionEdit(doc.Content, edit)
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

	hunks, err := diff.ComputeHunks(doc.Content, newContent, 3)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}
	req.Diff = &DiffPayload{
		Hunks: hunks,
		Stats: diff.ComputeStats(hunks),
	}
	return req, nil
}

// executeTool executes a named tool with the given JSON arguments string.
// It maps agent-specific tool names to canonical names and delegates to the
// shared executor, except for web_search which is agent-specific.
func (s *Service) executeTool(ctx context.Context, vaultID, toolName, arguments string) (string, *tools.ToolResultMeta, error) {
	if toolName == "web_search" {
		return s.execWebSearch(ctx, arguments)
	}

	canonical, ok := agentToolToCanonical[toolName]
	if !ok {
		return "", nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	return s.executor.ExecuteTool(ctx, vaultID, canonical, arguments)
}

func (s *Service) execWebSearch(ctx context.Context, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse web_search input: %w", err)
	}
	if s.tavily == nil {
		return "", nil, fmt.Errorf("web search not configured")
	}

	start := time.Now()
	result, webResults, err := s.tavily.Search(ctx, input.Query)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("web search: %w", err)
	}

	var webSources []tools.ToolWebRef
	for _, r := range webResults {
		webSources = append(webSources, tools.ToolWebRef{Title: r.Title, URL: r.URL})
	}

	meta := &tools.ToolResultMeta{
		DurationMs:     durationMs,
		WebResultCount: new(len(webResults)),
		WebSources:     webSources,
	}
	return result, meta, nil
}

func (s *Service) autoTitle(ctx context.Context, conversationID, firstMessage string) {
	model := s.getModel()
	if model == nil {
		slog.Debug("skipping auto-title: no LLM model configured", "conversation_id", conversationID)
		return
	}
	prompt := fmt.Sprintf("Generate a very short title (3-6 words, no quotes) for a conversation that starts with this message:\n\n%s", firstMessage)
	title, err := model.GenerateWithSystem(ctx, "You generate short conversation titles. Respond with ONLY the title, nothing else.", prompt)
	if err != nil {
		slog.Warn("auto-title failed", "conversation_id", conversationID, "error", err)
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	if err := s.db.UpdateConversationTitle(ctx, conversationID, title); err != nil {
		slog.Warn("failed to update conversation title", "conversation_id", conversationID, "error", err)
	}
}
