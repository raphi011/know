# Architecture Patterns: Go MCP Server

**Domain:** MCP server migration (Python to Go)
**Researched:** 2026-02-01
**Confidence:** HIGH (official Go guidance + official SDK docs)

## Recommended Architecture

```
memcp-go/
├── cmd/
│   └── memcp/
│       └── main.go              # Entry point, minimal code
├── internal/
│   ├── server/
│   │   └── server.go            # MCP server setup, tool/resource registration
│   ├── tools/
│   │   ├── search.go            # search, get_entity, list_labels, etc.
│   │   ├── persist.go           # remember, forget
│   │   ├── graph.go             # traverse, find_path
│   │   ├── episode.go           # log_episode, search_episodes
│   │   ├── procedure.go         # save_procedure, search_procedures
│   │   └── maintenance.go       # reflect, check_contradictions
│   ├── db/
│   │   ├── db.go                # Connection, lifecycle, run_query
│   │   ├── queries.go           # All SurrealQL query functions
│   │   └── schema.go            # Schema SQL constant
│   ├── embedding/
│   │   └── embedding.go         # Ollama embedding client
│   └── models/
│       └── models.go            # All structs (EntityResult, SearchResult, etc.)
├── go.mod
├── go.sum
└── README.md
```

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `cmd/memcp` | Entry point, config loading, server start | `internal/server` |
| `internal/server` | MCP server setup, tool registration, lifecycle | `internal/tools/*`, mcp-go SDK |
| `internal/tools/*` | Tool handlers (business logic) | `internal/db`, `internal/embedding`, `internal/models` |
| `internal/db` | SurrealDB connection, query execution | surrealdb.go SDK |
| `internal/embedding` | Ollama embedding generation | Ollama API (HTTP) |
| `internal/models` | Shared data structures | None (leaf package) |

### Data Flow

```
MCP Client (Claude Code)
        │
        ▼ (JSON-RPC over stdio)
    mcp-go SDK
        │
        ▼
  internal/server
        │
        ├──▶ internal/tools/search
        │         │
        │         ├──▶ internal/embedding (embed query)
        │         └──▶ internal/db (query_hybrid_search)
        │
        └──▶ internal/tools/persist
                  │
                  ├──▶ internal/embedding (embed content)
                  └──▶ internal/db (query_upsert_entity)
```

## Patterns to Follow

### Pattern 1: Constructor Injection (Idiomatic Go DI)

**What:** Pass dependencies through constructor functions (`NewXxx`), not globals.

**Why:** Explicit dependencies, testable, no hidden state.

**Example:**

```go
// internal/db/db.go
type DB struct {
    client *surrealdb.DB
    config Config
}

func NewDB(cfg Config) (*DB, error) {
    client, err := surrealdb.New(cfg.URL)
    if err != nil {
        return nil, fmt.Errorf("connect to surrealdb: %w", err)
    }
    // ... signin, use namespace/database
    return &DB{client: client, config: cfg}, nil
}

// internal/embedding/embedding.go
type Embedder struct {
    ollamaURL string
    model     string
}

func NewEmbedder(ollamaURL, model string) *Embedder {
    return &Embedder{ollamaURL: ollamaURL, model: model}
}

// internal/tools/search.go
type SearchService struct {
    db       *db.DB
    embedder *embedding.Embedder
}

func NewSearchService(db *db.DB, embedder *embedding.Embedder) *SearchService {
    return &SearchService{db: db, embedder: embedder}
}
```

### Pattern 2: Interface for Testing

**What:** Define small interfaces where you need to mock dependencies.

**Example:**

```go
// internal/tools/search.go
type Querier interface {
    HybridSearch(ctx context.Context, query string, embedding []float64, labels []string, limit int, context *string) ([]models.Entity, error)
    UpdateAccess(ctx context.Context, entityID string) error
}

type Embedder interface {
    Embed(ctx context.Context, text string) ([]float64, error)
}

type SearchService struct {
    db       Querier
    embedder Embedder
}
```

### Pattern 3: Tool Handler as Method

**What:** Register tool handlers as methods on service structs.

**Why:** Access to dependencies without globals, clean handler signatures.

**Example:**

```go
// internal/tools/search.go
func (s *SearchService) Search(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    query, _ := req.RequireString("query")
    limit, _ := req.GetInt("limit", 10)

    embedding, err := s.embedder.Embed(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("embed query: %w", err)
    }

    entities, err := s.db.HybridSearch(ctx, query, embedding, nil, limit, nil)
    if err != nil {
        return nil, fmt.Errorf("search: %w", err)
    }

    result := models.SearchResult{Entities: entities, Count: len(entities)}
    return mcp.NewToolResultJSON(result), nil
}

// internal/server/server.go
func RegisterTools(srv *server.MCPServer, searchSvc *tools.SearchService) {
    searchTool := mcp.NewTool("search",
        mcp.WithDescription("Search persistent memory..."),
        mcp.WithString("query", mcp.Required()),
        mcp.WithNumber("limit"),
    )
    srv.AddTool(searchTool, searchSvc.Search)
}
```

### Pattern 4: Composition Root in main.go

**What:** Wire all dependencies in `main.go`, nowhere else.

**Why:** Single place to understand how components connect.

**Example:**

