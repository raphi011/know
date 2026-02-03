---
phase: 07-procedure-tools
plan: 01
subsystem: api
tags: [procedure, workflow, crud, mcp-tools, surrealdb]

# Dependency graph
requires:
  - phase: 06-episode-tools
    provides: Episode CRUD patterns, query layer structure
provides:
  - Procedure query functions (QueryCreateProcedure, QueryGetProcedure, QueryUpdateProcedureAccess, QueryDeleteProcedure)
  - Procedure tool handlers (create_procedure, get_procedure, delete_procedure)
  - Step-based workflow storage with embedding
affects: [07-procedure-tools-02, 08-polish]

# Tech tracking
tech-stack:
  added: []
  patterns: [procedure-step-indexing, name-based-procedure-id]

key-files:
  created:
    - internal/tools/procedure.go
  modified:
    - internal/db/queries.go
    - internal/tools/registry.go

key-decisions:
  - "Procedure ID format: context:slugified-name (reuses entity ID pattern)"
  - "Step order field uses 1-based indexing"
  - "Embedding generated from combined name + description + steps content"
  - "Action field returns 'created' or 'updated' to indicate upsert result"

patterns-established:
  - "Procedure CRUD follows same pattern as Episode CRUD"
  - "generateProcedureID reuses slugify from remember.go"

# Metrics
duration: 6min
completed: 2026-02-03
---

# Phase 7 Plan 1: Procedure CRUD Summary

**Procedure CRUD tools with step-ordered storage, embedding generation, and upsert semantics**

## Performance

- **Duration:** 6 min
- **Started:** 2026-02-03T11:50:12Z
- **Completed:** 2026-02-03T11:56:06Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- Added 4 procedure query functions to db/queries.go
- Created procedure.go with 3 tool handlers and input/result types
- Registered 3 new tools bringing total to 16

## Task Commits

Each task was committed atomically:

1. **Task 1: Add procedure query functions** - `9c13fb0` (feat)
2. **Task 2: Create procedure.go with handlers** - `df2cf93` (feat)
3. **Task 3: Register procedure CRUD tools** - `888ab96` (feat)

## Files Created/Modified
- `internal/db/queries.go` - Added QueryCreateProcedure, QueryGetProcedure, QueryUpdateProcedureAccess, QueryDeleteProcedure
- `internal/tools/procedure.go` - Created with CreateProcedureInput/Result, GetProcedureInput/Result, DeleteProcedureInput/Result and handlers
- `internal/tools/registry.go` - Registered create_procedure, get_procedure, delete_procedure

## Decisions Made
- Procedure ID format: `context:slugified-name` (reuses existing ID pattern from remember.go)
- Step order uses 1-based indexing (more intuitive for users)
- Embedding generated from combined text: `name + " " + description + " " + steps.content.join(" ")`
- Pre-check existence to return `action: "created"` or `action: "updated"` indicator

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Procedure CRUD complete, ready for search_procedures in 07-02
- Query layer follows same pattern as episodes
- 16 tools total registered

---
*Phase: 07-procedure-tools*
*Completed: 2026-02-03*
