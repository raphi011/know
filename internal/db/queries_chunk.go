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

		row := map[string]any{
			"file":       surrealmodels.RecordID{Table: "file", ID: bareID("file", ch.FileID)},
			"text":       ch.Text,
			"position":   ch.Position,
			"source_loc": optionalString(ch.SourceLoc),
			"labels":     labels,
			"mime_type":  ch.MimeType,
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

func (c *Client) GetChunks(ctx context.Context, fileID string) ([]models.Chunk, error) {
	defer c.logOp(ctx, "chunk.get", time.Now())
	sql := `SELECT * FROM chunk WHERE file = type::record("file", $file_id) ORDER BY position ASC`
	results, err := surrealdb.Query[[]models.Chunk](ctx, c.DB(), sql, map[string]any{
		"file_id": fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)
	}
	return allResults(results), nil
}

// GetUnembeddedChunks returns chunks for a file that have no embedding yet.
func (c *Client) GetUnembeddedChunks(ctx context.Context, fileID string) ([]models.Chunk, error) {
	defer c.logOp(ctx, "chunk.get_unembedded", time.Now())
	sql := `SELECT * FROM chunk WHERE file = type::record("file", $file_id) AND embedding IS NONE ORDER BY position ASC`
	results, err := surrealdb.Query[[]models.Chunk](ctx, c.DB(), sql, map[string]any{
		"file_id": fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("get unembedded chunks: %w", err)
	}
	return allResults(results), nil
}

func (c *Client) DeleteChunks(ctx context.Context, fileID string) error {
	defer c.logOp(ctx, "chunk.delete", time.Now())
	sql := `DELETE FROM chunk WHERE file = type::record("file", $file_id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id": fileID,
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

