# Domain Pitfalls: Python to Go MCP Server Migration

**Domain:** Python MCP server (FastMCP + SurrealDB + sentence-transformers) to Go (mcp-go + surrealdb.go + Ollama)
**Researched:** 2026-02-01
**Confidence:** MEDIUM (verified with official sources + codebase analysis)

---

## Critical Pitfalls

Mistakes that cause rewrites or major issues.

---

### Pitfall 1: Embedding Dimension Mismatch Breaking Existing Data

**What goes wrong:** The existing SurrealDB schema uses 384-dimensional embeddings from `all-MiniLM-L6-v2`. Switching to a different embedding model (e.g., `nomic-embed-text` at 768 dims) makes all existing vectors incompatible with new embeddings. Vector similarity queries return garbage results or fail.

**Why it happens:**
- Teams assume "embeddings are embeddings" without checking dimensions
- `nomic-embed-text` is commonly recommended for Ollama but outputs 768 dimensions
- SurrealDB HNSW index is defined with `DIMENSION 384` - dimension mismatch causes query failures

**Consequences:**
- All existing entity/episode/procedure embeddings become useless
- Either re-embed all data (expensive) or drop vector search entirely
- HNSW index rebuild required if dimension changes

**Prevention:**
1. Use `all-minilm:l6-v2` in Ollama which produces exactly 384 dimensions
2. Verify dimension match in first integration test before any production migration
3. Add dimension assertion: `if len(embedding) != 384 { panic("dimension mismatch") }`

**Detection:**
- Early: Query returns 0 results despite matching content
- Late: `vector::similarity::cosine` returns NaN or errors
- Test: Compare `len(embedding)` between Python and Go outputs

**Phase:** Address in Phase 1 (Core Infrastructure) before any data migration

