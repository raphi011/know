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
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create vault member: no result returned")
	}
	return &(*results)[0].Result[0], nil
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
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
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
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// UpdateVaultMemberRole updates the role of a vault member.
func (c *Client) UpdateVaultMemberRole(ctx context.Context, userID, vaultID string, role models.VaultRole) error {
	defer c.logOp(ctx, "vault_member.update_role", time.Now())
	sql := `UPDATE vault_member SET role = $role WHERE user = type::record("user", $user_id) AND vault = type::record("vault", $vault_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"user_id":  bareID("user", userID),
		"vault_id": bareID("vault", vaultID),
		"role":     string(role),
	}); err != nil {
		return fmt.Errorf("update vault member role: %w", err)
	}
	return nil
}

// DeleteVaultMember removes a user's membership from a vault.
func (c *Client) DeleteVaultMember(ctx context.Context, userID, vaultID string) error {
	defer c.logOp(ctx, "vault_member.delete", time.Now())
	sql := `DELETE FROM vault_member WHERE user = type::record("user", $user_id) AND vault = type::record("vault", $vault_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"user_id":  bareID("user", userID),
		"vault_id": bareID("vault", vaultID),
	}); err != nil {
		return fmt.Errorf("delete vault member: %w", err)
	}
	return nil
}
