# Project Research Summary

**Project:** memcp Go migration
**Domain:** MCP server migration (Python to Go)
**Researched:** 2026-02-01
**Confidence:** HIGH

## Executive Summary

This project migrates a Python MCP (Model Context Protocol) server from FastMCP to Go using the official `modelcontextprotocol/go-sdk`. The server provides persistent memory for AI agents via SurrealDB with vector embeddings through Ollama. Research confirms Go is well-suited for this migration with mature SDKs and clear architectural patterns.

The recommended approach uses official SDKs throughout: `modelcontextprotocol/go-sdk` for MCP protocol, `surrealdb/surrealdb.go` for database access, and `ollama/ollama/api` for embeddings. Go 1.22+ enables type-safe tool definitions with generics while maintaining compatibility with existing 384-dimensional embeddings from `all-minilm:l6-v2`.

Critical risks center on embedding dimension compatibility (must maintain 384-dim), WebSocket connection stability (requires `rews` package for auto-reconnect), and concurrency patterns (Go's preemptive goroutines differ from Python's cooperative async). Following constructor injection patterns and enabling race detection prevents most pitfalls.

## Key Findings

### Recommended Stack

**Core framework decision:** Use official `modelcontextprotocol/go-sdk` v1.2.0 over community `mark3labs/mcp-go`. GitHub migrated their MCP server to the official SDK in late 2025, signaling it's the strategic choice. The SDK provides type-safe tool definitions via generics (`AddTool[In, Out]`) with auto-schema generation from struct tags.

**Core technologies:**
- **modelcontextprotocol/go-sdk v1.2.0**: MCP protocol implementation — official SDK maintained by Anthropic + Google, type-safe tools, future-proof
- **surrealdb/surrealdb.go v1.2.0**: SurrealDB client — official SDK with generic `Query[T]` method, WebSocket + HTTP support, relation queries
- **ollama/ollama/api v0.5.12+**: Embedding generation — official client for local Ollama, supports `all-minilm:l6-v2` (384-dim)
- **log/slog**: Structured logging — stdlib in Go 1.21+, JSON/text handlers, no external dependency
- **Go 1.22+**: Language version — required for generics improvements, loop variable fix, slog stdlib

**Critical version requirements:**
- Ollama model: `all-minilm:l6-v2` (384 dimensions) — must match existing SurrealDB HNSW index dimensions
- SurrealDB: Use `contrib/rews` package for reliable WebSocket with auto-reconnect and session restoration

### Expected Features

**Must have (table stakes):**
- Parameter extraction (required vs optional) — core MCP requirement, validated at SDK level
- Error responses — `mcp.NewToolResultError(msg)` for tool failures
- Text/JSON responses — basic output formats for MCP clients
- Context propagation — standard Go `context.Context` first parameter
- Tool descriptions — `mcp.WithDescription()` for discoverability

**Should have (competitive):**
- Structured I/O with generics — `WithInputSchema[T]()` + `NewStructuredToolHandler()` for type safety
- JSON schema via struct tags — `jsonschema:"required"` reduces boilerplate
- Tool annotations — `WithReadOnlyHintAnnotation()`, `WithDestructiveHintAnnotation()` hint behavior
- Middleware — `WithToolHandlerMiddleware()` for cross-cutting concerns
- Recovery — `server.WithRecovery()` for panic handling

**Defer (v2+):**
- NLI-based contradiction detection — Python uses `cross-encoder/nli-deberta-v3-base` which has no Go equivalent; requires external API call or Ollama LLM prompt-based approach
- Complex nested object params manually — use `WithInputSchema[T]()` with structs instead

### Architecture Approach

**Pattern: Constructor injection with explicit dependencies.** All services (SearchService, PersistService, etc.) receive dependencies via `NewXxx()` constructors. Wire everything in `main.go` composition root. No global mutable state, no framework-based DI for this project size. Tool handlers are methods on service structs that close over dependencies.

**Major components:**
1. **internal/server** — MCP server setup, tool/resource registration, lifecycle management
2. **internal/tools** — Business logic grouped by domain (search.go, persist.go, graph.go, episode.go, procedure.go, maintenance.go)
3. **internal/db** — SurrealDB connection, query execution, schema management
4. **internal/embedding** — Ollama client wrapper for embedding generation
5. **internal/models** — Shared data structures (leaf package, no dependencies)

