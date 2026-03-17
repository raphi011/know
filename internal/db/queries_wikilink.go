package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// stemNormSQL is the SurrealDB expression that normalizes a raw_target to a stem:
// strips .md extension, lowercases, replaces spaces and underscores with hyphens.
// Used in both ResolveDanglingLinksByStem and UnresolveStemOnlyLinks.
const stemNormSQL = `string::replace(string::replace(string::lowercase(string::replace(raw_target, ".md", "")), " ", "-"), "_", "-")`

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

// ResolveDanglingLinksByStem resolves dangling stem-only links in a vault matching the given stem.
// Only links whose raw_target does not contain "/" are matched (pure stem references).
func (c *Client) ResolveDanglingLinksByStem(ctx context.Context, vaultID, stem, toFileID string) (int, error) {
	defer c.logOp(ctx, "wikilink.resolve_dangling_by_stem", time.Now())
	// Normalize raw_target to a stem by: stripping .md, lowering, replacing spaces/underscores with hyphens.
	// This allows [[Beta Notes]] to match stem "beta-notes".
	sql := `
		UPDATE wiki_link SET to_file = type::record("file", $to_file_id)
		WHERE vault = type::record("vault", $vault_id)
			AND to_file IS NONE
			AND !string::contains(raw_target, "/")
			AND ` + stemNormSQL + ` = $stem
	`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"stem":       stem,
		"to_file_id": bareID("file", toFileID),
	})
	if err != nil {
		return 0, fmt.Errorf("resolve dangling links by stem: %w", err)
	}
	return countResults(results), nil
}

// UnresolveStemOnlyLinks sets to_file = NONE on resolved stem-only links matching stem.
// Used when the target file is deleted or renamed, making those links dangling again.
func (c *Client) UnresolveStemOnlyLinks(ctx context.Context, vaultID, stem string) (int, error) {
	defer c.logOp(ctx, "wikilink.unresolve_stem_only", time.Now())
	sql := `
		UPDATE wiki_link SET to_file = NONE
		WHERE vault = type::record("vault", $vault_id)
			AND to_file IS NOT NONE
			AND !string::contains(raw_target, "/")
			AND ` + stemNormSQL + ` = $stem
	`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"stem":     stem,
	})
	if err != nil {
		return 0, fmt.Errorf("unresolve stem only links: %w", err)
	}
	return countResults(results), nil
}

// GetWikiLinksToFile returns all wiki-links where to_file points to the given file.
func (c *Client) GetWikiLinksToFile(ctx context.Context, fileID string) ([]models.WikiLink, error) {
	defer c.logOp(ctx, "wikilink.get_to_file", time.Now())
	sql := `SELECT * FROM wiki_link WHERE to_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return nil, fmt.Errorf("get wiki links to file: %w", err)
	}
	return allResults(results), nil
}

// UpdateWikiLinkRawTarget updates the raw_target of a single wiki-link by ID.
func (c *Client) UpdateWikiLinkRawTarget(ctx context.Context, linkID, newTarget string) error {
	defer c.logOp(ctx, "wikilink.update_raw_target", time.Now())
	sql := `UPDATE type::record("wiki_link", $link_id) SET raw_target = $new_target`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"link_id":    bareID("wiki_link", linkID),
		"new_target": newTarget,
	}); err != nil {
		return fmt.Errorf("update wiki link raw target: %w", err)
	}
	return nil
}

// WikiLinkWithTarget holds outgoing link info with the resolved target's path and title.
type WikiLinkWithTarget struct {
	RawTarget string  `json:"raw_target"`
	Path      *string `json:"path"`
	Title     *string `json:"title"`
}

// GetWikiLinksWithTargetInfo returns outgoing links from a file with resolved target path and title.
func (c *Client) GetWikiLinksWithTargetInfo(ctx context.Context, fileID string) ([]WikiLinkWithTarget, error) {
	defer c.logOp(ctx, "wikilink.get_with_target_info", time.Now())
	sql := `SELECT raw_target, to_file.path AS path, to_file.title AS title FROM wiki_link WHERE from_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]WikiLinkWithTarget](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return nil, fmt.Errorf("get wiki links with target info: %w", err)
	}
	return allResults(results), nil
}
