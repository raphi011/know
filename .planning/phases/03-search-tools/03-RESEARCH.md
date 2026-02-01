# Phase 3: Search Tools - Research

**Researched:** 2026-02-01
**Domain:** SurrealDB hybrid search (BM25 + vector), MCP tool handlers, Go query patterns
**Confidence:** HIGH

## Summary

Phase 3 implements the four search tools that enable users to query the knowledge graph: `search` (hybrid BM25 + vector with RRF fusion), `get_entity` (by ID), `list_labels` (all labels with counts), and `list_types` (entity types with descriptions). These are read-only tools that follow the handler factory pattern established in Phase 2.

The core challenge is implementing SurrealDB's `search::rrf()` function for hybrid search, which combines BM25 full-text search with HNSW vector similarity using Reciprocal Rank Fusion. The Go SDK's generic `surrealdb.Query[T]()` function handles type-safe results. Each tool handler calls a corresponding `db.Query*` function that encapsulates the SurrealQL.

**Primary recommendation:** Create a `internal/db/queries.go` file with typed query functions (`QueryHybridSearch`, `QueryGetEntity`, `QueryListLabels`, `QueryListTypes`) that return domain models, then create tool handlers in `internal/tools/search.go` that wrap these queries with input validation and response formatting.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/surrealdb/surrealdb.go` | v1.2.0 | SurrealDB client with generic Query | Already installed, `Query[T]()` returns typed results |
| `github.com/modelcontextprotocol/go-sdk/mcp` | v1.2.0 | MCP tool handlers | Already installed, `AddTool[In,Out]` pattern established |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `encoding/json` | stdlib | Format search results | Serialize entities for TextContent |
| `strings` | stdlib | String manipulation | Query building, ID extraction |
| `fmt` | stdlib | Error formatting | Error messages with context |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Raw SQL strings | Query builder | Raw SQL is more explicit, matches Python implementation |
| JSON output | Markdown format | JSON is machine-readable, better for agent consumption |

**Already Installed:**
```bash
# No new dependencies needed - all from Phase 1/2
```

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── db/
│   ├── client.go       # (existing) Connection management
│   ├── schema.go       # (existing) Schema SQL
│   └── queries.go      # (NEW) Query functions for all tools
├── tools/
│   ├── deps.go         # (existing) Dependencies struct
│   ├── errors.go       # (existing) ErrorResult/TextResult helpers
│   ├── registry.go     # (existing, update) Add search tools
│   ├── ping.go         # (existing) Ping tool
│   └── search.go       # (NEW) Search tool handlers
└── models/
    └── entity.go       # (existing) Entity, SearchResult structs
```

### Pattern 1: Query Function Layer (db/queries.go)

**What:** Encapsulate SurrealQL queries in typed functions that return domain models.

**When to use:** Every database operation.

**Example:**
```go
// Source: Derived from Python memcp/db.py patterns
package db

import (
    "context"
    "fmt"

    "github.com/surrealdb/surrealdb.go"
    "github.com/raphaelgruber/memcp-go/internal/models"
)

// QueryHybridSearch performs RRF fusion of BM25 + vector search results.
// Returns entities ranked by combined relevance score.
func (c *Client) QueryHybridSearch(
    ctx context.Context,
    query string,
    embedding []float32,
    labels []string,
    limit int,
    context_ *string,
) ([]models.Entity, error) {
    // Build filter clauses
    labelFilter := ""
    if len(labels) > 0 {
        labelFilter = "AND labels CONTAINSANY $labels"
    }
    contextFilter := ""
    if context_ != nil {
        contextFilter = "AND context = $context"
    }

    sql := fmt.Sprintf(`
        SELECT * FROM search::rrf([
            (SELECT id, type, labels, content, confidence, source, decay_weight, context, importance, accessed, access_count
             FROM entity
             WHERE embedding <|%d,40|> $emb %s %s),
            (SELECT id, type, labels, content, confidence, source, decay_weight, context, importance, accessed, access_count
             FROM entity
             WHERE content @0@ $q %s %s)
        ], $limit, 60)
    `, limit*2, labelFilter, contextFilter, labelFilter, contextFilter)

    results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
        "q":       query,
        "emb":     embedding,
        "labels":  labels,
        "context": context_,
        "limit":   limit,
    })
    if err != nil {
        return nil, fmt.Errorf("hybrid search: %w", err)
    }

    // Extract from query result wrapper
    if results != nil && len(*results) > 0 {
        return (*results)[0].Result, nil
    }
    return []models.Entity{}, nil
}
```

