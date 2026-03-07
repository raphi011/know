# Search History Embedding Cache — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Cache query embeddings in SurrealDB to skip redundant ~250ms embedding API calls on repeated searches.

**Architecture:** New `search_query` table stores normalized query -> embedding mappings. The search service checks cache before calling the embedder. Cache writes are async (fire-and-forget). Cache reads are synchronous (~1-2ms indexed lookup).

**Tech Stack:** Go, SurrealDB, surrealdb.go driver

---

### Task 1: NormalizeQuery function + tests

**Files:**
- Create: `internal/search/normalize.go`
- Create: `internal/search/normalize_test.go`

**Step 1: Write the test**

```go
// internal/search/normalize_test.go
package search

import "testing"

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Claude Sandbox", "claude sandbox"},
		{"  claude   sandbox  ", "claude sandbox"},
		{" HELLO   World ", "hello world"},
		{"single", "single"},
		{"", ""},
		{"  ", ""},
		{"already normalized", "already normalized"},
		{"\t tabs \n newlines ", "tabs newlines"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeQuery(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `just test`
Expected: FAIL — `NormalizeQuery` not defined

**Step 3: Write implementation**

```go
// internal/search/normalize.go
package search

import "strings"

// NormalizeQuery normalizes a search query for embedding cache lookup.
// Lowercases, collapses whitespace, trims. "  Claude   Sandbox  " -> "claude sandbox".
func NormalizeQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(query), " "))
}
```

**Step 4: Run test to verify it passes**

Run: `just test`
Expected: PASS

**Step 5: Commit**

```
feat(search): add NormalizeQuery for embedding cache keys
```

---

### Task 2: SearchQuery model

**Files:**
- Create: `internal/models/search_query.go`

**Step 1: Write the model**

```go
// internal/models/search_query.go
package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type SearchQuery struct {
	ID              surrealmodels.RecordID `json:"id"`
	Query           string                 `json:"query"`
	Embedding       []float32              `json:"embedding"`
	HitCount        int                    `json:"hit_count"`
	FirstSearchedAt time.Time              `json:"first_searched_at"`
	LastSearchedAt  time.Time              `json:"last_searched_at"`
}
```

**Step 2: Build to verify compilation**

Run: `just build-all`
Expected: PASS

**Step 3: Commit**

```
feat(models): add SearchQuery model for embedding cache
```

---

### Task 3: DDL schema for search_query table

**Files:**
- Modify: `internal/db/schema.go`

**Step 1: Add the table DDL**

Add before the closing backtick/dimension format in `SchemaSQL()`, after the message cascade event (line 253):

```sql
    -- ==========================================================================
    -- SEARCH_QUERY TABLE (embedding cache)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS search_query SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS query             ON search_query TYPE string;
    DEFINE FIELD IF NOT EXISTS embedding         ON search_query TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS hit_count         ON search_query TYPE int DEFAULT 1;
    DEFINE FIELD IF NOT EXISTS first_searched_at ON search_query TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS last_searched_at  ON search_query TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_search_query_query ON search_query FIELDS query UNIQUE;
```

**Step 2: Build to verify**

Run: `just build-all`
Expected: PASS

**Step 3: Commit**

```
feat(db): add search_query table DDL for embedding cache
```

---

### Task 4: DB query functions for search_query

**Files:**
- Create: `internal/db/queries_search_query.go`

**Step 1: Write the query functions**

```go
// internal/db/queries_search_query.go
package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// LookupQueryEmbedding looks up a cached embedding for the given normalized query.
// Returns nil if not found.
func (c *Client) LookupQueryEmbedding(ctx context.Context, normalizedQuery string) (*models.SearchQuery, error) {
	start := c.startOp()
	defer c.recordTiming("db.lookup_query_embedding", start)

	sql := `SELECT * FROM search_query WHERE query = $query LIMIT 1`
	results, err := surrealdb.Query[[]models.SearchQuery](ctx, c.DB(), sql, map[string]any{
		"query": normalizedQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("lookup query embedding: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// UpsertQueryEmbedding inserts a new search query cache entry or updates hit_count
// and last_searched_at if the query already exists.
func (c *Client) UpsertQueryEmbedding(ctx context.Context, normalizedQuery string, embedding []float32) error {
	start := c.startOp()
	defer c.recordTiming("db.upsert_query_embedding", start)

	sql := `
		UPSERT search_query
		SET query = $query,
			embedding = $embedding,
			hit_count += 1,
			last_searched_at = time::now()
		WHERE query = $query
	`
	_, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"query":     normalizedQuery,
		"embedding": embedding,
	})
	if err != nil {
		return fmt.Errorf("upsert query embedding: %w", err)
	}
	return nil
}
```

**Step 2: Build to verify**

Run: `just build-all`
Expected: PASS

**Step 3: Commit**

```
feat(db): add LookupQueryEmbedding and UpsertQueryEmbedding
```

---

### Task 5: Integrate cache into search service

**Files:**
- Modify: `internal/search/service.go` (lines 100-110)

**Step 1: Update the embedding section of `Search()`**

Replace lines 100-110 (the embedder block) with cache-aware logic:

```go
	// If no embedder, aggregate BM25 chunks into results
	if s.embedder == nil {
		return chunksToResults(ctx, s.db, bm25Chunks, limit)
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
			return chunksToResults(ctx, s.db, bm25Chunks, limit)
		}
		// Store in cache (fire-and-forget)
		go func() {
			if err := s.db.UpsertQueryEmbedding(context.Background(), normalized, queryEmbedding); err != nil {
				slog.Warn("failed to cache query embedding", "error", err)
			}
		}()
	}
```

The rest of the function (vector search, RRF fusion) stays unchanged — it uses `queryEmbedding` which is now populated from either cache or API.

**Step 2: Build and test**

Run: `just build-all && just test`
Expected: PASS (existing tests don't touch the embedder path — they test RRF/aggregation only)

**Step 3: Commit**

```
feat(search): use embedding cache before calling embedder API
```

---

### Task 6: Manual integration test

**Step 1: Bootstrap and start server**

```bash
just bootstrap
just dev
```

**Step 2: First search (cache miss)**

Search for something in the agent chat. Note the duration in the tool card (should be ~250-300ms as before).

**Step 3: Second search (cache hit)**

Search for the exact same query. Duration should drop to ~30-50ms (DB queries only, no embedding API call).

**Step 4: Verify cache entry**

```bash
surreal sql --endpoint http://localhost:8000 --namespace knowhow --database knowhow
> SELECT query, hit_count, first_searched_at, last_searched_at FROM search_query;
```

Should show the query with `hit_count >= 2`.

**Step 5: Normalized duplicate**

Search for the same query with different casing/spacing. Should also be a cache hit (same duration as step 3).

---

## Files Summary

| File | Action |
|------|--------|
| `internal/search/normalize.go` | Create |
| `internal/search/normalize_test.go` | Create |
| `internal/models/search_query.go` | Create |
| `internal/db/schema.go` | Modify (add DDL) |
| `internal/db/queries_search_query.go` | Create |
| `internal/search/service.go` | Modify (cache integration) |
