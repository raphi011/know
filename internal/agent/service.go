package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/diff"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/pathutil"
	"github.com/raphi011/knowhow/internal/search"
)

// StreamEvent is sent to the client via SSE.
type StreamEvent struct {
	Type     string           `json:"type"`              // "text" | "tool_start" | "tool_end" | "tool_approval_required" | "msg_start" | "msg_end" | "conv_id" | "error"
	Content  string           `json:"content,omitempty"`
	ConvID   string           `json:"convId,omitempty"`
	MsgID    string           `json:"msgId,omitempty"`
	CallID   string           `json:"callId,omitempty"`
	Tool     string           `json:"tool,omitempty"`
	Input    map[string]any   `json:"input,omitempty"`
	Meta     *ToolResultMeta  `json:"meta,omitempty"`
	Approval *ApprovalRequest `json:"approval,omitempty"`
}

// ToolResultMeta contains structured metadata about a tool execution result.
type ToolResultMeta struct {
	DurationMs     int64        `json:"durationMs"`
	ResultCount    *int         `json:"resultCount,omitempty"`
	ChunkCount     *int         `json:"chunkCount,omitempty"`
	MatchedDocs    []ToolDocRef `json:"matchedDocs,omitempty"`
	DocumentPath   *string      `json:"documentPath,omitempty"`
	DocumentTitle  *string      `json:"documentTitle,omitempty"`
	ContentLength  *int         `json:"contentLength,omitempty"`
	WebResultCount *int         `json:"webResultCount,omitempty"`
	WebSources     []ToolWebRef `json:"webSources,omitempty"`
}

