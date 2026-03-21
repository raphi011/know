package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateOAuthAuthCode(ctx context.Context, code, encryptedToken, codeChallenge, redirectURI string, expiresAt time.Time) error {
	defer c.logOp(ctx, "oauth_auth_code.create", time.Now())
	sql := `CREATE oauth_auth_code SET code = $code, encrypted_token = $encrypted_token, code_challenge = $code_challenge, redirect_uri = $redirect_uri, expires_at = $expires_at`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"code":            code,
		"encrypted_token": encryptedToken,
		"code_challenge":  codeChallenge,
		"redirect_uri":    redirectURI,
		"expires_at":      expiresAt,
	}); err != nil {
		return fmt.Errorf("create oauth auth code: %w", err)
	}
	return nil
}

func (c *Client) GetOAuthAuthCode(ctx context.Context, code string) (*models.OAuthAuthCode, error) {
	defer c.logOp(ctx, "oauth_auth_code.get", time.Now())
	sql := `SELECT * FROM oauth_auth_code WHERE code = $code LIMIT 1`
	results, err := surrealdb.Query[[]models.OAuthAuthCode](ctx, c.DB(), sql, map[string]any{
		"code": code,
	})
	if err != nil {
		return nil, fmt.Errorf("get oauth auth code: %w", err)
	}
	return firstResultOpt(results), nil
}

// ConsumeOAuthAuthCode atomically deletes an auth code and returns it.
// Returns nil if the code does not exist (already consumed or never created).
func (c *Client) ConsumeOAuthAuthCode(ctx context.Context, code string) (*models.OAuthAuthCode, error) {
	defer c.logOp(ctx, "oauth_auth_code.consume", time.Now())
	sql := `DELETE oauth_auth_code WHERE code = $code AND expires_at > time::now() RETURN BEFORE`
	results, err := surrealdb.Query[[]models.OAuthAuthCode](ctx, c.DB(), sql, map[string]any{
		"code": code,
	})
	if err != nil {
		return nil, fmt.Errorf("consume oauth auth code: %w", err)
	}
	return firstResultOpt(results), nil
}

func (c *Client) DeleteOAuthAuthCode(ctx context.Context, code string) error {
	defer c.logOp(ctx, "oauth_auth_code.delete", time.Now())
	sql := `DELETE oauth_auth_code WHERE code = $code`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"code": code,
	}); err != nil {
		return fmt.Errorf("delete oauth auth code: %w", err)
	}
	return nil
}

func (c *Client) DeleteExpiredOAuthAuthCodes(ctx context.Context) error {
	defer c.logOp(ctx, "oauth_auth_code.delete_expired", time.Now())
	sql := `DELETE oauth_auth_code WHERE expires_at < time::now()`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, nil); err != nil {
		return fmt.Errorf("delete expired oauth auth codes: %w", err)
	}
	return nil
}
