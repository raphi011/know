---
phase: 04-persistence-tools
plan: 01
subsystem: database
tags: [surrealdb, upsert, embedding, mcp-tool]

# Dependency graph
requires:
  - phase: 03-search-tools
    provides: Query function pattern, context detection, handler factory
provides:
  - QueryUpsertEntity function with additive label merge
  - remember tool for entity storage with auto-generated embeddings
  - Composite ID generation (context:slugified-name)
affects: [04-02 (relations), 04-03 (forget), 05 (context)]

# Tech tracking
tech-stack:
  added: []
  patterns: [upsert-with-label-merge, composite-id-generation, wasCreated-indicator]

key-files:
  created:
    - internal/tools/remember.go
    - internal/tools/remember_test.go
  modified:
    - internal/db/queries.go
    - internal/tools/registry.go
    - internal/tools/search_test.go
    - internal/tools/tools_test.go

key-decisions:
  - "Use array::union for additive label merge in SQL (atomic, no race conditions)"
  - "Pre-check existence to return wasCreated indicator for action field"
  - "Schema validation at SDK level validates required fields before handler"
  - "Default entity type to 'concept', confidence to 1.0"

patterns-established:
  - "Composite ID: context:slugified-name for entity uniqueness"
  - "Upsert wasCreated pattern: check existence, then upsert, return indicator"
  - "EntityResult response type (excludes embedding from response)"

# Metrics
duration: 5min
completed: 2026-02-02
---

# Phase 04 Plan 01: Remember Tool Summary

**Entity upsert with auto-generated embeddings via QueryUpsertEntity and remember MCP tool**

## Performance

- **Duration:** 5 min
- **Started:** 2026-02-02T07:23:32Z
- **Completed:** 2026-02-02T07:28:28Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- QueryUpsertEntity function with additive label merge via array::union
- remember tool accepts entities array with name+content, generates embeddings
- Composite ID generation: context:slugified-name for context-scoped uniqueness
- Response includes created/updated counts without exposing embeddings

## Task Commits

Each task was committed atomically:

1. **Task 1: Add QueryUpsertEntity function** - `b6577b1` (feat)
2. **Task 2: Implement remember handler** - `04ee8db` (feat)
3. **Task 3: Add tests for remember tool** - `9b54f88` (test)

## Files Created/Modified
- `internal/db/queries.go` - Added QueryUpsertEntity with label merge and wasCreated indicator
- `internal/tools/remember.go` - RememberInput, EntityInput, handler with embedding generation
- `internal/tools/registry.go` - Registered remember tool
- `internal/tools/remember_test.go` - Registration and validation tests
- `internal/tools/search_test.go` - Updated tool count expectation (5->6)
- `internal/tools/tools_test.go` - Updated tool count expectation (5->6)

## Decisions Made
- **Label merge in SQL:** Use `array::union(labels ?? [], $labels)` for atomic additive merge
- **Existence check pattern:** Pre-check existence to determine created vs updated (SurrealDB UPSERT doesn't indicate action)
- **Default values:** Entity type defaults to "concept", confidence defaults to 1.0
- **Schema validation:** SDK validates required fields (name, content) before handler is called

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- QueryUpsertEntity ready for relation creation (04-02)
- remember tool registered and functional
- Next: Relations in 04-02, forget tool in 04-03

---
*Phase: 04-persistence-tools*
*Completed: 2026-02-02*
