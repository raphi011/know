package file

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphi011/know/internal/models"
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

// Resolve resolves a wiki-link target within a vault using Foam-style stem matching.
// Strips .md extension, computes stem, then matches by stem. If multiple matches and
// target contains a path separator, disambiguates by path suffix. Returns nil if not
// found or ambiguous (dangling link).
func (r *LinkResolver) Resolve(ctx context.Context, vaultID, target string) (*models.File, error) {
	// Normalize: strip .md extension, then derive stem
	normalized := strings.TrimSuffix(target, ".md")
	stem := models.FilenameStem("/" + normalized + ".md")

	// Stem match: find all files with matching stem in this vault
	sql := `SELECT * FROM file WHERE is_folder = false AND vault = type::record("vault", $vault_id) AND stem = $stem`
	results, err := surrealdb.Query[[]models.File](ctx, r.db.DB(), sql, map[string]any{
		"vault_id": vaultID,
		"stem":     stem,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve link (stem): %w", err)
	}

	var matches []models.File
	if results != nil && len(*results) > 0 {
		matches = (*results)[0].Result
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		// Multiple matches: disambiguate by path suffix only if target contains /
		if !strings.Contains(normalized, "/") {
			return nil, nil // ambiguous, no path hint
		}
		lowered := strings.ToLower(normalized)
		suffix := "/" + lowered + ".md"
		var found *models.File
		for i := range matches {
			if strings.HasSuffix(strings.ToLower(matches[i].Path), suffix) {
				if found != nil {
					return nil, nil // still ambiguous
				}
				found = &matches[i]
			}
		}
		return found, nil
	}
}
