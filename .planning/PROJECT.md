# memcp-go

## What This Is

A Go MCP server providing persistent memory for AI agents using SurrealDB. Enables Claude and other MCP clients to store and retrieve knowledge across sessions via semantic search, episodic memory, graph traversal, and procedural memory.

## Core Value

Agents can remember and recall knowledge across sessions with sub-second semantic search.

## Current State

**Version:** v1.0 (shipped 2026-02-03)

**Capabilities:**
- 19 MCP tools for knowledge management
- Hybrid search (BM25 + vector RRF fusion)
- Entity, Episode, and Procedure memory types
- Graph traversal (traverse, find_path)
- Maintenance operations (decay, similar pair detection)

**Tech stack:**
- Go 1.22+ with modelcontextprotocol/go-sdk
- SurrealDB v3.0 with WebSocket + auto-reconnect
- Ollama all-minilm (384-dim embeddings)
- 6,364 lines of Go across 39 files

**Known limitations:**
- Go SDK v1.2.0 CBOR decode issue with SurrealDB v3 graph range syntax
- Graph tools (traverse, find_path) may fail at runtime until SDK update

## Requirements

### Validated

- ✓ Go MCP server using modelcontextprotocol/go-sdk — v1.0
- ✓ SurrealDB connection with WebSocket and auto-reconnect — v1.0
- ✓ Ollama integration for all-minilm embeddings (384-dim) — v1.0
- ✓ Hybrid search (BM25 + vector similarity with RRF fusion) — v1.0
- ✓ Entity CRUD (upsert, get, delete) with embeddings — v1.0
- ✓ Episode CRUD (create, search, get, delete) — v1.0
- ✓ Procedure CRUD (create, search, get, delete, list) — v1.0
- ✓ Graph traversal (neighbors, path finding) — v1.0
- ✓ Maintenance operations (reflect with decay/similar) — v1.0
- ✓ Context/namespace filtering — v1.0
- ✓ MCP stdio transport — v1.0
- ✓ Unit tests for query functions (29/31 pass) — v1.0

### Active

(None — define in next milestone)

### Out of Scope

| Feature | Reason |
|---------|--------|
| Contradiction detection | Requires NLI model, defer to v2 |
| REST/GraphQL HTTP endpoints | Frontend not needed for MCP-only usage |
| Dashboard web UI | Separate concern, can add later |
| Document parsing (docling) | Python-only library, defer to v2 |

## Context

**Shipped v1.0** with complete Go rewrite:
- Migrated all 19 tools from Python
- SurrealDB v3.0 compatible
- Clean architecture (query layer, handler factories, composition root)

**Tech debt:**
- Go SDK CBOR issue with graph traversal (awaiting SDK update)
- Integration tests need running services for full verification

## Constraints

- **Embedding dimension**: 384 (matches existing HNSW indices)
- **MCP library**: modelcontextprotocol/go-sdk (official)
- **Database**: SurrealDB v3.0+
- **Ollama model**: all-minilm (local inference)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Big bang migration | User wants complete replacement | ✓ Good — clean codebase |
| modelcontextprotocol/go-sdk | Official SDK, better maintained | ✓ Good |
| all-minilm via Ollama | 384-dim matches existing schema | ✓ Good |
| Generic Embedder interface | Future-proof for other backends | ✓ Good |
| Query function layer | SQL isolation, testability | ✓ Good |
| Handler factory pattern | Clean DI, testable handlers | ✓ Good |
| SurrealDB v3 migration | Latest features, future-proof | ✓ Good (with SDK workarounds) |
| Skip contradiction detection | Requires NLI model, out of scope | ✓ Good — kept scope focused |
| Skip HTTP endpoints | Frontend not needed for MCP-only | ✓ Good — kept scope focused |

---
*Last updated: 2026-02-03 after v1.0 milestone*