### Pattern 2: Tool Handler with Embedding Generation

**What:** Tool handlers that generate embeddings before querying.

**When to use:** Search tool that needs vector similarity.

**Example:**
```go
// Source: Derived from official MCP SDK patterns
package tools

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchInput defines input schema for the search tool.
type SearchInput struct {
    Query   string   `json:"query" jsonschema:"required,The search query"`
    Labels  []string `json:"labels,omitempty" jsonschema:"Filter by labels"`
    Limit   int      `json:"limit,omitempty" jsonschema:"Max results (default 10, max 100)"`
    Context string   `json:"context,omitempty" jsonschema:"Project namespace filter"`
}

// NewSearchHandler creates the search tool handler.
func NewSearchHandler(deps *Dependencies) mcp.ToolHandlerFor[SearchInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (
        *mcp.CallToolResult, any, error,
    ) {
        // Input validation
        if input.Query == "" {
            return ErrorResult("Query cannot be empty", "Provide a search query"), nil, nil
        }

        // Set defaults
        limit := input.Limit
        if limit <= 0 {
            limit = 10
        }
        if limit > 100 {
            return ErrorResult("Limit must be 1-100", "Reduce limit value"), nil, nil
        }

        // Generate embedding for query
        embedding, err := deps.Embedder.Embed(ctx, input.Query)
        if err != nil {
            deps.Logger.Error("embedding failed", "error", err)
            return ErrorResult("Failed to generate query embedding", "Check Ollama connection"), nil, nil
        }

        // Detect context if not provided
        var contextPtr *string
        if input.Context != "" {
            contextPtr = &input.Context
        } else {
            // Context detection logic here (env var / git / cwd)
            detected := detectContext()
            if detected != "" {
                contextPtr = &detected
            }
        }

        // Execute hybrid search
        entities, err := deps.DB.QueryHybridSearch(ctx, input.Query, embedding, input.Labels, limit, contextPtr)
        if err != nil {
            deps.Logger.Error("search failed", "error", err)
            return ErrorResult("Search failed", "Database may be unavailable"), nil, nil
        }

        // Update access tracking for each result
        for _, e := range entities {
            _ = deps.DB.QueryUpdateAccess(ctx, extractID(e.ID))
        }

        // Format response as JSON
        result := models.SearchResult{
            Entities: entities,
            Count:    len(entities),
        }
        jsonBytes, _ := json.MarshalIndent(result, "", "  ")

        deps.Logger.Info("search completed", "query", truncate(input.Query, 30), "results", len(entities))
        return TextResult(string(jsonBytes)), nil, nil
    }
}
```

### Pattern 3: Simple Query Handler (no embedding)

**What:** Tool handlers for queries that don't need vector search.

**When to use:** get_entity, list_labels, list_types.

**Example:**
```go
// GetEntityInput for entity retrieval by ID.
type GetEntityInput struct {
    ID string `json:"id" jsonschema:"required,The entity ID to retrieve"`
}

// NewGetEntityHandler creates the get_entity tool handler.
func NewGetEntityHandler(deps *Dependencies) mcp.ToolHandlerFor[GetEntityInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input GetEntityInput) (
        *mcp.CallToolResult, any, error,
    ) {
        if input.ID == "" {
            return ErrorResult("ID cannot be empty", "Provide an entity ID"), nil, nil
        }

        // Extract bare ID from "entity:xxx" format if needed
        id := extractID(input.ID)

        entity, err := deps.DB.QueryGetEntity(ctx, id)
        if err != nil {
            deps.Logger.Error("get_entity failed", "id", id, "error", err)
            return ErrorResult("Failed to retrieve entity", "Check database connection"), nil, nil
        }

        if entity == nil {
            return ErrorResult(
                fmt.Sprintf("Entity not found: %s", id),
                "Try search first to find valid IDs",
            ), nil, nil
        }

        // Update access tracking
        _ = deps.DB.QueryUpdateAccess(ctx, id)

        // Return as JSON
        jsonBytes, _ := json.MarshalIndent(entity, "", "  ")
        return TextResult(string(jsonBytes)), nil, nil
    }
}
```

