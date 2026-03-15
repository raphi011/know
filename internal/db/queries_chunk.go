package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func (c *Client) CreateChunks(ctx context.Context, chunks []models.ChunkInput) error {
	defer c.logOp(ctx, "chunk.create", time.Now())
	if len(chunks) == 0 {
		return nil
	}

	// Split into two batches: chunks with embeddings and without.
	// The HNSW index rejects NONE values, so we must omit the embedding
	// key entirely for chunks that don't have one yet.
	var withEmbed, withoutEmbed []map[string]any

	for _, ch := range chunks {
		labels := ch.Labels
		if labels == nil {
			labels = []string{}
		}

		var embedAt any = surrealmodels.None
		if ch.EmbedAt != nil {
			embedAt = *ch.EmbedAt
		}

		row := map[string]any{
			"document":     surrealmodels.RecordID{Table: "document", ID: bareID("document", ch.DocumentID)},
			"content":      ch.Content,
			"position":     ch.Position,
			"heading_path": optionalString(ch.HeadingPath),
			"labels":       labels,
			"embed_at":     embedAt,
		}

		if len(ch.Embedding) > 0 {
			row["embedding"] = ch.Embedding
			withEmbed = append(withEmbed, row)
		} else {
			withoutEmbed = append(withoutEmbed, row)
		}
	}

	tx, err := c.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("create chunks begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	if len(withoutEmbed) > 0 {
		if _, err := surrealdb.Query[any](ctx, tx,
			"INSERT INTO chunk $chunks RETURN NONE", map[string]any{"chunks": withoutEmbed},
		); err != nil {
			return fmt.Errorf("create chunks (no embedding): %w", err)
		}
	}

	if len(withEmbed) > 0 {
		if _, err := surrealdb.Query[any](ctx, tx,
			"INSERT INTO chunk $chunks RETURN NONE", map[string]any{"chunks": withEmbed},
		); err != nil {
			return fmt.Errorf("create chunks (with embedding): %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("create chunks commit: %w", err)
	}
	return nil
}

func (c *Client) GetChunks(ctx context.Context, documentID string) ([]models.Chunk, error) {
	defer c.logOp(ctx, "chunk.get", time.Now())
	sql := `SELECT * FROM chunk WHERE document = type::record("document", $doc_id) ORDER BY position ASC`
	results, err := surrealdb.Query[[]models.Chunk](ctx, c.DB(), sql, map[string]any{
		"doc_id": documentID,
	})
	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)
	}
	return allResults(results), nil
}

func (c *Client) DeleteChunks(ctx context.Context, documentID string) error {
	defer c.logOp(ctx, "chunk.delete", time.Now())
	sql := `DELETE FROM chunk WHERE document = type::record("document", $doc_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"doc_id": documentID,
	}); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}

// DeleteChunkByID deletes a single chunk by its ID.
func (c *Client) DeleteChunkByID(ctx context.Context, id string) error {
	defer c.logOp(ctx, "chunk.delete_by_id", time.Now())
	sql := `DELETE type::record("chunk", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id": id,
	}); err != nil {
		return fmt.Errorf("delete chunk %s: %w", id, err)
	}
	return nil
}

// UpdateChunkPosition updates only the position field of a chunk.
func (c *Client) UpdateChunkPosition(ctx context.Context, id string, position int) error {
	defer c.logOp(ctx, "chunk.update_position", time.Now())
	sql := `UPDATE type::record("chunk", $id) SET position = $position`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":       id,
		"position": position,
	}); err != nil {
		return fmt.Errorf("update chunk position: %w", err)
	}
	return nil
}

// ClaimChunksForEmbedding atomically claims chunks that are due for embedding.
// It uses a subquery to SELECT candidate chunks (with ORDER/LIMIT), then UPDATEs
// those specific records — clearing embed_at and returning the BEFORE state so the
// caller gets the content to embed. This prevents double-processing.
func (c *Client) ClaimChunksForEmbedding(ctx context.Context, limit int) ([]models.Chunk, error) {
	defer c.logOp(ctx, "chunk.claim_for_embedding", time.Now())
	sql := `
		UPDATE (
			SELECT id, embed_at FROM chunk
			WHERE embed_at IS NOT NONE AND embed_at <= time::now()
			ORDER BY embed_at ASC
			LIMIT $limit
		)
		SET embed_at = NONE
		RETURN BEFORE
	`
	results, err := surrealdb.Query[[]models.Chunk](ctx, c.DB(), sql, map[string]any{
		"limit": limit,
	})
	if err != nil {
		return nil, fmt.Errorf("claim chunks for embedding: %w", err)
	}
	return allResults(results), nil
}

// UpdateChunkEmbedding sets the embedding vector on a chunk after the worker embeds it.
func (c *Client) UpdateChunkEmbedding(ctx context.Context, id string, embedding []float32) error {
	defer c.logOp(ctx, "chunk.update_embedding", time.Now())
	sql := `UPDATE type::record("chunk", $id) SET embedding = $embedding`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":        id,
		"embedding": embedding,
	}); err != nil {
		return fmt.Errorf("update chunk embedding: %w", err)
	}
	return nil
}

// ChunkEmbeddingUpdate pairs a chunk ID with its embedding vector for batch updates.
type ChunkEmbeddingUpdate struct {
	ID        string
	Embedding []float32
}

// BatchUpdateChunkEmbeddings updates multiple chunk embeddings in a single transaction.
func (c *Client) BatchUpdateChunkEmbeddings(ctx context.Context, updates []ChunkEmbeddingUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	defer c.logOp(ctx, "chunk.batch_update_embedding", time.Now())

	// Build a single transaction with one UPDATE per chunk
	var b strings.Builder
	vars := make(map[string]any, len(updates)*2)
	b.WriteString("BEGIN TRANSACTION;\n")
	for i, u := range updates {
		idKey := fmt.Sprintf("id_%d", i)
		embKey := fmt.Sprintf("emb_%d", i)
		fmt.Fprintf(&b, "UPDATE type::record(\"chunk\", $%s) SET embedding = $%s;\n", idKey, embKey)
		vars[idKey] = u.ID
		vars[embKey] = u.Embedding
	}
	b.WriteString("COMMIT TRANSACTION;")

	if _, err := surrealdb.Query[any](ctx, c.DB(), b.String(), vars); err != nil {
		return fmt.Errorf("batch update embeddings: %w", err)
	}
	return nil
}

// RescheduleChunkEmbedding re-schedules a chunk for embedding after a failure.
// Uses a 30s backoff to avoid tight retry storms during provider outages.
func (c *Client) RescheduleChunkEmbedding(ctx context.Context, id string) error {
	defer c.logOp(ctx, "chunk.reschedule_embedding", time.Now())
	retryAt := time.Now().UTC().Add(30 * time.Second)
	sql := `UPDATE type::record("chunk", $id) SET embed_at = $embed_at`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":       id,
		"embed_at": retryAt,
	}); err != nil {
		return fmt.Errorf("reschedule chunk embedding: %w", err)
	}
	return nil
}
