# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-01)

**Core value:** Agents can remember and recall knowledge across sessions with sub-second semantic search
**Current focus:** Phase 6 - Episode Tools (IN PROGRESS)

## Current Position

Phase: 6 of 8 (Episode Tools)
Plan: 1 of 2 in current phase - COMPLETE
Status: In progress
Last activity: 2026-02-02 - Completed 06-01-PLAN.md

Progress: [████████░░] 78% (7 of 9 plans complete)

## Performance Metrics

**Velocity:**
- Total plans completed: 7
- Average duration: ~8 min per plan
- Total execution time: ~88 min

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1 | 3 | 30m | 10m |
| 2 | 2 | 6m | 3m |
| 3 | 2 | 13m | 6.5m |
| 4 | 2 | 17m | 8.5m |
| 5 | 2 | 14m | 7m |
| 6 | 1 | 8m | 8m |

**Recent Trend:**
- Last 5 plans: 04-02, 05-01, 05-02, 06-01
- Trend: Consistent 7-8m per plan

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Init]: Use official modelcontextprotocol/go-sdk over mark3labs/mcp-go
- [Init]: Lock to all-minilm:l6-v2 (384-dim) for embedding compatibility
- [Init]: Use rews package for SurrealDB WebSocket auto-reconnect
- [Phase-1]: Generic Embedder interface supports multiple backends (Ollama, Anthropic/Voyage)
- [02-01]: Middleware uses SDK's MethodHandler signature with method string parameter
- [02-01]: Slow request threshold: 100ms for WARN level logging
- [02-01]: Argument truncation: 200 chars max in logs
- [02-02]: jsonschema tag uses direct description text, not key=value format
- [02-02]: Handler factory pattern: NewXxxHandler(deps) returns mcp.ToolHandlerFor[In, any]
- [03-01]: Query function layer in db/queries.go for SQL isolation
- [03-01]: Context detection: explicit > config.DefaultContext > git origin > cwd
- [03-01]: RRF parameters: k=60, vector limit=2x for diversity
- [03-02]: get_entity has no context detection (entity IDs are globally unique)
- [03-02]: list_labels and list_types use context detection for scoping
- [04-01]: array::union for additive label merge in SQL
- [04-01]: Pre-check existence to return wasCreated indicator
- [04-01]: Schema validation at SDK level for required fields
- [04-02]: Relation creation validates entity existence before RELATE
- [04-02]: Delete uses RETURN BEFORE to count actual deletions
- [04-02]: forget tool resolves names to IDs if no colon present
- [05-01]: Use fmt.Sprintf for depth injection (SurrealDB requires literal depth)
- [05-01]: Default depth 2, max 10 for traverse performance
- [05-02]: Default max_depth 5, max 20 for path finding
- [05-02]: PathFound boolean for clear no-path vs error distinction
- [06-01]: Episode ID format: ep_YYYY-MM-DDTHH-MM-SSZ (timestamp-based)
- [06-01]: Entity linking logs failures but does not fail episode creation
- [06-01]: Content truncated to 8000 chars for embedding, 500 chars for preview

### Pending Todos

None.

### Blockers/Concerns

- Human verification needed: Integration tests require running SurrealDB and Ollama instances

## Session Continuity

Last session: 2026-02-02T21:28:00Z
Stopped at: Completed 06-01-PLAN.md
Resume file: None

## Phase 6 Summary (IN PROGRESS)

**Plan 01 (COMPLETE):**
- QueryCreateEpisode, QueryGetEpisode, QueryDeleteEpisode, QueryUpdateEpisodeAccess
- QueryLinkEntityToEpisode, QueryGetLinkedEntities
- episode.go with add_episode, get_episode, delete_episode handlers
- Timestamp-based ID generation (ep_YYYY-MM-DDTHH-MM-SSZ)

**12 tools now registered:** ping, search, get_entity, list_labels, list_types, remember, forget, traverse, find_path, add_episode, get_episode, delete_episode

**Patterns Established:**
- Timestamp-based episode ID generation
- Soft-fail entity linking (logs warnings, doesn't block)
- Content truncation for embedding (8000 chars)

**Next:** Plan 02 (Episode Search/List Tools)
