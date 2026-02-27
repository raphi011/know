package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

type SearchFilter struct {
	VaultID string
	Labels  []string
	DocType *string
	Folder  *string
	Limit   int
}

type DocumentWithScore struct {
	models.Document
	Score float64 `json:"score"`
}

type ChunkWithScore struct {
	models.Chunk
	Score float64 `json:"score"`
}

// BM25Search performs fulltext search on document content_body and title.
func (c *Client) BM25Search(ctx context.Context, query string, filter SearchFilter) ([]DocumentWithScore, error) {
	var conditions []string
	vars := map[string]any{
		"vault_id": filter.VaultID,
		"query":    query,
	}

	conditions = append(conditions, `vault = type::record("vault", $vault_id)`)

	if len(filter.Labels) > 0 {
		conditions = append(conditions, `labels CONTAINSANY $labels`)
		vars["labels"] = filter.Labels
	}
	if filter.DocType != nil {
		conditions = append(conditions, `doc_type = $doc_type`)
		vars["doc_type"] = *filter.DocType
	}
	if filter.Folder != nil {
		conditions = append(conditions, `string::starts_with(path, $folder)`)
		vars["folder"] = *filter.Folder
	}

	limit := 20
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	sql := fmt.Sprintf(`
		SELECT *, search::score(1) + search::score(2) AS score
		FROM document
		WHERE (%s)
			AND (content_body @1@ $query OR title @2@ $query)
		ORDER BY score DESC
		LIMIT %d
	`, strings.Join(conditions, " AND "), limit)

	results, err := surrealdb.Query[[]DocumentWithScore](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// VectorSearch performs HNSW vector similarity search on document embeddings.
func (c *Client) VectorSearch(ctx context.Context, embedding []float32, filter SearchFilter) ([]DocumentWithScore, error) {
	var conditions []string
	vars := map[string]any{
		"vault_id":  filter.VaultID,
		"embedding": embedding,
	}

	conditions = append(conditions, `vault = type::record("vault", $vault_id)`)

	if len(filter.Labels) > 0 {
		conditions = append(conditions, `labels CONTAINSANY $labels`)
		vars["labels"] = filter.Labels
	}
	if filter.DocType != nil {
		conditions = append(conditions, `doc_type = $doc_type`)
		vars["doc_type"] = *filter.DocType
	}
	if filter.Folder != nil {
		conditions = append(conditions, `string::starts_with(path, $folder)`)
		vars["folder"] = *filter.Folder
	}

	limit := 20
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	sql := fmt.Sprintf(`
		SELECT *, vector::similarity::cosine(embedding, $embedding) AS score
		FROM document
		WHERE %s
			AND embedding <|%d|> $embedding
		ORDER BY score DESC
		LIMIT %d
	`, strings.Join(conditions, " AND "), limit, limit)

	results, err := surrealdb.Query[[]DocumentWithScore](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// ChunkVectorSearch performs HNSW vector similarity search on chunk embeddings.
func (c *Client) ChunkVectorSearch(ctx context.Context, embedding []float32, vaultID string, limit int) ([]ChunkWithScore, error) {
	if limit <= 0 {
		limit = 20
	}

	sql := fmt.Sprintf(`
		SELECT *, vector::similarity::cosine(embedding, $embedding) AS score
		FROM chunk
		WHERE document.vault = type::record("vault", $vault_id)
			AND embedding <|%d|> $embedding
		ORDER BY score DESC
		LIMIT %d
	`, limit, limit)

	results, err := surrealdb.Query[[]ChunkWithScore](ctx, c.DB(), sql, map[string]any{
		"vault_id":  vaultID,
		"embedding": embedding,
	})
	if err != nil {
		return nil, fmt.Errorf("chunk vector search: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}
