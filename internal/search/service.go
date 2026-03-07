package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"unicode/utf8"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"

	"github.com/raphi011/knowhow/internal/llm"
)

// Service provides search with graceful degradation.
// Base: BM25 fulltext on chunks (always). Semantic: chunk vector search (if embedder configured).
type Service struct {
	db       *db.Client
	embedder *llm.Embedder // optional
}

// NewService creates a new search service.
func NewService(db *db.Client, embedder *llm.Embedder) *Service {
	return &Service{db: db, embedder: embedder}
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
// With embedder: hybrid search (BM25 chunks + vector chunks, RRF fusion).
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

	// Always run BM25 on chunks
	bm25Chunks, err := s.db.BM25ChunkSearch(ctx, input.Query, filter)
	if err != nil {
		return nil, fmt.Errorf("bm25 chunk search: %w", err)
	}

	// If no embedder, aggregate BM25 chunks into results
	if s.embedder == nil {
		return chunksToResults(ctx, s.db, bm25Chunks, limit, input.FullContent)
	}

	// Try embedding cache first
	normalized := NormalizeQuery(input.Query)
	var queryEmbedding []float32

	cached, err := s.db.LookupQueryEmbedding(ctx, normalized)
	if err != nil {
		slog.Warn("embedding cache lookup failed", "error", err)
	}

	if cached != nil {
		// Cache hit — use stored embedding
		queryEmbedding = cached.Embedding
		go func() {
			if err := s.db.UpsertQueryEmbedding(context.Background(), normalized, queryEmbedding); err != nil {
				slog.Warn("failed to update embedding cache hit count", "error", err)
			}
		}()
	} else {
		// Cache miss — call embedder
		queryEmbedding, err = s.embedder.Embed(ctx, input.Query)
		if err != nil {
			slog.Warn("failed to embed query, falling back to BM25", "error", err)
			return chunksToResults(ctx, s.db, bm25Chunks, limit, input.FullContent)
		}
		// Store in cache (fire-and-forget)
		go func() {
			if err := s.db.UpsertQueryEmbedding(context.Background(), normalized, queryEmbedding); err != nil {
				slog.Warn("failed to cache query embedding", "error", err)
			}
		}()
	}

	// Search chunks for semantic matches
	vectorChunks, err := s.db.ChunkVectorSearch(ctx, queryEmbedding, filter)
	if err != nil {
		slog.Warn("chunk vector search failed, falling back to BM25", "error", err)
		return chunksToResults(ctx, s.db, bm25Chunks, limit, input.FullContent)
	}

	// Collect unique doc IDs from both chunk lists
	docIDs := collectDocIDs(bm25Chunks, vectorChunks)

	// Batch fetch parent documents
	docMap, err := fetchDocMap(ctx, s.db, docIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch parent documents: %w", err)
	}

	return rrfFusion(bm25Chunks, vectorChunks, docMap, limit, input.FullContent), nil
}

// HasSemanticSearch returns true if vector search is available.
func (s *Service) HasSemanticSearch() bool {
	return s.embedder != nil
}

// collectDocIDs returns deduplicated document IDs from multiple chunk slices.
func collectDocIDs(chunkSets ...[]db.ChunkWithScore) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, chunks := range chunkSets {
		for _, ch := range chunks {
			docID, err := models.RecordIDString(ch.Document)
			if err != nil {
				slog.Warn("failed to extract chunk document ID in collectDocIDs", "error", err)
				continue
			}
			if seen[docID] {
				continue
			}
			seen[docID] = true
			ids = append(ids, docID)
		}
	}
	return ids
}

// buildDocMap converts a document slice into a map keyed by string ID.
func buildDocMap(docs []models.Document) map[string]models.Document {
	m := make(map[string]models.Document, len(docs))
	for _, d := range docs {
		id, err := models.RecordIDString(d.ID)
		if err != nil {
			slog.Warn("failed to extract document ID in buildDocMap", "error", err)
			continue
		}
		m[id] = d
	}
	return m
}

