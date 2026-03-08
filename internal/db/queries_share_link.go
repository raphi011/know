package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// CreateShareLink creates a new share link for a doc/folder path.
func (c *Client) CreateShareLink(ctx context.Context, vaultID, tokenHash, path string, isFolder bool, createdBy string, expiresAt *string) (*models.ShareLink, error) {
	var expiry any = surrealmodels.None
	if expiresAt != nil {
		expiry = *expiresAt
	}

	sql := `
		CREATE share_link SET
			vault = type::record("vault", $vault_id),
			token_hash = $token_hash,
			path = $path,
			is_folder = $is_folder,
			created_by = type::record("user", $created_by),
			expires_at = $expires_at
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.ShareLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"token_hash": tokenHash,
		"path":       path,
		"is_folder":  isFolder,
		"created_by": bareID("user", createdBy),
		"expires_at": expiry,
	})
	if err != nil {
		return nil, fmt.Errorf("create share link: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create share link: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

// GetShareLinkByHash looks up a share link by its token hash.
func (c *Client) GetShareLinkByHash(ctx context.Context, hash string) (*models.ShareLink, error) {
	sql := `SELECT * FROM share_link WHERE token_hash = $hash LIMIT 1`
	results, err := surrealdb.Query[[]models.ShareLink](ctx, c.DB(), sql, map[string]any{
		"hash": hash,
	})
	if err != nil {
		return nil, fmt.Errorf("get share link by hash: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// GetShareLink looks up a share link by ID.
func (c *Client) GetShareLink(ctx context.Context, id string) (*models.ShareLink, error) {
	sql := `SELECT * FROM type::record("share_link", $id)`
	results, err := surrealdb.Query[[]models.ShareLink](ctx, c.DB(), sql, map[string]any{
		"id": bareID("share_link", id),
	})
	if err != nil {
		return nil, fmt.Errorf("get share link: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// ListShareLinks returns all share links for a vault.
func (c *Client) ListShareLinks(ctx context.Context, vaultID string) ([]models.ShareLink, error) {
	sql := `SELECT * FROM share_link WHERE vault = type::record("vault", $vault_id) ORDER BY created_at DESC`
	results, err := surrealdb.Query[[]models.ShareLink](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// DeleteShareLink removes a share link by ID.
func (c *Client) DeleteShareLink(ctx context.Context, id string) error {
	sql := `DELETE type::record("share_link", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": bareID("share_link", id)}); err != nil {
		return fmt.Errorf("delete share link: %w", err)
	}
	return nil
}
