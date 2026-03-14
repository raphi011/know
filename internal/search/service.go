package search

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// Service provides search with graceful degradation.
// Base: BM25 fulltext on chunks (always). Semantic: hybrid BM25+vector via search::rrf (if embedder configured).
type Service struct {
	db       *db.Client
	embedder atomic.Pointer[llm.Embedder] // optional
}

// NewService creates a new search service.
func NewService(db *db.Client, embedder *llm.Embedder) *Service {
	s := &Service{db: db}
	s.embedder.Store(embedder)
	return s
}

// SetEmbedder atomically replaces the embedder (used by SIGHUP reload).
func (s *Service) SetEmbedder(e *llm.Embedder) {
	s.embedder.Store(e)
}

// getEmbedder returns the current embedder via an atomic load.
func (s *Service) getEmbedder() *llm.Embedder {
	return s.embedder.Load()
}

type SearchInput struct {
	VaultID     string
	Query       string
	Labels      []string
	DocType     *string
	Folder      *string
	Limit       int
	FullContent bool // When true, chunks carry full content instead of truncated snippets
	BM25Only    bool // When true, skip vector search even if embedder is configured
}

type SearchResult struct {
	DocumentID    string
	Path          string
	Title         string
	Labels        []string
	DocType       *string
	Score         float64
	MatchedChunks []ChunkMatch
	Degraded      bool // True when hybrid search fell back to BM25-only due to embed/vector failure
}

type ChunkMatch struct {
	Snippet     string
	HeadingPath *string
	Position    int
	Score       float64
}

const maxSnippetLen = 200

func truncateSnippet(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	// Find a word boundary near maxLen to avoid cutting mid-word
	cut := maxLen
	for cut > maxLen-30 && cut > 0 && runes[cut] != ' ' {
		cut--
	}
	if cut <= maxLen-30 || cut == 0 {
		cut = maxLen // no nearby space, just hard cut
	}
	return string(runes[:cut]) + "…"
}

const maxLimit = 100

// Search performs search with graceful degradation.
// With embedder: hybrid search via SurrealDB's search::rrf() (BM25 + vector, fused in DB).
// Without embedder: BM25-only chunk search.
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

	// BM25-only path: no embedder or explicitly requested
	embedder := s.getEmbedder()
	if embedder == nil || input.BM25Only {
		chunks, err := s.db.BM25ChunkSearch(ctx, input.Query, filter)
		if err != nil {
			return nil, fmt.Errorf("bm25 chunk search: %w", err)
		}
		return assembleResults(ctx, chunks, limit, input.FullContent, false), nil
	}

	logger := logutil.FromCtx(ctx)

	// Hybrid path: embed query, then run fused BM25+vector search
	queryEmbedding, err := s.embedQuery(ctx, *embedder, input.Query)
	if err != nil {
		logger.Warn("failed to embed query, falling back to BM25", "error", err)
		chunks, bm25Err := s.db.BM25ChunkSearch(ctx, input.Query, filter)
		if bm25Err != nil {
			return nil, fmt.Errorf("bm25 fallback after embed failure (%v): %w", err, bm25Err)
		}
		return assembleResults(ctx, chunks, limit, input.FullContent, true), nil
	}

	chunks, err := s.db.HybridSearch(ctx, input.Query, queryEmbedding, filter)
	if err != nil {
		logger.Warn("hybrid search failed, falling back to BM25", "error", err)
		chunks, bm25Err := s.db.BM25ChunkSearch(ctx, input.Query, filter)
		if bm25Err != nil {
			return nil, fmt.Errorf("bm25 fallback after hybrid failure (%v): %w", err, bm25Err)
		}
		return assembleResults(ctx, chunks, limit, input.FullContent, true), nil
	}

	return assembleResults(ctx, chunks, limit, input.FullContent, false), nil
}

// embedQuery returns the embedding for a query, using the cache when available.
func (s *Service) embedQuery(ctx context.Context, embedder llm.Embedder, query string) ([]float32, error) {
	logger := logutil.FromCtx(ctx)
	normalized := NormalizeQuery(query)

	cached, err := s.db.LookupQueryEmbedding(ctx, normalized)
	if err != nil {
		logger.Warn("embedding cache lookup failed", "error", err)
	}

	const cacheTimeout = 5 * time.Second

	if cached != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), cacheTimeout)
			defer cancel()
			if err := s.db.UpsertQueryEmbedding(bgCtx, normalized, cached.Embedding); err != nil {
				logger.Warn("failed to update embedding cache hit count", "error", err)
			}
		}()
		return cached.Embedding, nil
	}

	embedding, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), cacheTimeout)
		defer cancel()
		if err := s.db.UpsertQueryEmbedding(bgCtx, normalized, embedding); err != nil {
			logger.Warn("failed to cache query embedding", "error", err)
		}
	}()

	return embedding, nil
}

// HasSemanticSearch returns true if vector search is available.
func (s *Service) HasSemanticSearch() bool {
	return s.getEmbedder() != nil
}

// chunkToMatch converts a ChunkWithScore to a ChunkMatch for search results.
func chunkToMatch(ch db.ChunkWithScore, fullContent bool) ChunkMatch {
	snippet := ch.Content
	if !fullContent {
		snippet = truncateSnippet(ch.Content, maxSnippetLen)
	}
	return ChunkMatch{
		Snippet:     snippet,
		HeadingPath: ch.HeadingPath,
		Position:    ch.Position,
		Score:       ch.Score,
	}
}

// assembleResults groups chunks by parent document, sums chunk scores per document,
// and returns up to limit results. Document metadata comes from the query result
// (via record link traversal), not a separate fetch.
func assembleResults(ctx context.Context, chunks []db.ChunkWithScore, limit int, fullContent bool, degraded bool) []SearchResult {
	if len(chunks) == 0 {
		return []SearchResult{}
	}

	type docAgg struct {
		docID   string
		path    string
		title   string
		labels  []string
		docType *string
		score   float64
		chunks  []ChunkMatch
	}

	agg := make(map[string]*docAgg)
	order := make([]string, 0) // preserve encounter order for stable sort tie-breaking

	for _, ch := range chunks {
		docID, err := models.RecordIDString(ch.Document)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to extract chunk document ID", "chunk_id", ch.ID, "error", err)
			continue
		}
		if _, ok := agg[docID]; !ok {
			labels := ch.DocLabels
			if labels == nil {
				labels = []string{}
			}
			agg[docID] = &docAgg{
				docID:   docID,
				path:    ch.DocPath,
				title:   ch.DocTitle,
				labels:  labels,
				docType: ch.DocType,
			}
			order = append(order, docID)
		}
		a := agg[docID]
		a.score += ch.Score
		a.chunks = append(a.chunks, chunkToMatch(ch, fullContent))
	}

	sort.SliceStable(order, func(i, j int) bool {
		return agg[order[i]].score > agg[order[j]].score
	})

	if len(order) > limit {
		order = order[:limit]
	}

	results := make([]SearchResult, len(order))
	for i, docID := range order {
		a := agg[docID]
		results[i] = SearchResult{
			DocumentID:    a.docID,
			Path:          a.path,
			Title:         a.title,
			Labels:        a.labels,
			DocType:       a.docType,
			Score:         a.score,
			MatchedChunks: a.chunks,
			Degraded:      degraded,
		}
	}
	return results
}
