package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// ListTombstonesSince returns tombstones for a vault created after the given timestamp.
func (c *Client) ListTombstonesSince(ctx context.Context, vaultID string, since time.Time) ([]models.FileTombstone, error) {
	defer c.logOp(ctx, "tombstone.list_since", time.Now())

	// Format as RFC3339Nano string — passing time.Time directly loses sub-second
	// precision in the SurrealDB Go SDK's CBOR encoding.
	sql := `SELECT * FROM file_tombstone WHERE vault = type::record("vault", $vault_id) AND deleted_at >= <datetime>$since ORDER BY deleted_at ASC LIMIT 10000`
	results, err := surrealdb.Query[[]models.FileTombstone](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"since":    since.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("list tombstones since: %w", err)
	}
	return allResults(results), nil
}

// PurgeTombstonesBefore deletes tombstones older than the given timestamp.
// Returns the number of deleted tombstones.
func (c *Client) PurgeTombstonesBefore(ctx context.Context, vaultID string, before time.Time) (int, error) {
	defer c.logOp(ctx, "tombstone.purge", time.Now())

	sql := `DELETE FROM file_tombstone WHERE vault = type::record("vault", $vault_id) AND deleted_at < <datetime>$before RETURN BEFORE`
	results, err := surrealdb.Query[[]models.FileTombstone](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"before":   before.Format(time.RFC3339Nano),
	})
	if err != nil {
		return 0, fmt.Errorf("purge tombstones: %w", err)
	}
	return countResults(results), nil
}
