package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateVersion stores a new version snapshot of a document's content.
func (c *Client) CreateVersion(ctx context.Context, input models.DocumentVersionInput, version int) (*models.DocumentVersion, error) {
	start := c.startOp()
	defer c.recordTiming("db.create_version", start)

	sql := `
		CREATE document_version SET
			document = type::record("document", $document_id),
			vault = type::record("vault", $vault_id),
			version = $version,
			content = $content,
			content_hash = $content_hash,
			title = $title,
			source = $source
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.DocumentVersion](ctx, c.DB(), sql, map[string]any{
		"document_id":  input.DocumentID,
		"vault_id":     bareID("vault", input.VaultID),
		"version":      version,
		"content":      input.Content,
		"content_hash": input.ContentHash,
		"title":        input.Title,
		"source":       string(input.Source),
	})
	if err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create version: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

// GetVersion retrieves a single version by its ID.
func (c *Client) GetVersion(ctx context.Context, id string) (*models.DocumentVersion, error) {
	start := c.startOp()
	defer c.recordTiming("db.get_version", start)

	sql := `SELECT * FROM type::record("document_version", $id)`
	results, err := surrealdb.Query[[]models.DocumentVersion](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// GetVersionByNumber retrieves a specific version by document ID and version number.
func (c *Client) GetVersionByNumber(ctx context.Context, documentID string, version int) (*models.DocumentVersion, error) {
	start := c.startOp()
	defer c.recordTiming("db.get_version_by_number", start)

	sql := `SELECT * FROM document_version WHERE document = type::record("document", $document_id) AND version = $version LIMIT 1`
	results, err := surrealdb.Query[[]models.DocumentVersion](ctx, c.DB(), sql, map[string]any{
		"document_id": documentID,
		"version":     version,
	})
	if err != nil {
		return nil, fmt.Errorf("get version by number: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// ListVersions returns versions for a document, paginated, newest first.
func (c *Client) ListVersions(ctx context.Context, documentID string, limit, offset int) ([]models.DocumentVersion, error) {
	start := c.startOp()
	defer c.recordTiming("db.list_versions", start)

	if limit <= 0 {
		limit = 20
	}

	sql := fmt.Sprintf(
		`SELECT * FROM document_version WHERE document = type::record("document", $document_id) ORDER BY version DESC LIMIT %d START %d`,
		limit, offset,
	)
	results, err := surrealdb.Query[[]models.DocumentVersion](ctx, c.DB(), sql, map[string]any{
		"document_id": documentID,
	})
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// GetLatestVersion returns the most recent version for a document.
func (c *Client) GetLatestVersion(ctx context.Context, documentID string) (*models.DocumentVersion, error) {
	start := c.startOp()
	defer c.recordTiming("db.get_latest_version", start)

	sql := `SELECT * FROM document_version WHERE document = type::record("document", $document_id) ORDER BY version DESC LIMIT 1`
	results, err := surrealdb.Query[[]models.DocumentVersion](ctx, c.DB(), sql, map[string]any{
		"document_id": documentID,
	})
	if err != nil {
		return nil, fmt.Errorf("get latest version: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// CountVersions returns the total number of versions for a document.
func (c *Client) CountVersions(ctx context.Context, documentID string) (int, error) {
	start := c.startOp()
	defer c.recordTiming("db.count_versions", start)

	sql := `SELECT count() AS total FROM document_version WHERE document = type::record("document", $document_id) GROUP ALL`
	type countResult struct {
		Total int `json:"total"`
	}
	results, err := surrealdb.Query[[]countResult](ctx, c.DB(), sql, map[string]any{
		"document_id": documentID,
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
func (c *Client) DeleteOldestVersions(ctx context.Context, documentID string, keepCount int) (int, error) {
	start := c.startOp()
	defer c.recordTiming("db.delete_oldest_versions", start)

	sql := `
		DELETE FROM document_version
		WHERE document = type::record("document", $document_id)
		  AND id NOT IN (
			SELECT VALUE id FROM document_version
			WHERE document = type::record("document", $document_id)
			ORDER BY version DESC LIMIT $keep_count
		  )
		RETURN BEFORE
	`
	results, err := surrealdb.Query[[]models.DocumentVersion](ctx, c.DB(), sql, map[string]any{
		"document_id": documentID,
		"keep_count":  keepCount,
	})
	if err != nil {
		return 0, fmt.Errorf("delete oldest versions: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// NextVersionNumber returns the next version number for a document.
func (c *Client) NextVersionNumber(ctx context.Context, documentID string) (int, error) {
	start := c.startOp()
	defer c.recordTiming("db.next_version_number", start)

	sql := `SELECT count() AS total FROM document_version WHERE document = type::record("document", $document_id) GROUP ALL`
	type countResult struct {
		Total int `json:"total"`
	}
	results, err := surrealdb.Query[[]countResult](ctx, c.DB(), sql, map[string]any{
		"document_id": documentID,
	})
	if err != nil {
		return 0, fmt.Errorf("next version number: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return 1, nil
	}
	return (*results)[0].Result[0].Total + 1, nil
}
