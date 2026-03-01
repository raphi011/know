package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateRelation(ctx context.Context, input models.DocRelationInput) (*models.DocRelation, error) {
	sql := `
		RELATE type::record("document", $from_id)->doc_relation->type::record("document", $to_id) SET
			rel_type = $rel_type,
			source = $source
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.DocRelation](ctx, c.DB(), sql, map[string]any{
		"from_id":  input.FromDocID,
		"to_id":    input.ToDocID,
		"rel_type": input.RelType,
		"source":   input.Source,
	})
	if err != nil {
		return nil, fmt.Errorf("create relation: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create relation: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetRelations(ctx context.Context, documentID string) ([]models.DocRelation, error) {
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

func (c *Client) DeleteRelation(ctx context.Context, id string) error {
	sql := `DELETE type::record("doc_relation", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	return nil
}
