# memcp-go

## What This Is

A Go rewrite of the memcp MCP server — a persistent memory layer for AI agents using SurrealDB as the knowledge graph backend. Enables Claude and other MCP clients to store and retrieve knowledge across sessions via semantic search, episodic memory, and graph traversal.

## Core Value

Agents can remember and recall knowledge across sessions with sub-second semantic search.

## Requirements

### Validated

<!-- These are the existing Python capabilities being migrated -->

- ✓ SurrealDB connection with async queries — existing
- ✓ Entity CRUD (upsert, get, delete) with embeddings — existing
- ✓ Hybrid search (BM25 + vector similarity with RRF fusion) — existing
- ✓ Episode CRUD (create, search, get, delete) — existing
- ✓ Procedure CRUD (create, search, get, delete, list) — existing
- ✓ Graph traversal (neighbors, path finding) — existing
- ✓ Maintenance operations (reflect, decay) — existing
- ✓ Context/namespace filtering — existing
- ✓ MCP stdio transport — existing

### Active

<!-- What we're building in this migration -->

- [ ] Go MCP server using mark3labs/mcp-go
- [ ] Ollama integration for all-minilm embeddings (384-dim)
- [ ] SurrealDB Go client with connection management
- [ ] All query functions ported from Python
- [ ] MCP tools: search, get_entity, list_labels, list_types
- [ ] MCP tools: remember, forget
- [ ] MCP tools: add_episode, search_episodes, get_episode, delete_episode
- [ ] MCP tools: create_procedure, search_procedures, get_procedure, delete_procedure, list_procedures
- [ ] MCP tools: traverse, find_path
- [ ] MCP tools: reflect (maintenance)
- [ ] Unit tests for query functions
- [ ] Integration tests with SurrealDB
- [ ] Integration tests with Ollama

### Out of Scope

- Contradiction detection (check_contradictions tool, NLI in remember) — requires ML model, defer to later
- REST/GraphQL HTTP endpoints — frontend not needed for MCP-only usage
- Dashboard web UI — separate concern, can add later
- Document parsing (docling integration) — defer to later
- Python code maintenance — big bang replacement

## Context

**Migration context:**
- Existing Python codebase has 1,300+ lines of SurrealDB queries in `memcp/db.py`
- 6 sub-servers with ~20 MCP tools total
- Schema uses 384-dim HNSW indices (matches all-minilm)
- Tests exist for Python version (can inform Go test structure)

**Technical environment:**
- Go 1.22+ (latest stable)
- SurrealDB v3.0+ (existing instance)
- Ollama running locally (user's machine)
- macOS development (Darwin)

**Why Go:**
- Single binary deployment (no Python runtime)
- Better performance for embedding operations
- Simpler dependency management

## Constraints

- **Embedding dimension**: 384 (must match existing HNSW indices and all-minilm model)
- **MCP library**: mark3labs/mcp-go (user preference)
- **Database**: Existing SurrealDB schema (no schema changes)
- **Ollama model**: all-minilm (local, no external API calls)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Big bang migration | User wants complete replacement, not gradual | — Pending |
| mark3labs/mcp-go | Most popular Go MCP library, user preference | — Pending |
| all-minilm via Ollama | 384-dim matches existing schema, local inference | — Pending |
| Skip contradiction detection | Requires NLI model, out of scope for v1 | — Pending |
| Skip HTTP endpoints | Frontend not needed for MCP-only usage | — Pending |

---
*Last updated: 2026-02-01 after initialization*
