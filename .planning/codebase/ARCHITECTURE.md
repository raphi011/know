# Architecture

**Analysis Date:** 2026-02-01

## Pattern Overview

**Overall:** Modular MCP (Model Context Protocol) server with layered responsibilities. Uses FastMCP for the MCP protocol layer, sub-servers for feature organization, and SurrealDB as the knowledge graph backend.

**Key Characteristics:**
- Sub-server architecture: Each memory type (search, persist, episode, etc.) is an isolated FastMCP server mounted on the main server
- Separation of concerns: Database layer, model layer, and server/tool layer are separate
- GraphQL + REST API: FastAPI + Strawberry GraphQL for web UI
- Async-first: All database and I/O operations are async

## Layers

**Protocol Layer:**
- Purpose: MCP protocol implementation and tool/resource exposure
- Location: `memcp/server.py`
- Contains: Main FastMCP app, resource definitions, prompt templates
- Depends on: Sub-servers, database queries, models
- Used by: Claude via MCP stdio transport

**Sub-Server Layer:**
- Purpose: Organize MCP tools by feature domain (search, persist, episode, procedure, graph, maintenance)
- Location: `memcp/servers/` directory
- Contains: 6 FastMCP sub-servers, each defining tools for a specific capability
- Depends on: Database layer, models, utilities
- Used by: Main server (mounts all sub-servers)

**Sub-Servers:**
- `search.py`: Search, get_entity, list_labels, list_types - query memory
- `persist.py`: Remember, forget - store/delete knowledge
- `episode.py`: add_episode, search_episodes, get_episode, delete_episode - episodic memory
- `procedure.py`: create_procedure, search_procedures, get_procedure, delete_procedure - procedural memory
- `graph.py`: traverse, find_path - graph navigation
- `maintenance.py`: reflect, check_contradictions - memory optimization

**Database Layer:**
- Purpose: All SurrealDB interactions, query execution, connection management
- Location: `memcp/db.py` (1,300+ lines)
- Contains: Query functions (prefixed `query_`), connection management, lifespan handling, schema definition
- Depends on: SurrealDB driver (`surrealdb` async client), FastMCP Context
- Used by: All sub-servers and protocol layer

**Model Layer:**
- Purpose: Pydantic response models for type safety
- Location: `memcp/models.py`
- Contains: EntityResult, SearchResult, EpisodeResult, ProcedureResult, ReflectResult, MemoryStats
- Depends on: Pydantic
- Used by: All sub-servers and API

**Utility Layer:**
- Purpose: Cross-cutting concerns and helpers
- Location: `memcp/utils/` directory
- Contains: Embedding generation, contradiction checking, logging, record ID parsing
- Depends on: sentence-transformers, logging, subprocess
- Used by: Sub-servers and database layer

**Web API Layer (Optional):**
- Purpose: REST/GraphQL interface for dashboard UI
- Location: `memcp/api/main.py` and `memcp/api/schema.py`
- Contains: FastAPI app, GraphQL resolvers, Strawberry schema definitions
- Depends on: FastAPI, Strawberry, database layer
- Used by: Next.js dashboard frontend

**Frontend Layer (Optional):**
- Purpose: Web UI for memory browsing and management
- Location: `dashboard/` directory (Next.js app)
- Contains: React components, pages, lib utilities
- Depends on: Next.js, React, Recharts, GraphQL queries
- Used by: End users via browser

## Data Flow

**Search Flow:**
1. User/Claude calls `search()` tool via MCP
2. `search.py` receives query, validates input
3. Generates embedding for query via `embed()` utility
4. Calls `query_hybrid_search()` from database layer
5. Database executes BM25 keyword + vector similarity search (RRF fusion)
6. Results converted to `EntityResult` models
7. Access stats updated via `query_update_access()`
8. Returns SearchResult with entity list

**Persist Flow:**
1. User/Claude calls `remember()` tool with entities and relations
2. `persist.py` validates all entities and relations
3. Optional contradiction detection via `check_contradiction()` utility
4. For each entity: generate embedding, upsert to DB via `query_upsert_entity()`
5. For each relation: create via `query_create_relation()`
6. Recalculate importance scores via `query_recalculate_importance()`
7. Returns RememberResult with stored counts and contradictions

