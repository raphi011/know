package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/search"
)

const maxToolIterations = 5

// StreamEvent is sent to the client via SSE.
type StreamEvent struct {
	Type    string          `json:"type"`           // "token" | "tool_call" | "tool_result" | "done" | "error" | "message_id" | "conversation_id"
	Content string          `json:"content"`        // token text, tool result, error message, or message ID
	Tool    string          `json:"tool,omitempty"` // tool name for tool_call/tool_result events
	Meta    *ToolResultMeta `json:"meta,omitempty"` // structured metadata for tool_result events
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

// Service orchestrates the agent loop: intent detection → tool execution → streaming answer.
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

// toolAction is the LLM's decision from intent detection.
type toolAction struct {
	Action string          `json:"action"` // "tool" or "answer"
	Tool   string          `json:"tool"`   // "kb_search", "read_document", or "web_search"
	Input  json.RawMessage `json:"input"`
}

type searchInput struct {
	Query string `json:"query"`
}

type readDocInput struct {
	Path string `json:"path"`
}

type webSearchInput struct {
	Query string `json:"query"`
}

// buildSystemPromptBase constructs the base system prompt, conditionally including web_search when Tavily is configured.
func (s *Service) buildSystemPromptBase() string {
	base := `You are a helpful knowledge assistant for the Knowhow knowledge base. You help users find and understand information stored in their documents.

You have access to the following tools:
1. kb_search(query) - Search the knowledge base for relevant documents
2. read_document(path) - Read the full content of a specific document by path`

	if s.tavily != nil {
		base += `
3. web_search(query) - Search the web for information not found in the knowledge base

IMPORTANT: NEVER call web_search directly. If kb_search returns no or insufficient results, tell the user and ask "Would you like me to search the web?" Only call web_search after the user explicitly confirms.`
	}

	base += `

When the user asks a question:
- If you need to find relevant information, use kb_search first
- If you need the full content of a specific document, use read_document
- If you already have enough context from previous tool results, answer directly

Always cite document paths when referencing information from the knowledge base.
Be concise and helpful. Answer based on the knowledge base content when available.
Do not include a sources section at the end of your response — one will be added automatically.`

	return base
}

// buildIntentPrompt constructs the intent-detection prompt with tool options matching the available tools.
func (s *Service) buildIntentPrompt() string {
	base := `Based on the conversation so far, decide your next action.

If you need to search for information, respond with EXACTLY this JSON (no other text):
{"action":"tool","tool":"kb_search","input":{"query":"your search query"}}

If you need to read a specific document, respond with EXACTLY this JSON (no other text):
{"action":"tool","tool":"read_document","input":{"path":"/document/path"}}`

	if s.tavily != nil {
		base += `

If the user has explicitly asked you to search the web, respond with EXACTLY this JSON (no other text):
{"action":"tool","tool":"web_search","input":{"query":"your search query"}}`
	}

	base += `

If you have enough information to answer the user's question, respond with EXACTLY this JSON (no other text):
{"action":"answer"}

Respond with ONLY the JSON, no explanation.`

	return base
}

// buildSystemPrompt constructs the system prompt, optionally appending the vault's folder tree.
func (s *Service) buildSystemPrompt(ctx context.Context, vaultID string) string {
	base := s.buildSystemPromptBase()

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

type docSource struct {
	Title string
	Path  string
}

// ChatRequest contains the parameters for a chat request.
type ChatRequest struct {
	ConversationID string
	VaultID        string
	UserID         string
	Content        string
	DocRefs        []string
}

// Chat runs the agent loop and emits SSE events via the callback.
func (s *Service) Chat(ctx context.Context, req ChatRequest, emit func(StreamEvent)) error {
	if s.model == nil {
		emit(StreamEvent{Type: "error", Content: "agent not available: no LLM configured"})
		return nil
	}

	// Create conversation if needed
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
		emit(StreamEvent{Type: "conversation_id", Content: convID})
	}

	// Store user message
	userMsg, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleUser, req.Content, req.DocRefs, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("create user message: %w", err)
	}
	userMsgID, err := models.RecordIDString(userMsg.ID)
	if err != nil {
		return fmt.Errorf("extract user message ID: %w", err)
	}
	emit(StreamEvent{Type: "message_id", Content: userMsgID})

	// Build dynamic system prompt with folder tree
	sysPrompt := s.buildSystemPrompt(ctx, req.VaultID)

	// Build conversation history from DB
	messages, err := s.db.ListMessages(ctx, req.ConversationID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	// Build context from @-referenced documents
	var refContext strings.Builder
	for _, ref := range req.DocRefs {
		doc, err := s.db.GetDocumentByPath(ctx, req.VaultID, ref)
		if err != nil {
			slog.Warn("failed to read referenced doc", "path", ref, "error", err)
			emit(StreamEvent{Type: "error", Content: fmt.Sprintf("Could not read referenced document: %s", ref)})
			continue
		}
		if doc != nil {
			fmt.Fprintf(&refContext, "\n--- Document: %s ---\n%s\n", doc.Path, doc.ContentBody)
		}
	}

	// Agent loop: intent detection → tool execution → repeat or answer
	var toolContext strings.Builder
	if refContext.Len() > 0 {
		toolContext.WriteString("Referenced documents:\n")
		toolContext.WriteString(refContext.String())
		toolContext.WriteString("\n")
	}

	// Track all documents referenced during tool execution for the sources section
	seen := map[string]bool{}
	var sources []docSource

	for i := range maxToolIterations {
		// Build history for intent detection
		history := s.buildHistory(messages, toolContext.String())

		// Intent detection (non-streaming)
		intentQuery := req.Content
		if i > 0 {
			intentQuery = s.buildIntentPrompt()
		}

		decision, err := s.detectIntent(ctx, history, intentQuery, i == 0, sysPrompt)
		if err != nil {
			slog.Warn("intent detection failed, falling back to direct answer", "error", err)
			emit(StreamEvent{Type: "error", Content: "Knowledge base search unavailable, answering from general knowledge"})
			break // fall through to streaming answer
		}

		if decision.Action == "answer" {
			break // proceed to streaming answer
		}

		if decision.Action == "tool" {
			result, toolSources, meta, err := s.executeTool(ctx, req.VaultID, decision, emit)
			if err != nil {
				slog.Warn("tool execution failed", "tool", decision.Tool, "error", err)
				emit(StreamEvent{Type: "error", Content: fmt.Sprintf("tool %s failed: %v", decision.Tool, err)})
				break
			}
			for _, src := range toolSources {
				if !seen[src.Path] {
					seen[src.Path] = true
					sources = append(sources, src)
				}
			}
			fmt.Fprintf(&toolContext, "\n--- Tool result (%s) ---\n%s\n", decision.Tool, result)

			// Store tool call and result as messages
			toolInput, err := json.Marshal(decision.Input)
			if err != nil {
				slog.Error("failed to marshal tool input", "tool", decision.Tool, "error", err)
				toolInput = []byte(decision.Input) // fallback: use raw bytes to avoid storing empty string
			}
			toolInputStr := string(toolInput)
			_, err = s.db.CreateMessage(ctx, req.ConversationID, models.RoleToolCall, "", nil, &decision.Tool, &toolInputStr, nil)
			if err != nil {
				slog.Error("failed to store tool call message", "conversation_id", req.ConversationID, "tool", decision.Tool, "error", err)
			}

			// Marshal meta to JSON for storage
			var toolMetaStr *string
			if meta != nil {
				metaJSON, metaErr := json.Marshal(meta)
				if metaErr != nil {
					slog.Error("failed to marshal tool meta", "tool", decision.Tool, "error", metaErr)
				} else {
					s := string(metaJSON)
					toolMetaStr = &s
				}
			}
			_, err = s.db.CreateMessage(ctx, req.ConversationID, models.RoleToolResult, result, nil, &decision.Tool, nil, toolMetaStr)
			if err != nil {
				slog.Error("failed to store tool result message", "conversation_id", req.ConversationID, "tool", decision.Tool, "error", err)
			}

			continue
		}

		// Unknown action, fall through to answer
		break
	}

	// Stream the final answer
	history := s.buildHistory(messages, toolContext.String())
	var answer strings.Builder

	err = s.model.GenerateWithSystemStreamMultiTurn(ctx, sysPrompt, history, req.Content, func(token string) error {
		answer.WriteString(token)
		emit(StreamEvent{Type: "token", Content: token})
		return nil
	})
	if err != nil {
		slog.Error("LLM streaming generation failed", "conversation_id", req.ConversationID, "error", err)
		emit(StreamEvent{Type: "error", Content: fmt.Sprintf("generation failed: %v", err)})
		return nil
	}

	// Append deterministic sources section with clickable links
	if len(sources) > 0 {
		var srcSection strings.Builder
		srcSection.WriteString("\n\n---\n**Sources:**\n")
		for _, src := range sources {
			fmt.Fprintf(&srcSection, "- [%s](/docs%s)\n", src.Title, src.Path)
		}
		srcText := srcSection.String()
		answer.WriteString(srcText)
		emit(StreamEvent{Type: "token", Content: srcText})
	}

	// Store assistant message
	assistantMsg, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleAssistant, answer.String(), nil, nil, nil, nil)
	if err != nil {
		slog.Error("failed to store assistant message", "conversation_id", req.ConversationID, "error", err)
		emit(StreamEvent{Type: "error", Content: "Warning: response may not be saved to conversation history"})
	} else {
		msgID, err := models.RecordIDString(assistantMsg.ID)
		if err != nil {
			slog.Warn("unexpected assistant message ID format", "conversation_id", req.ConversationID, "error", err)
		} else {
			emit(StreamEvent{Type: "message_id", Content: msgID})
		}
	}

	emit(StreamEvent{Type: "done", Content: ""})

	// Auto-title: if this is the first user message in the conversation, generate a title
	if len(messages) <= 1 {
		go s.autoTitle(context.Background(), req.ConversationID, req.Content)
	}

	return nil
}

