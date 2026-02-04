# Knowhow Implementation Roadmap

> A personal knowledge RAG database - like Obsidian/second brain but searchable, indexable, and AI-augmented.

---

## Phase Overview

| Phase | Focus | Status |
|-------|-------|--------|
| **Phase 1** | Foundation | ✅ Complete |
| **Phase 2** | Core CRUD | ✅ Complete |
| **Phase 3** | Search & Ask | ✅ Complete |
| **Phase 4** | Relations | ✅ Complete |
| **Phase 5** | AI Features | ✅ Complete |
| **Phase 6** | Polish | ✅ Mostly Complete |

---

## Phase 1: Foundation

**Goal:** Set up project structure, schema, GraphQL server, and basic infrastructure.

### Tasks

- [x] Create new `knowhow` directory structure
- [x] Upgrade SurrealDB Go SDK v1.2.0 → v1.3.0
- [x] Define SurrealDB schema (entity, chunk, template, relations, token_usage)
- [x] Create Go models (Entity, Chunk, Template, Relation, TokenUsage)
- [x] Reuse DB client from memcp (WebSocket, reconnect, CBOR)
- [x] Reuse config loader from memcp (env-based)
- [x] Set up GraphQL server with gqlgen (`cmd/knowhow-server`)
- [x] Define GraphQL schema (queries, mutations, subscriptions)
- [x] Scaffold CLI as GraphQL client (`cmd/knowhow`)

### Schema

```surql
-- Entity (flexible knowledge atom)
DEFINE TABLE entity SCHEMAFULL;
DEFINE FIELD type ON entity TYPE string;
DEFINE FIELD name ON entity TYPE string;
DEFINE FIELD content ON entity TYPE option<string>;
DEFINE FIELD summary ON entity TYPE option<string>;
DEFINE FIELD labels ON entity TYPE array<string>;
DEFINE FIELD verified ON entity TYPE bool DEFAULT false;
DEFINE FIELD confidence ON entity TYPE float DEFAULT 0.5;
DEFINE FIELD source ON entity TYPE string DEFAULT "manual";
DEFINE FIELD source_path ON entity TYPE option<string>;
DEFINE FIELD metadata ON entity TYPE option<object> FLEXIBLE;
DEFINE FIELD embedding ON entity TYPE option<array<float>>;
DEFINE FIELD created_at ON entity TYPE datetime DEFAULT time::now();
DEFINE FIELD updated_at ON entity TYPE datetime VALUE time::now();
DEFINE FIELD accessed ON entity TYPE datetime DEFAULT time::now();
DEFINE FIELD access_count ON entity TYPE int DEFAULT 0;

-- Indexes
DEFINE INDEX idx_entity_type ON entity FIELDS type;
DEFINE INDEX idx_entity_labels ON entity FIELDS labels;
DEFINE INDEX idx_entity_verified ON entity FIELDS verified;
DEFINE ANALYZER entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
DEFINE INDEX idx_entity_content_ft ON entity FIELDS content FULLTEXT ANALYZER entity_analyzer BM25;
DEFINE INDEX idx_entity_embedding ON entity FIELDS embedding
    HNSW DIMENSION 384 DIST COSINE TYPE F32 EFC 150 M 12;

-- Chunk (RAG pieces for long content)
DEFINE TABLE chunk SCHEMAFULL;
DEFINE FIELD entity ON chunk TYPE record<entity>;
DEFINE FIELD content ON chunk TYPE string;
DEFINE FIELD position ON chunk TYPE int;
DEFINE FIELD heading_path ON chunk TYPE option<string>;
DEFINE FIELD labels ON chunk TYPE array<string>;
DEFINE FIELD embedding ON chunk TYPE array<float>;
DEFINE FIELD created_at ON chunk TYPE datetime DEFAULT time::now();

DEFINE INDEX idx_chunk_entity ON chunk FIELDS entity;
DEFINE INDEX idx_chunk_labels ON chunk FIELDS labels;
DEFINE INDEX idx_chunk_embedding ON chunk FIELDS embedding
    HNSW DIMENSION 384 DIST COSINE TYPE F32 EFC 150 M 12;

-- Cascade delete chunks
DEFINE EVENT cascade_delete_chunks ON entity
WHEN $event = "DELETE" THEN {
    DELETE FROM chunk WHERE entity = $before.id
};

-- Template (output rendering)
DEFINE TABLE template SCHEMAFULL;
DEFINE FIELD name ON template TYPE string;
DEFINE FIELD description ON template TYPE option<string>;
DEFINE FIELD content ON template TYPE string;
DEFINE FIELD created_at ON template TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_template_name ON template FIELDS name UNIQUE;

-- Relations
DEFINE TABLE relates_to SCHEMAFULL TYPE RELATION FROM entity TO entity;
DEFINE FIELD rel_type ON relates_to TYPE string;
DEFINE FIELD strength ON relates_to TYPE float DEFAULT 1.0;
DEFINE FIELD source ON relates_to TYPE string DEFAULT "manual";
DEFINE FIELD metadata ON relates_to TYPE option<object> FLEXIBLE;
DEFINE FIELD created_at ON relates_to TYPE datetime DEFAULT time::now();

-- Cascade delete relations
DEFINE EVENT cascade_delete_relations ON entity
WHEN $event = "DELETE" THEN {
    DELETE FROM relates_to WHERE in = $before.id OR out = $before.id
};

-- Token usage tracking
DEFINE TABLE token_usage SCHEMAFULL;
DEFINE FIELD operation ON token_usage TYPE string;
DEFINE FIELD model ON token_usage TYPE string;
DEFINE FIELD input_tokens ON token_usage TYPE int;
DEFINE FIELD output_tokens ON token_usage TYPE int;
DEFINE FIELD total_tokens ON token_usage TYPE int;
DEFINE FIELD cost_usd ON token_usage TYPE option<float>;
DEFINE FIELD entity_id ON token_usage TYPE option<string>;
DEFINE FIELD created_at ON token_usage TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_usage_operation ON token_usage FIELDS operation;
DEFINE INDEX idx_usage_created ON token_usage FIELDS created_at;
```

