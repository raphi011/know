---
phase: 08-maintenance-tools
plan: 01
subsystem: api
tags: [mcp, maintenance, decay, similarity, cosine, embedding]

# Dependency graph
requires:
  - phase: 07-procedure-tools
    provides: Tool registration pattern, context detection, query function layer
provides:
  - reflect tool with decay and similar actions
  - QueryApplyDecay for stale entity maintenance
  - QueryFindSimilarPairs for duplicate detection
  - DecayedEntity, DecayResult, SimilarPair, SimilarPairsResult model types
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Single tool with action parameter for related operations"
    - "Two-step SELECT then UPDATE for capturing before/after values"
    - "Cosine similarity for embedding comparison"

key-files:
  created:
    - internal/models/reflect.go
    - internal/tools/reflect.go
  modified:
    - internal/db/queries.go
    - internal/tools/registry.go

key-decisions:
  - "Single reflect tool with action=decay|similar instead of separate tools"
  - "Decay factor 0.9 (10% reduction per run) with floor at 0.1"
  - "Similar pairs uses e1.id < e2.id to deduplicate pairs"
  - "Similar action is identify-only (always dry_run=true in result)"
  - "Default thresholds: decay_days=30, similarity_threshold=0.85, limit=10"

patterns-established:
  - "Maintenance tools: single tool with action parameter for related operations"
  - "Before/after capture: two-step SELECT then UPDATE pattern"
  - "Pair deduplication: e1.id < e2.id in cross-join"

# Metrics
duration: 6min
completed: 2026-02-03
---

# Phase 8 Plan 1: Reflect Tool Summary

**Memory maintenance tool with decay for stale entities and similar pairs detection using cosine similarity**

## Performance

- **Duration:** 6 min
- **Started:** 2026-02-03T12:15:00Z
- **Completed:** 2026-02-03T12:21:00Z
- **Tasks:** 5/5
- **Files modified:** 4

## Accomplishments
- Reflect tool with action=decay and action=similar for memory maintenance
- QueryApplyDecay reduces importance of entities not accessed in N days
- QueryFindSimilarPairs identifies potential duplicates by embedding similarity
- Dry-run support for preview before applying changes
- Context scoping with global override

## Task Commits

Each task was committed atomically:

1. **Task 1: Create reflect model types** - `0db5134` (feat)
2. **Task 2: Add decay query function** - `b0b7d8d` (feat)
3. **Task 3: Add similar pairs query function** - `e3c2f65` (feat)
4. **Task 4: Create reflect tool handler** - `47d74c0` (feat)
5. **Task 5: Register reflect tool** - `9849cc9` (feat)

## Files Created/Modified
- `internal/models/reflect.go` - DecayedEntity, DecayResult, SimilarPair, SimilarPairsResult types
- `internal/db/queries.go` - QueryApplyDecay and QueryFindSimilarPairs functions
- `internal/tools/reflect.go` - NewReflectHandler with handleDecay and handleSimilar
- `internal/tools/registry.go` - reflect tool registration (now 19 total tools)

## Decisions Made
- **Single tool vs multiple:** Used single reflect tool with action parameter for cleaner interface
- **Decay factor:** 0.9 multiplier (10% reduction per run) balances gradual decay with meaningful effect
- **Decay floor:** 0.1 minimum prevents complete decay, allows recovery on access
- **Two-step query:** SELECT then UPDATE captures before/after values without relying on unverified SurrealDB RETURN BEFORE syntax
- **Pair deduplication:** e1.id < e2.id in WHERE clause ensures each pair returned once (A-B not B-A)
- **Similar is identify-only:** Always sets dry_run=true since merging is manual (safer)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- reflect tool ready for memory maintenance operations
- 19 tools total now registered
- Integration tests would require running SurrealDB + Ollama

---
*Phase: 08-maintenance-tools*
*Completed: 2026-02-03*
