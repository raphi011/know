package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

// ListConversations fetches conversations for a vault.
func (c *Client) ListConversations(ctx context.Context, vaultID string) ([]api.Conversation, error) {
	var convs []api.Conversation
	err := c.rest.Get(ctx, "/api/conversations?vault="+vaultID, &convs)
	return convs, err
}

// GetConversation fetches a conversation with messages.
func (c *Client) GetConversation(ctx context.Context, id string) (*api.Conversation, error) {
	var conv api.Conversation
	err := c.rest.Get(ctx, "/api/conversations/"+id, &conv)
	return &conv, err
}

// CreateConversation creates a new conversation in a vault.
func (c *Client) CreateConversation(ctx context.Context, vaultID string) (*api.Conversation, error) {
	var conv api.Conversation
	err := c.rest.Post(ctx, "/api/conversations", map[string]string{"vaultId": vaultID}, &conv)
	return &conv, err
}

// DeleteConversation deletes a conversation.
func (c *Client) DeleteConversation(ctx context.Context, id string) error {
	return c.rest.Delete(ctx, "/api/conversations/"+id)
}

// RenameConversation renames a conversation.
func (c *Client) RenameConversation(ctx context.Context, id, title string) (*api.Conversation, error) {
	var conv api.Conversation
	err := c.rest.Patch(ctx, "/api/conversations/"+id, map[string]string{"title": title}, &conv)
	return &conv, err
}

// ListVaults fetches accessible vaults.
func (c *Client) ListVaults(ctx context.Context) ([]api.Vault, error) {
	var vaults []api.Vault
	err := c.rest.Get(ctx, "/api/vaults", &vaults)
	return vaults, err
}

// StreamEvent represents a server-sent event from the agent chat endpoint.
type StreamEvent struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	// Additional fields for tool events
	ToolName string `json:"toolName,omitempty"`
	CallID   string `json:"callId,omitempty"`
	Diff     string `json:"diff,omitempty"`
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
				continue
			}
			select {
			case ch <- event:
			case <-ctx.Done():
				return
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
