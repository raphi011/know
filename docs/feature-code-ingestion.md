# Feature: Code Ingestion with Source Attribution

## Overview

Enable knowhow to ingest source code from microservice repositories, providing semantic search with file:line source attribution in results.

## Problem Statement

Currently knowhow only supports markdown documentation. Teams need to:
- Query codebases semantically ("Where is authentication implemented?")
- Get precise source attribution in answers (file + line numbers)
- Search across multiple microservices with consistent UX

## Proposed Solution

### Approach: AST-based Chunking with Separate Tables

Use language-specific AST parsing (tree-sitter) to chunk code at semantic boundaries (functions, classes, properties). Store code chunks in dedicated `code_chunk` table to avoid polluting the existing markdown-focused `chunk` table.

### Why AST-based?

| Approach | Pros | Cons |
|----------|------|------|
| Regex/heuristic | Fast to build | Misses edge cases, breaks on complex syntax |
| **AST (tree-sitter)** | Accurate boundaries, language-agnostic framework | Requires per-language grammar |
| Full compiler | Most accurate | Heavy dependency, slow |

Tree-sitter is the sweet spot: fault-tolerant parsing (works on incomplete code), pure Go bindings available, same pattern extends to multiple languages.

### Why Separate Tables?

- **Clean schemas**: No optional fields cluttering existing chunk table
- **Optimized indexes**: Code needs symbol search; markdown needs BM25 fulltext
- **Extensibility**: Add `go_chunk`, `typescript_chunk` later with same pattern
- **Type safety**: All fields required (not optional) for their specific chunk type

## Data Model

### New Table: `code_chunk`

```sql
DEFINE TABLE code_chunk SCHEMAFULL;

-- Core
DEFINE FIELD entity ON code_chunk TYPE record<entity>;
DEFINE FIELD content ON code_chunk TYPE string;
DEFINE FIELD position ON code_chunk TYPE int;

-- Source Attribution (required for code)
DEFINE FIELD start_line ON code_chunk TYPE int;
DEFINE FIELD end_line ON code_chunk TYPE int;
DEFINE FIELD language ON code_chunk TYPE string;      -- "kotlin", "go", "typescript"
DEFINE FIELD node_type ON code_chunk TYPE string;     -- "function", "class", "property"
DEFINE FIELD symbol_name ON code_chunk TYPE string;   -- "processPayment"
DEFINE FIELD symbol_path ON code_chunk TYPE string;   -- "UserService > processPayment"

-- Organization & Search
DEFINE FIELD labels ON code_chunk TYPE array<string> DEFAULT [];
DEFINE FIELD embedding ON code_chunk TYPE array<float<32>>;
DEFINE FIELD created_at ON code_chunk TYPE datetime DEFAULT time::now();

-- Indexes
DEFINE INDEX idx_code_chunk_entity ON code_chunk FIELDS entity;
DEFINE INDEX idx_code_chunk_language ON code_chunk FIELDS language;
DEFINE INDEX idx_code_chunk_symbol ON code_chunk FIELDS symbol_name;
DEFINE INDEX hnsw_code_chunk_embedding ON code_chunk FIELDS embedding HNSW DIMENSION 1024 DIST COSINE;
```

### Go Models

```go
// CodeChunk represents a semantic code unit extracted via AST parsing.
type CodeChunk struct {
    ID         surrealmodels.RecordID `json:"id"`
    Entity     surrealmodels.RecordID `json:"entity"`
    Content    string                 `json:"content"`
    Position   int                    `json:"position"`

    // Source Attribution
    StartLine  int    `json:"start_line"`
    EndLine    int    `json:"end_line"`
    Language   string `json:"language"`    // "kotlin", "go", "typescript"
    NodeType   string `json:"node_type"`   // "function", "class", "property"
    SymbolName string `json:"symbol_name"` // "processPayment"
    SymbolPath string `json:"symbol_path"` // "UserService > processPayment"

    // Organization & Search
    Labels    []string  `json:"labels"`
    Embedding []float32 `json:"embedding"`
    CreatedAt time.Time `json:"created_at"`
}

// CodeChunkInput is the input structure for creating code chunks.
type CodeChunkInput struct {
    EntityID   string    `json:"entity_id"`
    Content    string    `json:"content"`
    Position   int       `json:"position"`
    StartLine  int       `json:"start_line"`
    EndLine    int       `json:"end_line"`
    Language   string    `json:"language"`
    NodeType   string    `json:"node_type"`
    SymbolName string    `json:"symbol_name"`
    SymbolPath string    `json:"symbol_path"`
    Labels     []string  `json:"labels,omitempty"`
    Embedding  []float32 `json:"embedding"`
}
```

### Chunk Boundaries

For Kotlin (initial language), chunks correspond to:

| AST Node | Chunk Type | Example |
|----------|------------|---------|
| `function_declaration` | function | `fun processPayment(...)` |
| `class_declaration` | class | `class UserService` |
| `property_declaration` | property | `val balance: Money` |
| `object_declaration` | object | `object Config` |
| `companion_object` | companion | `companion object` |