### Pattern 4: Aggregation Query

**What:** Queries that return counts or grouped data.

**When to use:** list_labels, list_types.

**Example:**
```go
// QueryListLabels returns all unique labels from entities.
func (c *Client) QueryListLabels(ctx context.Context) ([]string, error) {
    // Get all labels from entities
    results, err := surrealdb.Query[[]map[string]any](ctx, c.db,
        "SELECT labels FROM entity", nil)
    if err != nil {
        return nil, fmt.Errorf("list labels: %w", err)
    }

    // Flatten and deduplicate
    labelSet := make(map[string]struct{})
    if results != nil && len(*results) > 0 {
        for _, row := range (*results)[0].Result {
            if labels, ok := row["labels"].([]any); ok {
                for _, l := range labels {
                    if s, ok := l.(string); ok {
                        labelSet[s] = struct{}{}
                    }
                }
            }
        }
    }

    // Convert to sorted slice
    labels := make([]string, 0, len(labelSet))
    for l := range labelSet {
        labels = append(labels, l)
    }
    sort.Strings(labels)
    return labels, nil
}
```

### Anti-Patterns to Avoid

- **Inline SQL in handlers:** Put all SQL in db/queries.go for testability
- **Missing context propagation:** Always pass ctx to DB and Embedder calls
- **Ignoring access tracking:** Update accessed/access_count for retrieved entities
- **Hardcoded limits:** Use constants or config for default/max limits
- **Swallowing errors:** Log errors before returning ErrorResult

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Hybrid search ranking | Custom score normalization | `search::rrf()` | RRF handles incompatible score scales (BM25 vs cosine) |
| JSON schema generation | Manual input schema | `jsonschema` struct tags | SDK generates from tags automatically |
| Vector search | Cosine similarity calc | HNSW index `<\|k,ef\|>` | Database-level optimization, sub-linear search |
| ID extraction | Regex parsing | `strings.TrimPrefix` | RecordIDs are predictable "table:id" format |

**Key insight:** SurrealDB handles both BM25 and vector search natively with proper indexes. Don't implement scoring or ranking in Go - let the database do it.

## Common Pitfalls

### Pitfall 1: search::rrf Returns Empty on No Matches

**What goes wrong:** Query returns nil/empty when no BM25 or vector matches found.

**Why it happens:** RRF requires at least one result from one of the sub-queries.

**How to avoid:**
- Check for nil/empty results before processing
- Return empty SearchResult, not error
- Python handles this with `entities = entities or []`

**Warning signs:** Nil pointer panic when iterating results.

### Pitfall 2: RecordID Format Mismatch

**What goes wrong:** Query fails when using "entity:xxx" vs bare "xxx" ID.

**Why it happens:** SurrealDB `type::record()` expects bare ID, not full record ID string.

**How to avoid:**
- Create `extractID(id string) string` helper
- Handle both "entity:xxx" and "xxx" formats
- Use `strings.TrimPrefix(id, "entity:")`

**Warning signs:** "record not found" errors when ID looks correct.

### Pitfall 3: Vector Dimension Mismatch

**What goes wrong:** Vector search returns no results or errors.

**Why it happens:** Query embedding dimension doesn't match HNSW index (384-dim).

**How to avoid:**
- Use same embedding model for queries as inserts (all-minilm:l6-v2)
- Add dimension validation in Embedder interface
- Log embedding dimension in debug output

