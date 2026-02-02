---
phase: 06-episode-tools
plan: 01
subsystem: api
tags: [mcp, surrealdb, episodic-memory, crud]

# Dependency graph
requires:
  - phase: 05-graph-tools
    provides: Graph traversal patterns, query function layer
provides:
  - Episode CRUD operations (add_episode, get_episode, delete_episode)
  - Entity-to-episode linking via extracted_from relation
  - Timestamp-based episode ID generation
affects: [06-02, episode-search, temporal-queries]

# Tech tracking
tech-stack:
  added: []
  patterns: [episode-id-generation, entity-episode-linking]

key-files:
  created:
    - internal/tools/episode.go
  modified:
    - internal/db/queries.go
    - internal/tools/registry.go

key-decisions:
  - "Episode ID format: ep_YYYY-MM-DDTHH-MM-SSZ (timestamp-based)"
  - "Entity linking logs failures but does not fail episode creation"
  - "Content truncated to 8000 chars for embedding, 500 chars for preview"

patterns-established:
  - "Episode ID generation: timestamp-based with ep_ prefix and hyphen-replaced colons"
  - "Soft-fail linking: entity links log errors but don't block main operation"

# Metrics
duration: 8min
completed: 2026-02-02
---

# Phase 06 Plan 01: Episode CRUD Summary

**Episode CRUD tools with timestamp-based IDs, embedding generation, and entity linking**

## Performance

- **Duration:** 8 min
- **Started:** 2026-02-02T21:20:00Z
- **Completed:** 2026-02-02T21:28:00Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- 6 query functions for episode operations in db/queries.go
- 3 tool handlers in episode.go with input validation
- 12 total MCP tools registered

## Task Commits

Each task was committed atomically:

1. **Task 1: Add episode query functions** - `3dc835a` (feat)
2. **Task 2: Create episode.go handlers** - `99e8d51` (feat)
3. **Task 3: Register episode tools** - `fdad6e0` (feat)

## Files Created/Modified
- `internal/db/queries.go` - Added QueryCreateEpisode, QueryGetEpisode, QueryUpdateEpisodeAccess, QueryDeleteEpisode, QueryLinkEntityToEpisode, QueryGetLinkedEntities
- `internal/tools/episode.go` - Episode tool handlers with input types and helpers
- `internal/tools/registry.go` - Registered add_episode, get_episode, delete_episode

## Decisions Made
- Episode ID uses timestamp format (ep_2024-01-15T14-30-45Z) for natural ordering
- Entity linking uses soft-fail pattern (logs warnings, doesn't break episode creation)
- Content truncation: 8000 chars for embedding, 500 chars for response preview
- Added QueryGetLinkedEntities helper for include_entities option

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Episode CRUD complete, ready for search/list tools in Plan 02
- Query functions established for episode search implementation
- Entity-episode relationship ready for temporal queries

---
*Phase: 06-episode-tools*
*Completed: 2026-02-02*