// fetchDocMap fetches documents by ID and returns them as a map keyed by string ID.
func fetchDocMap(ctx context.Context, dbClient *db.Client, docIDs []string) (map[string]models.Document, error) {
	if len(docIDs) == 0 {
		return map[string]models.Document{}, nil
	}
	docs, err := dbClient.GetDocumentsByIDs(ctx, docIDs)
	if err != nil {
		return nil, err
	}
	return buildDocMap(docs), nil
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

// chunksToResults aggregates chunks by parent document and returns search results.
// Used when only one search path is available (BM25-only fallback).
func chunksToResults(ctx context.Context, dbClient *db.Client, chunks []db.ChunkWithScore, limit int, fullContent bool) ([]SearchResult, error) {
	if len(chunks) == 0 {
		return []SearchResult{}, nil
	}

	docIDs := collectDocIDs(chunks)
	docMap, err := fetchDocMap(ctx, dbClient, docIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch parent documents: %w", err)
	}

	return aggregateChunks(chunks, docMap, limit, fullContent), nil
}

// aggregateChunks groups chunks by parent document, ranks by best chunk score,
// and returns up to limit results. Each document's score is the max of its chunk scores.
func aggregateChunks(chunks []db.ChunkWithScore, docMap map[string]models.Document, limit int, fullContent bool) []SearchResult {
	type docAgg struct {
		info   docInfo
		score  float64
		chunks []ChunkMatch
	}
	agg := make(map[string]*docAgg)
	order := make([]string, 0) // preserve encounter order for stable sort tie-breaking

	for _, ch := range chunks {
		docID, err := models.RecordIDString(ch.Document)
		if err != nil {
			slog.Warn("failed to extract chunk document ID in aggregateChunks", "error", err)
			continue
		}
		if _, ok := agg[docID]; !ok {
			doc, found := docMap[docID]
			if !found {
				slog.Warn("chunk parent document not found in docMap", "chunk_doc_id", docID)
				continue
			}
			agg[docID] = &docAgg{info: docInfoFromModel(doc)}
			order = append(order, docID)
		}
		a := agg[docID]
		if ch.Score > a.score {
			a.score = ch.Score
		}
		a.chunks = append(a.chunks, chunkToMatch(ch, fullContent))
	}

	// Sort by best chunk score, stable by encounter order
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
			DocumentID:    a.info.id,
			Path:          a.info.path,
			Title:         a.info.title,
			Labels:        a.info.labels,
			DocType:       a.info.docType,
			Score:         a.score,
			MatchedChunks: a.chunks,
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

// chunkKey returns a unique identifier for deduplication of chunks across search paths.
func chunkKey(ch db.ChunkWithScore) string {
	id, err := models.RecordIDString(ch.ID)
	if err != nil {
		// Fall back to position-based key for dedup
		docID, _ := models.RecordIDString(ch.Document)
		return fmt.Sprintf("%s-%d", docID, ch.Position)
	}
	return id
}

// rrfFusion combines BM25 chunk and vector chunk results using Reciprocal Rank Fusion.
// docMap provides full document data for all chunk parents.
func rrfFusion(bm25Chunks []db.ChunkWithScore, vectorChunks []db.ChunkWithScore, docMap map[string]models.Document, limit int, fullContent bool) []SearchResult {
	const k = 60.0 // RRF constant

	type docScore struct {
		info   docInfo
		score  float64
		chunks []ChunkMatch
	}

	scores := make(map[string]*docScore)
	seenChunks := make(map[string]map[string]bool) // docID → set of chunk keys

	addChunks := func(chunks []db.ChunkWithScore, startRank int) {
		for rank, ch := range chunks {
			docID, err := models.RecordIDString(ch.Document)
			if err != nil {
				slog.Warn("failed to extract chunk document ID", "error", err)
				continue
			}
			if _, ok := scores[docID]; !ok {
				doc, found := docMap[docID]
				if !found {
					slog.Warn("chunk parent document not found in docMap", "chunk_doc_id", docID)
					continue
				}
				scores[docID] = &docScore{info: docInfoFromModel(doc)}
				seenChunks[docID] = make(map[string]bool)
			}
			// Always contribute to RRF score even if chunk is a duplicate
			scores[docID].score += 1.0 / (k + float64(startRank+rank+1))

			// Only add to MatchedChunks if not already seen (dedup across BM25/vector)
			ck := chunkKey(ch)
			if !seenChunks[docID][ck] {
				seenChunks[docID][ck] = true
				scores[docID].chunks = append(scores[docID].chunks, chunkToMatch(ch, fullContent))
			}
		}
	}

	addChunks(bm25Chunks, 0)
	addChunks(vectorChunks, 0)

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
			DocumentID:    ds.info.id,
			Path:          ds.info.path,
			Title:         ds.info.title,
			Labels:        ds.info.labels,
			DocType:       ds.info.docType,
			Score:         ds.score,
			MatchedChunks: ds.chunks,
		}
	}
	return results
}