**Warning signs:** HNSW operator silently returns empty results.

### Pitfall 4: Context Filter Logic

**What goes wrong:** Search misses entities or returns wrong project's data.

**Why it happens:** Context filter applied incorrectly or nil pointer.

**How to avoid:**
- Use pointer for optional context: `*string` not `string`
- Only add "AND context = $context" when context is non-nil
- Test with both nil and non-nil context values

**Warning signs:** Cross-project data leakage or unexpectedly empty results.

### Pitfall 5: SurrealDB Query Result Wrapper

**What goes wrong:** Attempt to use results directly causes type errors.

**Why it happens:** `surrealdb.Query[T]()` returns `*[]QueryResult[T]`, not `[]T` directly.

**How to avoid:**
- Check `results != nil && len(*results) > 0`
- Access via `(*results)[0].Result`
- Handle multiple statement results if query has multiple SELECTs

**Warning signs:** Type assertion failures, nil pointer dereferences.

## Code Examples

Verified patterns for this phase.

### Complete Hybrid Search Query

```go
// Source: Derived from Python memcp/db.py:451 query_hybrid_search
func (c *Client) QueryHybridSearch(
    ctx context.Context,
    query string,
    embedding []float32,
    labels []string,
    limit int,
    contextFilter *string,
) ([]models.Entity, error) {
    // Build dynamic filter clauses
    labelClause := ""
    if len(labels) > 0 {
        labelClause = "AND labels CONTAINSANY $labels"
    }
    contextClause := ""
    if contextFilter != nil {
        contextClause = "AND context = $context"
    }

    // RRF fusion query - combines vector (2x limit for variety) with BM25
    sql := fmt.Sprintf(`
        SELECT * FROM search::rrf([
            (SELECT id, type, labels, content, confidence, source, decay_weight,
                    context, importance, accessed, access_count
             FROM entity
             WHERE embedding <|%d,40|> $emb %s %s),
            (SELECT id, type, labels, content, confidence, source, decay_weight,
                    context, importance, accessed, access_count
             FROM entity
             WHERE content @0@ $q %s %s)
        ], $limit, 60)
    `, limit*2, labelClause, contextClause, labelClause, contextClause)

    results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
        "q":       query,
        "emb":     embedding,
        "labels":  labels,
        "context": contextFilter,
        "limit":   limit,
    })
    if err != nil {
        return nil, fmt.Errorf("hybrid search: %w", err)
    }

    if results == nil || len(*results) == 0 {
        return []models.Entity{}, nil
    }
    return (*results)[0].Result, nil
}
```

### Get Entity by ID

```go
// Source: Derived from Python memcp/db.py:494 query_get_entity
func (c *Client) QueryGetEntity(ctx context.Context, id string) (*models.Entity, error) {
    results, err := surrealdb.Query[[]models.Entity](ctx, c.db, `
        SELECT * FROM type::record("entity", $id)
    `, map[string]any{"id": id})

    if err != nil {
        return nil, fmt.Errorf("get entity: %w", err)
    }

    if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
        return nil, nil
    }
    return &(*results)[0].Result[0], nil
}
```

### Update Access Tracking

```go
// Source: Derived from Python memcp/db.py:487 query_update_access
func (c *Client) QueryUpdateAccess(ctx context.Context, id string) error {
    _, err := surrealdb.Query[any](ctx, c.db, `
        UPDATE type::record("entity", $id) SET
            accessed = time::now(),
            access_count += 1,
            decay_weight = 1.0
    `, map[string]any{"id": id})
    return err
}
```

### List Types with Counts

