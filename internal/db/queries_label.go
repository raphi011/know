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

		if found := firstResultOpt(existing); found != nil {
			id, err := models.RecordIDString(found.ID)
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
		newLabel, labelErr := firstResult(created, "ensure label")
		if labelErr != nil {
			return "", labelErr
		}
		id, err := models.RecordIDString(newLabel.ID)
		if err != nil {
			return "", fmt.Errorf("ensure label: extract id: %w", err)
		}
		return id, nil
	}

	return "", fmt.Errorf("ensure label: exhausted %d retries due to concurrent writes", maxRetries)
}

// SyncFileLabels replaces all has_label edges for a file with edges
// to the given labels. Labels are upserted and edges are created in a single
// database round-trip to avoid N+1 query overhead.
func (c *Client) SyncFileLabels(ctx context.Context, fileID, vaultID string, labels []string) error {
	defer c.logOp(ctx, "label.sync", time.Now())

	// Normalize labels in Go
	normalized := make([]string, 0, len(labels))
	for _, name := range labels {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			normalized = append(normalized, name)
		}
	}

	// Delete old edges first; if no labels remain we're done
	deleteSQL := `DELETE FROM has_label WHERE in = type::record("file", $file_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), deleteSQL, map[string]any{
		"file_id": bareID("file", fileID),
	}); err != nil {
		return fmt.Errorf("sync file labels: delete old edges: %w", err)
	}

	if len(normalized) == 0 {
		return nil
	}

	// Build label row objects for batch INSERT
	vaultRef := bareID("vault", vaultID)
	labelRows := make([]map[string]any, len(normalized))
	for i, name := range normalized {
		labelRows[i] = map[string]any{
			"name":  name,
			"vault": newRecordID("vault", vaultRef),
		}
	}

	// Single query: upsert all labels, then create all edges.
	// LET $file outside the FOR loop because type::record() isn't allowed inside FOR.
	sql := `
		LET $file = type::record("file", $file_id);
		LET $labels = INSERT INTO label $label_rows ON DUPLICATE KEY UPDATE id = id RETURN AFTER;
		FOR $lbl IN $labels {
			RELATE $file->has_label->$lbl.id;
		};
	`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id":    bareID("file", fileID),
		"label_rows": labelRows,
	}); err != nil {
		return fmt.Errorf("sync file labels: upsert and relate %v: %w", normalized, err)
	}

	return nil
}

// GetFilesByLabel returns all files in a vault that have a specific label,
// using graph traversal through has_label edges.
func (c *Client) GetFilesByLabel(ctx context.Context, vaultID, labelName string) ([]models.File, error) {
	defer c.logOp(ctx, "label.get_files", time.Now())
	sql := `
		SELECT * FROM file WHERE
			vault = type::record("vault", $vault_id)
			AND count(->has_label->label[WHERE name = $label_name]) > 0
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"label_name": labelName,
	})
	if err != nil {
		return nil, fmt.Errorf("get files by label: %w", err)
	}
	return allResults(results), nil
}

// GetLabelsForFile returns all label names for a file using graph traversal.
func (c *Client) GetLabelsForFile(ctx context.Context, fileID string) ([]string, error) {
	defer c.logOp(ctx, "label.get_for_file", time.Now())
	sql := `SELECT VALUE ->has_label->label.name FROM type::record("file", $file_id)`
	results, err := surrealdb.Query[[][]string](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return nil, fmt.Errorf("get labels for file: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return (*results)[0].Result[0], nil
}