### Directory Structure

```
knowhow/
├── cmd/
│   ├── knowhow/              # CLI (GraphQL client)
│   │   └── main.go
│   └── knowhow-server/       # GraphQL server
│       └── main.go
├── internal/
│   ├── config/               # Shared config (env-based)
│   ├── db/                   # Server-side only
│   │   ├── client.go
│   │   ├── schema.go
│   │   └── queries.go
│   ├── models/               # Shared between server & CLI
│   │   ├── entity.go
│   │   ├── chunk.go
│   │   └── template.go
│   ├── graph/                # gqlgen generated + resolvers
│   │   ├── schema.graphqls   # GraphQL schema
│   │   ├── generated.go      # gqlgen output
│   │   └── resolver.go       # Query/mutation implementations
│   ├── service/              # Server-side business logic
│   │   ├── entity.go
│   │   ├── search.go
│   │   └── ingest.go
│   ├── llm/                  # Server-side LLM integration
│   │   ├── embedder.go
│   │   └── model.go
│   └── client/               # CLI GraphQL client
│       └── client.go
└── go.mod
```

---

## Phase 2: Core CRUD

**Goal:** Entity and chunk CRUD operations, scrape command, embeddings.

### Tasks

- [x] Integrate langchaingo for embeddings (Ollama default)
- [x] Implement Markdown parser (frontmatter extraction)
- [x] Implement semantic chunker (heading-aware, 500-1000 chars)
- [x] Create EntityService (Create, Get, Update, Delete, List)
- [x] Create IngestService (scrape files → entities + chunks)
- [x] Implement `knowhow scrape <path>` command
- [x] Implement `knowhow add "<content>"` command

### CLI Commands

```bash
knowhow scrape ./docs/           # Ingest Markdown files
knowhow scrape ./docs/ --dry-run # Preview what would be created
knowhow add "Quick note" --type concept --labels "tech,note"
```

---

## Phase 3: Search & Ask

**Goal:** Hybrid search and LLM-powered question answering.

### Tasks

- [x] Integrate langchaingo for LLM completions
- [x] Implement SearchService (hybrid RRF search)
- [x] Implement LLMService (context assembly, generation)
- [x] Implement `knowhow search "<query>"` command
- [x] Implement `knowhow ask "<query>"` command
- [x] Add `--template` flag to ask command

### CLI Commands

```bash
knowhow search "authentication"              # Search only
knowhow search "auth" --type service         # Filter by type
knowhow ask "How does auth work?"            # Search + LLM synthesis
knowhow ask "John Doe" --template "Peer Review"  # With template
```

---

## Phase 4: Relations

**Goal:** Entity linking - manual, inferred, and graph queries.

### Tasks

- [x] Implement `knowhow link` command
- [x] Add `--relates-to` flag to add command
- [x] Detect [[wiki-links]] during scrape
- [x] Parse frontmatter `relates_to:` field
- [x] Implement relation queries (get related entities)

### Relation Creation Methods

| Method | Source Value | Trigger |
|--------|--------------|---------|
| Manual CLI | `"manual"` | `knowhow link A B --type works_on` |
| Add flag | `"manual"` | `knowhow add --relates-to "A:about"` |
| Wiki-links | `"inferred"` | `[[entity-name]]` in content |
| Frontmatter | `"inferred"` | `relates_to: [a, b]` in YAML |

