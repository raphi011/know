---
phase: 07-procedure-tools
plan: 02
subsystem: api
tags: [procedure, search, hybrid-search, bm25, vector, rrf, mcp-tools, surrealdb]

# Dependency graph
requires:
  - phase: 07-procedure-tools-01
    provides: Procedure CRUD queries and handlers
provides:
  - Hybrid BM25+vector search for procedures (QuerySearchProcedures)
  - Procedure listing with context filter (QueryListProcedures)
  - search_procedures tool with label/context filtering
  - list_procedures tool for procedure enumeration
affects: [08-polish]

# Tech tracking
tech-stack:
  added: []
  patterns: [procedure-search-rrf, procedure-summary-type]

key-files:
  created: []
  modified:
    - internal/db/queries.go
    - internal/tools/procedure.go
    - internal/tools/registry.go

key-decisions:
  - "Search returns ProcedureSummary (no full steps) for efficiency"
  - "Search default limit 10, max 50; list default 50, max 100"
  - "Fire-and-forget access tracking for search results"
  - "BM25 searches name (index 0) and description (index 1)"

patterns-established:
  - "ProcedureSummary type for lightweight search/list results"
  - "Same hybrid RRF pattern as entity and episode search"

# Metrics
duration: 5min
completed: 2026-02-03
---

# Phase 7 Plan 2: Procedure Search Summary

**Hybrid BM25+vector search for procedures with label/context filtering and RRF fusion**

## Performance

- **Duration:** 5 min
- **Started:** 2026-02-03T11:58:39Z
- **Completed:** 2026-02-03T12:03:30Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- Added 2 procedure search query functions to db/queries.go
- Added search_procedures and list_procedures handlers with input/result types
- Registered 2 new tools bringing total to 18
- Procedure discovery complete with semantic search and filtering

## Task Commits

Each task was committed atomically:

1. **Task 1: Add QuerySearchProcedures and QueryListProcedures** - `085bdd4` (feat)
2. **Task 2: Add search_procedures and list_procedures handlers** - `5ba2378` (feat)
3. **Task 3: Register search_procedures and list_procedures tools** - `a2e2fe8` (feat)

## Files Created/Modified
- `internal/db/queries.go` - Added QuerySearchProcedures (hybrid BM25+vector with RRF), QueryListProcedures (context filter, access order)
- `internal/tools/procedure.go` - Added SearchProceduresInput, ListProceduresInput, ProcedureSummary, ProcedureSearchResult types and handlers
- `internal/tools/registry.go` - Registered search_procedures, list_procedures

## Decisions Made
- Search returns ProcedureSummary (id, name, description, step_count, labels, context) for efficiency; use get_procedure for full steps
- Search default limit 10, max 50; list default limit 50, max 100
- Fire-and-forget access tracking for search results (same pattern as episode search)
- BM25 searches name (analyzer 0) and description (analyzer 1) fields

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All procedure tools complete (create, get, delete, search, list)
- 18 MCP tools total registered
- Phase 7 complete, ready for Phase 8 (polish)

---
*Phase: 07-procedure-tools*
*Completed: 2026-02-03*
