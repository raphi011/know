# Search History — Embedding Cache

## Problem

Every knowledge base search calls the embedding API to vectorize the query (~250ms). With only 2 documents in the DB, the API call accounts for ~90% of total search latency. Repeated and similar queries pay this cost every time.

## Solution

A `search_query` table in SurrealDB that caches query embeddings. On search, look up the normalized query first — cache hit skips the embedding API entirely.

## Schema

```sql
DEFINE TABLE IF NOT EXISTS search_query SCHEMAFULL;

DEFINE FIELD IF NOT EXISTS query            ON search_query TYPE string;
DEFINE FIELD IF NOT EXISTS embedding        ON search_query TYPE array<float>;
DEFINE FIELD IF NOT EXISTS hit_count        ON search_query TYPE int DEFAULT 1;
DEFINE FIELD IF NOT EXISTS first_searched_at ON search_query TYPE datetime DEFAULT time::now();
DEFINE FIELD IF NOT EXISTS last_searched_at  ON search_query TYPE datetime DEFAULT time::now();

DEFINE INDEX IF NOT EXISTS idx_search_query_query ON search_query FIELDS query UNIQUE;
```

- **Not vault-scoped** — embeddings are model-specific, not vault-specific. Same query produces same vector regardless of vault.
- **No TTL/eviction** — embeddings are valid as long as the model doesn't change. Model changes require re-embedding all documents anyway (full wipe).
- **No model name stored** — model changes are a migration event, not a runtime concern.

## Query Normalization

Cache key = `strings.ToLower(strings.Join(strings.Fields(query), " "))`

This collapses whitespace and lowercases: `"  Claude   Sandbox "` -> `"claude sandbox"`.

No stemming or ascii folding — different strings produce different embeddings from the API.

## Search Flow (updated)

```
Search(query)
  |
  +-- normalize(query)
  +-- DB lookup: SELECT * FROM search_query WHERE query = $normalized
  |
  +-- [CACHE HIT]
  |     +-- use cached embedding for vector search
  |     +-- async: UPDATE hit_count += 1, last_searched_at = now()
  |
  +-- [CACHE MISS]
        +-- call embedder.Embed(query)  (~250ms)
        +-- async: INSERT search_query { query, embedding, ... }
        +-- use fresh embedding for vector search
```

Cache write/update is fire-and-forget (goroutine) — doesn't add latency to search response. Cache read is synchronous (~1-2ms indexed lookup).

## Files to Change

| File | Change |
|------|--------|
| `internal/db/schema.go` | Add `search_query` table DDL |
| `internal/db/queries_search_history.go` | New file: `LookupQueryEmbedding`, `UpsertQueryEmbedding` |
| `internal/models/search_query.go` | New file: `SearchQuery` struct |
| `internal/search/service.go` | Check cache before calling embedder; write cache after |
| `internal/search/normalize.go` | New file: `NormalizeQuery()` function |

## Not In Scope

- Search analytics UI (popular queries, trends)
- GraphQL API for search history browsing
- Result caching (only embedding caching)
- Semantic deduplication (only exact normalized match)
