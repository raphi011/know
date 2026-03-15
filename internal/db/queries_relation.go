package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateRelation(ctx context.Context, input models.FileRelationInput) (*models.FileRelation, error) {
	defer c.logOp(ctx, "relation.create", time.Now())
	sql := `
		LET $from = type::record("file", $from_id);
		LET $to = type::record("file", $to_id);
		RELATE $from->file_relation->$to SET
			rel_type = $rel_type,
			source = $source
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.FileRelation](ctx, c.DB(), sql, map[string]any{
		"from_id":  bareID("file", input.FromFileID),
		"to_id":    bareID("file", input.ToFileID),
		"rel_type": input.RelType,
		"source":   input.Source,
	})
	if err != nil {
		return nil, fmt.Errorf("create relation: %w", err)
	}
	// Multi-statement query: LET, LET, RELATE — result is at index 2
	if results == nil || len(*results) < 3 || len((*results)[2].Result) == 0 {
		return nil, fmt.Errorf("create relation: no result returned")
	}
	return &(*results)[2].Result[0], nil
}

func (c *Client) GetRelations(ctx context.Context, fileID string) ([]models.FileRelation, error) {
	defer c.logOp(ctx, "relation.get", time.Now())
	sql := `SELECT * FROM file_relation WHERE in = type::record("file", $file_id) OR out = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]models.FileRelation](ctx, c.DB(), sql, map[string]any{
		"file_id": fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("get relations: %w", err)
	}
	return allResults(results), nil
}

func (c *Client) GetRelationByID(ctx context.Context, id string) (*models.FileRelation, error) {
	defer c.logOp(ctx, "relation.get_by_id", time.Now())
	sql := `SELECT * FROM type::record("file_relation", $id)`
	results, err := surrealdb.Query[[]models.FileRelation](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get relation by id: %w", err)
	}
	return firstResultOpt(results), nil
}

// DeleteRelationsBySource removes all file_relations originating from a file with the given source.
// Used to implement delete-then-recreate for frontmatter-derived relations.
func (c *Client) DeleteRelationsBySource(ctx context.Context, fileID, source string) error {
	defer c.logOp(ctx, "relation.delete_by_source", time.Now())
	sql := `DELETE FROM file_relation WHERE in = type::record("file", $file_id) AND source = $source`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
		"source":  source,
	}); err != nil {
		return fmt.Errorf("delete relations by source: %w", err)
	}
	return nil
}

func (c *Client) DeleteRelation(ctx context.Context, id string) error {
	defer c.logOp(ctx, "relation.delete", time.Now())
	sql := `DELETE type::record("file_relation", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	return nil
}
