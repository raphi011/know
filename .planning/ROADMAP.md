# Roadmap: memcp-go

## Overview

This roadmap delivers a complete Go rewrite of the Python MCP server for AI agent memory persistence. Starting with foundation infrastructure (SurrealDB connection, Ollama embeddings), we progressively build tool capabilities: search (read-only) first, then persistence (writes), then graph traversal, episodic memory, procedural memory, and finally maintenance operations. Each phase delivers testable, deployable capability increments.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation** - SurrealDB, Ollama embeddings, models, logging ✓
- [x] **Phase 2: MCP Server** - Server setup, stdio transport, tool framework ✓
- [ ] **Phase 3: Search Tools** - Hybrid search, get_entity, list_labels, list_types
- [ ] **Phase 4: Persistence Tools** - remember, forget with entity/relation handling
- [ ] **Phase 5: Graph Tools** - traverse, find_path for relationship navigation
- [ ] **Phase 6: Episode Tools** - add_episode, search_episodes, get_episode, delete_episode
- [ ] **Phase 7: Procedure Tools** - CRUD and list for procedural memory
- [ ] **Phase 8: Maintenance Tools** - reflect (decay, similar pairs identification)

## Phase Details

### Phase 1: Foundation
**Goal**: Establish bulletproof infrastructure — DB connection, embeddings, models
**Depends on**: Nothing (first phase)
**Requirements**: INFRA-01, INFRA-02, INFRA-03, INFRA-04, INFRA-05, INFRA-06, TEST-02, TEST-03
**Success Criteria** (what must be TRUE):
  1. SurrealDB connection authenticates and persists across reconnection events
  2. Ollama client generates embeddings and returns exactly 384 dimensions
  3. Data models (Entity, Episode, Procedure, Relation) serialize to JSON matching Python schema
  4. Log output appears in both stderr and configured log file
  5. Integration tests pass with running SurrealDB and Ollama instances
**Plans**: 3 plans

Plans:
- [x] 01-01-PLAN.md — Go module init with dependencies, data models, logging
- [x] 01-02-PLAN.md — SurrealDB connection with rews auto-reconnect
- [x] 01-03-PLAN.md — Ollama embedding client with generic Embedder interface

### Phase 2: MCP Server
**Goal**: Working MCP server with tool registration framework, no business logic yet
**Depends on**: Phase 1
**Requirements**: MCP-01, MCP-02, MCP-03, TEST-04
**Success Criteria** (what must be TRUE):
  1. MCP server starts and accepts stdio connections
  2. Claude Code can connect and list available tools
  3. Tool handler pattern established (service injection, error handling)
  4. MCP tool tests pass using in-memory transport
**Plans**: 2 plans

Plans:
- [x] 02-01-PLAN.md — MCP server wrapper, middleware, main entry point
- [x] 02-02-PLAN.md — Tool framework with Dependencies, error helpers, ping tool

### Phase 3: Search Tools
**Goal**: Users can search and retrieve entities from memory
**Depends on**: Phase 2
**Requirements**: SRCH-01, SRCH-02, SRCH-03, SRCH-04
**Success Criteria** (what must be TRUE):
  1. User can search entities with hybrid search (BM25 + vector) returning ranked results
  2. User can retrieve entity by ID with full details
  3. User can list all labels with entity counts
  4. User can list entity types with descriptions
**Plans**: TBD

Plans:
- [ ] 03-01: search tool (hybrid BM25 + vector with RRF fusion)
- [ ] 03-02: get_entity, list_labels, list_types tools

### Phase 4: Persistence Tools
**Goal**: Users can store and delete entities and relations
**Depends on**: Phase 3
**Requirements**: PERS-01, PERS-02, PERS-03
**Success Criteria** (what must be TRUE):
  1. User can store new entities with auto-generated embeddings
  2. User can create relations between entities
  3. User can delete entities by ID
  4. Stored entities appear in subsequent searches
**Plans**: TBD

Plans:
- [ ] 04-01: remember tool (entity upsert with embeddings)
- [ ] 04-02: remember tool (relations), forget tool

### Phase 5: Graph Tools
**Goal**: Users can navigate entity relationships
**Depends on**: Phase 4
**Requirements**: GRPH-01, GRPH-02
**Success Criteria** (what must be TRUE):
  1. User can get neighbors up to specified depth
  2. User can find shortest path between two entities
  3. Results include relationship types and directions
**Plans**: TBD

Plans:
- [ ] 05-01: traverse tool (neighbors with depth)
- [ ] 05-02: find_path tool (shortest path)

### Phase 6: Episode Tools
**Goal**: Users can store and search episodic memories
**Depends on**: Phase 2
**Requirements**: EPSD-01, EPSD-02, EPSD-03, EPSD-04
**Success Criteria** (what must be TRUE):
  1. User can store episodes with content and timestamp
  2. User can search episodes by semantic content
  3. User can retrieve episode by ID
  4. User can delete episode by ID
**Plans**: TBD

Plans:
- [ ] 06-01: add_episode, get_episode, delete_episode tools
- [ ] 06-02: search_episodes tool

### Phase 7: Procedure Tools
**Goal**: Users can store and manage procedural memory (how-to knowledge)
**Depends on**: Phase 2
**Requirements**: PROC-01, PROC-02, PROC-03, PROC-04, PROC-05
**Success Criteria** (what must be TRUE):
  1. User can create procedures with ordered steps
  2. User can search procedures by content
  3. User can retrieve procedure by ID
  4. User can delete procedure by ID
  5. User can list all procedures
**Plans**: TBD

Plans:
- [ ] 07-01: create_procedure, get_procedure, delete_procedure tools
- [ ] 07-02: search_procedures, list_procedures tools

### Phase 8: Maintenance Tools
**Goal**: Users can run maintenance operations on memory store
**Depends on**: Phase 4
**Requirements**: MAINT-01, MAINT-02, TEST-01
**Success Criteria** (what must be TRUE):
  1. User can apply decay to unused entities (reduce importance scores)
  2. User can identify similar entity pairs for potential merging
  3. All query functions have unit test coverage
**Plans**: TBD

Plans:
- [ ] 08-01: reflect tool (decay, similar pairs)
- [ ] 08-02: Query function unit tests

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7 -> 8

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 3/3 | ✓ Complete | 2026-02-01 |
| 2. MCP Server | 2/2 | ✓ Complete | 2026-02-01 |
| 3. Search Tools | 0/2 | Not started | - |
| 4. Persistence Tools | 0/2 | Not started | - |
| 5. Graph Tools | 0/2 | Not started | - |
| 6. Episode Tools | 0/2 | Not started | - |
| 7. Procedure Tools | 0/2 | Not started | - |
| 8. Maintenance Tools | 0/2 | Not started | - |

---
*Roadmap created: 2026-02-01*
*Phase 2 complete: 2026-02-01*
*Total plans: 17 (estimated)*
*Total requirements: 31*
