# Technology Stack

**Project:** memcp Go migration
**Researched:** 2026-02-01

## Recommended Stack

### Core Framework: MCP Server

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) | v1.2.0 | MCP protocol implementation | Official SDK, maintained by Anthropic + Google. GitHub recently migrated from mcp-go to this. Type-safe with generics, auto-schema generation from Go types |

**Rationale:**
- **Official SDK > Community SDK**: The official `modelcontextprotocol/go-sdk` is now recommended over `mark3labs/mcp-go`. GitHub's MCP server migrated to it in late 2025.
- **Type-safe tools**: Uses `AddTool[In, Out]` generic function that auto-generates JSON schemas from Go struct tags
- **Future-proof**: Will track MCP spec changes more closely than community alternatives
- **Transport options**: Supports stdio, SSE, HTTP streaming out of the box

**Alternative considered:** `mark3labs/mcp-go` v0.26+ - Still viable, 8.1k stars, larger existing user base. BUT: Official SDK is the strategic choice for new projects.

**Confidence:** HIGH (verified via [pkg.go.dev](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp), [GitHub](https://github.com/modelcontextprotocol/go-sdk))

### Database: SurrealDB Client

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| [surrealdb/surrealdb.go](https://github.com/surrealdb/surrealdb.go) | v1.2.0 | SurrealDB database client | Official SDK, generic Query[T] method, supports WebSocket + HTTP, relation queries |

**Rationale:**
- Only official Go client for SurrealDB
- Generic `Query[T]` method with type-safe results
- Supports parameterized queries (prevents injection)
- WebSocket for stateful connections, HTTP for stateless
- `contrib/rews` package for reliable WebSocket with auto-reconnect

**Key patterns:**
```go
// Connection
db, _ := surrealdb.FromEndpointURLString(ctx, "ws://localhost:8000")
db.SignIn(ctx, surrealdb.Auth{Username: "root", Password: "root"})
db.Use(ctx, "knowledge", "graph")

// Query with generics
results, _ := surrealdb.Query[[]Entity](ctx, db,
    "SELECT * FROM entity WHERE labels CONTAINSANY $labels",
    map[string]any{"labels": []string{"concept"}},
)
```

**Gotchas:**
- Uses CBOR encoding (not JSON) - use `models.CustomDateTime` instead of `time.Time`
- SDK is still "beta" status but actively maintained
- For RecordID: use `models.NewRecordID("entity", "myid")`

**Confidence:** HIGH (verified via [pkg.go.dev](https://pkg.go.dev/github.com/surrealdb/surrealdb.go), [SurrealDB docs](https://surrealdb.com/docs/sdk/golang))

### Embeddings: Ollama Client

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| [ollama/ollama/api](https://github.com/ollama/ollama) | v0.5.12+ | Embedding generation | Official client, native Go, supports `all-minilm` model for 384-dim embeddings |

**Rationale:**
- Official Ollama client maintained by Ollama team
- Direct HTTP to local Ollama server (no external API calls)
- `Embed` method supports batching multiple inputs
- Compatible with existing `all-minilm` model producing 384-dim vectors

**Key patterns:**
```go
import "github.com/ollama/ollama/api"

client, _ := api.ClientFromEnvironment()
resp, _ := client.Embed(ctx, &api.EmbedRequest{
    Model: "all-minilm",
    Input: "text to embed",
})
// resp.Embeddings[0] is []float32 with 384 dimensions
```

**Model:** `all-minilm` (46MB, 384 dimensions, 512 token context)
- Same model used in Python version via sentence-transformers
- HNSW indices already configured for 384 dimensions

**Confidence:** HIGH (verified via [pkg.go.dev](https://pkg.go.dev/github.com/ollama/ollama/api), [Ollama docs](https://docs.ollama.com/capabilities/embeddings))

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `log/slog` | stdlib | Structured logging | Always - built into Go 1.21+, JSON/text handlers |
| `github.com/stretchr/testify` | v1.9+ | Testing assertions | Unit/integration tests |
| `github.com/joho/godotenv` | v1.5+ | Environment loading | Development only |

**Logging approach:**
- Use `log/slog` (stdlib) - no external dependency needed
- Text handler for dev, JSON handler for production
- Pass logger via context or dependency injection

**Confidence:** HIGH (stdlib + well-established libraries)

## Go Version Requirements

| Requirement | Version | Reason |
|-------------|---------|--------|
| Go | 1.21+ | Required for `log/slog`, generics improvements |
| Recommended | 1.22+ | Better generic type inference, loop variable fix |

## Alternatives NOT Recommended

| Library | Why Not |
|---------|---------|
| `mark3labs/mcp-go` | Community SDK, official SDK now available and recommended |
| `go-surreal/som` | ORM approach adds complexity, raw queries better for SurrealQL |
| `zerolog`/`zap` | `log/slog` is now stdlib, sufficient for this use case |
| Direct HTTP to Ollama | Official client handles batching, error handling better |

## Installation

```bash
# Initialize module
go mod init github.com/your-org/memcp

# Core dependencies
go get github.com/modelcontextprotocol/go-sdk@v1.2.0
go get github.com/surrealdb/surrealdb.go@v1.2.0
go get github.com/ollama/ollama/api

# Dev dependencies
go get github.com/stretchr/testify
go get github.com/joho/godotenv
```

## Project Structure (Recommended)

```
memcp/
├── cmd/
│   └── memcp/
│       └── main.go           # Entry point, server setup
├── internal/
│   ├── db/
│   │   ├── client.go         # SurrealDB connection, lifespan
│   │   ├── entity.go         # Entity queries
│   │   ├── episode.go        # Episode queries
│   │   ├── procedure.go      # Procedure queries
│   │   └── relations.go      # Relation queries
│   ├── embedding/
│   │   └── ollama.go         # Ollama client wrapper
│   ├── tools/
│   │   ├── search.go         # search, semantic_search
│   │   ├── graph.go          # traverse, find_path, get_context
│   │   ├── persist.go        # remember, relate, forget
│   │   ├── maintenance.go    # reflect, decay, recalculate_importance
│   │   ├── episode.go        # log_episode, search_episodes
│   │   └── procedure.go      # save_procedure, get_procedure, search_procedures
│   └── models/
│       ├── entity.go         # Entity struct with json/cbor tags
│       ├── episode.go        # Episode struct
│       └── procedure.go      # Procedure struct
├── go.mod
├── go.sum
├── Dockerfile
└── README.md
```

**Key patterns:**
- `internal/` prevents external imports (Go convention)
- Separate `db/` from `tools/` - db is pure queries, tools are MCP handlers
- Models define types used by both db and tools
- Single `main.go` wires everything together

## Environment Variables

```bash
# SurrealDB connection (same as Python version)
SURREALDB_URL=ws://localhost:8000/rpc
SURREALDB_NAMESPACE=knowledge
SURREALDB_DATABASE=graph
SURREALDB_USER=root
SURREALDB_PASS=root

# Ollama (usually default)
OLLAMA_HOST=http://localhost:11434

# Logging
MEMCP_LOG_LEVEL=info  # debug, info, warn, error
MEMCP_LOG_FORMAT=text # text (dev) or json (prod)

# Context detection (same as Python)
MEMCP_DEFAULT_CONTEXT=
MEMCP_CONTEXT_FROM_CWD=false
```

## Type Mapping: Python to Go

| Python | Go | Notes |
|--------|-----|-------|
| `str` | `string` | |
| `list[str]` | `[]string` | |
| `list[float]` | `[]float32` | Ollama returns float32, SurrealDB accepts both |
| `dict[str, Any]` | `map[string]any` | |
| `datetime` | `models.CustomDateTime` | SurrealDB CBOR encoding |
| `Optional[str]` | `*string` or `string` with omitempty | |
| `RecordID` | `models.RecordID` | From surrealdb.go/pkg/models |

## Testing Strategy

```go
// Unit tests: mock DB interface
type DBQuerier interface {
    Query(ctx context.Context, sql string, vars map[string]any) error
}

// Integration tests: real SurrealDB in Docker
// Use testcontainers-go for ephemeral DB instances

// MCP tool tests: use InMemoryTransport from official SDK
transport := mcp.NewInMemoryTransport()
```

## Sources

- [modelcontextprotocol/go-sdk GitHub](https://github.com/modelcontextprotocol/go-sdk)
- [surrealdb/surrealdb.go pkg.go.dev](https://pkg.go.dev/github.com/surrealdb/surrealdb.go)
- [SurrealDB Go SDK Docs](https://surrealdb.com/docs/sdk/golang)
- [Ollama API pkg.go.dev](https://pkg.go.dev/github.com/ollama/ollama/api)
- [Ollama Embeddings Docs](https://docs.ollama.com/capabilities/embeddings)
- [Go slog Package](https://pkg.go.dev/log/slog)
- [GitHub MCP Server migration announcement](https://github.blog/changelog/2025-12-10-the-github-mcp-server-adds-support-for-tool-specific-configuration-and-more/)
