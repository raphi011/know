---
phase: 06-episode-tools
plan: 02
subsystem: api
tags: [mcp, surrealdb, episodic-memory, search, hybrid-search, bm25, vector-search]

# Dependency graph
requires:
  - phase: 06-episode-tools
    plan: 01
    provides: Episode CRUD operations, query function layer for episodes
provides:
  - Episode semantic search with hybrid BM25+vector RRF fusion
  - Time range filtering (before/after) for episodic queries
  - Context-scoped episode search
affects: [episode-queries, temporal-memory-retrieval]

# Tech tracking
tech-stack:
  added: []
  patterns: [episode-search-hybrid, time-filtering, timezone-normalization]

key-files:
  created: []
  modified:
    - internal/db/queries.go
    - internal/tools/episode.go
    - internal/tools/registry.go

key-decisions:
  - "Search results return FULL content, not truncated (user decision from CONTEXT.md)"
  - "ensureTimezone helper normalizes ISO 8601 timestamps without TZ info"
  - "Default limit 10, max 50 for search results"

patterns-established:
  - "Episode search: same RRF hybrid pattern as entity search, adapted for episode table"
  - "Timezone normalization: ensureTimezone appends Z if no TZ indicator present"

# Metrics
duration: 5min
completed: 2026-02-02
---

# Phase 06 Plan 02: Episode Search Summary

**Hybrid BM25+vector search for episodes with time range filtering and full content results**

## Performance

- **Duration:** 5 min
- **Started:** 2026-02-02T21:30:00Z
- **Completed:** 2026-02-02T21:35:00Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- QuerySearchEpisodes with hybrid RRF search and time/context filtering
- search_episodes handler with input validation and context detection
- 13 total MCP tools now registered

## Task Commits

Each task was committed atomically:

1. **Task 1: Add QuerySearchEpisodes to db/queries.go** - `44ba321` (feat)
2. **Task 2: Add search_episodes handler to episode.go** - `7cec092` (feat)
3. **Task 3: Register search_episodes in registry.go** - `71e8513` (feat)

## Files Created/Modified
- `internal/db/queries.go` - Added QuerySearchEpisodes with hybrid BM25+vector search, time filtering, context filtering
- `internal/tools/episode.go` - Added SearchEpisodesInput, EpisodeSearchResult, EpisodeResult types, ensureTimezone helper, NewSearchEpisodesHandler
- `internal/tools/registry.go` - Registered search_episodes tool (13 total tools)

## Decisions Made
- Search results return FULL episode content (not truncated) - honored CONTEXT.md user decision
- ensureTimezone normalizes timestamps by appending Z if no timezone indicator present
- Default search limit 10, max 50 to balance result completeness with performance
- Access tracking updates fire-and-forget (goroutine) to not block response

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Episode tools complete (Plan 01 CRUD + Plan 02 Search)
- Phase 06 complete, ready for Phase 07 (Decay/Maintenance)
- 13 MCP tools now available: ping, search, get_entity, list_labels, list_types, remember, forget, traverse, find_path, add_episode, get_episode, delete_episode, search_episodes

---
*Phase: 06-episode-tools*
*Completed: 2026-02-02*