### CLI Commands

```bash
knowhow link "John Doe" "auth-service" --type "works_on"
knowhow link "auth-service" "user-service" --type "depends_on"
knowhow add "Note about auth" --relates-to "auth-service:about,john-doe:mentioned"
```

---

## Phase 5: AI Features

**Goal:** AI-powered graph extraction, templates, token tracking.

### Tasks

- [x] Implement GraphRAG-style entity/relation extraction
- [x] Add `--extract-graph` flag to scrape command
- [x] Implement template CRUD (`knowhow template` commands)
- [x] Implement token usage tracking
- [x] Implement `knowhow usage` command

### CLI Commands

```bash
knowhow scrape ./docs --extract-graph     # AI relation detection
knowhow template list
knowhow template add ./templates/peer-review.md
knowhow usage                              # Token usage summary
knowhow usage --detailed --since "7 days"
```

---

## Phase 6: Polish

**Goal:** Export, multi-provider support, testing, documentation.

### Tasks

- [x] Implement `knowhow export` command
- [x] Implement `knowhow update` command
- [x] Implement `knowhow delete` command (with confirmation)
- [ ] Add config file support (~/.config/knowhow/config.yaml)
- [x] Support multiple LLM providers (Ollama, OpenAI, Anthropic)
- [x] Write unit tests for services
- [x] Write integration tests for DB queries
- [ ] Write e2e test script
- [ ] Update README with examples

### Export Format

```
backup/
├── entities/
│   ├── person/
│   │   └── john-doe.md
│   └── service/
│       └── auth-service.md
├── templates/
│   └── peer-review.md
├── relations.json
└── metadata.json
```

### CLI Commands

```bash
knowhow export ./backup/
knowhow export ./backup/ --verified-only
knowhow update "auth-service" --verified
knowhow delete "old-entity" --force
```

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Architecture | Client-server via GraphQL | CLI is thin client; server owns DB/LLM connections |
| API | gqlgen (GraphQL) | Type-safe, code-gen, supports subscriptions for future TUI |
| Core model | Flexible entity | Store anything: people, services, tasks, documents |
| Embeddings | 384-dim Ollama | Local-first, can migrate later |
| Search | Hybrid RRF | Combines semantic + keyword |
| Relations | AI-detected optional | `--extract-graph` flag for richer graphs |
| Templates | Output synthesis | Fill templates with gathered knowledge |
| Token tracking | Always on | Monitor costs and usage |

---

## Architecture

```
                    ┌──────────┐  ┌──────────┐
                    │   CLI    │  │   TUI    │
                    │ (cobra)  │  │ (future) │
                    └────┬─────┘  └────┬─────┘
                         └──────┬──────┘
                                │ HTTP/GraphQL
┌───────────────────────────────┼─────────────────────────────────────┐
│                               ▼                                      │
│                        knowhow-server                                │
│  ┌────────────────────────────────────────────┐  ┌──────────────┐  │
│  │           GraphQL Resolvers (gqlgen)       │  │     MCP      │  │
│  │  - queries (search, get, list)             │  │   (future)   │  │
│  │  - mutations (add, update, delete, scrape) │  │   (stdio)    │  │
│  └────────────────────┬───────────────────────┘  └───────┬──────┘  │
│                       │                                   │         │
│                       └─────────────┬─────────────────────┘         │
│                                     ▼                                │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │              Service Layer (transport-agnostic)               │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │  │
│  │  │   Entity    │  │   Search    │  │   Ingest    │          │  │
│  │  │  Service    │  │   Service   │  │   Service   │          │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘          │  │
│  └────────────────────────────┬─────────────────────────────────┘  │
│                               │                                      │
│  ┌────────────────────────────▼─────────────────────────────────┐  │
│  │                    Infrastructure                             │  │
│  │  ┌─────────────┐  ┌─────────────┐                            │  │
│  │  │  SurrealDB  │  │ langchaingo │                            │  │
│  │  │  (storage)  │  │ (LLM/embed) │                            │  │
│  │  └─────────────┘  └─────────────┘                            │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Remaining Work

- [ ] Config file support (~/.config/knowhow/config.yaml)
- [ ] E2E test script
- [ ] README with usage examples

---

## Future (Post-MVP)

- [ ] TUI (bubbletea) - document browser, search UI
- [ ] Folder/path hierarchy for navigation
- [ ] Review system (human-in-the-loop approval)
- [ ] MCP transport (reconnect existing tools)
- [ ] Contradiction detection between entities
- [ ] External integrations (calendar, email, todoist)
