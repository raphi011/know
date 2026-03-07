package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/search"
)

// StreamEvent is sent to the client via SSE.
type StreamEvent struct {
	Type    string          `json:"type"`              // "text" | "tool_start" | "tool_end" | "msg_start" | "msg_end" | "conv_id" | "error"
	Content string          `json:"content,omitempty"`
	ConvID  string          `json:"convId,omitempty"`
	MsgID   string          `json:"msgId,omitempty"`
	CallID  string          `json:"callId,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Input   map[string]any  `json:"input,omitempty"`
	Meta    *ToolResultMeta `json:"meta,omitempty"`
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
	db     *db.Client
	model  *llm.Model
	search *search.Service
	tavily *tavilyClient
}

// NewService creates a new agent service.
func NewService(db *db.Client, model *llm.Model, search *search.Service, tavilyAPIKey string) *Service {
	s := &Service{db: db, model: model, search: search}
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
			// Parse input for emit
			var inputMap map[string]any
			if jsonErr := json.Unmarshal([]byte(call.Function.Arguments), &inputMap); jsonErr != nil {
				inputMap = map[string]any{"raw": call.Function.Arguments}
			}
			emit(StreamEvent{Type: "tool_start", CallID: call.ID, Tool: call.Function.Name, Input: inputMap})

			result, meta, execErr := s.executeTool(ctx, req.VaultID, call.Function.Name, call.Function.Arguments)
			if execErr != nil {
				slog.Warn("tool execution failed", "tool", call.Function.Name, "error", execErr)
				emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: call.Function.Name, Content: fmt.Sprintf("error: %v", execErr)})
				toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: fmt.Sprintf("error: %v", execErr), meta: nil})
				return fmt.Sprintf("error: %v", execErr), nil
			}

			emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: call.Function.Name, Meta: meta})
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
			VaultID: vaultID,
			Query:   input.Query,
			Limit:   5,
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
