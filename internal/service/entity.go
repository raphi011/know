// Package service provides business logic for Knowhow operations.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/raphaelgruber/memcp-go/internal/parser"
)

// EntityService handles entity operations with LLM integration.
type EntityService struct {
	db       *db.Client
	embedder *llm.Embedder
	model    *llm.Model
}

// CreateResult contains the result of entity creation.
type CreateResult struct {
	Entity        *models.Entity
	ChunksCreated int
}

// NewEntityService creates a new entity service.
func NewEntityService(db *db.Client, embedder *llm.Embedder, model *llm.Model) *EntityService {
	return &EntityService{
		db:       db,
		embedder: embedder,
		model:    model,
	}
}

// Create creates a new entity with automatic embedding generation.
// For large content that will be chunked, we skip entity-level embedding
// and rely on chunk embeddings for search (chunks link back to entity).
// Returns CreateResult with entity and chunk count.
func (s *EntityService) Create(ctx context.Context, input models.EntityInput) (*CreateResult, error) {
	// Check if content will be chunked - if so, skip entity-level embedding
	willChunk := input.Content != nil && parser.ShouldChunk(*input.Content, parser.DefaultChunkConfig())

	// Generate embedding from content/summary (skip if content will be chunked)
	if s.embedder != nil && !willChunk {
		text := ""
		if input.Summary != nil {
			text = *input.Summary
		}
		if input.Content != nil {
			if text != "" {
				text += " "
			}
			text += *input.Content
		}
		if input.Name != "" {
			text = input.Name + " " + text
		}

		if text != "" {
			embedding, err := s.embedder.Embed(ctx, text)
			if err != nil {
				return nil, fmt.Errorf("generate embedding: %w", err)
			}
			input.Embedding = embedding
		}
	} else if willChunk {
		slog.Debug("skipping entity embedding - content will be chunked", "name", input.Name)
	} else {
		slog.Debug("creating entity without embedding - embedder not configured", "name", input.Name)
	}

	// Create entity
	entity, err := s.db.CreateEntity(ctx, input)
	if err != nil {
		return nil, err
	}

	result := &CreateResult{Entity: entity}

	// Check if content should be chunked (skip if content is empty)
	if input.Content != nil && *input.Content != "" && parser.ShouldChunk(*input.Content, parser.DefaultChunkConfig()) {
		idStr, idErr := models.RecordIDString(entity.ID)
		if idErr != nil {
			slog.Warn("failed to get entity ID for chunking", "error", idErr)
		} else if chunksCreated, err := s.chunkEntity(ctx, entity); err != nil {
			// Log but don't fail - entity was created successfully
			slog.Warn("failed to chunk entity", "entity", idStr, "error", err)
		} else {
			result.ChunksCreated = chunksCreated
			if chunksCreated > 0 {
				slog.Debug("chunked entity", "entity", idStr, "chunks", chunksCreated)
			}
		}
	}

	return result, nil
}

// chunkEntity creates chunks for an entity with long content.
// Returns the number of chunks created.
func (s *EntityService) chunkEntity(ctx context.Context, entity *models.Entity) (int, error) {
	if entity.Content == nil {
		return 0, nil
	}

	entityID, err := models.RecordIDString(entity.ID)
	if err != nil {
		return 0, fmt.Errorf("get entity ID: %w", err)
	}

	doc, err := parser.ParseMarkdown(*entity.Content)
	if err != nil {
		return 0, fmt.Errorf("parse markdown: %w", err)
	}

	chunks := parser.ChunkMarkdown(doc, parser.DefaultChunkConfig())
	if len(chunks) == 0 {
		// No meaningful content to chunk (e.g., all-empty sections)
		slog.Debug("no chunks produced - content may be empty sections only", "entity", entityID)
		return 0, nil
	}
	if len(chunks) == 1 {
		return 0, nil // No need to chunk - single chunk handled at entity level
	}

	// Batch embed all chunks at once
	var embeddings [][]float32
	if s.embedder != nil {
		texts := make([]string, len(chunks))
		for i, chunk := range chunks {
			texts[i] = chunk.Content
		}
		embeddings, err = s.embedder.EmbedBatch(ctx, texts)
		if err != nil {
			return 0, fmt.Errorf("batch embed chunks: %w", err)
		}
	}

	chunkInputs := make([]models.ChunkInput, 0, len(chunks))
	for i, chunk := range chunks {
		var embedding []float32
		if embeddings != nil {
			embedding = embeddings[i]
		}

		headingPath := chunk.HeadingPath
		chunkInputs = append(chunkInputs, models.ChunkInput{
			EntityID:    entityID,
			Content:     chunk.Content,
			Position:    chunk.Position,
			HeadingPath: &headingPath,
			Labels:      entity.Labels,
			Embedding:   embedding,
		})
	}

	if err := s.db.CreateChunks(ctx, entityID, chunkInputs); err != nil {
		return 0, err
	}
	return len(chunkInputs), nil
}

// Update updates an entity with re-chunking if content changed.
func (s *EntityService) Update(ctx context.Context, id string, update models.EntityUpdate) (*models.Entity, error) {
	// Re-generate embedding if content or summary changed
	if s.embedder != nil && (update.Content != nil || update.Summary != nil) {
		// Get current entity to merge text
		current, err := s.db.GetEntity(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get entity for embedding: %w", err)
		}
		if current == nil {
			return nil, fmt.Errorf("entity not found: %s", id)
		}

		text := current.Name
		if update.Summary != nil {
			text += " " + *update.Summary
		} else if current.Summary != nil {
			text += " " + *current.Summary
		}
		if update.Content != nil {
			text += " " + *update.Content
		} else if current.Content != nil {
			text += " " + *current.Content
		}

		embedding, err := s.embedder.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("generate embedding: %w", err)
		}
		update.Embedding = embedding
	}

	// Update entity
	entity, err := s.db.UpdateEntity(ctx, id, update)
	if err != nil {
		return nil, err
	}

	// Re-chunk if content changed
	if update.Content != nil {
		// Delete old chunks
		if err := s.db.DeleteChunks(ctx, id); err != nil {
			return nil, fmt.Errorf("delete old chunks: %w", err)
		}

		// Create new chunks if content is long
		if parser.ShouldChunk(*update.Content, parser.DefaultChunkConfig()) {
			if _, err := s.chunkEntity(ctx, entity); err != nil {
				slog.Warn("failed to re-chunk entity", "entity", id, "error", err)
			}
		}
	}

	return entity, nil
}

// Get retrieves an entity by ID and updates access tracking.
func (s *EntityService) Get(ctx context.Context, id string) (*models.Entity, error) {
	entity, err := s.db.GetEntity(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity != nil {
		if err := s.db.UpdateEntityAccess(ctx, id); err != nil {
			slog.Warn("failed to update entity access", "entity", id, "error", err)
		}
	}
	return entity, nil
}

// Delete deletes an entity by ID (chunks/relations cascade deleted by DB).
func (s *EntityService) Delete(ctx context.Context, id string) (bool, error) {
	return s.db.DeleteEntity(ctx, id)
}

// CreateRelation creates a relation between entities.
func (s *EntityService) CreateRelation(ctx context.Context, input models.RelationInput) error {
	return s.db.CreateRelation(ctx, input)
}

// GetRelations gets all relations for an entity.
func (s *EntityService) GetRelations(ctx context.Context, id string) ([]models.Relation, error) {
	return s.db.GetRelations(ctx, id)
}
