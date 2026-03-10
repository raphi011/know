package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// UpsertAsset creates or updates an asset by vault+path.
// Computes content_hash and size from the input data.
// Uses check-then-create/update to work around SurrealDB v3's UPSERT ... WHERE
// not creating new records when no match exists.
func (c *Client) UpsertAsset(ctx context.Context, input models.AssetInput) (*models.Asset, error) {
	// Normalize nil to empty slice — SurrealDB rejects NULL for bytes fields.
	if input.Data == nil {
		input.Data = []byte{}
	}
	h := sha256.Sum256(input.Data)
	contentHash := hex.EncodeToString(h[:])
	size := len(input.Data)

	const maxRetries = 3

	for attempt := range maxRetries {
		existing, err := c.GetAssetMetaByPath(ctx, input.VaultID, input.Path)
		if err != nil {
			return nil, fmt.Errorf("upsert asset: check existing: %w", err)
		}

		if existing == nil {
			asset, err := c.createAsset(ctx, input, contentHash, size)
			if err != nil {
				if isUniqueViolation(err) && attempt < maxRetries-1 {
					continue
				}
				return nil, fmt.Errorf("upsert asset: %w", err)
			}
			return asset, nil
		}

		asset, err := c.updateAsset(ctx, input, contentHash, size)
		if err != nil {
			return nil, fmt.Errorf("upsert asset: %w", err)
		}
		return asset, nil
	}

	return nil, fmt.Errorf("upsert asset: exhausted %d retries due to concurrent writes", maxRetries)
}

func (c *Client) createAsset(ctx context.Context, input models.AssetInput, contentHash string, size int) (*models.Asset, error) {
	sql := `
		CREATE asset SET
			vault = type::record("vault", $vault_id),
			path = $path,
			mime_type = $mime_type,
			size = $size,
			content_hash = $content_hash,
			data = $data
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Asset](ctx, c.DB(), sql, map[string]any{
		"vault_id":     bareID("vault", input.VaultID),
		"path":         input.Path,
		"mime_type":    input.MimeType,
		"size":         size,
		"content_hash": contentHash,
		"data":         input.Data,
	})
	if err != nil {
		return nil, fmt.Errorf("create asset: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create asset: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) updateAsset(ctx context.Context, input models.AssetInput, contentHash string, size int) (*models.Asset, error) {
	sql := `
		UPDATE asset SET
			mime_type = $mime_type,
			size = $size,
			content_hash = $content_hash,
			data = $data
		WHERE vault = type::record("vault", $vault_id) AND path = $path
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Asset](ctx, c.DB(), sql, map[string]any{
		"vault_id":     bareID("vault", input.VaultID),
		"path":         input.Path,
		"mime_type":    input.MimeType,
		"size":         size,
		"content_hash": contentHash,
		"data":         input.Data,
	})
	if err != nil {
		return nil, fmt.Errorf("update asset: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("update asset: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

// GetAssetByPath returns the full asset (including data) by vault+path.
func (c *Client) GetAssetByPath(ctx context.Context, vaultID, path string) (*models.Asset, error) {
	sql := `SELECT * FROM asset WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.Asset](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     path,
	})
	if err != nil {
		return nil, fmt.Errorf("get asset by path: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// GetAssetMetaByPath returns lightweight asset metadata (no data bytes).
func (c *Client) GetAssetMetaByPath(ctx context.Context, vaultID, path string) (*models.AssetMeta, error) {
	sql := `SELECT path, mime_type, size, content_hash, created_at, updated_at FROM asset WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.AssetMeta](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     path,
	})
	if err != nil {
		return nil, fmt.Errorf("get asset meta by path: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// ListAssetMetas returns lightweight metadata for assets in a vault, optionally filtered by folder prefix.
func (c *Client) ListAssetMetas(ctx context.Context, vaultID string, folder *string) ([]models.AssetMeta, error) {
	vars := map[string]any{
		"vault_id": bareID("vault", vaultID),
	}

	var conditions []string
	conditions = append(conditions, `vault = type::record("vault", $vault_id)`)

	if folder != nil {
		conditions = append(conditions, `string::starts_with(path, $folder)`)
		vars["folder"] = *folder
	}

	where := strings.Join(conditions, " AND ")
	sql := fmt.Sprintf("SELECT path, mime_type, size, content_hash, created_at, updated_at FROM asset WHERE %s ORDER BY path ASC LIMIT 10000", where)

	results, err := surrealdb.Query[[]models.AssetMeta](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list asset metas: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// DeleteAsset deletes an asset by vault+path.
func (c *Client) DeleteAsset(ctx context.Context, vaultID, path string) error {
	sql := `DELETE FROM asset WHERE vault = type::record("vault", $vault_id) AND path = $path`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     path,
	}); err != nil {
		return fmt.Errorf("delete asset: %w", err)
	}
	return nil
}

// DeleteAssetsByPrefix deletes all assets in a vault whose path starts with the given prefix.
// Returns the number of deleted assets.
func (c *Client) DeleteAssetsByPrefix(ctx context.Context, vaultID, pathPrefix string) (int, error) {
	sql := `DELETE FROM asset WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) RETURN BEFORE`
	results, err := surrealdb.Query[[]models.Asset](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   pathPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("delete assets by prefix: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// MoveAsset updates an asset's path and mime_type.
func (c *Client) MoveAsset(ctx context.Context, vaultID, oldPath, newPath string) error {
	newMime := models.MimeTypeFromExt(newPath)
	sql := `UPDATE asset SET path = $new_path, mime_type = $mime_type WHERE vault = type::record("vault", $vault_id) AND path = $old_path`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"vault_id":  bareID("vault", vaultID),
		"old_path":  oldPath,
		"new_path":  newPath,
		"mime_type": newMime,
	}); err != nil {
		return fmt.Errorf("move asset: %w", err)
	}
	return nil
}

// MoveAssetsByPrefix updates all assets whose path starts with oldPrefix to use newPrefix.
// Returns the number of moved assets.
func (c *Client) MoveAssetsByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	sql := `
		UPDATE asset
		SET path = string::concat($new_prefix, string::slice(path, string::len($old_prefix))),
		    mime_type = mime_type
		WHERE vault = type::record("vault", $vault_id)
		  AND string::starts_with(path, $old_prefix)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Asset](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("move assets by prefix: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}
