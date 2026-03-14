package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// EnsureLabel gets or creates a label record for the given vault and name, returning its ID.
// Uses check-then-create/update to work around SurrealDB v3's UPSERT ... WHERE
// not creating new records when no match exists.
func (c *Client) EnsureLabel(ctx context.Context, vaultID, name string) (string, error) {
	defer c.logOp(ctx, "label.ensure", time.Now())
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("ensure label: name must not be empty")
	}

	const maxRetries = 3

	for attempt := range maxRetries {
		// Check if label already exists
		selectSQL := `SELECT * FROM label WHERE vault = type::record("vault", $vault_id) AND name = $name LIMIT 1`
		existing, err := surrealdb.Query[[]models.Label](ctx, c.DB(), selectSQL, map[string]any{
			"vault_id": bareID("vault", vaultID),
			"name":     name,
		})
		if err != nil {
			return "", fmt.Errorf("ensure label: check existing: %w", err)
		}

		if existing != nil && len(*existing) > 0 && len((*existing)[0].Result) > 0 {
			id, err := models.RecordIDString((*existing)[0].Result[0].ID)
			if err != nil {
				return "", fmt.Errorf("ensure label: extract id: %w", err)
			}
			return id, nil
		}

		// Create new label
		createSQL := `CREATE label SET name = $name, vault = type::record("vault", $vault_id) RETURN AFTER`
		created, err := surrealdb.Query[[]models.Label](ctx, c.DB(), createSQL, map[string]any{
			"vault_id": bareID("vault", vaultID),
			"name":     name,
		})
		if err != nil {
			if isUniqueViolation(err) && attempt < maxRetries-1 {
				continue
			}
			return "", fmt.Errorf("ensure label: create: %w", err)
		}
		if created == nil || len(*created) == 0 || len((*created)[0].Result) == 0 {
			return "", fmt.Errorf("ensure label: no result returned from create")
		}
		id, err := models.RecordIDString((*created)[0].Result[0].ID)
		if err != nil {
			return "", fmt.Errorf("ensure label: extract id: %w", err)
		}
		return id, nil
	}

	return "", fmt.Errorf("ensure label: exhausted %d retries due to concurrent writes", maxRetries)
}

// SyncDocumentLabels replaces all has_label edges for a document with edges
// to the given labels. Labels are upserted as needed.
func (c *Client) SyncDocumentLabels(ctx context.Context, docID, vaultID string, labels []string) error {
	defer c.logOp(ctx, "label.sync", time.Now())
	// Delete existing edges from this document
	sql := `DELETE FROM has_label WHERE in = type::record("document", $doc_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"doc_id": bareID("document", docID),
	}); err != nil {
		return fmt.Errorf("sync document labels: delete old edges: %w", err)
	}

	for _, name := range labels {
		labelID, err := c.EnsureLabel(ctx, vaultID, name)
		if err != nil {
			return fmt.Errorf("sync document labels: %w", err)
		}

		relateSql := `
			LET $doc = type::record("document", $doc_id);
			LET $lbl = type::record("label", $label_id);
			RELATE $doc->has_label->$lbl RETURN NONE
		`
		if _, err := surrealdb.Query[any](ctx, c.DB(), relateSql, map[string]any{
			"doc_id":   bareID("document", docID),
			"label_id": bareID("label", labelID),
		}); err != nil {
			return fmt.Errorf("sync document labels: create edge for %q: %w", name, err)
		}
	}

	return nil
}

// GetDocumentsByLabel returns all documents in a vault that have a specific label,
// using graph traversal through has_label edges.
func (c *Client) GetDocumentsByLabel(ctx context.Context, vaultID, labelName string) ([]models.Document, error) {
	defer c.logOp(ctx, "label.get_documents", time.Now())
	sql := `
		SELECT * FROM document WHERE
			vault = type::record("vault", $vault_id)
			AND count(->has_label->label[WHERE name = $label_name]) > 0
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"label_name": labelName,
	})
	if err != nil {
		return nil, fmt.Errorf("get documents by label: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// GetLabelsForDocument returns all label names for a document using graph traversal.
func (c *Client) GetLabelsForDocument(ctx context.Context, docID string) ([]string, error) {
	defer c.logOp(ctx, "label.get_for_document", time.Now())
	sql := `SELECT VALUE ->has_label->label.name FROM type::record("document", $doc_id)`
	results, err := surrealdb.Query[[][]string](ctx, c.DB(), sql, map[string]any{
		"doc_id": bareID("document", docID),
	})
	if err != nil {
		return nil, fmt.Errorf("get labels for document: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return (*results)[0].Result[0], nil
}
