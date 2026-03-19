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

// GetUserByOIDCSubject finds a user by OIDC provider and subject.
func (c *Client) GetUserByOIDCSubject(ctx context.Context, provider, subject string) (*models.User, error) {
	defer c.logOp(ctx, "user.get_by_oidc_subject", time.Now())
	sql := `SELECT * FROM user WHERE oidc_provider = $provider AND oidc_subject = $subject LIMIT 1`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"provider": provider,
		"subject":  subject,
	})
	if err != nil {
		return nil, fmt.Errorf("get user by oidc subject: %w", err)
	}
	return firstResultOpt(results), nil
}

// GetUserByEmail finds a user by email address.
func (c *Client) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	defer c.logOp(ctx, "user.get_by_email", time.Now())
	sql := `SELECT * FROM user WHERE email = $email LIMIT 1`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"email": email,
	})
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return firstResultOpt(results), nil
}

// CreateUserFromOIDC creates a user from OIDC claims.
func (c *Client) CreateUserFromOIDC(ctx context.Context, provider, subject, name, email string) (*models.User, error) {
	defer c.logOp(ctx, "user.create_from_oidc", time.Now())
	sql := `
		CREATE user SET
			name = $name,
			email = $email,
			oidc_provider = $provider,
			oidc_subject = $subject
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, map[string]any{
		"name":     name,
		"email":    email,
		"provider": provider,
		"subject":  subject,
	})
	if err != nil {
		return nil, fmt.Errorf("create user from oidc: %w", err)
	}
	return firstResult(results, "create user from oidc")
}

// LinkOIDCIdentity links an OIDC identity to an existing user.
func (c *Client) LinkOIDCIdentity(ctx context.Context, userID, provider, subject string) error {
	defer c.logOp(ctx, "user.link_oidc", time.Now())
	sql := `UPDATE type::record("user", $id) SET oidc_provider = $provider, oidc_subject = $subject`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":       bareID("user", userID),
		"provider": provider,
		"subject":  subject,
	}); err != nil {
		return fmt.Errorf("link oidc identity: %w", err)
	}
	return nil
}

// ListUsers returns all users ordered by name.
func (c *Client) ListUsers(ctx context.Context) ([]models.User, error) {
	defer c.logOp(ctx, "user.list", time.Now())
	sql := `SELECT * FROM user ORDER BY name ASC`
	results, err := surrealdb.Query[[]models.User](ctx, c.DB(), sql, nil)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return allResults(results), nil
}

// ProvisionUser creates a user with a private vault and admin membership.
// Returns the created user and their private vault.
func (c *Client) ProvisionUser(ctx context.Context, input models.UserInput) (*models.User, *models.Vault, error) {
	defer c.logOp(ctx, "user.provision", time.Now())

	user, err := c.CreateUser(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("provision user: %w", err)
	}

	vault, err := c.provisionVaultForUser(ctx, user)
	if err != nil {
		return nil, nil, fmt.Errorf("provision user: %w", err)
	}

	return user, vault, nil
}

// ProvisionUserFromOIDC creates a user from OIDC claims with a private vault and admin membership.
func (c *Client) ProvisionUserFromOIDC(ctx context.Context, provider, subject, name, email string) (*models.User, *models.Vault, error) {
	defer c.logOp(ctx, "user.provision_from_oidc", time.Now())

	user, err := c.CreateUserFromOIDC(ctx, provider, subject, name, email)
	if err != nil {
		return nil, nil, fmt.Errorf("provision oidc user: %w", err)
	}

	vault, err := c.provisionVaultForUser(ctx, user)
	if err != nil {
		return nil, nil, fmt.Errorf("provision oidc user: %w", err)
	}

	return user, vault, nil
}

// provisionVaultForUser creates a private vault owned by the user and adds admin membership.
func (c *Client) provisionVaultForUser(ctx context.Context, user *models.User) (*models.Vault, error) {
	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		return nil, fmt.Errorf("extract user id: %w", err)
	}

	vault, err := c.CreateVaultWithOwner(ctx, userID, models.VaultInput{
		Name: user.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("create vault: %w", err)
	}

	vaultID, err := models.RecordIDString(vault.ID)
	if err != nil {
		return nil, fmt.Errorf("extract vault id: %w", err)
	}

	if _, err := c.CreateVaultMember(ctx, userID, vaultID, models.RoleAdmin); err != nil {
		return nil, fmt.Errorf("create membership: %w", err)
	}

	return vault, nil
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
