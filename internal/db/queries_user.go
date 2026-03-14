package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateUser(ctx context.Context, input models.UserInput) (*models.User, error) {
	defer c.logOp(ctx, "user.create", time.Now())
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
	return firstResult(results, "create user")
}

func (c *Client) CreateUserWithID(ctx context.Context, userID string, input models.UserInput) (*models.User, error) {
	defer c.logOp(ctx, "user.create_with_id", time.Now())
	sql := `
		CREATE type::record("user", $user_id) SET
			name = $name,
			email = $email
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"user_id": userID,
		"name":    input.Name,
		"email":   optionalString(input.Email),
	})
	if err != nil {
		return nil, fmt.Errorf("create user with id: %w", err)
	}
	return firstResult(results, "create user with id")
}

func (c *Client) GetUser(ctx context.Context, id string) (*models.User, error) {
	defer c.logOp(ctx, "user.get", time.Now())
	sql := `SELECT * FROM type::record("user", $id)`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return firstResultOpt(results), nil
}

func (c *Client) GetUserByName(ctx context.Context, name string) (*models.User, error) {
	defer c.logOp(ctx, "user.get_by_name", time.Now())
	sql := `SELECT * FROM user WHERE name = $name LIMIT 1`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"name": name,
	})
	if err != nil {
		return nil, fmt.Errorf("get user by name: %w", err)
	}
	return firstResultOpt(results), nil
}

// UpdateUserSystemAdmin sets the is_system_admin flag on a user.
func (c *Client) UpdateUserSystemAdmin(ctx context.Context, userID string, isAdmin bool) error {
	defer c.logOp(ctx, "user.update_system_admin", time.Now())
	sql := `UPDATE type::record("user", $id) SET is_system_admin = $is_admin`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":       bareID("user", userID),
		"is_admin": isAdmin,
	}); err != nil {
		return fmt.Errorf("update user system admin: %w", err)
	}
	return nil
}
