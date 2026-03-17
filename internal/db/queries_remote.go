package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateRemote creates a new remote server configuration.
func (c *Client) CreateRemote(ctx context.Context, userID string, input models.RemoteInput) (*models.Remote, error) {
	defer c.logOp(ctx, "remote.create", time.Now())

	sql := `
		CREATE remote SET
			name = $name,
			url = $url,
			token = $api_token,
			created_by = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Remote](ctx, c.DB(), sql, map[string]any{
		"name":      input.Name,
		"url":       input.URL,
		"api_token": input.Token,
		"user_id":   bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("create remote: %w", err)
	}
	return firstResult(results, "create remote")
}

// GetRemoteByName returns a remote by its unique name.
func (c *Client) GetRemoteByName(ctx context.Context, name string) (*models.Remote, error) {
	defer c.logOp(ctx, "remote.get_by_name", time.Now())

	sql := `SELECT * FROM remote WHERE name = $name LIMIT 1`
	results, err := surrealdb.Query[[]models.Remote](ctx, c.DB(), sql, map[string]any{
		"name": name,
	})
	if err != nil {
		return nil, fmt.Errorf("get remote by name: %w", err)
	}
	return firstResultOpt(results), nil
}

// ListRemotes returns all configured remotes.
func (c *Client) ListRemotes(ctx context.Context) ([]models.Remote, error) {
	defer c.logOp(ctx, "remote.list", time.Now())

	sql := `SELECT * FROM remote ORDER BY name ASC`
	results, err := surrealdb.Query[[]models.Remote](ctx, c.DB(), sql, nil)
	if err != nil {
		return nil, fmt.Errorf("list remotes: %w", err)
	}
	return allResults(results), nil
}

// DeleteRemote deletes a remote by name. Returns true if a record was deleted.
func (c *Client) DeleteRemote(ctx context.Context, name string) (bool, error) {
	defer c.logOp(ctx, "remote.delete", time.Now())

	sql := `DELETE FROM remote WHERE name = $name RETURN BEFORE`
	results, err := surrealdb.Query[[]models.Remote](ctx, c.DB(), sql, map[string]any{
		"name": name,
	})
	if err != nil {
		return false, fmt.Errorf("delete remote: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return false, nil
	}
	return len((*results)[0].Result) > 0, nil
}