// ToolDocRef references a matched KB document.
type ToolDocRef struct {
	Title string  `json:"title"`
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

// ToolWebRef references a web search result.
type ToolWebRef struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Service orchestrates the agent loop: native tool calling → streaming answer.
type Service struct {
	db              *db.Client
	model           *llm.Model
	search          *search.Service
	docService      *document.Service
	tavily          *tavilyClient
	activeApprovals sync.Map // map[conversationID]*approvalSession
}

// approvalSession pairs an approval registry with vault context for access checks.
type approvalSession struct {
	registry *approvalRegistry
	vaultID  string
}

// NewService creates a new agent service.
func NewService(db *db.Client, model *llm.Model, search *search.Service, docService *document.Service, tavilyAPIKey string) *Service {
	s := &Service{db: db, model: model, search: search, docService: docService}
	if tavilyAPIKey != "" {
		s.tavily = newTavilyClient(tavilyAPIKey)
	}
	return s
}

// Available returns true if the agent has an LLM model configured.
func (s *Service) Available() bool {
	return s.model != nil
}

// buildSystemPrompt constructs the system prompt, optionally appending the vault's folder tree.
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

	folders, err := s.db.ListFolders(ctx, vaultID)
	if err != nil {
		slog.Warn("failed to list folders for system prompt", "vault_id", vaultID, "error", err)
		return base
	}
	if len(folders) == 0 {
		return base
	}

	var sb strings.Builder
	sb.WriteString(base)
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
			Desc: "Read the full content of a specific document by its path",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {
					Type:     schema.String,
					Desc:     "The document path (e.g. /folder/document-name)",
					Required: true,
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
	AutoApprove    bool              // true = skip approval for write tools
	Approvals      *approvalRegistry // nil if auto-approve
}

// Chat runs the agent loop using native tool calling and emits SSE events via the callback.
func (s *Service) Chat(ctx context.Context, req ChatRequest, emit func(StreamEvent)) error {
	if s.model == nil {
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

	messages = append(messages, &schema.Message{Role: schema.User, Content: req.Content})

	// Track tool calls and results for storage
	type toolCallRecord struct {
		call   schema.ToolCall
		result string
		meta   *ToolResultMeta
	}
	var toolCallRecords []toolCallRecord
	var answer strings.Builder

	tools := s.buildTools()

	// 6. Call GenerateStreamWithTools
	err = s.model.GenerateStreamWithTools(ctx, messages, tools,
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
	if err != nil {
		slog.Error("LLM streaming generation failed", "conversation_id", req.ConversationID, "error", err)
		emit(StreamEvent{Type: "error", Content: fmt.Sprintf("generation failed: %v", err)})
		emit(StreamEvent{Type: "msg_end"})
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

	// 9. Emit msg_end with assistant message ID
	if assistantMsg != nil {
		assistantMsgID, idErr := models.RecordIDString(assistantMsg.ID)
		if idErr != nil {
			slog.Warn("unexpected assistant message ID format", "conversation_id", req.ConversationID, "error", idErr)
		} else {
			emit(StreamEvent{Type: "msg_end", MsgID: assistantMsgID})
		}
	} else {
		emit(StreamEvent{Type: "msg_end"})
	}

	// 10. Auto-title if first message
	if len(history) == 0 {
		go s.autoTitle(context.Background(), req.ConversationID, req.Content)
	}

	return nil
}

func intPtr(v int) *int { return &v }

// isWriteTool returns true for tools that modify documents.
func isWriteTool(name string) bool {
	return name == "create_document" || name == "edit_document"
}

// buildApprovalRequest computes the diff for a write tool call.
func (s *Service) buildApprovalRequest(ctx context.Context, vaultID string, call schema.ToolCall) (*ApprovalRequest, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
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

	if doc == nil {
		req.IsNew = true
		req.Content = args.Content
		return req, nil
	}

	hunks, err := diff.ComputeHunks(doc.Content, args.Content, 3)
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
func (s *Service) executeTool(ctx context.Context, vaultID, toolName, arguments string) (string, *ToolResultMeta, error) {
	switch toolName {
	case "kb_search":
		var input struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return "", nil, fmt.Errorf("parse kb_search input: %w", err)
		}

		start := time.Now()
		results, err := s.search.Search(ctx, search.SearchInput{
			VaultID:     vaultID,
			Query:       input.Query,
			Limit:       20,
			FullContent: true,
		})
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("search: %w", err)
		}

		var sb strings.Builder
		var matchedDocs []ToolDocRef
		totalChunks := 0
		for _, r := range results {
			fmt.Fprintf(&sb, "## %s (%s)\n", r.Title, r.Path)
			matchedDocs = append(matchedDocs, ToolDocRef{Title: r.Title, Path: r.Path, Score: r.Score})
			totalChunks += len(r.MatchedChunks)
			for _, ch := range r.MatchedChunks {
				sb.WriteString(ch.Snippet)
				sb.WriteString("\n\n")
			}
		}

		result := sb.String()
		if result == "" {
			result = "No results found."
		}

		meta := &ToolResultMeta{
			DurationMs:  durationMs,
			ResultCount: intPtr(len(results)),
			ChunkCount:  intPtr(totalChunks),
			MatchedDocs: matchedDocs,
		}
		return result, meta, nil

	case "read_document":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return "", nil, fmt.Errorf("parse read_document input: %w", err)
		}

		start := time.Now()
		doc, err := s.db.GetDocumentByPath(ctx, vaultID, input.Path)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("read document: %w", err)
		}
		if doc == nil {
			meta := &ToolResultMeta{DurationMs: durationMs}
			return fmt.Sprintf("Document not found: %s", input.Path), meta, nil
		}

		contentLen := len(doc.ContentBody)
		meta := &ToolResultMeta{
			DurationMs:    durationMs,
			DocumentPath:  &doc.Path,
			DocumentTitle: &doc.Title,
			ContentLength: &contentLen,
		}
		return fmt.Sprintf("# %s\n\n%s", doc.Title, doc.ContentBody), meta, nil

	case "list_labels":
		start := time.Now()
		labels, err := s.db.ListLabels(ctx, vaultID)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("list labels: %w", err)
		}

		result := "No labels found."
		if len(labels) > 0 {
			result = strings.Join(labels, ", ")
		}
		meta := &ToolResultMeta{
			DurationMs:  durationMs,
			ResultCount: intPtr(len(labels)),
		}
		return result, meta, nil

	case "list_folders":
		var input struct {
			Parent *string `json:"parent"`
		}
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return "", nil, fmt.Errorf("parse list_folders input: %w", err)
		}

		start := time.Now()
		folders, err := s.db.ListFolders(ctx, vaultID)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("list folders: %w", err)
		}

		if input.Parent != nil {
			parent := pathutil.NormalizeFolderPath(*input.Parent)
			var filtered []models.Folder
			for _, f := range folders {
				if pathutil.IsImmediateChildFolder(parent, f.Path) {
					filtered = append(filtered, f)
				}
			}
			folders = filtered
		}

		var sb strings.Builder
		for _, f := range folders {
			fmt.Fprintf(&sb, "%s (%s)\n", f.Path, f.Name)
		}
		result := sb.String()
		if result == "" {
			result = "No folders found."
		}
		meta := &ToolResultMeta{
			DurationMs:  durationMs,
			ResultCount: intPtr(len(folders)),
		}
		return result, meta, nil

	case "list_folder_contents":
		var input struct {
			Folder string `json:"folder"`
		}
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return "", nil, fmt.Errorf("parse list_folder_contents input: %w", err)
		}
		if input.Folder == "" {
			return "", nil, fmt.Errorf("folder is required")
		}
		folder := pathutil.NormalizeFolderPath(input.Folder)

		start := time.Now()

		// Fetch both documents and subfolders
		docs, err := s.db.ListDocuments(ctx, db.ListDocumentsFilter{
			VaultID: vaultID,
			Folder:  &folder,
		})
		if err != nil {
			return "", nil, fmt.Errorf("list folder contents: %w", err)
		}
		allFolders, err := s.db.ListFolders(ctx, vaultID)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("list folder subfolders: %w", err)
		}

		var sb strings.Builder
		count := 0

		// List immediate child folders
		for _, f := range allFolders {
			if pathutil.IsImmediateChildFolder(folder, f.Path) {
				fmt.Fprintf(&sb, "📁 %s/\n", f.Name)
				count++
			}
		}

		// List immediate child documents
		for _, d := range docs {
			if !pathutil.IsImmediateChild(folder, d.Path) {
				continue
			}
			labels := ""
			if len(d.Labels) > 0 {
				labels = " [" + strings.Join(d.Labels, ", ") + "]"
			}
			fmt.Fprintf(&sb, "📄 %s — %s%s\n", d.Path, d.Title, labels)
			count++
		}

		result := sb.String()
		if result == "" {
			result = fmt.Sprintf("No contents found in folder %s", folder)
		}
		meta := &ToolResultMeta{
			DurationMs:  durationMs,
			ResultCount: intPtr(count),
		}
		return result, meta, nil

	case "create_document":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", nil, fmt.Errorf("parse create_document input: %w", err)
		}
		if args.Path == "" {
			return "", nil, fmt.Errorf("path is required")
		}
		if args.Content == "" {
			return "", nil, fmt.Errorf("content is required")
		}

		// Check if document already exists
		existing, err := s.db.GetDocumentByPath(ctx, vaultID, args.Path)
		if err != nil {
			return "", nil, fmt.Errorf("check existing document: %w", err)
		}
		if existing != nil {
			return "", nil, fmt.Errorf("document already exists at path: %s", args.Path)
		}

		start := time.Now()
		doc, err := s.docService.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    args.Path,
			Content: args.Content,
			Source:  models.SourceAIGenerated,
		})
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("create document: %w", err)
		}

		meta := &ToolResultMeta{
			DurationMs:    durationMs,
			DocumentPath:  &doc.Path,
			DocumentTitle: &doc.Title,
		}
		return fmt.Sprintf("Document created: %s (%s)", doc.Title, doc.Path), meta, nil

	case "edit_document":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", nil, fmt.Errorf("parse edit_document input: %w", err)
		}
		if args.Path == "" {
			return "", nil, fmt.Errorf("path is required")
		}
		if args.Content == "" {
			return "", nil, fmt.Errorf("content is required")
		}

		// Check document exists
		existing, err := s.db.GetDocumentByPath(ctx, vaultID, args.Path)
		if err != nil {
			return "", nil, fmt.Errorf("check document: %w", err)
		}
		if existing == nil {
			return "", nil, fmt.Errorf("document not found: %s", args.Path)
		}

		start := time.Now()
		doc, err := s.docService.Create(ctx, models.DocumentInput{
			VaultID: vaultID,
			Path:    args.Path,
			Content: args.Content,
			Source:  models.SourceAIGenerated,
		})
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, fmt.Errorf("edit document: %w", err)
		}

		meta := &ToolResultMeta{
			DurationMs:    durationMs,
			DocumentPath:  &doc.Path,
			DocumentTitle: &doc.Title,
		}
		return fmt.Sprintf("Document updated: %s (%s)", doc.Title, doc.Path), meta, nil

	case "web_search":
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

		var webSources []ToolWebRef
		for _, r := range webResults {
			webSources = append(webSources, ToolWebRef{Title: r.Title, URL: r.URL})
		}

		meta := &ToolResultMeta{
			DurationMs:     durationMs,
			WebResultCount: intPtr(len(webResults)),
			WebSources:     webSources,
		}
		return result, meta, nil

	default:
		return "", nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (s *Service) autoTitle(ctx context.Context, conversationID, firstMessage string) {
	prompt := fmt.Sprintf("Generate a very short title (3-6 words, no quotes) for a conversation that starts with this message:\n\n%s", firstMessage)
	title, err := s.model.GenerateWithSystem(ctx, "You generate short conversation titles. Respond with ONLY the title, nothing else.", prompt)
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
