package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

// SearchService handles search operations with LLM synthesis.
type SearchService struct {
	db       *db.Client
	embedder *llm.Embedder
	model    *llm.Model
}

// NewSearchService creates a new search service.
func NewSearchService(db *db.Client, embedder *llm.Embedder, model *llm.Model) *SearchService {
	return &SearchService{
		db:       db,
		embedder: embedder,
		model:    model,
	}
}

// SearchOptions configures a search operation.
type SearchOptions struct {
	Query        string
	Labels       []string
	Types        []string
	VerifiedOnly bool
	Limit        int
}

// Search performs hybrid search without LLM synthesis.
func (s *SearchService) Search(ctx context.Context, opts SearchOptions) ([]models.Entity, error) {
	// Generate query embedding
	var embedding []float32
	if s.embedder != nil {
		var err error
		embedding, err = s.embedder.Embed(ctx, opts.Query)
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
	}

	dbOpts := db.SearchOptions{
		Query:        opts.Query,
		Embedding:    embedding,
		Labels:       opts.Labels,
		Types:        opts.Types,
		VerifiedOnly: opts.VerifiedOnly,
		Limit:        opts.Limit,
	}

	results, err := s.db.HybridSearch(ctx, dbOpts)
	if err != nil {
		return nil, err
	}

	// Update access for returned entities
	for _, entity := range results {
		if idStr, err := models.RecordIDString(entity.ID); err == nil {
			if err := s.db.UpdateEntityAccess(ctx, idStr); err != nil {
				slog.Warn("failed to update entity access", "entity", idStr, "error", err)
			}
		} else {
			slog.Warn("failed to get entity ID for access tracking", "error", err)
		}
	}

	return results, nil
}

// SearchWithChunks performs search including chunk matches.
func (s *SearchService) SearchWithChunks(ctx context.Context, opts SearchOptions) ([]models.EntitySearchResult, error) {
	// Generate query embedding
	var embedding []float32
	if s.embedder != nil {
		var err error
		embedding, err = s.embedder.Embed(ctx, opts.Query)
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
	}

	dbOpts := db.SearchOptions{
		Query:        opts.Query,
		Embedding:    embedding,
		Labels:       opts.Labels,
		Types:        opts.Types,
		VerifiedOnly: opts.VerifiedOnly,
		Limit:        opts.Limit,
	}

	results, err := s.db.SearchWithChunks(ctx, dbOpts)
	if err != nil {
		return nil, err
	}

	// Update access for returned entities
	for _, result := range results {
		if idStr, err := models.RecordIDString(result.ID); err == nil {
			if err := s.db.UpdateEntityAccess(ctx, idStr); err != nil {
				slog.Warn("failed to update entity access", "entity", idStr, "error", err)
			}
		} else {
			slog.Warn("failed to get entity ID for access tracking", "error", err)
		}
	}

	return results, nil
}

// Ask performs search and synthesizes an answer using LLM.
func (s *SearchService) Ask(ctx context.Context, query string, opts SearchOptions) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("LLM model not configured")
	}

	// Search for relevant context
	opts.Query = query
	if opts.Limit == 0 {
		opts.Limit = 20
	}

	results, err := s.SearchWithChunks(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		return "No relevant knowledge found for this query.", nil
	}

	// Build context from results
	contextParts := make([]string, 0, len(results))
	for _, result := range results {
		part := fmt.Sprintf("## %s (%s)\n", result.Name, result.Type)
		if result.Summary != nil {
			part += *result.Summary + "\n"
		}

		// Add matched chunks
		if len(result.MatchedChunks) > 0 {
			for _, chunk := range result.MatchedChunks {
				if chunk.HeadingPath != nil {
					part += fmt.Sprintf("\n### %s\n", *chunk.HeadingPath)
				}
				part += chunk.Content + "\n"
			}
		} else if result.Content != nil {
			// Use full content if no chunks
			content := *result.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			part += content + "\n"
		}

		contextParts = append(contextParts, part)
	}

	searchContext := strings.Join(contextParts, "\n---\n")

	// Synthesize answer
	return s.model.SynthesizeAnswer(ctx, query, searchContext)
}

// AskWithTemplate fills a template with knowledge from search.
func (s *SearchService) AskWithTemplate(ctx context.Context, query string, templateName string, opts SearchOptions) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("LLM model not configured")
	}

	// Get template
	template, err := s.db.GetTemplate(ctx, templateName)
	if err != nil {
		return "", fmt.Errorf("get template: %w", err)
	}
	if template == nil {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	// Search for relevant context
	opts.Query = query
	if opts.Limit == 0 {
		opts.Limit = 30 // More context for template filling
	}

	results, err := s.SearchWithChunks(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no relevant knowledge found for %q", query)
	}

	// Build knowledge context
	knowledgeParts := make([]string, 0, len(results))
	for _, result := range results {
		part := fmt.Sprintf("Entity: %s (type: %s)\n", result.Name, result.Type)
		if result.Summary != nil {
			part += fmt.Sprintf("Summary: %s\n", *result.Summary)
		}
		if len(result.Labels) > 0 {
			part += fmt.Sprintf("Labels: %v\n", result.Labels)
		}
		if result.Content != nil {
			part += fmt.Sprintf("Content:\n%s\n", *result.Content)
		}
		knowledgeParts = append(knowledgeParts, part)
	}

	knowledge := strings.Join(knowledgeParts, "\n---\n")

	// Fill template with LLM
	return s.model.FillTemplate(ctx, template.Content, knowledge)
}
