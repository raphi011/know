package db

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateChunks(ctx context.Context, chunks []models.ChunkInput) error {
	if len(chunks) == 0 {
		return nil
	}

	for i, ch := range chunks {
		labels := ch.Labels
		if labels == nil {
			labels = []string{}
		}

		sql := `
			CREATE chunk SET
				document = type::record("document", $doc_id),
				content = $content,
				position = $position,
				heading_path = $heading_path,
				labels = $labels,
				embedding = $embedding
		`
		if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
			"doc_id":       ch.DocumentID,
			"content":      ch.Content,
			"position":     ch.Position,
			"heading_path": optionalString(ch.HeadingPath),
			"labels":       labels,
			"embedding":    ch.Embedding,
		}); err != nil {
			return fmt.Errorf("create chunk %d: %w", i, err)
		}
	}
	return nil
}

func (c *Client) GetChunks(ctx context.Context, documentID string) ([]models.Chunk, error) {
	sql := `SELECT * FROM chunk WHERE document = type::record("document", $doc_id) ORDER BY position ASC`
	results, err := surrealdb.Query[[]models.Chunk](ctx, c.DB(), sql, map[string]any{
		"doc_id": documentID,
	})
	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) DeleteChunks(ctx context.Context, documentID string) error {
	sql := `DELETE FROM chunk WHERE document = type::record("document", $doc_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"doc_id": documentID,
	}); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}
