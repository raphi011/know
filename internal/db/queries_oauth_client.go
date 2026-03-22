package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateOAuthClient(ctx context.Context, clientID, clientName string, redirectURIs []string) error {
	defer c.logOp(ctx, "oauth_client.create", time.Now())
	sql := `CREATE oauth_client SET client_id = $client_id, client_name = $client_name, redirect_uris = $redirect_uris`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"client_id":     clientID,
		"client_name":   clientName,
		"redirect_uris": redirectURIs,
	}); err != nil {
		return fmt.Errorf("create: %w", err)
	}
	return nil
}

func (c *Client) GetOAuthClient(ctx context.Context, clientID string) (*models.OAuthClient, error) {
	defer c.logOp(ctx, "oauth_client.get", time.Now())
	sql := `SELECT * FROM oauth_client WHERE client_id = $client_id LIMIT 1`
	results, err := surrealdb.Query[[]models.OAuthClient](ctx, c.DB(), sql, map[string]any{
		"client_id": clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return firstResultOpt(results), nil
}
