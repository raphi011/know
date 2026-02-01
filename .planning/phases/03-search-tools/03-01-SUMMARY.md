---
phase: 03
plan: 01
subsystem: search
tags: [surrealdb, hybrid-search, rrf, bm25, vector, mcp-tools]
dependency-graph:
  requires: [02-01, 02-02]
  provides: [search-tool, query-functions, context-detection]
  affects: [03-02, 04-01]
tech-stack:
  added: []
  patterns: [query-function-layer, hybrid-rrf-search, context-detection]
file-tracking:
  key-files:
    created:
      - internal/db/queries.go
      - internal/tools/search.go
      - internal/tools/context.go
      - internal/tools/search_test.go
    modified:
      - internal/tools/registry.go
      - cmd/memcp/main.go
      - internal/tools/tools_test.go
decisions:
  - id: search-query-layer
    choice: "Separate db/queries.go for all query functions"
    rationale: "Testability, separation of SQL from handlers"
  - id: context-detection-order
    choice: "explicit > config.DefaultContext > git origin > cwd"
    rationale: "Match Python behavior, allow override"
  - id: rrf-parameters
    choice: "k=60, vector limit=2x for variety"
    rationale: "Standard RRF constant, more vector candidates for diversity"
metrics:
  duration: 8m
  completed: 2026-02-01
---

# Phase 03 Plan 01: Search Tool Implementation Summary

Hybrid BM25 + vector search with RRF fusion using SurrealDB's search::rrf() function.

## What Was Built

### Query Functions Layer (internal/db/queries.go)

Five typed query functions on db.Client:

| Function | Purpose | Return |
|----------|---------|--------|
| QueryHybridSearch | RRF fusion of BM25 + HNSW vector | []models.Entity |
| QueryGetEntity | Retrieve by ID | *models.Entity |
| QueryUpdateAccess | Update accessed timestamp, count, decay_weight | error |
| QueryListLabels | Unique labels with counts | []LabelCount |
| QueryListTypes | Entity types with counts | []TypeCount |

Key patterns:
- Dynamic SQL building for optional filters (labels, context)
- Nil-safe result extraction from surrealdb.Query wrapper
- HNSW vector search with ef=40 for recall
- RRF k=60 standard fusion constant

### Context Detection (internal/tools/context.go)

`DetectContext(cfg *config.Config) *string`:
1. Explicit config.DefaultContext
2. If ContextFromCWD enabled: git origin > cwd basename
3. Returns nil if detection disabled

Helper: `extractID()` strips "entity:" prefix for bare IDs.

### Search Tool Handler (internal/tools/search.go)

`NewSearchHandler(deps, cfg)` handler factory:
- Input validation: non-empty query, limit 1-100
- Embedding generation via deps.Embedder
- Context detection with explicit override
- Hybrid search execution
- Access tracking for each result
- JSON response with SearchResult struct

### Registry Update

`RegisterAll` now accepts `*config.Config` for context-aware tools.

## Commits

| Hash | Description |
|------|-------------|
| 45ac2a7 | feat(03-01): create query functions layer |
| 4826698 | feat(03-01): implement search tool handler |
| 5ed6b49 | feat(03-01): update main and add search tests |

## Decisions Made

1. **Query function layer**: Separate db/queries.go keeps SQL isolated from handlers
2. **Context detection order**: explicit > config > git > cwd (matches Python)
3. **RRF parameters**: k=60 standard constant, 2x limit for vector diversity
4. **Access tracking**: Loop update per result (simple, can batch optimize later)

## Deviations from Plan

None - plan executed exactly as written.

## Test Coverage

- TestSearchToolRegistered: verifies search in tools/list
- TestSearchToolValidation: empty query, limit > 100 errors
- TestPingTool: updated for new RegisterAll signature

Integration tests require running SurrealDB + Ollama.

## Next Phase Readiness

**Ready for 03-02** (get_entity, list_labels, list_types tools):
- Query functions QueryGetEntity, QueryListLabels, QueryListTypes ready
- Handler factory pattern established
- Registry accepts config for all tools
