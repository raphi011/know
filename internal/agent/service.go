package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/search"
)

const maxToolIterations = 5

// StreamEvent is sent to the client via SSE.
type StreamEvent struct {
	Type    string `json:"type"`           // "token" | "tool_call" | "tool_result" | "done" | "error" | "message_id" | "conversation_id"
	Content string `json:"content"`        // token text, tool result, error message, or message ID
	Tool    string `json:"tool,omitempty"` // tool name for tool_call/tool_result events
}

// Service orchestrates the agent loop: intent detection → tool execution → streaming answer.
type Service struct {
	db     *db.Client
	model  *llm.Model
	search *search.Service
}

// NewService creates a new agent service.
func NewService(db *db.Client, model *llm.Model, search *search.Service) *Service {
	return &Service{db: db, model: model, search: search}
}

// Available returns true if the agent has an LLM model configured.
func (s *Service) Available() bool {
	return s.model != nil
}

// toolAction is the LLM's decision from intent detection.
type toolAction struct {
	Action string          `json:"action"` // "tool" or "answer"
	Tool   string          `json:"tool"`   // "kb_search" or "read_document"
	Input  json.RawMessage `json:"input"`
}

type searchInput struct {
	Query string `json:"query"`
}

type readDocInput struct {
	Path string `json:"path"`
}

const systemPrompt = `You are a helpful knowledge assistant for the Knowhow knowledge base. You help users find and understand information stored in their documents.

You have access to two tools:
1. kb_search(query) - Search the knowledge base for relevant documents
2. read_document(path) - Read the full content of a specific document by path

When the user asks a question:
- If you need to find relevant information, use kb_search first
- If you need the full content of a specific document, use read_document
- If you already have enough context from previous tool results, answer directly

Always cite document paths when referencing information from the knowledge base.
Be concise and helpful. Answer based on the knowledge base content when available.
Do not include a sources section at the end of your response — one will be added automatically.`

const intentPrompt = `Based on the conversation so far, decide your next action.

If you need to search for information, respond with EXACTLY this JSON (no other text):
{"action":"tool","tool":"kb_search","input":{"query":"your search query"}}

If you need to read a specific document, respond with EXACTLY this JSON (no other text):
{"action":"tool","tool":"read_document","input":{"path":"/document/path"}}

If you have enough information to answer the user's question, respond with EXACTLY this JSON (no other text):
{"action":"answer"}

Respond with ONLY the JSON, no explanation.`

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
	userMsg, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleUser, req.Content, req.DocRefs, nil, nil)
	if err != nil {
		return fmt.Errorf("create user message: %w", err)
	}
	userMsgID, err := models.RecordIDString(userMsg.ID)
	if err != nil {
		return fmt.Errorf("extract user message ID: %w", err)
	}
	emit(StreamEvent{Type: "message_id", Content: userMsgID})

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
			intentQuery = intentPrompt
		}

		decision, err := s.detectIntent(ctx, history, intentQuery, i == 0)
		if err != nil {
			slog.Warn("intent detection failed, falling back to direct answer", "error", err)
			emit(StreamEvent{Type: "error", Content: "Knowledge base search unavailable, answering from general knowledge"})
			break // fall through to streaming answer
		}

		if decision.Action == "answer" {
			break // proceed to streaming answer
		}

		if decision.Action == "tool" {
			result, toolSources, err := s.executeTool(ctx, req.VaultID, decision, emit)
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
			}
			toolInputStr := string(toolInput)
			_, err = s.db.CreateMessage(ctx, req.ConversationID, models.RoleToolCall, "", nil, &decision.Tool, &toolInputStr)
			if err != nil {
				slog.Error("failed to store tool call message", "conversation_id", req.ConversationID, "tool", decision.Tool, "error", err)
			}
			_, err = s.db.CreateMessage(ctx, req.ConversationID, models.RoleToolResult, result, nil, &decision.Tool, nil)
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

	err = s.model.GenerateWithSystemStreamMultiTurn(ctx, systemPrompt, history, req.Content, func(token string) error {
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
	assistantMsg, err := s.db.CreateMessage(ctx, req.ConversationID, models.RoleAssistant, answer.String(), nil, nil, nil)
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

func (s *Service) detectIntent(ctx context.Context, history []llm.ChatMessage, query string, firstTurn bool) (*toolAction, error) {
	prompt := systemPrompt
	if !firstTurn {
		prompt = systemPrompt + "\n\n" + intentPrompt
	}

	// For intent detection, append the intent prompt to the user query
	fullQuery := query
	if firstTurn {
		fullQuery = query + "\n\n" + intentPrompt
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

func (s *Service) executeTool(ctx context.Context, vaultID string, action *toolAction, emit func(StreamEvent)) (string, []docSource, error) {
	switch action.Tool {
	case "kb_search":
		var input searchInput
		if err := json.Unmarshal(action.Input, &input); err != nil {
			return "", nil, fmt.Errorf("parse search input: %w", err)
		}
		emit(StreamEvent{Type: "tool_call", Content: input.Query, Tool: "kb_search"})

		results, err := s.search.Search(ctx, search.SearchInput{
			VaultID: vaultID,
			Query:   input.Query,
			Limit:   5,
		})
		if err != nil {
			return "", nil, fmt.Errorf("search: %w", err)
		}

		var sb strings.Builder
		var sources []docSource
		for _, r := range results {
			fmt.Fprintf(&sb, "## %s (%s)\n", r.Title, r.Path)
			sources = append(sources, docSource{Title: r.Title, Path: r.Path})
			for _, ch := range r.MatchedChunks {
				sb.WriteString(ch.Snippet)
				sb.WriteString("\n\n")
			}
		}

		result := sb.String()
		if result == "" {
			result = "No results found."
		}
		emit(StreamEvent{Type: "tool_result", Content: fmt.Sprintf("%d results", len(results)), Tool: "kb_search"})
		return result, sources, nil

	case "read_document":
		var input readDocInput
		if err := json.Unmarshal(action.Input, &input); err != nil {
			return "", nil, fmt.Errorf("parse read_document input: %w", err)
		}
		emit(StreamEvent{Type: "tool_call", Content: input.Path, Tool: "read_document"})

		doc, err := s.db.GetDocumentByPath(ctx, vaultID, input.Path)
		if err != nil {
			return "", nil, fmt.Errorf("read document: %w", err)
		}
		if doc == nil {
			emit(StreamEvent{Type: "tool_result", Content: "not found", Tool: "read_document"})
			return fmt.Sprintf("Document not found: %s", input.Path), nil, nil
		}

		emit(StreamEvent{Type: "tool_result", Content: doc.Path, Tool: "read_document"})
		return fmt.Sprintf("# %s\n\n%s", doc.Title, doc.ContentBody), []docSource{{Title: doc.Title, Path: doc.Path}}, nil

	default:
		return "", nil, fmt.Errorf("unknown tool: %s", action.Tool)
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