## Search Behavior

### Hybrid Search Across Chunk Types

When searching, combine results from both tables using RRF:

```
User: "How does authentication work?"

1. Vector search on `chunk` table (markdown docs)
2. Vector search on `code_chunk` table (source code)
3. RRF merge results
4. Return with source attribution
```

### Source Attribution Format

Search results include precise location:

```
UserService.kt:45-67 (fun authenticate)
│            │  │     │
│            │  │     └── symbol name + type
│            │  └── end line
│            └── start line
└── filename
```

## GraphQL Schema Updates

```graphql
type CodeChunkMatch {
  content: String!
  position: Int!
  startLine: Int!
  endLine: Int!
  language: String!
  nodeType: String!
  symbolName: String!
  symbolPath: String!
}

type EntitySearchResult {
  entity: Entity!
  matchedChunks: [ChunkMatch!]!         # markdown chunks
  matchedCodeChunks: [CodeChunkMatch!]! # code chunks (new)
  score: Float!
}
```

## User Experience

### Ingestion

```bash
# Scrape a Kotlin microservice
knowhow scrape /path/to/user-service --labels=user-service

# Scrape multiple services
knowhow scrape /path/to/services --recursive --labels=backend
```

### Querying

```bash
knowhow ask "Where is the PaymentProcessor class defined?"
# → Found in payment-service/src/main/kotlin/PaymentProcessor.kt:12-89

knowhow ask "How does user authentication work?"
# → Returns relevant code chunks + markdown docs with attribution
```

## Implementation Phases

### Phase 1: Schema & Models
- [ ] Create `CodeChunk` and `CodeChunkInput` models in `internal/models/code_chunk.go`
- [ ] Add `code_chunk` table to `internal/db/schema.go`
- [ ] Update GraphQL schema with `CodeChunkMatch` type in `internal/graph/schema.graphqls`
- [ ] Run `just generate` to regenerate GraphQL code

### Phase 2: Kotlin Parser
- [ ] Add `go-tree-sitter` dependency
- [ ] Add `tree-sitter-kotlin` grammar
- [ ] Create `internal/parser/kotlin.go` with:
  - `ParseKotlin(source []byte) (*tree_sitter.Tree, error)`
  - `ChunkKotlin(tree *tree_sitter.Tree, source []byte) ([]CodeChunkInput, error)`
- [ ] Unit tests for parser in `internal/parser/kotlin_test.go`

### Phase 3: Database Layer
- [ ] `CreateCodeChunks(chunks []CodeChunkInput) error` - batch insert
- [ ] `GetCodeChunks(entityID string) ([]CodeChunk, error)` - fetch by entity
- [ ] `SearchCodeChunks(embedding []float32, limit int) ([]CodeChunk, error)` - vector search
- [ ] Add cascade delete event for code_chunk when entity deleted

### Phase 4: Ingest Pipeline
- [ ] Update `CollectFiles()` to include `.kt`, `.kts` extensions
- [ ] Add `ingestKotlinFile()` method in `internal/service/ingest.go`
- [ ] Route to appropriate chunker based on file extension:
  - `.md` → `ChunkMarkdown()`
  - `.kt`, `.kts` → `ChunkKotlin()`

### Phase 5: Search Integration
- [ ] Update `SearchWithChunks()` to also search `code_chunk` table
- [ ] RRF merge results from both chunk types
- [ ] Update `internal/graph/helpers.go` with `codeChunkToGraphQL()` helper
- [ ] Update GraphQL resolvers to return `matchedCodeChunks`

## Dependencies

```go
// go.mod additions
require (
    github.com/tree-sitter/go-tree-sitter v0.24.0
)

// Kotlin grammar (vendored or runtime-loaded)
// Option A: github.com/tree-sitter-grammars/tree-sitter-kotlin
// Option B: Embed grammar binary in repo
```

## Future Extensions

| Language | Grammar | Priority |
|----------|---------|----------|
| Go | `tree-sitter-go` | High (backend services) |
| TypeScript | `tree-sitter-typescript` | Medium (frontend) |
| Java | `tree-sitter-java` | Medium (legacy services) |
| Python | `tree-sitter-python` | Low |

Each language follows the same pattern: parser + chunker + tests.

## Success Criteria

1. **Functional**: Can ingest Kotlin codebase and query it semantically
2. **Attribution**: Search results include accurate file:line references
3. **Performance**: Ingestion of 1000-file service < 5 minutes
4. **Accuracy**: Chunks preserve semantic boundaries (no split functions)

## Open Questions

1. **Nested symbols**: How deep to track symbol paths? (`Class > InnerClass > method` vs just `method`)
2. **Comments**: Include doc comments in chunk content or separate field?
3. **Imports**: Chunk import blocks or skip them?
4. **Large classes**: Split large classes into multiple chunks or keep as single unit?
