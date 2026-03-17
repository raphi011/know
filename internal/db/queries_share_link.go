package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// GetShareLinkByHash looks up a share link by its token hash.
func (c *Client) GetShareLinkByHash(ctx context.Context, hash string) (*models.ShareLink, error) {
	defer c.logOp(ctx, "share_link.get_by_hash", time.Now())
	sql := `SELECT * FROM share_link WHERE token_hash = $hash LIMIT 1`
	results, err := surrealdb.Query[[]models.ShareLink](ctx, c.DB(), sql, map[string]any{
		"hash": hash,
	})
	if err != nil {
		return nil, fmt.Errorf("get share link by hash: %w", err)
	}
	return firstResultOpt(results), nil
}
