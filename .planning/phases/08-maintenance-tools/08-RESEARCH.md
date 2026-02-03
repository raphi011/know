# Phase 8: Maintenance Tools - Research

**Researched:** 2026-02-03
**Domain:** Database maintenance operations, similarity detection, unit testing
**Confidence:** HIGH (established codebase patterns, verified SurrealDB functions)

## Summary

Phase 8 implements a single `reflect` tool with decay and similar pairs detection, plus unit test coverage for query functions. The core functionality requires two new query functions (`QueryApplyDecay`, `QueryFindSimilarPairs`) that leverage existing SurrealDB patterns already established in the codebase.

The `reflect` tool follows the established handler factory pattern, using an `action` parameter to select between operations (matching the single-tool design decision from CONTEXT.md). Decay reduces `decay_weight` and `importance_score` for stale entities. Similar pairs detection uses `vector::similarity::cosine` with embedding-based KNN search, returning pairs above a user-configurable threshold.

**Primary recommendation:** Implement as single `reflect` tool with action enum (`decay`, `similar`), dry_run mode, and context scoping. Unit tests should focus on query functions using table-driven tests with testify suite for setup/teardown.

## Standard Stack

### Core (Already in Codebase)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| surrealdb.go | v3.0+ | SurrealDB client | Already used in db/client.go |
| testify | latest | Testing assertions, suite | Already used in *_test.go files |
| slog | stdlib | Structured logging | Already used in tools/deps.go |

### Supporting (Already Available)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| modelcontextprotocol/go-sdk/mcp | latest | MCP tool registration | Handler factory pattern |
| time | stdlib | Duration parsing for decay_days | Cutoff calculation |

**No new dependencies required.**

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── db/
│   └── queries.go        # Add QueryApplyDecay, QueryFindSimilarPairs
├── tools/
│   └── reflect.go        # Single tool handler with action dispatch
└── models/
    └── reflect.go        # ReflectResult, SimilarPair (new file)
```

### Pattern 1: Single Tool with Action Parameter
**What:** Unified `reflect` tool with `action` field for operation selection
**When to use:** Multiple related operations that share context scoping/dry-run semantics

**Example (from existing codebase pattern):**
```go
// ReflectInput defines the input schema for the reflect tool.
type ReflectInput struct {
    Action             string  `json:"action" jsonschema:"required,enum=decay|similar,The maintenance action to perform"`
    DryRun             bool    `json:"dry_run,omitempty" jsonschema:"Preview changes without applying (default: false)"`
    Context            string  `json:"context,omitempty" jsonschema:"Project namespace filter"`
    // Decay-specific
    DecayDays          int     `json:"decay_days,omitempty" jsonschema:"Days of inactivity before decay (default: 30)"`
    // Similar-specific
    SimilarityThreshold float64 `json:"similarity_threshold,omitempty" jsonschema:"Minimum similarity score 0-1 (default: 0.85)"`
    Limit              int     `json:"limit,omitempty" jsonschema:"Max similar pairs to return (default: 10)"`
}
```

### Pattern 2: Decay Query with Dual Score Reduction
**What:** Single UPDATE query reducing both decay_weight and importance_score
**When to use:** Apply temporal decay to unused entities

**Example (adapting Python pattern to Go):**
```go
// QueryApplyDecay reduces scores for old entities.
// Returns count of affected entities.
func (c *Client) QueryApplyDecay(
    ctx context.Context,
    cutoffDays int,
    contextFilter *string,
    dryRun bool,
) ([]DecayedEntity, error) {
    cutoff := time.Now().Add(-time.Duration(cutoffDays) * 24 * time.Hour)

    contextClause := ""
    vars := map[string]any{
        "cutoff": cutoff.Format(time.RFC3339),
    }
    if contextFilter != nil {
        contextClause = "AND context = $context"
        vars["context"] = *contextFilter
    }

    if dryRun {
        // Preview only - return entities that WOULD be decayed
        sql := fmt.Sprintf(`
            SELECT id, type, content, decay_weight, importance, accessed
            FROM entity
            WHERE accessed < <datetime>$cutoff
              AND decay_weight > 0.1
              %s
        `, contextClause)
        // ... execute and return preview
    }

    // Apply decay: multiply both scores by 0.9
    sql := fmt.Sprintf(`
        UPDATE entity SET
            decay_weight = decay_weight * 0.9,
            importance = importance * 0.9
        WHERE accessed < <datetime>$cutoff
          AND decay_weight > 0.1
          %s
        RETURN AFTER
    `, contextClause)
    // ... execute and return affected entities
}
```

### Pattern 3: Similar Pairs via Embedding KNN
**What:** Find similar entity pairs using cosine similarity
**When to use:** Identify potential duplicates for user review

**Example (SurrealDB cosine similarity - verified from official docs):**
```go
// QueryFindSimilarPairs finds entities with high embedding similarity.
// Uses vector::similarity::cosine with KNN search for efficiency.
func (c *Client) QueryFindSimilarPairs(
    ctx context.Context,
    threshold float64,
    limit int,
    contextFilter *string,
) ([]SimilarPair, error) {
    contextClause := ""
    vars := map[string]any{
        "threshold": threshold,
        "limit":     limit,
    }
    if contextFilter != nil {
        contextClause = "WHERE context = $context"
        vars["context"] = *contextFilter
    }

    // For each entity, find similar neighbors above threshold
    // Uses KNN operator <|n,40|> for efficient HNSW search
    sql := fmt.Sprintf(`
        SELECT
            id AS id1,
            content AS content1,
            (SELECT id, content,
                    vector::similarity::cosine(embedding, $parent.embedding) AS similarity
             FROM entity
             WHERE embedding <|10,40|> $parent.embedding
               AND id != $parent.id
               AND vector::similarity::cosine(embedding, $parent.embedding) >= $threshold
               %s
             ORDER BY similarity DESC
             LIMIT 5
            ) AS matches
        FROM entity %s
        LIMIT $limit
    `, contextClause, contextClause)
    // Post-process to deduplicate pairs (A,B) and (B,A)
}
```

### Pattern 4: Query Function Unit Tests
**What:** Integration tests with real SurrealDB, table-driven test cases
**When to use:** Testing query functions in db/queries.go

**Example (following existing test patterns):**
```go
//go:build integration