```go
// Source: Derived from Python memcp/db.py:1096 query_list_types
func (c *Client) QueryListTypes(ctx context.Context, contextFilter *string) ([]TypeCount, error) {
    contextClause := ""
    vars := map[string]any{}
    if contextFilter != nil {
        contextClause = "WHERE context = $context"
        vars["context"] = *contextFilter
    }

    results, err := surrealdb.Query[[]TypeCount](ctx, c.db, fmt.Sprintf(`
        SELECT type, count() AS count FROM entity %s GROUP BY type
    `, contextClause), vars)

    if err != nil {
        return nil, fmt.Errorf("list types: %w", err)
    }

    if results == nil || len(*results) == 0 {
        return []TypeCount{}, nil
    }
    return (*results)[0].Result, nil
}

// TypeCount represents a type with its count.
type TypeCount struct {
    Type  string `json:"type"`
    Count int    `json:"count"`
}
```

### ID Extraction Helper

```go
// extractID removes "entity:" prefix if present.
func extractID(id string) string {
    return strings.TrimPrefix(id, "entity:")
}
```

### Context Detection

```go
// detectContext determines project context from environment or git.
func detectContext(cfg *config.Config) *string {
    // 1. Check explicit default context from config
    if cfg.DefaultContext != "" {
        return &cfg.DefaultContext
    }

    // 2. Check if context detection from CWD is enabled
    if !cfg.ContextFromCWD {
        return nil
    }

    // 3. Try git remote origin
    if origin := getGitOriginName(); origin != "" {
        return &origin
    }

    // 4. Fall back to CWD basename
    if cwd, err := os.Getwd(); err == nil {
        base := filepath.Base(cwd)
        return &base
    }

    return nil
}

func getGitOriginName() string {
    cmd := exec.Command("git", "config", "--get", "remote.origin.url")
    output, err := cmd.Output()
    if err != nil {
        return ""
    }
    url := strings.TrimSpace(string(output))

    // Parse git@github.com:owner/repo.git or https://github.com/owner/repo.git
    // ... (implementation matches Python detect_context)
    return ""
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Separate vector + BM25 queries | `search::rrf()` built-in | SurrealDB 2.0 | Single query, optimal ranking |
| `SELECT DISTINCT labels` | `GROUP BY` for unique values | SurrealDB v3.0 | DISTINCT deprecated |
| MTREE vector index | HNSW vector index | SurrealDB 2.0+ | Better recall, faster search |

**Deprecated/outdated:**
- `DISTINCT` keyword: Use `GROUP BY` for unique values in v3.0
- `SELECT count() FROM ... GROUP ALL`: Works but prefer explicit column list

## Open Questions

1. **Empty embedding handling**
   - What we know: Empty query text returns embedding, but meaningless
   - What's unclear: Should we reject empty queries or let them through?
   - Recommendation: Validate non-empty query in handler before embedding

2. **Access tracking batching**
   - What we know: Python updates access for each result in loop
   - What's unclear: Performance impact of N queries vs batch update
   - Recommendation: Start with loop (simpler), optimize if needed

3. **Label count aggregation**
   - What we know: Python returns flat list, not counts
   - What's unclear: SRCH-03 says "list labels with counts" - do we add counts?
   - Recommendation: Add counts like list_types does - more useful for UX

## Sources

### Primary (HIGH confidence)
- [SurrealDB Vector Search Reference](https://surrealdb.com/docs/surrealdb/reference-guide/vector-search) - HNSW, search::rrf() syntax
- [SurrealDB Go SDK Query Method](https://surrealdb.com/docs/sdk/golang/methods/query) - Generic Query[T]() pattern
- Python memcp/db.py (lines 451-523) - Verified query patterns

### Secondary (MEDIUM confidence)
- [SurrealDB Hybrid Search Blog](https://surrealdb.com/blog/hybrid-vector-text-search-in-the-terminal-with-surrealdb-and-ratatui) - RRF implementation example
- Existing Go codebase (internal/db/client.go) - Query wrapper pattern

### Tertiary (LOW confidence)
- WebSearch results on hybrid search best practices

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All libraries already installed, APIs verified
- Architecture: HIGH - Follows established Phase 2 patterns
- Query patterns: HIGH - Verified against Python implementation and SurrealDB docs
- Pitfalls: MEDIUM - Based on Python experience, some Go-specific gaps

**Research date:** 2026-02-01
**Valid until:** 2026-03-01 (30 days - stable patterns)
