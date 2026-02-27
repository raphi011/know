package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"

	"github.com/raphaelgruber/memcp-go/internal/llm"
)

// Service provides search with graceful degradation.
// Base: BM25 fulltext (always). Semantic: vector search (if embedder configured).
type Service struct {
	db       *v2db.Client
	embedder *llm.Embedder // optional
}

// NewService creates a new search service.
func NewService(db *v2db.Client, embedder *llm.Embedder) *Service {
	return &Service{db: db, embedder: embedder}
}

type SearchInput struct {
	VaultID string
	Query   string
	Labels  []string
	DocType *string
	Folder  *string
	Limit   int
}

type SearchResult struct {
	Document      models.Document
	Score         float64
	MatchedChunks []ChunkMatch
}

type ChunkMatch struct {
	Content     string
	HeadingPath *string
	Position    int
	Score       float64
}

// Search performs search with graceful degradation.
// With embedder: hybrid search (BM25 + vector, RRF fusion).
// Without embedder: BM25-only fulltext search.
func (s *Service) Search(ctx context.Context, input SearchInput) ([]SearchResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	filter := v2db.SearchFilter{
		VaultID: input.VaultID,
		Labels:  input.Labels,
		DocType: input.DocType,
		Folder:  input.Folder,
		Limit:   limit,
	}

	// Always run BM25
	bm25Results, err := s.db.BM25Search(ctx, input.Query, filter)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	// If no embedder, return BM25 results directly
	if s.embedder == nil {
		return toBM25Results(bm25Results), nil
	}

	// Embed query for vector search
	queryEmbedding, err := s.embedder.Embed(ctx, input.Query)
	if err != nil {
		slog.Warn("failed to embed query, falling back to BM25", "error", err)
		return toBM25Results(bm25Results), nil
	}

	// Run vector search
	vectorResults, err := s.db.VectorSearch(ctx, queryEmbedding, filter)
	if err != nil {
		slog.Warn("vector search failed, falling back to BM25", "error", err)
		return toBM25Results(bm25Results), nil
	}

	// Also search chunks for granular matches
	chunkResults, err := s.db.ChunkVectorSearch(ctx, queryEmbedding, input.VaultID, limit)
	if err != nil {
		slog.Warn("chunk vector search failed", "error", err)
	}

	// RRF fusion
	return rrfFusion(bm25Results, vectorResults, chunkResults, limit), nil
}

// HasSemanticSearch returns true if vector search is available.
func (s *Service) HasSemanticSearch() bool {
	return s.embedder != nil
}

func toBM25Results(docs []v2db.DocumentWithScore) []SearchResult {
	results := make([]SearchResult, len(docs))
	for i, d := range docs {
		results[i] = SearchResult{
			Document: d.Document,
			Score:    d.Score,
		}
	}
	return results
}

// rrfFusion combines BM25 and vector results using Reciprocal Rank Fusion.
func rrfFusion(bm25, vector []v2db.DocumentWithScore, chunks []v2db.ChunkWithScore, limit int) []SearchResult {
	const k = 60.0 // RRF constant

	type docScore struct {
		doc    models.Document
		score  float64
		chunks []ChunkMatch
	}

	scores := make(map[string]*docScore)

	getID := func(doc models.Document) string {
		id, _ := models.RecordIDString(doc.ID)
		return id
	}

	// BM25 scores
	for rank, d := range bm25 {
		id := getID(d.Document)
		if _, ok := scores[id]; !ok {
			scores[id] = &docScore{doc: d.Document}
		}
		scores[id].score += 1.0 / (k + float64(rank+1))
	}

	// Vector scores
	for rank, d := range vector {
		id := getID(d.Document)
		if _, ok := scores[id]; !ok {
			scores[id] = &docScore{doc: d.Document}
		}
		scores[id].score += 1.0 / (k + float64(rank+1))
	}

	// Attach chunk matches
	for _, ch := range chunks {
		docID, _ := models.RecordIDString(ch.Document)
		if ds, ok := scores[docID]; ok {
			ds.chunks = append(ds.chunks, ChunkMatch{
				Content:     ch.Content,
				HeadingPath: ch.HeadingPath,
				Position:    ch.Position,
				Score:       ch.Score,
			})
		}
	}

	// Sort by fused score
	sorted := make([]*docScore, 0, len(scores))
	for _, ds := range scores {
		sorted = append(sorted, ds)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	results := make([]SearchResult, len(sorted))
	for i, ds := range sorted {
		results[i] = SearchResult{
			Document:      ds.doc,
			Score:         ds.score,
			MatchedChunks: ds.chunks,
		}
	}
	return results
}
