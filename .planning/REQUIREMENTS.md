# Requirements: memcp-go

**Defined:** 2026-02-01
**Core Value:** Agents can remember and recall knowledge across sessions with sub-second semantic search

## v1 Requirements

Requirements for Go migration. Each maps to roadmap phases.

### Infrastructure

- [x] **INFRA-01**: Go module initialized with all dependencies (mcp-go-sdk, surrealdb.go, ollama)
- [x] **INFRA-02**: SurrealDB connection with WebSocket and auto-reconnect (rews package)
- [x] **INFRA-03**: Ollama embedding client generating 384-dimensional vectors
- [x] **INFRA-04**: Shared data models as Go structs with JSON tags
- [x] **INFRA-05**: Structured logging with slog (file + stderr)
- [x] **INFRA-06**: Environment variable configuration matching Python version

### MCP Server

- [x] **MCP-01**: MCP server using official modelcontextprotocol/go-sdk
- [x] **MCP-02**: Stdio transport for Claude Code integration
- [x] **MCP-03**: Tool registration framework with handler pattern

### Search Tools

- [x] **SRCH-01**: `search` tool — hybrid search combining BM25 and vector similarity (RRF fusion)
- [x] **SRCH-02**: `get_entity` tool — retrieve entity by ID
- [x] **SRCH-03**: `list_labels` tool — list all labels with counts
- [x] **SRCH-04**: `list_types` tool — list entity types with descriptions

### Persistence Tools

- [x] **PERS-01**: `remember` tool — store entities with embeddings (without contradiction detection)
- [x] **PERS-02**: `remember` tool — create relations between entities
- [x] **PERS-03**: `forget` tool — delete entity by ID

### Graph Tools

- [x] **GRPH-01**: `traverse` tool — get neighbors up to specified depth
- [x] **GRPH-02**: `find_path` tool — find shortest path between two entities

### Episode Tools

- [x] **EPSD-01**: `add_episode` tool — store episodic memory with embedding
- [x] **EPSD-02**: `search_episodes` tool — search episodes by content
- [x] **EPSD-03**: `get_episode` tool — retrieve episode by ID
- [x] **EPSD-04**: `delete_episode` tool — delete episode by ID

### Procedure Tools

- [x] **PROC-01**: `create_procedure` tool — store procedural memory with steps
- [x] **PROC-02**: `search_procedures` tool — search procedures by content
- [x] **PROC-03**: `get_procedure` tool — retrieve procedure by ID
- [x] **PROC-04**: `delete_procedure` tool — delete procedure by ID
- [x] **PROC-05**: `list_procedures` tool — list all procedures

### Maintenance Tools

- [ ] **MAINT-01**: `reflect` tool — apply decay to unused entities
- [ ] **MAINT-02**: `reflect` tool — identify similar entity pairs

### Testing

- [ ] **TEST-01**: Unit tests for all query functions
- [x] **TEST-02**: Integration tests with SurrealDB (requires running instance)
- [x] **TEST-03**: Integration tests with Ollama (requires running instance)
- [x] **TEST-04**: MCP tool tests using in-memory transport

## v2 Requirements

Deferred to future release. Not in current roadmap.

### Contradiction Detection

- **CNTR-01**: `check_contradictions` tool — detect contradictions between entities
- **CNTR-02**: Contradiction detection in `remember` via NLI model or API

### HTTP API

- **HTTP-01**: REST API endpoints for CRUD operations
- **HTTP-02**: GraphQL schema and resolvers
- **HTTP-03**: CORS configuration for dashboard

### Dashboard

- **DASH-01**: Next.js frontend for memory browsing
- **DASH-02**: Entity detail views
- **DASH-03**: Graph visualization

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| NLI contradiction detection | Requires ML model not available in Go, defer to v2 |
| REST/GraphQL HTTP endpoints | Frontend not needed for MCP-only usage |
| Dashboard web UI | Separate concern, can add in v2 |
| Document parsing (docling) | Python-only library, defer to v2 |
| Python code maintenance | Big bang replacement, not dual maintenance |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| INFRA-01 | Phase 1 | Complete |
| INFRA-02 | Phase 1 | Complete |
| INFRA-03 | Phase 1 | Complete |
| INFRA-04 | Phase 1 | Complete |
| INFRA-05 | Phase 1 | Complete |
| INFRA-06 | Phase 1 | Complete |
| MCP-01 | Phase 2 | Complete |
| MCP-02 | Phase 2 | Complete |
| MCP-03 | Phase 2 | Complete |
| SRCH-01 | Phase 3 | Complete |
| SRCH-02 | Phase 3 | Complete |
| SRCH-03 | Phase 3 | Complete |
| SRCH-04 | Phase 3 | Complete |
| PERS-01 | Phase 4 | Complete |
| PERS-02 | Phase 4 | Complete |
| PERS-03 | Phase 4 | Complete |
| GRPH-01 | Phase 5 | Complete |
| GRPH-02 | Phase 5 | Complete |
| EPSD-01 | Phase 6 | Complete |
| EPSD-02 | Phase 6 | Complete |
| EPSD-03 | Phase 6 | Complete |
| EPSD-04 | Phase 6 | Complete |
| PROC-01 | Phase 7 | Complete |
| PROC-02 | Phase 7 | Complete |
| PROC-03 | Phase 7 | Complete |
| PROC-04 | Phase 7 | Complete |
| PROC-05 | Phase 7 | Complete |
| MAINT-01 | Phase 8 | Pending |
| MAINT-02 | Phase 8 | Pending |
| TEST-01 | Phase 8 | Pending |
| TEST-02 | Phase 1 | Complete |
| TEST-03 | Phase 1 | Complete |
| TEST-04 | Phase 2 | Complete |

**Coverage:**
- v1 requirements: 31 total
- Mapped to phases: 31
- Unmapped: 0

---
*Requirements defined: 2026-02-01*
*Last updated: 2026-02-03 after Phase 7 completion*
