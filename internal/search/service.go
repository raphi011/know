package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/models"

	"github.com/raphaelgruber/memcp-go/internal/llm"
)

// Service provides search with graceful degradation.
// Base: BM25 fulltext (always). Semantic: chunk vector search (if embedder configured).
type Service struct {
	db       *db.Client
	embedder *llm.Embedder // optional
}

// NewService creates a new search service.
func NewService(db *db.Client, embedder *llm.Embedder) *Service {
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
// With embedder: hybrid search (BM25 + chunk vector, RRF fusion).
// Without embedder: BM25-only fulltext search.
func (s *Service) Search(ctx context.Context, input SearchInput) ([]SearchResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	filter := db.SearchFilter{
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

	// Embed query for chunk vector search
	queryEmbedding, err := s.embedder.Embed(ctx, input.Query)
	if err != nil {
		slog.Warn("failed to embed query, falling back to BM25", "error", err)
		return toBM25Results(bm25Results), nil
	}

	// Search chunks for semantic matches
	chunkResults, err := s.db.ChunkVectorSearch(ctx, queryEmbedding, input.VaultID, limit)
	if err != nil {
		slog.Warn("chunk vector search failed, falling back to BM25", "error", err)
		return toBM25Results(bm25Results), nil
	}

	// Build a set of document IDs already in BM25 results
	bm25DocIDs := make(map[string]bool, len(bm25Results))
	for _, d := range bm25Results {
		id, err := models.RecordIDString(d.ID)
		if err != nil {
			continue
		}
		bm25DocIDs[id] = true
	}

	// Fetch parent documents for chunk hits not in BM25 results
	chunkDocs := make(map[string]models.Document)
	for _, ch := range chunkResults {
		docID, err := models.RecordIDString(ch.Document)
		if err != nil {
			continue
		}
		if bm25DocIDs[docID] {
			continue
		}
		if _, ok := chunkDocs[docID]; ok {
			continue
		}
		doc, err := s.db.GetDocumentByID(ctx, docID)
		if err != nil {
			slog.Warn("failed to fetch chunk parent document", "doc_id", docID, "error", err)
			continue
		}
		if doc != nil {
			chunkDocs[docID] = *doc
		}
	}

	// RRF fusion
	return rrfFusion(bm25Results, chunkResults, chunkDocs, limit), nil
}

// HasSemanticSearch returns true if vector search is available.
func (s *Service) HasSemanticSearch() bool {
	return s.embedder != nil
}

func toBM25Results(docs []db.DocumentWithScore) []SearchResult {
	results := make([]SearchResult, len(docs))
	for i, d := range docs {
		results[i] = SearchResult{
			Document: d.Document,
			Score:    d.Score,
		}
	}
	return results
}

// rrfFusion combines BM25 and chunk vector results using Reciprocal Rank Fusion.
// chunkDocs provides full document data for chunk parents not in BM25 results.
func rrfFusion(bm25 []db.DocumentWithScore, chunks []db.ChunkWithScore, chunkDocs map[string]models.Document, limit int) []SearchResult {
	const k = 60.0 // RRF constant

	type docScore struct {
		doc    models.Document
		score  float64
		chunks []ChunkMatch
	}

	scores := make(map[string]*docScore)

	getID := func(doc models.Document) string {
		id, err := models.RecordIDString(doc.ID)
		if err != nil {
			slog.Warn("failed to extract document ID in search ranking", "error", err)
			return fmt.Sprintf("unknown-%v", doc.ID.ID)
		}
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

	// Chunk vector scores — contribute to document RRF ranking and attach matches
	for rank, ch := range chunks {
		docID, err := models.RecordIDString(ch.Document)
		if err != nil {
			slog.Warn("failed to extract chunk document ID", "error", err)
			continue
		}
		if _, ok := scores[docID]; !ok {
			// Chunk's parent not in BM25 — promote it using fetched document data
			if doc, found := chunkDocs[docID]; found {
				scores[docID] = &docScore{doc: doc}
			} else {
				continue
			}
		}
		scores[docID].score += 1.0 / (k + float64(rank+1))
		scores[docID].chunks = append(scores[docID].chunks, ChunkMatch{
			Content:     ch.Content,
			HeadingPath: ch.HeadingPath,
			Position:    ch.Position,
			Score:       ch.Score,
		})
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