package db_test

func TestQueryApplyDecay(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    client := setupTestDB(t, ctx)
    defer client.Close(ctx)

    tests := []struct {
        name       string
        setup      func(t *testing.T)  // Create test entities
        cutoffDays int
        dryRun     bool
        wantCount  int
        wantFields map[string]any      // Expected field values
    }{
        {
            name:       "decays old entities",
            cutoffDays: 7,
            wantCount:  2,
        },
        {
            name:       "dry run returns preview",
            cutoffDays: 7,
            dryRun:     true,
            wantCount:  2,
        },
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup/teardown per test for isolation
            if tt.setup != nil {
                tt.setup(t)
            }
            // ... test execution
        })
    }
}
```

### Anti-Patterns to Avoid
- **Mocking SurrealDB queries:** The user decided integration tests with real SurrealDB. Don't use go-sqlmock or similar.
- **Global test fixtures:** Use per-test setup/teardown for isolation.
- **Auto-merge in similar pairs:** User decision is identify-only (safer, manual merge via forget+remember).
- **Multiple tools for maintenance:** Use single `reflect` tool with action parameter.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Cosine similarity | Manual math | `vector::similarity::cosine()` | SurrealDB built-in, GPU-accelerated |
| KNN search | Full table scan | `embedding <\|n,40\|>` | HNSW index, O(log n) |
| Time-based cutoff | Manual timestamp math | `time::now()` + duration | SurrealDB datetime functions |
| Pair deduplication | Complex set ops | Sort ID pair before comparison | Simple string comparison |

**Key insight:** SurrealDB provides all the vector operations needed. Don't implement custom similarity calculations in Go.

## Common Pitfalls

### Pitfall 1: Duplicate Pair Detection
**What goes wrong:** Finding (A,B) and (B,A) as separate pairs
**Why it happens:** KNN returns neighbors for each entity independently
**How to avoid:** Normalize pair keys by sorting IDs, use `seen_pairs` set
**Warning signs:** Duplicate pairs in output, double-counted statistics

```go
// Deduplication pattern (from Python reference)
id1, id2 := entity1.ID, entity2.ID
if id1 > id2 {
    id1, id2 = id2, id1
}
pairKey := id1 + ":" + id2
if _, seen := seenPairs[pairKey]; seen {
    continue
}
seenPairs[pairKey] = struct{}{}
```

### Pitfall 2: Decay Weight Minimum Threshold
**What goes wrong:** Entities decay to 0 and never recover
**Why it happens:** No floor on decay_weight
**How to avoid:** Stop decay at 0.1 (`WHERE decay_weight > 0.1`)
**Warning signs:** Entities with 0 weight that are never returned in search

### Pitfall 3: Test Isolation Failures
**What goes wrong:** Tests pass individually but fail when run together
**Why it happens:** Shared database state, test order dependence
**How to avoid:** Per-test setup/teardown with unique IDs or fresh namespace
**Warning signs:** Flaky tests, different results in `-count=1` vs `-count=10`

### Pitfall 4: Access Restoration Not Working
**What goes wrong:** Decayed entities stay decayed even after access
**Why it happens:** Missing auto-restore logic
**How to avoid:** QueryUpdateAccess already resets decay_weight to 1.0 (line 107 in queries.go)
**Warning signs:** Accessed entities still have low decay_weight

## Code Examples

Verified patterns from existing codebase:

### Handler Factory Pattern
```go
// Source: internal/tools/search.go
func NewReflectHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[ReflectInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input ReflectInput) (
        *mcp.CallToolResult, any, error,
    ) {
        switch input.Action {
        case "decay":
            return handleDecay(ctx, deps, cfg, input)
        case "similar":
            return handleSimilar(ctx, deps, cfg, input)
        default:
            return ErrorResult("Unknown action", "Use 'decay' or 'similar'"), nil, nil
        }
    }
}
```

### Context Detection Pattern
```go
// Source: internal/tools/search.go (lines 49-55)
var contextPtr *string
if input.Context != "" {
    contextPtr = &input.Context
} else {
    contextPtr = DetectContext(cfg)
}
```

### Query Result Extraction Pattern
```go
// Source: internal/db/queries.go (lines 71-80)
results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, vars)
if err != nil {
    return nil, fmt.Errorf("query: %w", err)
}
if results != nil && len(*results) > 0 {
    return (*results)[0].Result, nil
}
return []models.Entity{}, nil
```

### Test Setup Pattern
```go
// Source: internal/db/client_test.go (lines 35-50)
func TestQueryXxx(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    cfg := getTestConfig()
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
    client, err := db.NewClient(ctx, cfg, logger)
    require.NoError(t, err)
    defer client.Close(ctx)

    err = client.InitSchema(ctx)
    require.NoError(t, err)

    // ... test body
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Multiple tools | Single tool with action | Phase 8 design | Simpler interface |
| Auto-merge duplicates | Identify-only | User decision | Safer, manual control |
| Unit tests with mocks | Integration tests | User decision | Real query validation |

**Deprecated/outdated:**
- The Python `auto_merge` parameter exists but is disabled by default. Go version will not implement merge (identify-only per user decision).
- `recalculate_importance` action from Python is deferred (not in Phase 8 requirements).

## Open Questions

Things that couldn't be fully resolved:

1. **Similar pairs limit scope**
   - What we know: Python uses limit=10 for similar pairs
   - What's unclear: Is this per-entity or total pairs?
   - Recommendation: Interpret as total pairs (stop when found enough), more useful

2. **Global vs context scope default**
   - What we know: Other tools default to context-scoped
   - What's unclear: Should reflect maintenance be context-scoped by default?
   - Recommendation: Yes, context-scoped by default with `global: true` flag (matches other tools)

## Sources

### Primary (HIGH confidence)
- `/Users/raphaelgruber/Git/memcp/migrate-to-go/internal/db/queries.go` - Existing query patterns (lines 26-80, 102-113)
- `/Users/raphaelgruber/Git/memcp/migrate-to-go/internal/tools/search.go` - Handler factory pattern (lines 24-87)
- `/Users/raphaelgruber/Git/memcp/migrate-to-go/memcp/servers/maintenance.py` - Python reflect implementation (lines 26-127)
- `/Users/raphaelgruber/Git/memcp/migrate-to-go/memcp/db.py` - Python query_apply_decay (lines 649-655)
- [SurrealDB Vector Search](https://surrealdb.com/docs/surrealdb/models/vector) - Official vector::similarity::cosine documentation

### Secondary (MEDIUM confidence)
- [Three Dots Labs Database Testing](https://threedots.tech/post/database-integration-testing/) - Integration test principles
- [Testify Suite Package](https://pkg.go.dev/github.com/stretchr/testify/suite) - Setup/teardown patterns

### Tertiary (LOW confidence)
- None - all patterns verified against codebase or official docs

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All libraries already in use
- Architecture: HIGH - Follows established patterns exactly
- Pitfalls: HIGH - Documented from Python implementation and codebase patterns

**Research date:** 2026-02-03
**Valid until:** 30 days (stable patterns, no external dependencies)
