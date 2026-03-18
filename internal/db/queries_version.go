package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateVersion stores a new version snapshot of a file's content.
func (c *Client) CreateVersion(ctx context.Context, input models.FileVersionInput, version int) (*models.FileVersion, error) {
	defer c.logOp(ctx, "version.create", time.Now())

	sql := `
		CREATE file_version SET
			file = type::record("file", $file_id),
			vault = type::record("vault", $vault_id),
			version = $version,
			content_hash = $content_hash,
			title = $title
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.FileVersion](ctx, c.DB(), sql, map[string]any{
		"file_id":      bareID("file", input.FileID),
		"vault_id":     bareID("vault", input.VaultID),
		"version":      version,
		"content_hash": input.ContentHash,
		"title":        input.Title,
	})
	if err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}
	return firstResult(results, "create version")
}

// GetVersion retrieves a single version by its ID.
func (c *Client) GetVersion(ctx context.Context, id string) (*models.FileVersion, error) {
	defer c.logOp(ctx, "version.get", time.Now())

	sql := `SELECT * FROM type::record("file_version", $id)`
	results, err := surrealdb.Query[[]models.FileVersion](ctx, c.DB(), sql, map[string]any{
		"id": bareID("file_version", id),
	})
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	return firstResultOpt(results), nil
}

// GetVersionByNumber retrieves a specific version by file ID and version number.
func (c *Client) GetVersionByNumber(ctx context.Context, fileID string, version int) (*models.FileVersion, error) {
	defer c.logOp(ctx, "version.get_by_number", time.Now())

	sql := `SELECT * FROM file_version WHERE file = type::record("file", $file_id) AND version = $version LIMIT 1`
	results, err := surrealdb.Query[[]models.FileVersion](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
		"version": version,
	})
	if err != nil {
		return nil, fmt.Errorf("get version by number: %w", err)
	}
	return firstResultOpt(results), nil
}

// ListVersions returns versions for a file, paginated, newest first.
func (c *Client) ListVersions(ctx context.Context, fileID string, limit, offset int) ([]models.FileVersion, error) {
	defer c.logOp(ctx, "version.list", time.Now())

	if limit <= 0 {
		limit = 20
	}

	sql := fmt.Sprintf(
		`SELECT * FROM file_version WHERE file = type::record("file", $file_id) ORDER BY version DESC LIMIT %d START %d`,
		limit, offset,
	)
	results, err := surrealdb.Query[[]models.FileVersion](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return []models.FileVersion{}, nil
	}
	return (*results)[0].Result, nil
}

// GetLatestVersion returns the most recent version for a file.
func (c *Client) GetLatestVersion(ctx context.Context, fileID string) (*models.FileVersion, error) {
	defer c.logOp(ctx, "version.get_latest", time.Now())

	sql := `SELECT * FROM file_version WHERE file = type::record("file", $file_id) ORDER BY version DESC LIMIT 1`
	results, err := surrealdb.Query[[]models.FileVersion](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return nil, fmt.Errorf("get latest version: %w", err)
	}
	return firstResultOpt(results), nil
}

// CountVersions returns the total number of versions for a file.
func (c *Client) CountVersions(ctx context.Context, fileID string) (int, error) {
	defer c.logOp(ctx, "version.count", time.Now())

	sql := `SELECT count() AS total FROM file_version WHERE file = type::record("file", $file_id) GROUP ALL`
	type countResult struct {
		Total int `json:"total"`
	}
	results, err := surrealdb.Query[[]countResult](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return 0, fmt.Errorf("count versions: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return 0, nil
	}
	return (*results)[0].Result[0].Total, nil
}

// DeleteOldestVersions deletes versions beyond the retention cap, keeping the newest keepCount.
// Returns the number of deleted versions.
func (c *Client) DeleteOldestVersions(ctx context.Context, fileID string, keepCount int) (int, error) {
	defer c.logOp(ctx, "version.delete_oldest", time.Now())

	sql := `
		DELETE FROM file_version
		WHERE file = type::record("file", $file_id)
		  AND id NOT IN (
			SELECT id, version FROM file_version
			WHERE file = type::record("file", $file_id)
			ORDER BY version DESC LIMIT $keep_count
		  ).id
		RETURN BEFORE
	`
	results, err := surrealdb.Query[[]models.FileVersion](ctx, c.DB(), sql, map[string]any{
		"file_id":    bareID("file", fileID),
		"keep_count": keepCount,
	})
	if err != nil {
		return 0, fmt.Errorf("delete oldest versions: %w", err)
	}
	return countResults(results), nil
}

// NextVersionNumber returns the next version number for a file.
// Uses max(version) instead of count() to avoid collisions after retention pruning.
func (c *Client) NextVersionNumber(ctx context.Context, fileID string) (int, error) {
	defer c.logOp(ctx, "version.next_number", time.Now())

	sql := `SELECT version FROM file_version WHERE file = type::record("file", $file_id) ORDER BY version DESC LIMIT 1`
	type versionResult struct {
		Version int `json:"version"`
	}
	results, err := surrealdb.Query[[]versionResult](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return 0, fmt.Errorf("next version number: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return 1, nil
	}
	return (*results)[0].Result[0].Version + 1, nil
}
