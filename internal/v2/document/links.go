package document

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/v2/models"
	"github.com/surrealdb/surrealdb.go"
)

// LinkResolver resolves wiki-link targets within a vault.
type LinkResolver struct {
	db dbQuerier
}

// dbQuerier is the subset of db.Client needed for link resolution.
type dbQuerier interface {
	DB() *surrealdb.DB
}

// NewLinkResolver creates a new link resolver.
func NewLinkResolver(db dbQuerier) *LinkResolver {
	return &LinkResolver{db: db}
}

// Resolve resolves a wiki-link target within a vault.
// Tries exact path match first, then title match (shallowest path wins).
// Returns nil if not found (dangling link).
func (r *LinkResolver) Resolve(ctx context.Context, vaultID, target string) (*models.Document, error) {
	// Try exact path match
	sql := `SELECT * FROM document WHERE vault = type::record("vault", $vault_id) AND path = $target LIMIT 1`
	results, err := surrealdb.Query[[]models.Document](ctx, r.db.DB(), sql, map[string]any{
		"vault_id": vaultID,
		"target":   target,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve link (path): %w", err)
	}
	if results != nil && len(*results) > 0 && len((*results)[0].Result) > 0 {
		return &(*results)[0].Result[0], nil
	}

	// Try title match (shallowest path wins)
	sql = `SELECT * FROM document WHERE vault = type::record("vault", $vault_id) AND title = $target ORDER BY array::len(string::split(path, '/')) ASC LIMIT 1`
	results, err = surrealdb.Query[[]models.Document](ctx, r.db.DB(), sql, map[string]any{
		"vault_id": vaultID,
		"target":   target,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve link (title): %w", err)
	}
	if results != nil && len(*results) > 0 && len((*results)[0].Result) > 0 {
		return &(*results)[0].Result[0], nil
	}

	return nil, nil
}
