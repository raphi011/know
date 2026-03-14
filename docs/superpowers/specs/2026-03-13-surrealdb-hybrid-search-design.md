# Phase 6: SurrealDB Hybrid Search (`search::rrf`)

## Goal

Replace Go-side RRF fusion with SurrealDB's built-in `search::rrf()`. Reduces three DB round-trips + Go fusion to a single query. Deletes ~200 lines of Go code, simplifies `Search()` from ~85 to ~40 lines.

## Context

Current hybrid search flow:
1. `BM25ChunkSearch` → SurrealDB (fulltext on chunks)
2. Embed query → embedder API (with cache)
3. `ChunkVectorSearch` → SurrealDB (HNSW cosine on chunk embeddings)
4. `rrfFusion()` → Go (RRF with k=60, chunk dedup, doc aggregation)
5. `fetchDocMap()` → SurrealDB (batch fetch parent docs by ID)
6. Assemble results → Go

SurrealDB v3.0 provides `search::rrf(lists, limit, k?)` which performs the same fusion at the DB level. Default k=60 matches our current constant.

## Design

### DB Layer

**New method**: `HybridSearch(ctx, query, embedding, filter) ([]HybridChunkResult, error)`

Single SurrealQL query using `search::rrf()` with inlined subqueries.

**Gotchas discovered during implementation**:
- `LET` variables return `None` when passed to `search::rrf()` — subqueries must be inlined
- `search::score()` cannot be used inside parenthesized subqueries within `search::rrf()` (parse error) — omit `ORDER BY` since BM25 `@1@` and KNN `<|K,EF|>` return results in relevance order implicitly

```sql
SELECT *,
    document.path AS doc_path,
    document.title AS doc_title,
    document.labels AS doc_labels,
    document.doc_type AS doc_type
FROM search::rrf([
    (SELECT * FROM chunk
     WHERE <filter conditions>
       AND content @1@ $query
     ORDER BY search::score(1) DESC
     LIMIT $limit),
    (SELECT * FROM chunk
     WHERE <filter conditions>
       AND embedding <|$limit,40|> $embedding
     ORDER BY vector::similarity::cosine(embedding, $embedding) DESC
     LIMIT $limit)
], $limit, 60)
```

Filter conditions reuse `buildChunkFilterConditions()`.

**New struct**: `HybridChunkResult` extends `ChunkWithScore` with parent doc metadata:

```go
type HybridChunkResult struct {
    models.Chunk
    Score     float64  `json:"score"`
    DocPath   string   `json:"doc_path"`
    DocTitle  string   `json:"doc_title"`
    DocLabels []string `json:"doc_labels"`
    DocType   *string  `json:"doc_type"`
}
```

Document ID is extracted from the embedded `Chunk.Document` field via `models.RecordIDString()` in `assembleFromHybrid`.

**Deleted**: `ChunkVectorSearch` — no longer called from anywhere.

**Kept**:
- `BM25ChunkSearch` — updated to return `[]HybridChunkResult` with doc fields via record link traversal (see Option B below). Renamed to reflect new return type.
- `GetDocumentsByIDs` — still used by `document/service.go:fetchDocumentTitles` for ingestion. Not touched in this phase.

### Search Service

**Simplified `Search()` flow** (~40 lines):

```
1. Normalize limit, build filter
2. If no embedder or BM25Only:
     → BM25Search → assembleFromHybrid, done
3. Embed query (with cache — unchanged)
4. If embedding fails:
     → BM25Search → assembleFromHybrid, mark Degraded
5. HybridSearch(query, embedding, filter) → []HybridChunkResult
6. If HybridSearch fails:
     → BM25Search → assembleFromHybrid, mark Degraded
7. assembleFromHybrid(results) → []SearchResult
```

Both paths (hybrid and BM25-only) flow through `assembleFromHybrid` since both return `[]HybridChunkResult`.

**Deleted functions** (~200 lines):
- `rrfFusion()` — replaced by `search::rrf()` in DB
- `collectDocIDs()` — no longer needed (doc metadata in query result)
- `fetchDocMap()` — no longer needed
- `buildDocMap()` — no longer needed
- `aggregateChunks()` — replaced by `assembleFromHybrid()`
- `chunksToResults()` — replaced by unified path through `assembleFromHybrid()`
- `chunkKey()` — dedup handled by DB-level RRF
- `docInfo` struct + `docInfoFromModel()` — replaced by `HybridChunkResult` fields

**New function**: `assembleFromHybrid([]HybridChunkResult, limit, fullContent) []SearchResult`
- Groups chunks by document (using `models.RecordIDString(chunk.Document)`)
- Builds `SearchResult` with `MatchedChunks` per document
- Doc score = **sum** of chunk RRF scores (preserves the current behavior where documents appearing in both BM25 and vector get a boost from both signals)
- Applies `truncateSnippet` for non-fullContent mode
- Does NOT set `Degraded` — callers handle that for fallback paths

**Chunk dedup**: `search::rrf()` deduplicates across input lists (same chunk appearing in both BM25 and vector gets a boosted fused score, not two entries). `assembleFromHybrid` does not need its own dedup logic. If during implementation we discover `search::rrf()` does NOT dedup, add a dedup step keyed by chunk ID (same as current `chunkKey` logic).

**Kept unchanged**:
- `truncateSnippet()`, `chunkToMatch()` — still needed
- Embedding cache logic (normalize, lookup, embed, store)
- `SearchInput`, `SearchResult`, `ChunkMatch` structs — public API unchanged

### Graceful Degradation

Same behavior as today:
- **No embedder**: BM25-only via `BM25Search`
- **BM25Only flag**: BM25-only via `BM25Search`
- **Embedding fails**: fall back to BM25-only, `Degraded: true`
- **HybridSearch fails**: fall back to BM25-only, `Degraded: true`

### BM25-only Fallback Path

**Option B (chosen)**: Update `BM25ChunkSearch` to include doc fields via record link traversal, returning `[]HybridChunkResult`. This unifies both paths through `assembleFromHybrid` and eliminates `chunksToResults`, `fetchDocMap`, and `buildDocMap` entirely.

### Tests

**Deleted**:
- All `TestRRFFusion_*` tests — logic moved to DB level
- All `TestAggregateChunks_*` tests — `aggregateChunks` deleted
- `TestChunkVectorSearch` in `queries_search_test.go` — `ChunkVectorSearch` deleted

**Kept**:
- `TestTruncateSnippet_*` tests — utility function unchanged
- `TestChunkToMatch_*` tests — utility function unchanged
- `TestBM25ChunkSearch` in `queries_search_test.go` — updated for new return type

**New** — unit tests for `assembleFromHybrid()` covering:
- Multiple chunks per document (grouping)
- Limit enforcement
- Empty input
- Stable ordering for equal scores
- HeadingPath preservation
- Full content vs truncated snippets
- Document score is sum of chunk scores

**New** — integration test for `HybridSearch` DB method (requires running SurrealDB).

## Key Files

| File | Change |
|------|--------|
| `internal/db/queries_search.go` | New `HybridSearch`, update `BM25ChunkSearch` to return `HybridChunkResult` with doc fields, delete `ChunkVectorSearch` |
| `internal/search/service.go` | Delete fusion code (~200 lines), simplify `Search()`, new `assembleFromHybrid()` |
| `internal/search/service_test.go` | Delete RRF/aggregate tests, add `assembleFromHybrid` tests |
| `internal/db/queries_search_test.go` | Delete `ChunkVectorSearch` test, update BM25 test for new return type |

## Effort: S

Deletion-heavy. ~200 lines removed, ~80 lines added (new DB method + assembly function + tests).
