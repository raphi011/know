package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateRelation(ctx context.Context, input models.DocRelationInput) (*models.DocRelation, error) {
	defer c.logOp(ctx, "relation.create", time.Now())
	sql := `
		LET $from = type::record("document", $from_id);
		LET $to = type::record("document", $to_id);
		RELATE $from->doc_relation->$to SET
			rel_type = $rel_type,
			source = $source
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.DocRelation](ctx, c.DB(), sql, map[string]any{
		"from_id":  bareID("document", input.FromDocID),
		"to_id":    bareID("document", input.ToDocID),
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

func (c *Client) GetRelations(ctx context.Context, documentID string) ([]models.DocRelation, error) {
	defer c.logOp(ctx, "relation.get", time.Now())
	sql := `SELECT * FROM doc_relation WHERE in = type::record("document", $doc_id) OR out = type::record("document", $doc_id)`
	results, err := surrealdb.Query[[]models.DocRelation](ctx, c.DB(), sql, map[string]any{
		"doc_id": documentID,
	})
	if err != nil {
		return nil, fmt.Errorf("get relations: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) GetRelationByID(ctx context.Context, id string) (*models.DocRelation, error) {
	defer c.logOp(ctx, "relation.get_by_id", time.Now())
	sql := `SELECT * FROM type::record("doc_relation", $id)`
	results, err := surrealdb.Query[[]models.DocRelation](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get relation by id: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// DeleteRelationsBySource removes all doc_relations originating from a document with the given source.
// Used to implement delete-then-recreate for frontmatter-derived relations.
func (c *Client) DeleteRelationsBySource(ctx context.Context, docID, source string) error {
	defer c.logOp(ctx, "relation.delete_by_source", time.Now())
	sql := `DELETE FROM doc_relation WHERE in = type::record("document", $doc_id) AND source = $source`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"doc_id": bareID("document", docID),
		"source": source,
	}); err != nil {
		return fmt.Errorf("delete relations by source: %w", err)
	}
	return nil
}

func (c *Client) DeleteRelation(ctx context.Context, id string) error {
	defer c.logOp(ctx, "relation.delete", time.Now())
	sql := `DELETE type::record("doc_relation", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	return nil
}
