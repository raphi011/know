// Package service provides business logic for Knowhow operations.
package service

import (
	"context"
	"fmt"

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

// NewEntityService creates a new entity service.
func NewEntityService(db *db.Client, embedder *llm.Embedder, model *llm.Model) *EntityService {
	return &EntityService{
		db:       db,
		embedder: embedder,
		model:    model,
	}
}

// Create creates a new entity with automatic embedding generation.
func (s *EntityService) Create(ctx context.Context, input models.EntityInput) (*models.Entity, error) {
	// Generate embedding from content/summary
	if s.embedder != nil {
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
	}

	// Create entity
	entity, err := s.db.CreateEntity(ctx, input)
	if err != nil {
		return nil, err
	}

	// Check if content should be chunked
	if input.Content != nil && parser.ShouldChunk(*input.Content, parser.DefaultChunkConfig()) {
		if err := s.chunkEntity(ctx, entity); err != nil {
			// Log but don't fail - entity was created successfully
			fmt.Printf("Warning: failed to chunk entity: %v\n", err)
		}
	}

	return entity, nil
}

// chunkEntity creates chunks for an entity with long content.
func (s *EntityService) chunkEntity(ctx context.Context, entity *models.Entity) error {
	if entity.Content == nil {
		return nil
	}

	doc, err := parser.ParseMarkdown(*entity.Content)
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}

	chunks := parser.ChunkMarkdown(doc, parser.DefaultChunkConfig())
	if len(chunks) <= 1 {
		return nil // No need to chunk
	}

	chunkInputs := make([]models.ChunkInput, 0, len(chunks))
	for _, chunk := range chunks {
		// Generate embedding for chunk
		var embedding []float32
		if s.embedder != nil {
			var err error
			embedding, err = s.embedder.Embed(ctx, chunk.Content)
			if err != nil {
				return fmt.Errorf("embed chunk %d: %w", chunk.Position, err)
			}
		}

		headingPath := chunk.HeadingPath
		chunkInputs = append(chunkInputs, models.ChunkInput{
			EntityID:    entity.ID.ID.(string),
			Content:     chunk.Content,
			Position:    chunk.Position,
			HeadingPath: &headingPath,
			Labels:      entity.Labels,
			Embedding:   embedding,
		})
	}

	return s.db.CreateChunks(ctx, entity.ID.ID.(string), chunkInputs)
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
			if err := s.chunkEntity(ctx, entity); err != nil {
				fmt.Printf("Warning: failed to re-chunk entity: %v\n", err)
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
		_ = s.db.UpdateEntityAccess(ctx, id)
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