```go
// cmd/memcp/main.go
func main() {
    cfg := loadConfig()

    // Create dependencies (bottom-up)
    db, err := db.NewDB(cfg.DB)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    embedder := embedding.NewEmbedder(cfg.OllamaURL, cfg.EmbeddingModel)

    // Create services
    searchSvc := tools.NewSearchService(db, embedder)
    persistSvc := tools.NewPersistService(db, embedder)
    graphSvc := tools.NewGraphService(db)
    episodeSvc := tools.NewEpisodeService(db, embedder)
    procedureSvc := tools.NewProcedureService(db, embedder)
    maintenanceSvc := tools.NewMaintenanceService(db, embedder)

    // Create MCP server
    srv := server.NewMCPServer("memcp", "1.0.0",
        server.WithToolCapabilities(true),
        server.WithResourceCapabilities(true),
    )

    // Register all tools
    server.RegisterSearchTools(srv, searchSvc)
    server.RegisterPersistTools(srv, persistSvc)
    server.RegisterGraphTools(srv, graphSvc)
    server.RegisterEpisodeTools(srv, episodeSvc)
    server.RegisterProcedureTools(srv, procedureSvc)
    server.RegisterMaintenanceTools(srv, maintenanceSvc)

    // Run
    server.ServeStdio(srv)
}
```

### Pattern 5: Error Wrapping

**What:** Wrap errors with context using `fmt.Errorf("...: %w", err)`.

**Why:** Preserves error chain, enables `errors.Is()` and `errors.As()`.

**Example:**

```go
func (d *DB) HybridSearch(ctx context.Context, query string, ...) ([]Entity, error) {
    result, err := d.client.Query(ctx, hybridSearchSQL, vars)
    if err != nil {
        return nil, fmt.Errorf("hybrid search query: %w", err)
    }
    // ...
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Global Package Variables for State

**What:** Using `var db *surrealdb.DB` at package level.

**Why bad:** Hidden dependencies, hard to test, race conditions.

**Instead:** Use constructor injection as shown above.

### Anti-Pattern 2: One Package per File

**What:** Creating `internal/search/`, `internal/persist/`, etc. for single files.

**Why bad:** Over-structuring. Go packages are not directories-for-organization.

**Instead:** Group related tools in `internal/tools/` with multiple files.

### Anti-Pattern 3: Circular Dependencies

**What:** `tools` importing `server`, `server` importing `tools`.

**Why bad:** Go compiler rejects circular imports.

**Instead:** Define interfaces in the consumer, implement in provider:

```go
// internal/server/server.go - defines what it needs
type ToolHandler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// internal/tools/search.go - provides implementation
func (s *SearchService) Search(...) (*mcp.CallToolResult, error) { ... }
```

### Anti-Pattern 4: Premature Abstraction

**What:** Creating interfaces before you have two implementations.

**Why bad:** Interfaces add indirection without value if there's only one implementation.

**Instead:** Start with concrete types. Extract interface when testing demands it or when you have a second implementation.

### Anti-Pattern 5: Framework-Based DI for Small Projects

**What:** Using Wire, Dig, or Fx for simple dependency graphs.

**Why bad:** Adds complexity, hides wiring, learning curve.

**Instead:** Manual constructor injection in `main.go`. Consider frameworks only for very large projects with 50+ components.

## Build Order Recommendations

Given the dependency graph, build in this order:

1. **Phase 1: Foundation**
   - `internal/models/` - Data structures (no dependencies)
   - `internal/db/` - SurrealDB client (depends on models)
   - `internal/embedding/` - Ollama client (standalone)

2. **Phase 2: Tools**
   - `internal/tools/search.go` - First tool (uses db, embedding, models)
   - `internal/tools/persist.go` - Second tool
   - Remaining tools one at a time

3. **Phase 3: Server**
   - `internal/server/` - MCP server setup, tool registration
   - `cmd/memcp/` - Main entry point, composition root

**Rationale:** Build leaves first (models), then shared infrastructure (db, embedding), then business logic (tools), then orchestration (server), then entry point (main).

## Key Differences from Python Structure

| Python | Go | Reason |
|--------|-----|--------|
| `memcp/servers/*.py` (sub-servers) | `internal/tools/*.go` (tool files) | Go: no sub-server concept in mcp-go |
| `memcp/db.py` (module) | `internal/db/` (package) | Go: package = directory |
| `memcp/models.py` | `internal/models/models.go` | Same structure, different convention |
| `FastMCP` sub-server mount | Direct tool registration | mcp-go uses flat tool registration |
| `@server.tool()` decorator | `srv.AddTool(tool, handler)` | Go: explicit registration, no decorators |
| `app_lifespan` context manager | `defer db.Close()` in main | Go: explicit cleanup with defer |

## Testing Strategy

```
internal/
├── db/
│   ├── db.go
│   └── db_test.go          # Integration tests (needs SurrealDB)
├── embedding/
│   ├── embedding.go
│   └── embedding_test.go   # Integration tests (needs Ollama)
├── tools/
│   ├── search.go
│   └── search_test.go      # Unit tests with mocks
└── models/
    ├── models.go
    └── models_test.go      # Unit tests for validation
```

**Mocking strategy:**
- Define interfaces in `tools/` for db and embedding dependencies
- Use interface implementations in tests
- Integration tests in `db/` and `embedding/` test real connections

## Sources

- [Official Go Module Layout Guide](https://go.dev/doc/modules/layout) - HIGH confidence
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) - MEDIUM (community, not official)
- [mcp-go by mark3labs](https://github.com/mark3labs/mcp-go) - HIGH (primary MCP SDK)
- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) - HIGH (alternative SDK option)
- [SurrealDB Go SDK](https://github.com/surrealdb/surrealdb.go) - HIGH (official SDK)
- [Go DI Best Practices](https://www.glukhov.org/post/2025/12/dependency-injection-in-go/) - MEDIUM
- [Parakeet (Ollama Go library)](https://github.com/parakeet-nest/parakeet) - MEDIUM (embedding reference)
