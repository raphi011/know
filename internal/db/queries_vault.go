package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func (c *Client) CreateVault(ctx context.Context, userID string, input models.VaultInput) (*models.Vault, error) {
	sql := `
		CREATE vault SET
			name = $name,
			description = $description,
			created_by = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"name":        input.Name,
		"description": optionalString(input.Description),
		"user_id":     userID,
	})
	if err != nil {
		return nil, fmt.Errorf("create vault: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create vault: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) CreateVaultWithID(ctx context.Context, vaultID string, userID string, input models.VaultInput) (*models.Vault, error) {
	sql := `
		CREATE type::record("vault", $vault_id) SET
			name = $name,
			description = $description,
			created_by = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"vault_id":    bareID("vault", vaultID),
		"name":        input.Name,
		"description": optionalString(input.Description),
		"user_id":     userID,
	})
	if err != nil {
		return nil, fmt.Errorf("create vault with id: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create vault with id: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetVault(ctx context.Context, id string) (*models.Vault, error) {
	sql := `SELECT * FROM type::record("vault", $id)`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get vault: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetVaultByName(ctx context.Context, name string) (*models.Vault, error) {
	sql := `SELECT * FROM vault WHERE name = $name LIMIT 1`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"name": name,
	})
	if err != nil {
		return nil, fmt.Errorf("get vault by name: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) ListVaults(ctx context.Context) ([]models.Vault, error) {
	sql := `SELECT * FROM vault ORDER BY name ASC`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, nil)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) DeleteVault(ctx context.Context, id string) error {
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete vault begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	// Delete all vault data, then the vault itself.
	// Document cascade events (ASYNC) handle chunks, wiki_links, and relations after commit.
	vars := map[string]any{"id": id}
	for _, t := range []struct {
		name string
		sql  string
	}{
		{"document", `DELETE FROM document WHERE vault = type::record("vault", $id)`},
		{"folder", `DELETE FROM folder WHERE vault = type::record("vault", $id)`},
		{"template", `DELETE FROM template WHERE vault = type::record("vault", $id)`},
		{"vault", `DELETE type::record("vault", $id)`},
	} {
		if _, err := surrealdb.Query[any](ctx, tx, t.sql, vars); err != nil {
			return fmt.Errorf("delete vault %s: %w", t.name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete vault commit: %w", err)
	}
	return nil
}

// ListDocumentPaths returns all document paths in a vault (for folder derivation).
func (c *Client) ListDocumentPaths(ctx context.Context, vaultID string) ([]string, error) {
	sql := `SELECT path FROM document WHERE vault = type::record("vault", $vault_id)`
	results, err := surrealdb.Query[[]struct {
		Path string `json:"path"`
	}](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list document paths: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	paths := make([]string, len((*results)[0].Result))
	for i, r := range (*results)[0].Result {
		paths[i] = r.Path
	}
	return paths, nil
}

// UpdateVaultTokenAccess adds a vault to a token's vault_access list.
func (c *Client) UpdateVaultTokenAccess(ctx context.Context, tokenID string, vaultID string) error {
	sql := `UPDATE type::record("api_token", $token_id) SET vault_access += [type::record("vault", $vault_id)]`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"token_id": tokenID,
		"vault_id": bareID("vault", vaultID),
	}); err != nil {
		return fmt.Errorf("update vault token access: %w", err)
	}
	return nil
}

func newRecordID(table, id string) surrealmodels.RecordID {
	return surrealmodels.RecordID{Table: table, ID: id}
}
