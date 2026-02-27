package db

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/v2/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateUser(ctx context.Context, input models.UserInput) (*models.User, error) {
	sql := `
		CREATE user SET
			name = $name,
			email = $email
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"name":  input.Name,
		"email": optionalString(input.Email),
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create user: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetUser(ctx context.Context, id string) (*models.User, error) {
	sql := `SELECT * FROM type::record("user", $id)`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetUserByName(ctx context.Context, name string) (*models.User, error) {
	sql := `SELECT * FROM user WHERE name = $name LIMIT 1`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"name": name,
	})
	if err != nil {
		return nil, fmt.Errorf("get user by name: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}
