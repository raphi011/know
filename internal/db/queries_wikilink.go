package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// CreateWikiLinks creates wiki-link records for a document.
func (c *Client) CreateWikiLinks(ctx context.Context, fromDocID, vaultID string, links []WikiLinkInput) error {
	for i, link := range links {
		sql := `
			CREATE wiki_link SET
				from_doc = type::record("document", $from_doc_id),
				to_doc = $to_doc,
				raw_target = $raw_target,
				vault = type::record("vault", $vault_id)
		`
		var toDoc any
		if link.ToDocID != nil {
			toDoc = newRecordID("document", *link.ToDocID)
		} else {
			toDoc = surrealmodels.None
		}

		if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
			"from_doc_id": fromDocID,
			"to_doc":      toDoc,
			"raw_target":  link.RawTarget,
			"vault_id":    vaultID,
		}); err != nil {
			return fmt.Errorf("create wiki link %d: %w", i, err)
		}
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
		"vault_id":   vaultID,
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