func (s *Service) buildHistory(messages []models.Message, toolContext string) []llm.ChatMessage {
	var history []llm.ChatMessage

	// Inject tool/reference context as a synthetic user-assistant pair, since the LLM API doesn't support separate system context in multi-turn history
	if toolContext != "" {
		history = append(history, llm.ChatMessage{
			Role:    "user",
			Content: "Context from knowledge base:\n" + toolContext,
		})
		history = append(history, llm.ChatMessage{
			Role:    "assistant",
			Content: "I'll use this context to answer your question.",
		})
	}

	for _, msg := range messages {
		switch msg.Role {
		case models.RoleUser:
			history = append(history, llm.ChatMessage{Role: "user", Content: msg.Content})
		case models.RoleAssistant:
			history = append(history, llm.ChatMessage{Role: "assistant", Content: msg.Content})
			// Skip tool_call and tool_result — current turn's results are in toolContext; historical tool messages are omitted
		}
	}

	return history
}

func (s *Service) detectIntent(ctx context.Context, history []llm.ChatMessage, query string, firstTurn bool, sysPrompt string) (*toolAction, error) {
	intentPr := s.buildIntentPrompt()

	prompt := sysPrompt
	if !firstTurn {
		prompt = sysPrompt + "\n\n" + intentPr
	}

	// For intent detection, append the intent prompt to the user query
	fullQuery := query
	if firstTurn {
		fullQuery = query + "\n\n" + intentPr
	}

	response, err := s.model.GenerateWithSystem(ctx, prompt, buildHistoryText(history, fullQuery))
	if err != nil {
		return nil, fmt.Errorf("intent detection: %w", err)
	}

	// Parse JSON response
	response = strings.TrimSpace(response)
	// Strip markdown code fences if present
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var action toolAction
	if err := json.Unmarshal([]byte(response), &action); err != nil {
		slog.Warn("intent detection returned unparseable response, falling back to direct answer",
			"error", err, "response", response)
		return &toolAction{Action: "answer"}, nil
	}
	return &action, nil
}

