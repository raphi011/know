package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateToken creates a new API token with an expiration time.
// expiresAt must not be nil — callers should enforce a max lifetime.
func (c *Client) CreateToken(ctx context.Context, userID, tokenHash, name string, expiresAt time.Time) (*models.APIToken, error) {
	defer c.logOp(ctx, "token.create", time.Now())
	sql := `
		CREATE api_token SET
			user = type::record("user", $user_id),
			token_hash = $token_hash,
			name = $name,
			expires_at = $expires_at
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.APIToken](ctx, c.DB(), sql, map[string]any{
		"user_id":    bareID("user", userID),
		"token_hash": tokenHash,
		"name":       name,
		"expires_at": expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	return firstResult(results, "create token")
}

// GetTokenByID retrieves a token by its record ID.
func (c *Client) GetTokenByID(ctx context.Context, id string) (*models.APIToken, error) {
	defer c.logOp(ctx, "token.get_by_id", time.Now())
	sql := `SELECT * FROM type::record("api_token", $id)`
	results, err := surrealdb.Query[[]models.APIToken](ctx, c.DB(), sql, map[string]any{
		"id": bareID("api_token", id),
	})
	if err != nil {
		return nil, fmt.Errorf("get token by id: %w", err)
	}
	return firstResultOpt(results), nil
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
	return firstResultOpt(results), nil
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
		"user_id": bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return allResults(results), nil
}

func (c *Client) DeleteToken(ctx context.Context, id string) error {
	defer c.logOp(ctx, "token.delete", time.Now())
	sql := `DELETE type::record("api_token", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": bareID("api_token", id)}); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}

// DeleteExpiredTokens removes all tokens whose expires_at has passed.
func (c *Client) DeleteExpiredTokens(ctx context.Context) error {
	defer c.logOp(ctx, "token.delete_expired", time.Now())
	sql := `DELETE api_token WHERE expires_at != NONE AND expires_at < time::now()`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, nil); err != nil {
		return fmt.Errorf("delete expired tokens: %w", err)
	}
	return nil
}

// CountTokens returns the number of tokens owned by a user.
func (c *Client) CountTokens(ctx context.Context, userID string) (int, error) {
	defer c.logOp(ctx, "token.count", time.Now())
	sql := `SELECT count() as count FROM api_token WHERE user = type::record("user", $user_id) GROUP ALL`
	type countResult struct {
		Count int `json:"count"`
	}
	results, err := surrealdb.Query[[]countResult](ctx, c.DB(), sql, map[string]any{
		"user_id": bareID("user", userID),
	})
	if err != nil {
		return 0, fmt.Errorf("count tokens: %w", err)
	}
	r := firstResultOpt(results)
	if r == nil {
		return 0, nil
	}
	return r.Count, nil
}

// RotateToken atomically creates a new token and deletes the old one in a transaction.
// Returns the newly created token. The fetch-after-commit is a separate query; if it
// fails (extremely unlikely transient error), the rotation was still successful — the
// caller should handle this gracefully.
func (c *Client) RotateToken(ctx context.Context, oldTokenID, userID, newHash, name string, newExpiresAt time.Time) (*models.APIToken, error) {
	defer c.logOp(ctx, "token.rotate", time.Now())
	sql := `
		BEGIN TRANSACTION;
		CREATE api_token SET
			user = type::record("user", $user_id),
			token_hash = $new_hash,
			name = $name,
			expires_at = $expires_at;
		DELETE type::record("api_token", $old_id);
		COMMIT TRANSACTION;
	`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"user_id":    bareID("user", userID),
		"new_hash":   newHash,
		"name":       name,
		"expires_at": newExpiresAt,
		"old_id":     bareID("api_token", oldTokenID),
	}); err != nil {
		return nil, fmt.Errorf("rotate token: %w", err)
	}
	return c.GetTokenByHash(ctx, newHash)
}
