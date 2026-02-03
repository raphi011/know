# Knowhow Implementation Roadmap

> A personal knowledge RAG database - like Obsidian/second brain but searchable, indexable, and AI-augmented.

---

## Phase Overview

| Phase | Focus | Deliverables |
|-------|-------|--------------|
| **Phase 1** | Foundation | Schema, models, DB client, basic CLI scaffold |
| **Phase 2** | Core CRUD | Entity/chunk services, scrape command, embeddings |
| **Phase 3** | Search & Ask | Hybrid search, LLM integration, ask command |
| **Phase 4** | Relations | Manual linking, inferred relations during scrape |
| **Phase 5** | AI Features | Graph extraction, templates, token tracking |
| **Phase 6** | Polish | Export, multi-provider config, testing, docs |

---

## Phase 1: Foundation

**Goal:** Set up project structure, schema, and basic infrastructure.

### Tasks

- [ ] Create new `knowhow` directory structure
- [ ] Upgrade SurrealDB Go SDK v1.2.0 → v1.3.0
- [ ] Define SurrealDB schema (entity, chunk, template, relations, token_usage)
- [ ] Create Go models (Entity, Chunk, Template, Relation, TokenUsage)
- [ ] Reuse DB client from memcp (WebSocket, reconnect, CBOR)
- [ ] Reuse config loader from memcp (env-based)
- [ ] Scaffold CLI with cobra (no commands yet)

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
├── cmd/knowhow/
│   └── main.go
├── internal/
│   ├── config/           # Reuse from memcp
│   ├── db/
│   │   ├── client.go     # Reuse from memcp
│   │   ├── schema.go     # New schema
│   │   └── queries.go    # Entity/chunk queries
│   └── models/
│       ├── entity.go
│       ├── chunk.go
│       └── template.go
└── go.mod
```

### Verification

```bash
go build ./cmd/knowhow
surreal start --user root --pass root
go test ./internal/db/... -v
```

---

## Phase 2: Core CRUD

**Goal:** Entity and chunk CRUD operations, scrape command, embeddings.

### Tasks

- [ ] Integrate langchaingo for embeddings (Ollama default)
- [ ] Implement Markdown parser (frontmatter extraction)
- [ ] Implement semantic chunker (heading-aware, 500-1000 chars)
- [ ] Create EntityService (Create, Get, Update, Delete, List)
- [ ] Create IngestService (scrape files → entities + chunks)
- [ ] Implement `knowhow scrape <path>` command
- [ ] Implement `knowhow add "<content>"` command

### Chunking Strategy

```go
const (
    ChunkThreshold = 1500  // Only chunk if content > this
    ChunkTargetSize = 750  // Target chunk size
    ChunkOverlap = 100     // Overlap between chunks
)

func chunkMarkdown(content string) []Chunk {
    // 1. Split on heading boundaries (h1-h6)
    // 2. If chunk > 1000 chars, split on paragraphs
    // 3. Add 100 char overlap
    // 4. Track heading_path for each chunk
}
```

### CLI Commands

```bash
knowhow scrape ./docs/           # Ingest Markdown files
knowhow scrape ./docs/ --dry-run # Preview what would be created
knowhow add "Quick note" --type concept --labels "tech,note"
```

### Verification

```bash
knowhow scrape ./testdata/docs/
surreal sql --ns knowledge --db graph "SELECT count() FROM entity"
surreal sql --ns knowledge --db graph "SELECT count() FROM chunk"
```

---

## Phase 3: Search & Ask

**Goal:** Hybrid search and LLM-powered question answering.

### Tasks

- [ ] Integrate langchaingo for LLM completions
- [ ] Implement SearchService (hybrid RRF search)
- [ ] Implement LLMService (context assembly, generation)
- [ ] Implement `knowhow search "<query>"` command
- [ ] Implement `knowhow ask "<query>"` command
- [ ] Add `--template` flag to ask command

### Hybrid Search (RRF)

```surql
SELECT
    id, type, name, summary, labels, verified,
    array::group(matched_chunks) AS matched_chunks,
    count() AS relevance
