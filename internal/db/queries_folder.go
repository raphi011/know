package db

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateFolder creates a single folder record. Returns the created folder.
func (c *Client) CreateFolder(ctx context.Context, vaultID, folderPath string) (*models.Folder, error) {
	defer c.logOp(ctx, "folder.create", time.Now())
	name := path.Base(folderPath)

	sql := `
		INSERT INTO folder {
			vault: type::record("vault", $vault_id),
			path: $path,
			name: $name
		} ON DUPLICATE KEY UPDATE id = id
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Folder](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
		"name":     name,
	})
	if err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create folder: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

// EnsureFolders idempotently creates all ancestor folders for a document path,
// caching recent calls to skip redundant DB round-trips (60s TTL).
// For example, given "/guides/sub/file.md", it creates "/guides" and "/guides/sub".
func (c *Client) EnsureFolders(ctx context.Context, vaultID, docPath string) error {
	defer c.logOp(ctx, "folder.ensure", time.Now())
	docPath = models.NormalizePath(docPath)
	dir := path.Dir(docPath)
	if dir == "/" || dir == "." {
		return nil // root-level document, no folders to create
	}

	// Check cache — skip DB call if this folder was recently ensured
	cacheKey := vaultID + ":" + dir
	if expiry, ok := c.folderCache.Load(cacheKey); ok {
		if t, ok := expiry.(time.Time); ok && t.After(time.Now()) {
			return nil
		}
		c.folderCache.Delete(cacheKey)
	}

	if err := c.EnsureFolderPath(ctx, vaultID, dir); err != nil {
		return err
	}

	// Cache for 60 seconds
	c.folderCache.Store(cacheKey, time.Now().Add(60*time.Second))
	return nil
}

// EnsureFolderPath idempotently creates a folder and all its ancestors.
// For example, given "/guides/sub", it creates "/guides" and "/guides/sub".
// All folders are inserted in a single batched query to avoid N+1 round-trips.
func (c *Client) EnsureFolderPath(ctx context.Context, vaultID, folderPath string) error {
	defer c.logOp(ctx, "folder.ensure_path", time.Now())
	folderPath = models.NormalizePath(folderPath)
	if folderPath == "/" || folderPath == "." {
		return nil
	}

	// Collect the target folder and all its ancestors
	var folders []string
	for cur := folderPath; cur != "/" && cur != "."; cur = path.Dir(cur) {
		folders = append(folders, cur)
	}

	// Build batch of folder rows for a single INSERT
	rows := make([]map[string]any, len(folders))
	vid := bareID("vault", vaultID)
	for i, fp := range folders {
		rows[i] = map[string]any{
			"vault": newRecordID("vault", vid),
			"path":  fp,
			"name":  path.Base(fp),
		}
	}

	sql := `INSERT INTO folder $folders ON DUPLICATE KEY UPDATE id = id`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"folders": rows,
	}); err != nil {
		return fmt.Errorf("ensure folder path %q: %w", folderPath, err)
	}

	return nil
}

// ListFolders returns all folders in a vault, ordered by path.
func (c *Client) ListFolders(ctx context.Context, vaultID string) ([]models.Folder, error) {
	defer c.logOp(ctx, "folder.list", time.Now())
	sql := `SELECT * FROM folder WHERE vault = type::record("vault", $vault_id) ORDER BY path ASC`
	results, err := surrealdb.Query[[]models.Folder](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// ListChildFolders returns immediate child folders of parentPath in a vault.
// Only folders whose path starts with parentPath+"/" are fetched from the DB,
// then filtered in Go to exclude nested descendants (much faster than loading all folders).
func (c *Client) ListChildFolders(ctx context.Context, vaultID, parentPath string) ([]models.Folder, error) {
	defer c.logOp(ctx, "folder.list_children", time.Now())
	prefix := parentPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	sql := `SELECT * FROM folder WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) ORDER BY path ASC`
	results, err := surrealdb.Query[[]models.Folder](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("list child folders: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}

	// Filter to immediate children only (no additional "/" after prefix)
	var children []models.Folder
	for _, f := range (*results)[0].Result {
		rel := strings.TrimPrefix(f.Path, prefix)
		if rel != "" && !strings.Contains(rel, "/") {
			children = append(children, f)
		}
	}
	return children, nil
}

// GetFolderByPath returns a single folder by vault and path, or nil if not found.
func (c *Client) GetFolderByPath(ctx context.Context, vaultID, folderPath string) (*models.Folder, error) {
	defer c.logOp(ctx, "folder.get_by_path", time.Now())
	sql := `SELECT * FROM folder WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.Folder](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
	})
	if err != nil {
		return nil, fmt.Errorf("get folder by path: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// DeleteFolder deletes a single folder and all its children (paths starting with folderPath + "/").
func (c *Client) DeleteFolder(ctx context.Context, vaultID, folderPath string) error {
	defer c.logOp(ctx, "folder.delete", time.Now())
	if _, err := c.deleteFolderTree(ctx, vaultID, folderPath); err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}

// DeleteFoldersByPrefix deletes all folders whose path starts with the given prefix
// or matches the prefix itself (without trailing slash). Returns the number of deleted folders.
func (c *Client) DeleteFoldersByPrefix(ctx context.Context, vaultID, prefix string) (int, error) {
	defer c.logOp(ctx, "folder.delete_by_prefix", time.Now())
	folderPath := strings.TrimSuffix(prefix, "/")
	return c.deleteFolderTree(ctx, vaultID, folderPath)
}

// InvalidateFolderCache removes cached folder entries for a vault+path prefix.
// Scans all cached entries (expected to be small — one entry per recently-used folder).
func (c *Client) InvalidateFolderCache(vaultID, folderPath string) {
	prefix := vaultID + ":" + folderPath
	c.folderCache.Range(func(key, _ any) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			c.folderCache.Delete(key)
		}
		return true
	})
}

