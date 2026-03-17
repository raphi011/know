package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateVaultMember creates a new vault membership for a user.
func (c *Client) CreateVaultMember(ctx context.Context, userID, vaultID string, role models.VaultRole) (*models.VaultMember, error) {
	defer c.logOp(ctx, "vault_member.create", time.Now())
	sql := `
		CREATE vault_member SET
			user = type::record("user", $user_id),
			vault = type::record("vault", $vault_id),
			role = $role
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.VaultMember](ctx, c.DB(), sql, map[string]any{
		"user_id":  bareID("user", userID),
		"vault_id": bareID("vault", vaultID),
		"role":     string(role),
	})
	if err != nil {
		return nil, fmt.Errorf("create vault member: %w", err)
	}
	return firstResult(results, "create vault member")
}

// GetVaultMemberships returns all vault memberships for a user.
func (c *Client) GetVaultMemberships(ctx context.Context, userID string) ([]models.VaultMember, error) {
	defer c.logOp(ctx, "vault_member.get_memberships", time.Now())
	sql := `SELECT * FROM vault_member WHERE user = type::record("user", $user_id)`
	results, err := surrealdb.Query[[]models.VaultMember](ctx, c.DB(), sql, map[string]any{
		"user_id": bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("get vault memberships: %w", err)
	}
	return allResults(results), nil
}

// GetVaultMembers returns all members of a vault.
func (c *Client) GetVaultMembers(ctx context.Context, vaultID string) ([]models.VaultMember, error) {
	defer c.logOp(ctx, "vault_member.get_members", time.Now())
	sql := `SELECT * FROM vault_member WHERE vault = type::record("vault", $vault_id)`
	results, err := surrealdb.Query[[]models.VaultMember](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("get vault members: %w", err)
	}
	return allResults(results), nil
}
