package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// LookupQueryEmbedding looks up a cached embedding for the given normalized query.
// Returns nil if not found.
func (c *Client) LookupQueryEmbedding(ctx context.Context, normalizedQuery string) (*models.SearchQuery, error) {
	start := c.startOp()
	defer c.recordTiming("db.lookup_query_embedding", start)

	sql := `SELECT * FROM search_query WHERE query = $query LIMIT 1`
	results, err := surrealdb.Query[[]models.SearchQuery](ctx, c.DB(), sql, map[string]any{
		"query": normalizedQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("lookup query embedding: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// UpsertQueryEmbedding inserts a new search query cache entry or updates hit_count
// and last_searched_at if the query already exists.
func (c *Client) UpsertQueryEmbedding(ctx context.Context, normalizedQuery string, embedding []float32) error {
	start := c.startOp()
	defer c.recordTiming("db.upsert_query_embedding", start)

	sql := `
		UPSERT search_query
		SET query = $query,
			embedding = $embedding,
			hit_count += 1,
			last_searched_at = time::now()
		WHERE query = $query
	`
	_, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"query":     normalizedQuery,
		"embedding": embedding,
	})
	if err != nil {
		return fmt.Errorf("upsert query embedding: %w", err)
	}
	return nil
}
