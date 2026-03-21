package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateBookmark adds a bookmark for a user on a file. Idempotent — if the
// bookmark already exists (unique index on user+file), the operation is a no-op.
func (c *Client) CreateBookmark(ctx context.Context, userID, fileID, vaultID string) error {
	defer c.logOp(ctx, "bookmark.create", time.Now())
	sql := `
		INSERT INTO bookmark {
			user: type::record("user", $user_id),
			file: type::record("file", $file_id),
			vault: type::record("vault", $vault_id)
		} ON DUPLICATE KEY UPDATE id = id
	`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"user_id":  bareID("user", userID),
		"file_id":  bareID("file", fileID),
		"vault_id": bareID("vault", vaultID),
	}); err != nil {
		return fmt.Errorf("create bookmark: %w", err)
	}
	return nil
}

// DeleteBookmark removes a user's bookmark on a file. No-op if the bookmark
// does not exist.
func (c *Client) DeleteBookmark(ctx context.Context, userID, fileID string) error {
	defer c.logOp(ctx, "bookmark.delete", time.Now())
	sql := `DELETE FROM bookmark WHERE user = type::record("user", $user_id) AND file = type::record("file", $file_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"user_id": bareID("user", userID),
		"file_id": bareID("file", fileID),
	}); err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}
	return nil
}

// bookmarkWithFile is used to deserialize the FETCH file result.
type bookmarkWithFile struct {
	File models.File `json:"file"`
}

// ListBookmarks returns all files bookmarked by the user in the given vault,
// ordered by bookmark creation time (newest first).
func (c *Client) ListBookmarks(ctx context.Context, userID, vaultID string) ([]models.File, error) {
	defer c.logOp(ctx, "bookmark.list", time.Now())
	// FETCH file resolves the record link to a full file record.
	// created_at must be in the SELECT for ORDER BY to use it.
	sql := `
		SELECT file, created_at FROM bookmark
			WHERE user = type::record("user", $user_id)
			  AND vault = type::record("vault", $vault_id)
			ORDER BY created_at DESC
			FETCH file;
	`
	results, err := surrealdb.Query[[]bookmarkWithFile](ctx, c.DB(), sql, map[string]any{
		"user_id":  bareID("user", userID),
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	if results == nil || len(*results) < 1 {
		return nil, nil
	}
	rows := (*results)[0].Result
	files := make([]models.File, len(rows))
	for i, r := range rows {
		files[i] = r.File
	}
	return files, nil
}
