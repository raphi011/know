package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateConversation(ctx context.Context, vaultID, userID string) (*models.Conversation, error) {
	defer c.logOp(ctx, "conversation.create", time.Now())
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
	defer c.logOp(ctx, "conversation.get", time.Now())
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
	defer c.logOp(ctx, "conversation.list", time.Now())
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
	defer c.logOp(ctx, "conversation.update_title", time.Now())
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
	defer c.logOp(ctx, "conversation.delete", time.Now())
	sql := `DELETE type::record("conversation", $id)`
	_, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"id": bareID("conversation", id),
	})
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

func (c *Client) UpdateConversationTokens(ctx context.Context, id string, inputTokens, outputTokens int64) error {
	defer c.logOp(ctx, "conversation.update_tokens", time.Now())
	sql := `UPDATE type::record("conversation", $id) SET token_input += $input, token_output += $output`
	_, err := surrealdb.Query[[]models.Conversation](ctx, c.DB(), sql, map[string]any{
		"id":     bareID("conversation", id),
		"input":  inputTokens,
		"output": outputTokens,
	})
	if err != nil {
		return fmt.Errorf("update conversation tokens: %w", err)
	}
	return nil
}

func (c *Client) CreateMessage(ctx context.Context, conversationID string, role models.MessageRole, content string, docRefs []string, toolName, toolInput, toolMeta, toolCallID, toolCalls *string) (*models.Message, error) {
	defer c.logOp(ctx, "message.create", time.Now())
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
			tool_input = $tool_input,
			tool_meta = $tool_meta,
			tool_call_id = $tool_call_id,
			tool_calls = $tool_calls
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Message](ctx, c.DB(), sql, map[string]any{
		"conversation_id": bareID("conversation", conversationID),
		"role":            string(role),
		"content":         content,
		"doc_refs":        docRefs,
		"tool_name":       optionalString(toolName),
		"tool_input":      optionalString(toolInput),
		"tool_meta":       optionalString(toolMeta),
		"tool_call_id":    optionalString(toolCallID),
		"tool_calls":      optionalString(toolCalls),
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
	defer c.logOp(ctx, "message.list", time.Now())
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