**Episode Flow:**
1. User/Claude calls `add_episode()` with full interaction content
2. Generate episode ID from timestamp
3. Generate embedding for episode content
4. Store episode via `query_create_episode()`
5. Optionally link extracted entities via `query_link_entity_to_episode()`
6. Returns EpisodeResult with ID and metadata

**Graph Traversal Flow:**
1. User calls `traverse()` with entity ID and depth
2. Database layer recursively queries relations
3. Builds neighbor relationships up to specified depth
4. Returns flattened neighbor list

**State Management:**
- Session state: Stored in FastMCP Context (passed between tools)
- Database connection: Managed by async context manager in `app_lifespan()`
- Embedding cache: Optional caching via `embed()` function
- Memory decay: Applied via scheduled `query_apply_decay()` calls

## Key Abstractions

**Entity:**
- Purpose: Core knowledge unit with semantic meaning
- Examples: `memcp/models.py` EntityResult, database entity table
- Pattern: Every entity has ID, content, embedding, labels, context, importance

**Relation:**
- Purpose: Represents relationships between entities
- Examples: `relates` table in SurrealDB, created via `query_create_relation()`
- Pattern: Typed edges (rel_type) with optional weight, prevents duplicates via unique_key

**Episode:**
- Purpose: First-class episodic memory - full interactions recorded as units
- Examples: Conversation sessions, meeting notes
- Pattern: Can have extracted entities linked via `extracted_from` relation

**Procedure:**
- Purpose: Structured procedural memory - step-by-step workflows
- Examples: Deployment procedures, setup guides
- Pattern: Ordered steps with optional flags, can be searched and linked

**Context (Project Namespace):**
- Purpose: Isolate memories by project/domain
- Examples: `memcp/db.py` detect_context() function
- Pattern: Auto-detected from git remote origin or CWD basename, filterable in queries

## Entry Points

**MCP Server:**
- Location: `memcp/server.py` main()
- Triggers: Invoked by Claude via stdio
- Responsibilities: Mount sub-servers, expose resources, start FastMCP lifespan

**Web API:**
- Location: `memcp/api/main.py` lifespan() and FastAPI app
- Triggers: HTTP requests to /graphql and /api/* endpoints
- Responsibilities: GraphQL resolver execution, CORS handling, static file serving

**Dashboard:**
- Location: `dashboard/app/layout.tsx` RootLayout
- Triggers: Browser navigation
- Responsibilities: Route mounting, context provider setup

## Error Handling

**Strategy:** Async exception handling with tool-level validation and database error recovery

**Patterns:**
- Input validation before DB operations (prevent invalid data)
- ToolError exceptions for user-facing issues (search validation, missing IDs)
- Try-catch for entity linking (graceful degradation if link fails)
- Connection retry logic in `app_lifespan()` with timeout
- Embedding generation fallback (truncates content if too long)
- Query timeout enforcement (QUERY_TIMEOUT env var, default 30s)

## Cross-Cutting Concerns

**Logging:**
- File-based logging via `memcp/utils/logging.py`
- Operation timing tracked via `log_op()` function
- Configurable level/destination via MEMCP_LOG_LEVEL and MEMCP_LOG_FILE env vars

**Validation:**
- Entity validation: Required id/content, optional type/labels/confidence
- Relation validation: Required from/to/type
- Context validation: Detects from git origin or defaults
- Entity type validation: Predefined types + custom allowed if env var set

**Authentication:**
- MCP: No auth (Claude handles protocol security)
- Web API: Optional authentication (not currently configured)
- SurrealDB: Credentials via env vars (SURREALDB_USER, SURREALDB_PASS)

**Caching:**
- Embedding cache: Optional in-memory cache (via get_embed_cache_stats())
- Access tracking: Updated on every search to support decay calculation
- No HTTP caching configured

---

*Architecture analysis: 2026-02-01*
