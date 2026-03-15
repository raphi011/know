package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// ExternalLinkInput represents input for creating an external link.
type ExternalLinkInput struct {
	Hostname string
	URLPath  string
	FullURL  string
	LinkText *string
}

// ExternalLinkHostStats holds aggregated link counts by hostname.
type ExternalLinkHostStats struct {
	Hostname string `json:"hostname"`
	Count    int    `json:"count"`
}

// ExternalLinkFilter controls the external link list query.
type ExternalLinkFilter struct {
	VaultID  string
	Hostname *string
	FileID   *string
	Limit    int
	Offset   int
}

// CreateExternalLinks batch-inserts external links for a file.
func (c *Client) CreateExternalLinks(ctx context.Context, fromFileID, vaultID string, links []ExternalLinkInput) error {
	defer c.logOp(ctx, "external_link.create", time.Now())
	if len(links) == 0 {
		return nil
	}

	fromFile := newRecordID("file", bareID("file", fromFileID))
	vault := newRecordID("vault", bareID("vault", vaultID))

	rows := make([]map[string]any, len(links))
	for i, link := range links {
		rows[i] = map[string]any{
			"from_file": fromFile,
			"vault":     vault,
			"hostname":  link.Hostname,
			"url_path":  link.URLPath,
			"full_url":  link.FullURL,
			"link_text": optionalString(link.LinkText),
		}
	}

	sql := `INSERT INTO external_link $links RETURN NONE`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"links": rows,
	}); err != nil {
		return fmt.Errorf("create external links: %w", err)
	}
	return nil
}

// DeleteExternalLinksByFile removes all external links originating from a file.
func (c *Client) DeleteExternalLinksByFile(ctx context.Context, fromFileID string) error {
	defer c.logOp(ctx, "external_link.delete", time.Now())
	sql := `DELETE FROM external_link WHERE from_file = type::record("file", $file_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fromFileID),
	}); err != nil {
		return fmt.Errorf("delete external links: %w", err)
	}
	return nil
}

// GetExternalLinkStats returns link counts grouped by hostname for a vault.
func (c *Client) GetExternalLinkStats(ctx context.Context, vaultID string) ([]ExternalLinkHostStats, error) {
	defer c.logOp(ctx, "external_link.stats", time.Now())
	sql := `SELECT hostname, count() AS count
		FROM external_link
		WHERE vault = type::record("vault", $vault_id)
		GROUP BY hostname
		ORDER BY count DESC`
	results, err := surrealdb.Query[[]ExternalLinkHostStats](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("get external link stats: %w", err)
	}
	return allResults(results), nil
}

// externalLinkWhere builds a shared WHERE clause and params for external link queries.
func externalLinkWhere(filter ExternalLinkFilter) (string, map[string]any) {
	var conditions []string
	params := map[string]any{
		"vault_id": bareID("vault", filter.VaultID),
	}

	conditions = append(conditions, `vault = type::record("vault", $vault_id)`)

	if filter.Hostname != nil {
		conditions = append(conditions, `hostname = $hostname`)
		params["hostname"] = *filter.Hostname
	}
	if filter.FileID != nil {
		conditions = append(conditions, `from_file = type::record("file", $file_id)`)
		params["file_id"] = bareID("file", *filter.FileID)
	}

	return strings.Join(conditions, " AND "), params
}

// ListExternalLinks returns external links for a vault, optionally filtered.
func (c *Client) ListExternalLinks(ctx context.Context, filter ExternalLinkFilter) ([]models.ExternalLink, error) {
	defer c.logOp(ctx, "external_link.list", time.Now())

	where, params := externalLinkWhere(filter)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	params["limit"] = limit
	params["start"] = filter.Offset

	sql := fmt.Sprintf(`SELECT * FROM external_link WHERE %s ORDER BY hostname, full_url LIMIT $limit START $start`, where)

	results, err := surrealdb.Query[[]models.ExternalLink](ctx, c.DB(), sql, params)
	if err != nil {
		return nil, fmt.Errorf("list external links: %w", err)
	}
	return allResults(results), nil
}

// CountExternalLinks returns the total count of external links matching the filter.
func (c *Client) CountExternalLinks(ctx context.Context, filter ExternalLinkFilter) (int, error) {
	defer c.logOp(ctx, "external_link.count", time.Now())

	where, params := externalLinkWhere(filter)

	sql := fmt.Sprintf(`SELECT count() AS total FROM external_link WHERE %s GROUP ALL`, where)

	type countResult struct {
		Total int `json:"total"`
	}

	results, err := surrealdb.Query[[]countResult](ctx, c.DB(), sql, params)
	if err != nil {
		return 0, fmt.Errorf("count external links: %w", err)
	}

	r := firstResultOpt(results)
	if r == nil {
		return 0, nil
	}
	return r.Total, nil
}
