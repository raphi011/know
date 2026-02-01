# Phase 2: MCP Server - Context

**Gathered:** 2026-02-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Working MCP server with stdio transport and tool registration framework. No business logic yet — just the skeleton that all tools will plug into. Claude Code can connect and list available tools.

</domain>

<decisions>
## Implementation Decisions

### Server lifecycle
- Log version and config at startup (DB connection status, config values)
- Graceful shutdown: finish in-flight requests, close DB cleanly, then exit
- Exit with error if DB connection fails at startup (no degraded mode)
- No health check tool — just run without introspection

### Tool handler pattern
- Schema-based validation: MCP SDK validates required args and types before handler runs
- Match Python return format: tools return formatted text, not structured JSON
- Common wrapper for all handlers: logging, error handling, timing

### Error surfacing
- Minimal error messages to Claude: short like "Entity not found"
- Include recovery hints: e.g. "Entity not found. Try search first."
- DB connection errors mid-operation: fail immediately, let Claude retry
- Internal errors (panics): log full details to file, return generic "Internal error" to Claude

### Logging behavior
- Log all tool calls with name, args (sanitized), and duration
- Only log slow DB queries (>100ms threshold)

### Claude's Discretion
- Dependency injection approach (global vs context injection)
- Argument truncation length for logging
- Log output targets (dual stderr+file vs file-only based on MCP constraints)

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-mcp-server*
*Context gathered: 2026-02-01*