**Build order:** Models → DB + Embedding → Tools → Server → Main. Leaf packages first, orchestration last.

### Critical Pitfalls

1. **Embedding dimension mismatch** — Switching from `all-minilm:l6-v2` (384-dim) to `nomic-embed-text` (768-dim) breaks all existing vectors. Prevention: Lock to `all-minilm:l6-v2`, assert `len(embedding) == 384` in first integration test.

2. **Async/await to goroutine mental model** — Python's cooperative async doesn't translate 1:1 to Go's preemptive goroutines. Race conditions emerge from shared state (embedding cache, lazy-loaded models). Prevention: No global mutable state, use `sync.Mutex` or channels, enable `-race` flag in all tests.

3. **WebSocket state loss on reconnect** — surrealdb.go WebSocket connections lose auth and namespace context on reconnection. Prevention: Use `github.com/surrealdb/surrealdb.go/contrib/rews` from start for auto-reconnection with session restoration.

4. **MCP context lifecycle differences** — FastMCP's `Context` lifespan pattern doesn't map to mcp-go. Prevention: Initialize DB at server startup, pass via dependency injection, not context extraction.

5. **Tool parameter type translation** — Python's `labels: list[str] | None = None` requires explicit `mcp.WithArray("labels", mcp.WithStringItems())` in Go. Prevention: Map all 15+ Python tool signatures upfront before coding.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Core Infrastructure
**Rationale:** Foundation must be bulletproof before any tool implementation. Embedding dimension compatibility and WebSocket stability are critical dependencies for all subsequent phases.

**Delivers:**
- SurrealDB connection with `rews` auto-reconnect
- Ollama embedding client with 384-dim verification
- Data models matching Python Pydantic schemas
- Logging with slog
- Test infrastructure with `-race` enabled

