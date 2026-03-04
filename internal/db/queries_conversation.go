package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateConversation(ctx context.Context, vaultID, userID string) (*models.Conversation, error) {
	sql := `
		CREATE conversation SET
			vault = type::record("vault", $vault_id),
			user = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"user_id":  bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create conversation: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetConversation(ctx context.Context, id string) (*models.Conversation, error) {
	sql := `SELECT * FROM type::record("conversation", $id)`
	results, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"id": bareID("conversation", id),
	})
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) ListConversations(ctx context.Context, vaultID, userID string) ([]models.Conversation, error) {
	sql := `SELECT * FROM conversation WHERE vault = type::record("vault", $vault_id) AND user = type::record("user", $user_id) ORDER BY updated_at DESC`
	results, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"user_id":  bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) UpdateConversationTitle(ctx context.Context, id, title string) error {
	sql := `UPDATE type::record("conversation", $id) SET title = $title`
	_, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"id":    bareID("conversation", id),
		"title": title,
	})
	if err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	return nil
}

func (c *Client) DeleteConversation(ctx context.Context, id string) error {
	sql := `DELETE type::record("conversation", $id)`
	_, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"id": bareID("conversation", id),
	})
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

func (c *Client) CreateMessage(ctx context.Context, conversationID string, role models.MessageRole, content string, docRefs []string, toolName, toolInput *string) (*models.Message, error) {
	if docRefs == nil {
		docRefs = []string{}
	}
	sql := `
		CREATE message SET
			conversation = type::record("conversation", $conversation_id),
			role = $role,
			content = $content,
			doc_refs = $doc_refs,
			tool_name = $tool_name,
			tool_input = $tool_input
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Message](ctx, c.DB(), sql, map[string]any{
		"conversation_id": bareID("conversation", conversationID),
		"role":            string(role),
		"content":         content,
		"doc_refs":        docRefs,
		"tool_name":       optionalString(toolName),
		"tool_input":      optionalString(toolInput),
	})
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create message: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) ListMessages(ctx context.Context, conversationID string) ([]models.Message, error) {
	sql := `SELECT * FROM message WHERE conversation = type::record("conversation", $conversation_id) ORDER BY created_at ASC`
	results, err := surrealdb.Query[[]models.Message](ctx, c.DB(), sql, map[string]any{
		"conversation_id": bareID("conversation", conversationID),
	})
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}