FROM (
    SELECT id, type, name, summary, labels, verified,
           [] AS matched_chunks
    FROM entity WHERE embedding <|10,60|> $emb

    UNION ALL

    SELECT entity.id, entity.type, entity.name, entity.summary,
           entity.labels, entity.verified,
           [{ content: content, heading_path: heading_path }] AS matched_chunks
    FROM chunk WHERE embedding <|20,60|> $emb
)
GROUP BY id
ORDER BY relevance DESC
LIMIT $limit;
```

### CLI Commands

```bash
knowhow search "authentication"              # Search only
knowhow search "auth" --type service         # Filter by type
knowhow ask "How does auth work?"            # Search + LLM synthesis
knowhow ask "John Doe" --template "Peer Review"  # With template
```

### Verification

```bash
knowhow search "test" --count
knowhow ask "What services exist?"
```

---

## Phase 4: Relations

**Goal:** Entity linking - manual, inferred, and graph queries.

### Tasks

- [ ] Implement `knowhow link` command
- [ ] Add `--relates-to` flag to add command
- [ ] Detect [[wiki-links]] during scrape
- [ ] Detect @mentions during scrape
- [ ] Parse frontmatter `relates_to:` field
- [ ] Implement relation queries (get related entities)

### Relation Creation Methods

| Method | Source Value | Trigger |
|--------|--------------|---------|
| Manual CLI | `"manual"` | `knowhow link A B --type works_on` |
| Add flag | `"manual"` | `knowhow add --relates-to "A:about"` |
| Wiki-links | `"inferred"` | `[[entity-name]]` in content |
| Mentions | `"inferred"` | `@person-name` in content |
| Frontmatter | `"inferred"` | `relates_to: [a, b]` in YAML |

### CLI Commands

```bash
knowhow link "John Doe" "auth-service" --type "works_on"
knowhow link "auth-service" "user-service" --type "depends_on"
knowhow add "Note about auth" --relates-to "auth-service:about,john-doe:mentioned"
```

### Verification

```bash
surreal sql "SELECT ->relates_to->entity FROM entity:john_doe"
```

---

## Phase 5: AI Features

**Goal:** AI-powered graph extraction, templates, token tracking.

### Tasks

- [ ] Implement GraphRAG-style entity/relation extraction
- [ ] Add `--extract-graph` flag to scrape command
- [ ] Implement template CRUD (`knowhow template` commands)
- [ ] Implement token usage tracking
- [ ] Implement `knowhow usage` command
- [ ] Display token usage after operations

### Graph Extraction Prompt

```
You are a Knowledge Graph Specialist. Given the text below and these entity types
[person, service, concept, project, task], extract:

1. ENTITIES: All meaningful entities with name, type, and brief description
2. RELATIONS: Connections between entities with source, target, relation_type, description

Output format:
ENTITY|name|type|description
RELATION|source|target|rel_type|description

Text:
{chunk_content}

Existing entities that may be referenced:
{nearby_entity_names}
```

### CLI Commands

```bash
knowhow scrape ./docs --extract-graph     # AI relation detection
knowhow template list
knowhow template add ./templates/peer-review.md
knowhow usage                              # Token usage summary
knowhow usage --detailed --since "7 days"
```

### Verification

```bash
knowhow scrape ./testdata/ --extract-graph
surreal sql "SELECT count() FROM relates_to WHERE source = 'ai_detected'"
knowhow usage
```

---

## Phase 6: Polish

**Goal:** Export, multi-provider support, testing, documentation.

### Tasks

- [ ] Implement `knowhow export` command
- [ ] Implement `knowhow update` command
- [ ] Implement `knowhow delete` command (with confirmation)
- [ ] Add config file support (~/.config/knowhow/config.yaml)
- [ ] Support multiple LLM providers (Ollama, OpenAI, Anthropic)
- [ ] Write unit tests for services
- [ ] Write integration tests for DB queries
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

### Multi-Provider Config

```yaml
# ~/.config/knowhow/config.yaml
embedding:
  provider: ollama
  model: all-minilm:l6-v2

llm:
  provider: anthropic
  model: claude-3-haiku-20240307

database:
  url: ws://localhost:8000/rpc
  namespace: knowledge
  database: graph
```

### CLI Commands

```bash
knowhow export ./backup/
knowhow export ./backup/ --verified-only
knowhow update "auth-service" --verified
knowhow delete "old-entity" --force
```

### Verification

```bash
go test ./... -v
./scripts/e2e-test.sh
```

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Core model | Flexible entity | Store anything: people, services, tasks, documents |
| Embeddings | 384-dim Ollama | Local-first, can migrate later |
| Search | Hybrid RRF | Combines semantic + keyword |
| Relations | AI-detected optional | `--extract-graph` flag for richer graphs |
| Templates | Output synthesis | Fill templates with gathered knowledge |
| Token tracking | Always on | Monitor costs and usage |

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Transports                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │   CLI    │  │   TUI    │  │   MCP    │              │
│  │ (cobra)  │  │ (future) │  │ (future) │              │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘              │
│       └─────────────┼─────────────┘                     │
│                     │                                    │
├─────────────────────┼────────────────────────────────────┤
│              Service Layer (transport-agnostic)          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Entity    │  │   Search    │  │    LLM      │     │
│  │  Service    │  │   Service   │  │   Service   │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│                                                          │
├──────────────────────────────────────────────────────────┤
│                    Infrastructure                         │
│  ┌─────────────┐  ┌─────────────┐                       │
│  │  SurrealDB  │  │ langchaingo │                       │
│  │  (storage)  │  │ (LLM/embed) │                       │
│  └─────────────┘  └─────────────┘                       │
└──────────────────────────────────────────────────────────┘
```

---

## Future (Post-MVP)

- [ ] TUI (bubbletea) - document browser, search UI
- [ ] Folder/path hierarchy for navigation
- [ ] Review system (human-in-the-loop approval)
- [ ] GraphQL API layer
- [ ] MCP transport (reconnect existing tools)
- [ ] Contradiction detection between entities
- [ ] External integrations (calendar, email, todoist)
