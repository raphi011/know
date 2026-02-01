---
phase: 02-mcp-server
plan: 02
subsystem: server
tags: [mcp, go-sdk, tools, dependency-injection, handlers]

# Dependency graph
requires:
  - phase: 02-01
    provides: MCP server wrapper, logging middleware, main entry point
provides:
  - Tool registration framework with dependency injection
  - Error and result helpers for tool responses
  - Ping tool demonstrating handler factory pattern
affects: [03-search, 04-persistence, 05-labels, 06-relations, 07-contexts, 08-polish]

# Tech tracking
tech-stack:
  added: []
  patterns: [handler factory pattern, closure-based DI, typed tool handlers]

key-files:
  created:
    - internal/tools/deps.go
    - internal/tools/errors.go
    - internal/tools/ping.go
    - internal/tools/registry.go
    - internal/tools/tools_test.go
  modified:
    - cmd/memcp/main.go

key-decisions:
  - "jsonschema tag uses direct description text, not key=value format"
  - "Ping tool with nil deps is valid for testing (deps are optional)"
  - "Handler factory pattern captures deps in closure return"

patterns-established:
  - "Handler factory: NewXxxHandler(deps) returns mcp.ToolHandlerFor[Input, any]"
  - "Error helper: ErrorResult(msg, hint) creates IsError=true result with recovery hint"
  - "Text helper: TextResult(text) creates success result with text content"
  - "Registration: tools.RegisterAll(server, deps) called from main after Setup()"

# Metrics
duration: 3min
completed: 2026-02-01
---

# Phase 2 Plan 2: Tool Registry Summary

**Tool registration framework with Dependencies DI, ErrorResult/TextResult helpers, and ping tool demonstrating handler factory pattern**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-01T22:00:21Z
- **Completed:** 2026-02-01T22:03:06Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- Dependencies struct for service injection (DB, Embedder, Logger)
- ErrorResult/TextResult helpers for consistent tool responses
- Ping tool demonstrating complete handler factory pattern
- Integration tests verify tool registration and invocation via in-memory transport

## Task Commits

Each task was committed atomically:

1. **Task 1: Dependencies and Error Helpers** - `035abec` (feat)
2. **Task 2: Ping Tool and Registry** - `eaff174` (feat)
3. **Task 3: Wire Tools into Main and Test** - `0a318a3` (feat)

## Files Created/Modified
- `internal/tools/deps.go` - Dependencies struct for service injection
- `internal/tools/errors.go` - ErrorResult, TextResult, FormatResults helpers
- `internal/tools/ping.go` - PingInput struct, NewPingHandler factory
- `internal/tools/registry.go` - RegisterAll registers all tools
- `internal/tools/tools_test.go` - Integration tests for ping tool
- `cmd/memcp/main.go` - Wires tools package with deps

## Decisions Made
- jsonschema tag format: use direct description text (`jsonschema:"Text to echo back"`) not key=value format (`jsonschema:"description=Text to echo back"`)
- Handler factories allow nil deps for testing when tool doesn't need services
- Used mcp.ToolHandlerFor[In, any] with 3-return signature (result, output, error)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed jsonschema tag format**
- **Found during:** Task 3 (integration test)
- **Issue:** SDK panicked on `jsonschema:"description=..."` format
- **Fix:** Changed to direct description text `jsonschema:"Text to echo back"`
- **Files modified:** internal/tools/ping.go
- **Verification:** Integration tests pass
- **Committed in:** 0a318a3 (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** SDK documentation example showed different format than actual API. Fixed per SDK error message.

## Issues Encountered
- MCP SDK uses `*mcp.TextContent` (pointer) not value type for Content interface
- MCP SDK ToolHandlerFor has 3-return signature `(result, output, error)` not 2-return

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Tool framework complete, ready for business logic tools (Phase 3-8)
- Pattern established: NewXxxHandler(deps) returns typed handler
- All future tools follow same registration pattern in RegisterAll
- Tests verify tool list and call work via MCP protocol

---
*Phase: 02-mcp-server*
*Completed: 2026-02-01*