**Sources:**
- [all-minilm:l6-v2 on Ollama](https://ollama.com/library/all-minilm:l6-v2) - 384 dim confirmed
- [nomic-embed-text](https://ollama.com/library/nomic-embed-text) - 768 dim default
- Codebase: `DEFINE INDEX ... HNSW DIMENSION 384` in `db.py`

---

### Pitfall 2: Async/Await to Goroutine Mental Model Shift

**What goes wrong:** Python's FastMCP uses `async/await` with single-threaded event loop. Go's mcp-go uses goroutines with concurrent execution. Developers try to port 1:1, creating race conditions or deadlocks.

**Why it happens:**
- Python async is cooperative (one coroutine runs at a time within event loop)
- Go goroutines are preemptive and truly parallel
- Python's mutable state patterns don't survive concurrent access
- The Python code uses global state (e.g., `_embedder`, `_nli_model` lazy loading)

**Consequences:**
- Race conditions in shared state (embedding cache, DB connections)
- Deadlocks from improper channel usage
- Data corruption in concurrent operations
- Memory leaks from goroutine leaks

**Prevention:**
1. Don't use global mutable state - pass dependencies explicitly
2. Use `sync.Mutex` or channels for any shared state
3. Run Go tests with `-race` flag to detect races
4. Design for "share memory by communicating" not "communicate by sharing memory"
5. Replace Python's `lru_cache` with sync.Map or dedicated cache package

**Detection:**
- Early: `-race` flag catches at test time
- Late: Intermittent test failures, corruption in production
- Code smell: Global `var` without mutex protection

**Phase:** Phase 1 foundation - establish patterns before implementing tools

**Sources:**
- [Go Concurrency Guide](https://getstream.io/blog/goroutines-go-concurrency-guide/)
- Codebase: `_embedder = None` global in `embedding.py`

---

### Pitfall 3: SurrealDB Go SDK WebSocket State Loss

**What goes wrong:** surrealdb.go WebSocket connections are stateful (auth, namespace/database selection). On reconnection, state is lost. Tools fail silently or return auth errors.

**Why it happens:**
- Python SDK handles reconnection automatically (or app restarts fast enough)
- Go services run longer, encounter more network hiccups
- Default surrealdb.go client doesn't restore session on reconnect

**Consequences:**
- Random auth failures after network blip
- Namespace/database context lost mid-operation
- Live queries stop working after reconnect

**Prevention:**
1. Use `github.com/surrealdb/surrealdb.go/contrib/rews` for auto-reconnection with session restoration
2. Implement health check that verifies `USE namespace database` state
3. Wrap all DB operations with retry logic
4. Consider HTTP transport for stateless operations (though slower)

**Detection:**
- Long-running server suddenly returns "not authenticated" errors
- Test: Kill SurrealDB, restart, verify client recovers

**Phase:** Phase 1 (Database Layer) - must be correct from start

**Sources:**
- [SurrealDB Go SDK docs](https://surrealdb.com/docs/sdk/golang) - rews package mentioned
- [GitHub Issues](https://github.com/surrealdb/surrealdb.go/issues) - Issue #280 reconnection

---

## Moderate Pitfalls

Mistakes that cause delays or technical debt.

---

### Pitfall 4: MCP Context Lifecycle Misunderstanding

**What goes wrong:** FastMCP's `Context` provides lifespan-managed resources (DB connection). mcp-go has different patterns. Developers try to access resources that don't exist or leak resources.

**Why it happens:**
- Python: `ctx.request_context.lifespan_context.db` pattern
- Go: Different initialization and dependency injection patterns
- mcp-go server lifecycle is less documented

**Prevention:**
1. Study mcp-go server initialization patterns in examples
2. Use dependency injection for DB connection, not context extraction
3. Create wrapper struct that holds all dependencies
4. Initialize DB connection at server startup, not per-request

**Detection:**
- Nil pointer panics when accessing "context" data
- Resource leaks (connection pool exhaustion)

**Phase:** Phase 1 (MCP Server Setup)

**Sources:**
- [mcp-go README](https://github.com/mark3labs/mcp-go) - server patterns
- Codebase: `get_db(ctx)` pattern in Python tools

---

### Pitfall 5: Tool Parameter Type Differences

**What goes wrong:** Python's FastMCP infers parameter types from type hints. mcp-go requires explicit parameter definitions. Optional parameters with defaults don't translate directly.

**Why it happens:**
- Python: `labels: list[str] | None = None` just works
- Go: Must explicitly use `WithString()`, `WithNumber()` etc.
- Go doesn't have union types - need different approach for optional/nullable

**Prevention:**
1. Map all Python tool signatures explicitly before coding
2. Use pointer types for optional fields: `*string`, `*int`
3. Implement custom unmarshaling for complex types
4. Test all parameter combinations (null, empty, populated)

**Detection:**
- Tools fail with "invalid parameter" errors
- Default values not applied correctly

**Phase:** Phase 2 (Tool Implementation)

**Sources:**
- Codebase: All tool definitions in `servers/*.py`
- [mcp-go pkg docs](https://pkg.go.dev/github.com/mark3labs/mcp-go)

---

### Pitfall 6: Error Handling Philosophy Mismatch

**What goes wrong:** Python raises exceptions (`ToolError`). Go returns errors. Unhandled errors cause silent failures or incorrect results.

**Why it happens:**
- Python: `raise ToolError("message")` bubbles up
- Go: Must check every `err != nil`
- Easy to forget error checks during rapid porting

**Prevention:**
1. Use `errcheck` linter to catch unchecked errors
2. Establish error handling patterns early (wrap errors with context)
3. Map Python `ToolError` to mcp-go's `NewToolResultError()`
4. Log all errors before returning

**Detection:**
- Operations silently fail
- Inconsistent state in database
- Linter warnings

**Phase:** Phase 1 - establish patterns; Phase 2 - apply to all tools

**Sources:**
- [mcp-go errors](https://pkg.go.dev/github.com/mark3labs/mcp-go) - ErrInvalidParams etc.
- Codebase: `raise ToolError` usage in `search.py`

---

### Pitfall 7: NLI Model Replacement Complexity

**What goes wrong:** Python uses `cross-encoder/nli-deberta-v3-base` for contradiction detection. No direct Go equivalent. Ollama doesn't offer NLI models natively.

**Why it happens:**
- sentence-transformers ecosystem is Python-only
- Ollama focuses on generative models, not classification
- NLI requires specific model architecture

**Consequences:**
- `detect_contradictions` feature may need removal or API call
- Feature parity gap with Python version

**Prevention:**
1. Option A: Call external API (Hugging Face Inference, local Python service)
2. Option B: Remove feature initially, add later via API
3. Option C: Use Ollama LLM with prompt-based contradiction detection
4. Document feature gap clearly if deferring

**Detection:**
- Feature gap during requirements mapping
- Test failures on contradiction detection

**Phase:** Phase 3 (Advanced Features) - can defer initially

**Sources:**
- Codebase: `check_contradiction()` in `embedding.py`
- [Ollama models](https://ollama.com/library) - no NLI models

---

### Pitfall 8: RecordID Handling Differences

**What goes wrong:** Python SDK uses `RecordID('entity', id)` for escaping special characters in IDs. Go SDK may handle differently. Queries fail on IDs with hyphens, colons, etc.

**Why it happens:**
- SurrealDB record IDs need escaping: `entity:my-id` vs `entity:⟨my-id⟩`
- Python SDK has `RecordID` helper
- Go SDK behavior may differ

**Prevention:**
1. Test with IDs containing special characters early
2. Create ID escaping helper function
3. Document valid ID formats
4. Consider restricting ID format in Go version

**Detection:**
- Query errors on certain entity IDs
- "Parse error" from SurrealDB

**Phase:** Phase 1 (Database Layer)

**Sources:**
- Codebase: `RecordID('entity', from_id)` usage in `db.py`
- [surrealdb.go issues](https://github.com/surrealdb/surrealdb.go/issues) - RecordIDType (#336)

---

## Minor Pitfalls

Mistakes that cause annoyance but are fixable.

---

### Pitfall 9: Logging Differences

**What goes wrong:** Python uses `logging` module with file + stderr handlers. Go uses different logging patterns. Log output format doesn't match, making debugging harder.

**Prevention:**
1. Use structured logging (zerolog, zap)
2. Match log levels to Python equivalents
3. Configure file + stderr output similar to Python
4. Include request IDs for tracing

**Phase:** Phase 1 (Infrastructure)

---

### Pitfall 10: DateTime/Timestamp Format Handling

**What goes wrong:** Python code fixes timestamps by appending 'Z' for SurrealDB. Go's `time.Time` has different formatting. Queries fail on time comparisons.

**Prevention:**
1. Use RFC3339 format consistently: `time.Format(time.RFC3339)`
2. Test timestamp edge cases (no timezone, various formats)
3. Create timestamp helper functions

**Phase:** Phase 2 (Tool Implementation)

**Sources:**
- Codebase: `fix_timestamp()` helper in `db.py`

---

### Pitfall 11: Pydantic to Go Struct Mapping

**What goes wrong:** Python uses Pydantic models with `Field(default_factory=list)`. Go structs need explicit initialization. Nil slices vs empty slices cause JSON differences.

**Prevention:**
1. Use `omitempty` json tags carefully
2. Initialize slices in constructors: `Labels: []string{}`
3. Test JSON output matches Python format

**Phase:** Phase 2 (Response Models)

**Sources:**
- Codebase: `models.py` Pydantic definitions

---

## Phase-Specific Warnings

| Phase | Topic | Likely Pitfall | Mitigation |
|-------|-------|----------------|------------|
| 1 | Database Setup | WebSocket reconnection (#3) | Use rews package from start |
| 1 | Embeddings | Dimension mismatch (#1) | Verify 384-dim before any data work |
| 1 | Concurrency | Race conditions (#2) | Enable `-race` in all tests |
| 2 | Tool Params | Type mismatches (#5) | Map all signatures upfront |
| 2 | Errors | Silent failures (#6) | Use errcheck linter |
| 2 | Timestamps | Format issues (#10) | RFC3339 helper functions |
| 3 | NLI | Missing feature (#7) | Document gap, plan API solution |
| 3 | Advanced | RecordID escaping (#8) | Test special chars early |

---

## Migration-Specific Checklist

Before starting migration:

- [ ] Verify Ollama `all-minilm:l6-v2` produces 384-dim embeddings
- [ ] Set up surrealdb.go with rews auto-reconnect
- [ ] Configure Go tests with `-race` flag
- [ ] Map all 15+ Python tools to Go signatures
- [ ] Decide NLI feature fate (keep via API, defer, or drop)
- [ ] Create ID escaping helper for SurrealDB

---

## Sources

### Official Documentation
- [SurrealDB Go SDK](https://surrealdb.com/docs/sdk/golang) - MEDIUM confidence
- [mcp-go GitHub](https://github.com/mark3labs/mcp-go) - MEDIUM confidence
- [Ollama API](https://pkg.go.dev/github.com/ollama/ollama/api) - HIGH confidence

### Community Sources
- [Python to Go Migration Guide - Honeybadger](https://www.honeybadger.io/blog/migrate-from-python-golang/) - LOW confidence
- [Go Concurrency Guide](https://getstream.io/blog/goroutines-go-concurrency-guide/) - MEDIUM confidence
- [surrealdb.go Issues](https://github.com/surrealdb/surrealdb.go/issues) - HIGH confidence (direct reports)

### Codebase Analysis
- `memcp/db.py` - Schema definitions, query patterns
- `memcp/utils/embedding.py` - Embedding model usage
- `memcp/servers/*.py` - Tool implementations
- `memcp/models.py` - Response model structures