**Addresses:**
- Embedding dimension mismatch (Pitfall #1)
- WebSocket reconnection (Pitfall #3)
- Concurrency foundation (Pitfall #2)

**Avoids:**
- Starting tool implementation before verifying embedding compatibility
- Building on unstable DB connection

### Phase 2: MCP Server + First Tools
**Rationale:** Establish MCP server patterns with simple read-only tools before complex write operations. Search tools validate end-to-end flow (embedding → query → response).

**Delivers:**
- MCP server setup with official SDK
- Search tools (`search`, `semantic_search`, `get_entity`)
- Tool parameter extraction patterns
- Error handling conventions

**Uses:**
- `modelcontextprotocol/go-sdk` for type-safe tool registration
- Constructor injection for DB + embedding dependencies

**Implements:**
- internal/server architecture component
- internal/tools/search with service pattern

**Avoids:**
- Complex tool parameters before establishing extraction patterns (Pitfall #5)
- Write operations before read operations validated

### Phase 3: Persistence Tools
**Rationale:** Add write operations (`remember`, `forget`, `relate`) after read operations proven stable. These tools modify state and require careful error handling.

**Delivers:**
- Persistence tools with transaction handling
- Relation management
- Destructive operation annotations

**Addresses:**
- Tool annotation requirements (FEATURES.md differentiators)
- Error handling patterns (Pitfall #6)

**Implements:**
- internal/tools/persist service
- SurrealDB transaction patterns

### Phase 4: Graph Traversal Tools
**Rationale:** Graph operations (`traverse`, `find_path`, `get_context`) depend on relation data created in Phase 3. Complex SurrealQL queries need stable foundation.

**Delivers:**
- Graph traversal algorithms
- Path finding with relationship filtering
- Context aggregation

**Implements:**
- internal/tools/graph service
- Advanced SurrealQL relation queries

### Phase 5: Episode & Procedure Memory
**Rationale:** Episodic memory and procedure storage are independent features that build on core search/persist infrastructure.

**Delivers:**
- Episode logging with timeline queries
- Procedure save/search with pattern matching
- Temporal query patterns

**Implements:**
- internal/tools/episode service
- internal/tools/procedure service

### Phase 6: Maintenance & Advanced Features
**Rationale:** Housekeeping features (reflection, decay, importance recalculation) are lower priority than core memory operations.

**Delivers:**
- Memory reflection and summary
- Access decay and importance updates
- Contradiction detection (if NLI solution chosen)

**Addresses:**
- NLI model replacement complexity (Pitfall #7)

**Defers:**
- Full NLI feature parity — document as v2 feature if no API solution

### Phase Ordering Rationale

- **Infrastructure first:** Prevents building on unstable foundation (WebSocket, embeddings)
- **Read before write:** Validates data flow before allowing mutations
- **Simple before complex:** Search tools establish patterns for later persistence tools
- **Dependencies respected:** Graph tools need relations from Phase 3, episodes/procedures need search from Phase 2
- **Risk mitigation:** Critical pitfalls (#1, #2, #3) addressed in Phase 1 before they can cascade

### Research Flags

**Phases likely needing deeper research during planning:**
- **Phase 6:** NLI model replacement — sparse documentation on Go alternatives, may need API integration research

**Phases with standard patterns (skip research-phase):**
- **Phase 1:** Well-documented database client and embedding patterns
- **Phase 2:** Official SDK documentation covers tool registration
- **Phase 3-5:** SurrealQL patterns from existing Python implementation transfer directly

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All official SDKs verified via pkg.go.dev, GitHub migration confirms SDK choice |
| Features | HIGH | MCP tool patterns documented in official SDK, feature translation straightforward |
| Architecture | HIGH | Standard Go patterns (constructor injection, composition root), confirmed via official Go docs |
| Pitfalls | MEDIUM | Embedding dimension verified via Ollama docs, WebSocket issues confirmed in GitHub issues, concurrency patterns from Go guides |

**Overall confidence:** HIGH

### Gaps to Address

- **NLI model replacement:** No direct equivalent found. Options: (1) External API call to Hugging Face, (2) Local Python microservice, (3) Ollama LLM with prompt-based detection, (4) Defer to v2. Decision needed during Phase 6 planning.

- **RecordID escaping:** surrealdb.go handling of special characters in IDs needs validation during Phase 1. Python uses `RecordID('entity', id)` helper; Go equivalent unclear. Test with hyphens, colons, brackets early.

- **Error wrapping conventions:** Establish pattern in Phase 1 for consistent error context. Use `fmt.Errorf("operation: %w", err)` for all database and embedding errors.

## Sources

### Primary (HIGH confidence)
- [modelcontextprotocol/go-sdk GitHub](https://github.com/modelcontextprotocol/go-sdk) — Official SDK patterns, tool registration
- [surrealdb/surrealdb.go pkg.go.dev](https://pkg.go.dev/github.com/surrealdb/surrealdb.go) — Client API, query methods, rews package
- [Ollama API pkg.go.dev](https://pkg.go.dev/github.com/ollama/ollama/api) — Embedding API, model compatibility
- [Go Module Layout Guide](https://go.dev/doc/modules/layout) — Official project structure patterns
- [all-minilm:l6-v2 on Ollama](https://ollama.com/library/all-minilm:l6-v2) — 384-dim confirmation

### Secondary (MEDIUM confidence)
- [Go Concurrency Guide](https://getstream.io/blog/goroutines-go-concurrency-guide/) — Goroutine vs async patterns
- [surrealdb.go GitHub Issues](https://github.com/surrealdb/surrealdb.go/issues) — WebSocket reconnection issues (#280), RecordID handling (#336)

### Tertiary (LOW confidence)
- [Python to Go Migration Guide](https://www.honeybadger.io/blog/migrate-from-python-golang/) — General patterns, needs validation

### Codebase Analysis
- `memcp/db.py` — Schema definitions (HNSW DIMENSION 384), query patterns, timestamp fixing
- `memcp/utils/embedding.py` — all-MiniLM-L6-v2 usage, global state patterns
- `memcp/servers/*.py` — 15+ tool implementations with parameter signatures
- `memcp/models.py` — Pydantic model structures for response types

---
*Research completed: 2026-02-01*
*Ready for roadmap: yes*
