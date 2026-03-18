package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/raphi011/know/internal/api"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/diff"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tools"
)

// Client wraps apiclient.Client with TUI-specific methods.
type Client struct {
	rest    *apiclient.Client
	baseURL string
	token   string
}

// NewClient creates a new TUI client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		rest:    apiclient.New(baseURL, token),
		baseURL: baseURL,
		token:   token,
	}
}

// CreateConversation creates a new conversation in a vault.
func (c *Client) CreateConversation(ctx context.Context, vaultName string) (*api.Conversation, error) {
	var conv api.Conversation
	err := c.rest.Post(ctx, "/api/v1/conversations", map[string]string{"vaultId": vaultName}, &conv)
	return &conv, err
}

// StreamEvent is a TUI subset of agent.StreamEvent (internal/agent/service.go).
// Only fields used by the TUI are included; unknown JSON fields are ignored.
// Keep in sync with the canonical type when adding new fields.
type StreamEvent struct {
	Type              string                `json:"type"` // "text" | "tool_start" | "tool_end" | "interrupted" | "msg_end" | "conv_id" | "error"
	Content           string                `json:"content,omitempty"`
	ConvID            string                `json:"convId,omitempty"`
	Tool              string                `json:"tool,omitempty"`
	CallID            string                `json:"callId,omitempty"`
	InterruptID       string                `json:"interruptId,omitempty"`
	Input             map[string]any        `json:"input,omitempty"`
	Meta              *tools.ToolResultMeta `json:"meta,omitempty"`
	Approval          *Approval             `json:"approval,omitempty"`
	InputTokens       int64                 `json:"inputTokens,omitempty"`
	OutputTokens      int64                 `json:"outputTokens,omitempty"`
	ContextWindowMax  int                   `json:"contextWindowMax,omitempty"`
	ContextWindowUsed int64                 `json:"contextWindowUsed,omitempty"`
}

// Approval carries diff data for write-tool approval dialogs.
// Mirrors agent.ApprovalRequest fields needed by the TUI.
type Approval struct {
	Path    string       `json:"path"`
	IsNew   bool         `json:"isNew"`
	Diff    *DiffPayload `json:"diff,omitempty"`
	Content string       `json:"content,omitempty"`
}

// DiffPayload contains hunks and stats for an edit diff.
type DiffPayload struct {
	Hunks []diff.Hunk    `json:"hunks"`
	Stats diff.DiffStats `json:"stats"`
}

// chatStartResponse is the JSON body returned by POST /agent/chat (202 Accepted).
type chatStartResponse struct {
	ConversationID string `json:"conversationId"`
	Status         string `json:"status"`
}

// StartChat sends a chat request and returns the conversation ID.
// The agent runs in the background on the server.
func (c *Client) StartChat(ctx context.Context, conversationID, vaultName, content string, attachments []models.ChatAttachment, autoApprove bool) (string, error) {
	body := map[string]any{
		"conversationId": conversationID,
		"content":        content,
		"autoApprove":    autoApprove,
	}
	if len(attachments) > 0 {
		body["attachments"] = attachments
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/vaults/"+url.PathEscape(vaultName)+"/agent/chat", strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("chat request failed: HTTP %d", resp.StatusCode)
	}

	var result chatStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}

	return result.ConversationID, nil
}

// SubscribeEvents connects to the SSE event stream for a running agent.
func (c *Client) SubscribeEvents(ctx context.Context, conversationID string) (<-chan StreamEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/agent/events/"+conversationID, nil)
	if err != nil {
		return nil, fmt.Errorf("create events request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscribe events: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("events request failed: HTTP %d", resp.StatusCode)
	}

	ch := make(chan StreamEvent, 16)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var event StreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				slog.Warn("SSE event unmarshal failed", "data", data, "error", err)
				select {
				case ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("failed to parse server event: %v", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("stream read error: %v", err)}:
			case <-ctx.Done():
			}
		}
	}()

	return ch, nil
}

// Chat sends a message and returns a channel of SSE events.
// This is a convenience method that calls StartChat + SubscribeEvents.
func (c *Client) Chat(ctx context.Context, conversationID, vaultID, content string, attachments []models.ChatAttachment, autoApprove bool) (<-chan StreamEvent, error) {
	convID, err := c.StartChat(ctx, conversationID, vaultID, content, attachments, autoApprove)
	if err != nil {
		return nil, err
	}

	ch, err := c.SubscribeEvents(ctx, convID)
	if err != nil {
		return nil, err
	}

	return ch, nil
}

// Approve sends a tool approval to the server using eino's interrupt/resume.
func (c *Client) Approve(ctx context.Context, conversationID, interruptID, action string) error {
	return c.rest.Post(ctx, "/api/v1/agent/approval", map[string]string{
		"conversationId": conversationID,
		"interruptId":    interruptID,
		"action":         action,
	}, nil)
}
