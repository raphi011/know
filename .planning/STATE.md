# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-01)

**Core value:** Agents can remember and recall knowledge across sessions with sub-second semantic search
**Current focus:** Phase 7 - Procedure Tools (COMPLETE)

## Current Position

Phase: 7 of 8 (Procedure Tools) - COMPLETE
Plan: 2 of 2 in current phase - COMPLETE
Status: Phase 7 complete, ready for Phase 8
Last activity: 2026-02-03 - Completed 07-02-PLAN.md

Progress: [██████████] 100% (10 of 10 plans complete)

## Performance Metrics

**Velocity:**
- Total plans completed: 10
- Average duration: ~7 min per plan
- Total execution time: ~104 min

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1 | 3 | 30m | 10m |
| 2 | 2 | 6m | 3m |
| 3 | 2 | 13m | 6.5m |
| 4 | 2 | 17m | 8.5m |
| 5 | 2 | 14m | 7m |
| 6 | 2 | 13m | 6.5m |
| 7 | 2 | 11m | 5.5m |

**Recent Trend:**
- Last 5 plans: 06-01, 06-02, 07-01, 07-02
- Trend: Consistent 5-7m per plan

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
- [06-02]: Search results return FULL content (not truncated) - user decision
- [06-02]: ensureTimezone normalizes ISO 8601 timestamps without TZ info
- [06-02]: Episode search default limit 10, max 50
- [07-01]: Procedure ID format: context:slugified-name (reuses entity ID pattern)
- [07-01]: Step order uses 1-based indexing
- [07-01]: Embedding from combined name + description + steps content
- [07-01]: Pre-check existence for action: created/updated indicator
- [07-02]: Search returns ProcedureSummary for efficiency
- [07-02]: Search default limit 10, max 50; list default 50, max 100
- [07-02]: Fire-and-forget access tracking for search results

### Pending Todos

None.

### Blockers/Concerns

- Human verification needed: Integration tests require running SurrealDB and Ollama instances

## Session Continuity

Last session: 2026-02-03T12:03:30Z
Stopped at: Completed 07-02-PLAN.md
Resume file: None

## Phase 7 Summary (COMPLETE)

**Plan 01 (COMPLETE):**
- QueryCreateProcedure, QueryGetProcedure, QueryUpdateProcedureAccess, QueryDeleteProcedure
- procedure.go with create_procedure, get_procedure, delete_procedure handlers
- Name-based ID generation with context prefix

**Plan 02 (COMPLETE):**
- QuerySearchProcedures with hybrid BM25+vector search and RRF fusion
- QueryListProcedures with context filtering
- search_procedures and list_procedures handlers
- ProcedureSummary type for lightweight results

**18 tools now registered:** ping, search, get_entity, list_labels, list_types, remember, forget, traverse, find_path, add_episode, get_episode, delete_episode, search_episodes, create_procedure, get_procedure, delete_procedure, search_procedures, list_procedures

**Patterns Established:**
- Procedure CRUD follows same pattern as Episode CRUD
- generateProcedureID reuses slugify from remember.go
- Step ordering with 1-based indexing
- ProcedureSummary for lightweight search/list results

**Next:** Phase 8 (polish) if created, otherwise project complete
