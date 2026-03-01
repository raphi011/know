package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"

	"github.com/raphi011/knowhow/internal/llm"
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
	DocumentID    string
	Path          string
	Title         string
	Labels        []string
	DocType       *string
	Score         float64
	MatchedChunks []ChunkMatch
}

type ChunkMatch struct {
	Snippet     string
	HeadingPath *string
	Position    int
	Score       float64
}

const maxSnippetLen = 200

func truncateSnippet(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find a word boundary near maxLen to avoid cutting mid-word
	cut := maxLen
	for cut > maxLen-30 && cut > 0 && s[cut] != ' ' {
		cut--
	}
	if cut <= maxLen-30 || cut == 0 {
		cut = maxLen // no nearby space, just hard cut
	}
	return s[:cut] + "…"
}

const maxLimit = 100

// Search performs search with graceful degradation.
// With embedder: hybrid search (BM25 + chunk vector, RRF fusion).
// Without embedder: BM25-only fulltext search.
func (s *Service) Search(ctx context.Context, input SearchInput) ([]SearchResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > maxLimit {
		limit = maxLimit
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

	// Search chunks for semantic matches (same filters as BM25)
	chunkResults, err := s.db.ChunkVectorSearch(ctx, queryEmbedding, filter)
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

	// Collect unique doc IDs from chunks that aren't in BM25 results
	var missingDocIDs []string
	seen := make(map[string]bool)
	for _, ch := range chunkResults {
		docID, err := models.RecordIDString(ch.Document)
		if err != nil {
			continue
		}
		if bm25DocIDs[docID] || seen[docID] {
			continue
		}
		seen[docID] = true
		missingDocIDs = append(missingDocIDs, docID)
	}

	// Batch fetch parent documents for chunk hits not in BM25 results
	chunkDocs := make(map[string]models.Document)
	if len(missingDocIDs) > 0 {
		docs, err := s.db.GetDocumentsByIDs(ctx, missingDocIDs)
		if err != nil {
			slog.Warn("failed to batch fetch chunk parent documents", "error", err)
		} else {
			for _, d := range docs {
				id, err := models.RecordIDString(d.ID)
				if err != nil {
					continue
				}
				chunkDocs[id] = d
			}
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
		id, err := models.RecordIDString(d.ID)
		if err != nil {
			slog.Warn("failed to extract document ID in BM25 results", "error", err)
			id = fmt.Sprintf("%v", d.ID.ID)
		}
		labels := d.Labels
		if labels == nil {
			labels = []string{}
		}
		results[i] = SearchResult{
			DocumentID:    id,
			Path:          d.Path,
			Title:         d.Title,
			Labels:        labels,
			DocType:       d.DocType,
			Score:         d.Score,
			MatchedChunks: []ChunkMatch{},
		}
	}
	return results
}

// docInfo holds lightweight document metadata for RRF fusion.
type docInfo struct {
	id      string
	path    string
	title   string
	labels  []string
	docType *string
}

func docInfoFromModel(d models.Document) docInfo {
	id, err := models.RecordIDString(d.ID)
	if err != nil {
		slog.Warn("failed to extract document ID in search ranking", "error", err)
		id = fmt.Sprintf("unknown-%v", d.ID.ID)
	}
	labels := d.Labels
	if labels == nil {
		labels = []string{}
	}
	return docInfo{
		id:      id,
		path:    d.Path,
		title:   d.Title,
		labels:  labels,
		docType: d.DocType,
	}
}

// rrfFusion combines BM25 and chunk vector results using Reciprocal Rank Fusion.
// chunkDocs provides full document data for chunk parents not in BM25 results.
func rrfFusion(bm25 []db.DocumentWithScore, chunks []db.ChunkWithScore, chunkDocs map[string]models.Document, limit int) []SearchResult {
	const k = 60.0 // RRF constant

	type docScore struct {
		info   docInfo
		score  float64
		chunks []ChunkMatch
	}

	scores := make(map[string]*docScore)

	// BM25 scores
	for rank, d := range bm25 {
		info := docInfoFromModel(d.Document)
		if _, ok := scores[info.id]; !ok {
			scores[info.id] = &docScore{info: info}
		}
		scores[info.id].score += 1.0 / (k + float64(rank+1))
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
				scores[docID] = &docScore{info: docInfoFromModel(doc)}
			} else {
				continue
			}
		}
		scores[docID].score += 1.0 / (k + float64(rank+1))
		scores[docID].chunks = append(scores[docID].chunks, ChunkMatch{
			Snippet:     truncateSnippet(ch.Content, maxSnippetLen),
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
		chunks := ds.chunks
		if chunks == nil {
			chunks = []ChunkMatch{}
		}
		results[i] = SearchResult{
			DocumentID:    ds.info.id,
			Path:          ds.info.path,
			Title:         ds.info.title,
			Labels:        ds.info.labels,
			DocType:       ds.info.docType,
			Score:         ds.score,
			MatchedChunks: chunks,
		}
	}
	return results
}
