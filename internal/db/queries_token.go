package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateToken(ctx context.Context, userID, tokenHash, name string) (*models.APIToken, error) {
	defer c.logOp(ctx, "token.create", time.Now())
	sql := `
		CREATE api_token SET
			user = type::record("user", $user_id),
			token_hash = $token_hash,
			name = $name
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.APIToken](ctx, c.DB(), sql, map[string]any{
		"user_id":    bareID("user", userID),
		"token_hash": tokenHash,
		"name":       name,
	})
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create token: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetTokenByHash(ctx context.Context, hash string) (*models.APIToken, error) {
	defer c.logOp(ctx, "token.get_by_hash", time.Now())
	sql := `SELECT * FROM api_token WHERE token_hash = $hash LIMIT 1`
	results, err := surrealdb.Query[[]models.APIToken](ctx, c.DB(), sql, map[string]any{
		"hash": hash,
	})
	if err != nil {
		return nil, fmt.Errorf("get token by hash: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) UpdateTokenLastUsed(ctx context.Context, tokenID string) error {
	defer c.logOp(ctx, "token.update_last_used", time.Now())
	sql := `UPDATE type::record("api_token", $id) SET last_used = time::now()`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": tokenID}); err != nil {
		return fmt.Errorf("update token last used: %w", err)
	}
	return nil
}

func (c *Client) ListTokens(ctx context.Context, userID string) ([]models.APIToken, error) {
	defer c.logOp(ctx, "token.list", time.Now())
	sql := `SELECT * FROM api_token WHERE user = type::record("user", $user_id) ORDER BY created_at DESC`
	results, err := surrealdb.Query[[]models.APIToken](ctx, c.DB(), sql, map[string]any{
		"user_id": userID,
	})
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) DeleteToken(ctx context.Context, id string) error {
	defer c.logOp(ctx, "token.delete", time.Now())
	sql := `DELETE type::record("api_token", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}
