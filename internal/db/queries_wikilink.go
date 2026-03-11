package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// CreateWikiLinks creates wiki-link records for a document in a single batch INSERT.
func (c *Client) CreateWikiLinks(ctx context.Context, fromDocID, vaultID string, links []WikiLinkInput) error {
	if len(links) == 0 {
		return nil
	}

	fromDoc := newRecordID("document", bareID("document", fromDocID))
	vault := newRecordID("vault", bareID("vault", vaultID))

	rows := make([]map[string]any, len(links))
	for i, link := range links {
		var toDoc any
		if link.ToDocID != nil {
			toDoc = newRecordID("document", *link.ToDocID)
		} else {
			toDoc = surrealmodels.None
		}
		rows[i] = map[string]any{
			"from_doc":   fromDoc,
			"to_doc":     toDoc,
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
	ToDocID   *string
}

// GetWikiLinks returns outgoing wiki-links from a document.
func (c *Client) GetWikiLinks(ctx context.Context, fromDocID string) ([]models.WikiLink, error) {
	sql := `SELECT * FROM wiki_link WHERE from_doc = type::record("document", $doc_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"doc_id": fromDocID,
	})
	if err != nil {
		return nil, fmt.Errorf("get wiki links: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// GetBacklinks returns wiki-links pointing to a document.
func (c *Client) GetBacklinks(ctx context.Context, toDocID string) ([]models.WikiLink, error) {
	sql := `SELECT * FROM wiki_link WHERE to_doc = type::record("document", $doc_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"doc_id": toDocID,
	})
	if err != nil {
		return nil, fmt.Errorf("get backlinks: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// DeleteWikiLinks removes all wiki-links originating from a document.
func (c *Client) DeleteWikiLinks(ctx context.Context, fromDocID string) error {
	sql := `DELETE FROM wiki_link WHERE from_doc = type::record("document", $doc_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"doc_id": fromDocID,
	}); err != nil {
		return fmt.Errorf("delete wiki links: %w", err)
	}
	return nil
}

// UnresolveWikiLinksToDoc sets to_doc = NONE on all wiki_links pointing to the given document,
// making them dangling. Used during document deletion to preserve backlink references.
func (c *Client) UnresolveWikiLinksToDoc(ctx context.Context, docID string) (int, error) {
	sql := `UPDATE wiki_link SET to_doc = NONE WHERE to_doc = type::record("document", $doc_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"doc_id": bareID("document", docID),
	})
	if err != nil {
		return 0, fmt.Errorf("unresolve wiki links to doc: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// UpdateWikiLinkRawTargets updates raw_target for all wiki_links in a vault matching oldTarget.
// Used when a document is moved to keep raw_targets consistent with the new path/title.
func (c *Client) UpdateWikiLinkRawTargets(ctx context.Context, vaultID, oldTarget, newTarget string) (int, error) {
	sql := `UPDATE wiki_link SET raw_target = $new_target WHERE vault = type::record("vault", $vault_id) AND raw_target = $old_target`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_target": oldTarget,
		"new_target": newTarget,
	})
	if err != nil {
		return 0, fmt.Errorf("update wiki link raw targets: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// UpdateWikiLinkRawTargetsByPrefix updates raw_target for all wiki_links in a vault
// where raw_target starts with oldPrefix, replacing the prefix with newPrefix.
// Used when documents are moved by prefix (folder rename).
func (c *Client) UpdateWikiLinkRawTargetsByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	sql := `UPDATE wiki_link SET raw_target = string::concat($new_prefix, string::slice(raw_target, string::len($old_prefix))) WHERE vault = type::record("vault", $vault_id) AND string::starts_with(raw_target, $old_prefix)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("update wiki link raw targets by prefix: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// ResolveDanglingLinks finds dangling wiki-links in a vault matching a target
// and resolves them to point to the given document.
func (c *Client) ResolveDanglingLinks(ctx context.Context, vaultID, rawTarget, toDocID string) (int, error) {
	sql := `
		UPDATE wiki_link SET to_doc = type::record("document", $to_doc_id)
		WHERE vault = type::record("vault", $vault_id)
			AND raw_target = $raw_target
			AND to_doc IS NONE
	`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"raw_target": rawTarget,
		"to_doc_id":  toDocID,
	})
	if err != nil {
		return 0, fmt.Errorf("resolve dangling links: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}