// deleteFolderTree deletes a folder and all its children in a transaction.
// Two statements are needed because SurrealDB v3 doesn't support parenthesized OR in WHERE.
// Returns the total number of deleted folders.
func (c *Client) deleteFolderTree(ctx context.Context, vaultID, folderPath string) (int, error) {
	defer c.logOp(ctx, "folder.delete_tree", time.Now())
	c.InvalidateFolderCache(vaultID, folderPath)
	prefix := folderPath + "/"
	vars := map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
		"prefix":   prefix,
	}

	tx, err := c.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete folder tree begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	r1, err := surrealdb.Query[[]models.Folder](ctx, tx,
		`DELETE FROM folder WHERE vault = type::record("vault", $vault_id) AND path = $path RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("delete folder tree %q: %w", folderPath, err)
	}
	r2, err := surrealdb.Query[[]models.Folder](ctx, tx,
		`DELETE FROM folder WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("delete folder tree children %q: %w", folderPath, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("delete folder tree commit: %w", err)
	}

	return countResults(r1) + countResults(r2), nil
}

// MoveFoldersByPrefix renames all folders whose path starts with oldPrefix,
// replacing oldPrefix with newPrefix. Also updates the name field.
// Accepts folder paths (e.g. "/guides", "/docs") — trailing slashes are handled internally.
// Returns the number of moved folders.
func (c *Client) MoveFoldersByPrefix(ctx context.Context, vaultID, oldPath, newPath string) (int, error) {
	defer c.logOp(ctx, "folder.move_by_prefix", time.Now())
	oldFolderPath := strings.TrimSuffix(oldPath, "/")
	newFolderPath := strings.TrimSuffix(newPath, "/")

	c.InvalidateFolderCache(vaultID, oldFolderPath)
	c.InvalidateFolderCache(vaultID, newFolderPath)

	oldPrefix := oldFolderPath + "/"
	newPrefix := newFolderPath + "/"

	vars := map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_path":   oldFolderPath,
		"new_path":   newFolderPath,
		"new_name":   path.Base(newFolderPath),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	}

	tx, err := c.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("move folders begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	r1, err := surrealdb.Query[[]models.Folder](ctx, tx,
		`UPDATE folder SET path = $new_path, name = $new_name WHERE vault = type::record("vault", $vault_id) AND path = $old_path RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("move folder root: %w", err)
	}
	r2, err := surrealdb.Query[[]models.Folder](ctx, tx,
		`UPDATE folder SET
			path = string::concat($new_prefix, string::slice(path, string::len($old_prefix))),
			name = array::last(string::split(string::concat($new_prefix, string::slice(path, string::len($old_prefix))), "/"))
		WHERE vault = type::record("vault", $vault_id)
		AND string::starts_with(path, $old_prefix)
		RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("move folder children: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("move folders commit: %w", err)
	}

	return countResults(r1) + countResults(r2), nil
}

// countResults returns the number of results from the first statement in a query response.
func countResults[T any](results *[]surrealdb.QueryResult[[]T]) int {
	if results == nil || len(*results) == 0 {
		return 0
	}
	return len((*results)[0].Result)
}
