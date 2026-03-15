package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// CreateWikiLinks creates wiki-link records for a file in a single batch INSERT.
func (c *Client) CreateWikiLinks(ctx context.Context, fromFileID, vaultID string, links []WikiLinkInput) error {
	defer c.logOp(ctx, "wikilink.create", time.Now())
	if len(links) == 0 {
		return nil
	}

	fromFile := newRecordID("file", bareID("file", fromFileID))
	vault := newRecordID("vault", bareID("vault", vaultID))

	rows := make([]map[string]any, len(links))
	for i, link := range links {
		var toFile any
		if link.ToFileID != nil {
			toFile = newRecordID("file", *link.ToFileID)
		} else {
			toFile = surrealmodels.None
		}
		rows[i] = map[string]any{
			"from_file":  fromFile,
			"to_file":    toFile,
			"raw_target": link.RawTarget,
			"vault":      vault,
		}
	}

	sql := `INSERT INTO wiki_link $links RETURN NONE`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"links": rows,
	}); err != nil {
		return fmt.Errorf("create wiki links: %w", err)
	}
	return nil
}

// WikiLinkInput represents input for creating a wiki link.
type WikiLinkInput struct {
	RawTarget string
	ToFileID  *string
}

// GetWikiLinks returns outgoing wiki-links from a file.
func (c *Client) GetWikiLinks(ctx context.Context, fromFileID string) ([]models.WikiLink, error) {
	defer c.logOp(ctx, "wikilink.get", time.Now())
	sql := `SELECT * FROM wiki_link WHERE from_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"file_id": fromFileID,
	})
	if err != nil {
		return nil, fmt.Errorf("get wiki links: %w", err)
	}
	return allResults(results), nil
}

// GetBacklinks returns wiki-links pointing to a file.
func (c *Client) GetBacklinks(ctx context.Context, toFileID string) ([]models.WikiLink, error) {
	defer c.logOp(ctx, "wikilink.get_backlinks", time.Now())
	sql := `SELECT * FROM wiki_link WHERE to_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"file_id": toFileID,
	})
	if err != nil {
		return nil, fmt.Errorf("get backlinks: %w", err)
	}
	return allResults(results), nil
}

// DeleteWikiLinks removes all wiki-links originating from a file.
func (c *Client) DeleteWikiLinks(ctx context.Context, fromFileID string) error {
	defer c.logOp(ctx, "wikilink.delete", time.Now())
	sql := `DELETE FROM wiki_link WHERE from_file = type::record("file", $file_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id": fromFileID,
	}); err != nil {
		return fmt.Errorf("delete wiki links: %w", err)
	}
	return nil
}

// UnresolveWikiLinksToFile sets to_file = NONE on all wiki_links pointing to the given file,
// making them dangling. Used during file deletion to preserve backlink references.
func (c *Client) UnresolveWikiLinksToFile(ctx context.Context, fileID string) (int, error) {
	defer c.logOp(ctx, "wikilink.unresolve", time.Now())
	sql := `UPDATE wiki_link SET to_file = NONE WHERE to_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return 0, fmt.Errorf("unresolve wiki links to file: %w", err)
	}
	return countResults(results), nil
}

// UpdateWikiLinkRawTargets updates raw_target for all wiki_links in a vault matching oldTarget.
// Used when a file is moved to keep raw_targets consistent with the new path/title.
func (c *Client) UpdateWikiLinkRawTargets(ctx context.Context, vaultID, oldTarget, newTarget string) (int, error) {
	defer c.logOp(ctx, "wikilink.update_raw_targets", time.Now())
	sql := `UPDATE wiki_link SET raw_target = $new_target WHERE vault = type::record("vault", $vault_id) AND raw_target = $old_target`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_target": oldTarget,
		"new_target": newTarget,
	})
	if err != nil {
		return 0, fmt.Errorf("update wiki link raw targets: %w", err)
	}
	return countResults(results), nil
}

// UpdateWikiLinkRawTargetsByPrefix updates raw_target for all wiki_links in a vault
// where raw_target starts with oldPrefix, replacing the prefix with newPrefix.
// Used when files are moved by prefix (folder rename).
func (c *Client) UpdateWikiLinkRawTargetsByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	defer c.logOp(ctx, "wikilink.update_raw_targets_by_prefix", time.Now())
	sql := `UPDATE wiki_link SET raw_target = string::concat($new_prefix, string::slice(raw_target, string::len($old_prefix))) WHERE vault = type::record("vault", $vault_id) AND string::starts_with(raw_target, $old_prefix)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("update wiki link raw targets by prefix: %w", err)
	}
	return countResults(results), nil
}

// ResolveDanglingLinks finds dangling wiki-links in a vault matching a target
// and resolves them to point to the given file.
func (c *Client) ResolveDanglingLinks(ctx context.Context, vaultID, rawTarget, toFileID string) (int, error) {
	defer c.logOp(ctx, "wikilink.resolve_dangling", time.Now())
	sql := `
		UPDATE wiki_link SET to_file = type::record("file", $to_file_id)
		WHERE vault = type::record("vault", $vault_id)
			AND raw_target = $raw_target
			AND to_file IS NONE
	`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"raw_target": rawTarget,
		"to_file_id": toFileID,
	})
	if err != nil {
		return 0, fmt.Errorf("resolve dangling links: %w", err)
	}
	return countResults(results), nil
}