func buildHistoryText(history []llm.ChatMessage, currentQuery string) string {
	var sb strings.Builder
	for _, msg := range history {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, msg.Content))
	}
	sb.WriteString(fmt.Sprintf("[user]: %s", currentQuery))
	return sb.String()
}

func intPtr(v int) *int { return &v }

func (s *Service) executeTool(ctx context.Context, vaultID string, action *toolAction, emit func(StreamEvent)) (string, []docSource, *ToolResultMeta, error) {
	switch action.Tool {
	case "kb_search":
		var input searchInput
		if err := json.Unmarshal(action.Input, &input); err != nil {
			return "", nil, nil, fmt.Errorf("parse search input: %w", err)
		}
		emit(StreamEvent{Type: "tool_call", Content: input.Query, Tool: "kb_search"})

		start := time.Now()
		results, err := s.search.Search(ctx, search.SearchInput{
			VaultID: vaultID,
			Query:   input.Query,
			Limit:   5,
		})
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, nil, fmt.Errorf("search: %w", err)
		}

		var sb strings.Builder
		var sources []docSource
		var matchedDocs []ToolDocRef
		totalChunks := 0
		for _, r := range results {
			fmt.Fprintf(&sb, "## %s (%s)\n", r.Title, r.Path)
			sources = append(sources, docSource{Title: r.Title, Path: r.Path})
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
		emit(StreamEvent{Type: "tool_result", Content: fmt.Sprintf("%d results", len(results)), Tool: "kb_search", Meta: meta})
		return result, sources, meta, nil

	case "read_document":
		var input readDocInput
		if err := json.Unmarshal(action.Input, &input); err != nil {
			return "", nil, nil, fmt.Errorf("parse read_document input: %w", err)
		}
		emit(StreamEvent{Type: "tool_call", Content: input.Path, Tool: "read_document"})

		start := time.Now()
		doc, err := s.db.GetDocumentByPath(ctx, vaultID, input.Path)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, nil, fmt.Errorf("read document: %w", err)
		}
		if doc == nil {
			meta := &ToolResultMeta{DurationMs: durationMs}
			emit(StreamEvent{Type: "tool_result", Content: "not found", Tool: "read_document", Meta: meta})
			return fmt.Sprintf("Document not found: %s", input.Path), nil, meta, nil
		}

		contentLen := len(doc.ContentBody)
		meta := &ToolResultMeta{
			DurationMs:    durationMs,
			DocumentPath:  &doc.Path,
			DocumentTitle: &doc.Title,
			ContentLength: &contentLen,
		}
		emit(StreamEvent{Type: "tool_result", Content: doc.Path, Tool: "read_document", Meta: meta})
		return fmt.Sprintf("# %s\n\n%s", doc.Title, doc.ContentBody), []docSource{{Title: doc.Title, Path: doc.Path}}, meta, nil

	case "web_search":
		var input webSearchInput
		if err := json.Unmarshal(action.Input, &input); err != nil {
			return "", nil, nil, fmt.Errorf("parse web_search input: %w", err)
		}
		if s.tavily == nil {
			return "", nil, nil, fmt.Errorf("web search not configured")
		}
		emit(StreamEvent{Type: "tool_call", Content: input.Query, Tool: "web_search"})

		start := time.Now()
		result, webResults, err := s.tavily.Search(ctx, input.Query)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			return "", nil, nil, fmt.Errorf("web search: %w", err)
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
		emit(StreamEvent{Type: "tool_result", Content: "Web search complete", Tool: "web_search", Meta: meta})
		return result, nil, meta, nil

	default:
		return "", nil, nil, fmt.Errorf("unknown tool: %s", action.Tool)
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
