package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/raphi011/knowhow/internal/api"
	"github.com/raphi011/knowhow/internal/apiclient"
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
func (c *Client) CreateConversation(ctx context.Context, vaultID string) (*api.Conversation, error) {
	var conv api.Conversation
	err := c.rest.Post(ctx, "/api/conversations", map[string]string{"vaultId": vaultID}, &conv)
	return &conv, err
}

// StreamEvent represents a server-sent event from the agent chat endpoint.
// Field names and types must match the server's agent.StreamEvent JSON output.
type StreamEvent struct {
	Type         string `json:"type"`              // "text" | "tool_start" | "tool_end" | "tool_approval_required" | "msg_end" | "conv_id" | "error"
	Content      string `json:"content,omitempty"`
	ConvID       string `json:"convId,omitempty"`
	Tool         string `json:"tool,omitempty"`
	CallID       string `json:"callId,omitempty"`
	Diff         string `json:"diff,omitempty"`
	InputTokens  int64  `json:"inputTokens,omitempty"`
	OutputTokens int64  `json:"outputTokens,omitempty"`
}

// Chat sends a message and returns a channel of SSE events.
func (c *Client) Chat(ctx context.Context, conversationID, vaultID, content string, autoApprove bool) (<-chan StreamEvent, error) {
	body := map[string]any{
		"conversationId": conversationID,
		"vaultId":        vaultID,
		"content":        content,
		"autoApprove":    autoApprove,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/agent/chat", strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send chat request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("chat request failed: HTTP %d", resp.StatusCode)
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

// Approve sends a tool approval to the server.
func (c *Client) Approve(ctx context.Context, conversationID, callID, action string) error {
	return c.rest.Post(ctx, "/agent/approval", map[string]string{
		"conversationId": conversationID,
		"callId":         callID,
		"action":         action,
	}, nil)
}
