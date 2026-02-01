# Phase 1: Foundation - Research

**Researched:** 2026-02-01
**Domain:** Go MCP server infrastructure (SurrealDB, Ollama embeddings, slog logging)
**Confidence:** HIGH

## Summary

Phase 1 establishes the infrastructure foundation for the Go MCP server: database connectivity with auto-reconnect, embedding generation, data models, and dual-output logging. The research verified exact library versions, connection patterns, and confirmed critical requirements like the 384-dimensional embedding constraint.

The Go ecosystem provides mature, well-documented solutions for all requirements. The official MCP Go SDK (v1.2.0) offers type-safe tool registration with automatic JSON schema generation. SurrealDB's Go SDK (v1.2.0) includes a `contrib/rews` package for reliable WebSocket connections with automatic session restoration. Ollama's official Go client provides the `Embed` method returning `[]float32` embeddings.

**Primary recommendation:** Use constructor injection with a composition root in `main.go`. All dependencies are wired explicitly - no globals, no frameworks.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) | v1.2.0 | MCP protocol implementation | Official SDK (Anthropic + Google), type-safe `AddTool[In,Out]` with auto schema generation |
| [surrealdb/surrealdb.go](https://github.com/surrealdb/surrealdb.go) | v1.2.0 | SurrealDB client | Official SDK, generic `Query[T]`, WebSocket + CBOR support |
| [surrealdb.go/contrib/rews](https://pkg.go.dev/github.com/surrealdb/surrealdb.go/contrib/rews) | v1.2.0 | Auto-reconnecting WebSocket | Session restoration (auth, namespace, live queries) after reconnect |
| [ollama/ollama/api](https://pkg.go.dev/github.com/ollama/ollama/api) | v0.15.3 | Embedding generation | Official Ollama client, `Embed` method with batch support |
| [log/slog](https://pkg.go.dev/log/slog) | stdlib | Structured logging | Go 1.21+ stdlib, JSON/text handlers, no external dependency |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| [samber/slog-multi](https://github.com/samber/slog-multi) | v1.3+ | Multi-handler logging | Dual output (stderr + file) in single logger |
| [stretchr/testify](https://github.com/stretchr/testify) | v1.9+ | Test assertions | Unit and integration tests |
| [joho/godotenv](https://github.com/joho/godotenv) | v1.5+ | .env file loading | Development only |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| modelcontextprotocol/go-sdk | mark3labs/mcp-go | Community SDK, larger user base, but official SDK is strategic choice for new projects |
| slog + slog-multi | zerolog/zap | More features, but slog is stdlib and sufficient |
| godotenv | envconfig | envconfig offers struct binding, but godotenv is simpler for this use case |

**Installation:**
```bash
go mod init github.com/your-org/memcp-go

# Core dependencies
go get github.com/modelcontextprotocol/go-sdk@v1.2.0
go get github.com/surrealdb/surrealdb.go@v1.2.0
go get github.com/ollama/ollama/api

# Supporting
go get github.com/samber/slog-multi
go get github.com/stretchr/testify

# Dev only
go get github.com/joho/godotenv
```

## Architecture Patterns

### Recommended Project Structure

```
memcp-go/
├── cmd/
│   └── memcp/
│       └── main.go           # Composition root, minimal code
├── internal/
│   ├── config/
│   │   └── config.go         # Environment variable loading
│   ├── db/
│   │   ├── client.go         # SurrealDB connection with rews
│   │   └── schema.go         # Schema SQL constant
│   ├── embedding/
│   │   └── ollama.go         # Ollama embedding client
│   └── models/
│       ├── entity.go         # Entity struct
│       ├── episode.go        # Episode struct
│       ├── procedure.go      # Procedure struct
│       └── relation.go       # Relation struct
├── go.mod
├── go.sum
└── README.md
```

### Pattern 1: Constructor Injection

**What:** Pass dependencies through constructor functions, not globals.

**When to use:** Always - this is the idiomatic Go DI pattern.

**Example:**
```go
// internal/db/client.go
type Client struct {
    conn *rews.Connection
    db   *surrealdb.DB
}

func NewClient(cfg Config) (*Client, error) {
    // Create rews connection with auto-reconnect
    conn := rews.New(
        func(ctx context.Context) (connection.WebSocketConnection, error) {
            return gws.New(&connection.Config{
                BaseURL:     cfg.URL,
                Marshaler:   cbor.NewMarshaler(),
                Unmarshaler: cbor.NewUnmarshaler(),
            }), nil
        },
        5*time.Second,
        cbor.NewUnmarshaler(),
        slog.Default(),
    )

    // Configure exponential backoff
    retryer := rews.NewExponentialBackoffRetryer()
    retryer.InitialDelay = 1 * time.Second
    retryer.MaxDelay = 30 * time.Second
    retryer.MaxRetries = 10
    conn.Retryer = retryer

    if err := conn.Connect(ctx); err != nil {
        return nil, fmt.Errorf("connect: %w", err)
    }

    db, err := surrealdb.FromConnection(ctx, conn)
    if err != nil {
        return nil, fmt.Errorf("from connection: %w", err)
    }

    return &Client{conn: conn, db: db}, nil
}
```

### Pattern 2: Composition Root in main.go

**What:** Wire all dependencies in `main.go`, nowhere else.

**When to use:** Application entry point.

**Example:**
```go
// cmd/memcp/main.go
func main() {
    cfg := config.Load()

    // Setup logging
    logger := setupLogger(cfg.LogFile, cfg.LogLevel)
    slog.SetDefault(logger)

    // Create dependencies bottom-up
    dbClient, err := db.NewClient(cfg.DB)
    if err != nil {
        slog.Error("failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer dbClient.Close()

    embedder := embedding.NewOllamaClient(cfg.OllamaHost, cfg.EmbeddingModel)

    // Initialize schema
    if err := dbClient.InitSchema(context.Background()); err != nil {
        slog.Error("failed to initialize schema", "error", err)
        os.Exit(1)
    }

    slog.Info("memcp-go ready")
    // Server setup would go here in Phase 2
}
```

### Pattern 3: slog Multi-Handler for Dual Output

**What:** Log to both stderr and file simultaneously.

**Example:**
```go
// internal/config/logging.go
import (
    "log/slog"
    "os"
    slogmulti "github.com/samber/slog-multi"
)

func SetupLogger(logFile string, level slog.Level) *slog.Logger {
    // Stderr handler (text for readability)
    stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: level,
    })

    // File handler (JSON for machine parsing)
    file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        slog.Error("failed to open log file", "error", err)
        return slog.New(stderrHandler)
    }

    fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
        Level: level,
    })

    // Fanout to both handlers
    return slog.New(slogmulti.Fanout(stderrHandler, fileHandler))
}
```

### Anti-Patterns to Avoid

- **Global package variables:** `var db *surrealdb.DB` at package level - hard to test, race conditions
- **One package per file:** Don't create `internal/entity/`, `internal/episode/` for single files - use `internal/models/` with multiple files
- **Framework-based DI:** Wire, Dig, Fx add complexity without benefit for this project size

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| WebSocket reconnection | Custom reconnect loop | `contrib/rews` package | Session restoration (auth, namespace, live queries) is complex |
| Embedding generation | Direct HTTP to Ollama | `ollama/ollama/api` client | Handles batching, errors, environment config |
| JSON schema from structs | Manual schema definition | MCP SDK `AddTool[In,Out]` | Auto-generates from struct tags |
| Multi-output logging | Custom io.MultiWriter | `slog-multi` Fanout | Proper handler composition with level filtering |

**Key insight:** The SurrealDB WebSocket connection is stateful - authentication, namespace selection, and session variables must all be restored after reconnection. The `rews` package handles this automatically.

## Common Pitfalls

### Pitfall 1: Embedding Dimension Mismatch

**What goes wrong:** Embeddings generated with wrong dimensions fail HNSW index queries silently or with cryptic errors.

**Why it happens:** Different models produce different dimensions. Using wrong model or truncating embeddings.

**How to avoid:**
- ALWAYS use `all-minilm:l6-v2` model (384 dimensions)
- Verify embedding length: `if len(embedding) != 384 { return error }`
- Existing HNSW indices are configured for DIMENSION 384

**Warning signs:** Empty search results, index errors, cosine similarity returning unexpected values.

### Pitfall 2: SurrealDB CBOR Encoding for Time

**What goes wrong:** `time.Time` fields don't serialize/deserialize correctly with SurrealDB.

**Why it happens:** SurrealDB uses CBOR encoding with custom datetime tags, not JSON.

**How to avoid:**
- Use `models.CustomDateTime` from surrealdb.go instead of `time.Time`
- Or use string timestamps and parse client-side

**Warning signs:** Timezone issues, nil timestamps, panics during unmarshal.

### Pitfall 3: WebSocket State Not Restored

**What goes wrong:** After network interruption, queries fail with "not authenticated" or "no namespace selected".

**Why it happens:** Using base WebSocket connection without session restoration.

**How to avoid:**
- ALWAYS use `contrib/rews` package, not direct WebSocket
- Configure retryer with appropriate delays
- Test reconnection in integration tests

**Warning signs:** Intermittent auth failures, queries failing after network blip.

### Pitfall 4: Float32 vs Float64 Embeddings

**What goes wrong:** Type mismatch when storing/querying embeddings.

**Why it happens:** Ollama returns `[]float32`, but some code expects `[]float64`.

**How to avoid:**
- Ollama `EmbedResponse.Embeddings` is `[][]float32`
- SurrealDB accepts `[]float32` for vector fields
- Keep everything as `float32` throughout

**Warning signs:** Type assertion failures, unexpected embedding values.

### Pitfall 5: Missing JSON Tags on Models

**What goes wrong:** MCP schema generation fails or produces wrong field names.

**Why it happens:** Go struct field names are PascalCase, JSON expects snake_case.

**How to avoid:**
- ALWAYS add `json:"field_name"` tags
- Add `jsonschema:"description"` for MCP tool input docs

**Warning signs:** Empty tool parameters, mismatched field names in requests.

## Code Examples

### SurrealDB Connection with rews Auto-Reconnect

```go
// Source: https://pkg.go.dev/github.com/surrealdb/surrealdb.go/contrib/rews
package db

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/surrealdb/surrealdb.go"
    "github.com/surrealdb/surrealdb.go/contrib/rews"
    "github.com/surrealdb/surrealdb.go/pkg/connection"
    "github.com/surrealdb/surrealdb.go/pkg/connection/gws"
    "github.com/surrealdb/surrealdb.go/pkg/encoding/cbor"
)

type Config struct {
    URL       string
    Namespace string
    Database  string
    Username  string
    Password  string
}

type Client struct {
    conn *rews.Connection
    db   *surrealdb.DB
    cfg  Config
}

func NewClient(ctx context.Context, cfg Config, logger *slog.Logger) (*Client, error) {
    marshaler := cbor.NewMarshaler()
    unmarshaler := cbor.NewUnmarshaler()

    // Create rews connection with auto-reconnect
    conn := rews.New(
        func(ctx context.Context) (connection.WebSocketConnection, error) {
            ws := gws.New(&connection.Config{
                BaseURL:     cfg.URL,
                Marshaler:   marshaler,
                Unmarshaler: unmarshaler,
            })
            return ws, nil
        },
        5*time.Second,
        unmarshaler,
        logger,
    )

    // Configure exponential backoff
    retryer := rews.NewExponentialBackoffRetryer()
    retryer.InitialDelay = 1 * time.Second
    retryer.MaxDelay = 30 * time.Second
    retryer.Multiplier = 2.0
    retryer.MaxRetries = 10
    conn.Retryer = retryer

    // Connect
    if err := conn.Connect(ctx); err != nil {
        return nil, fmt.Errorf("connect: %w", err)
    }

    // Create DB wrapper
    db, err := surrealdb.FromConnection(ctx, conn)
    if err != nil {
        conn.Close(ctx)
        return nil, fmt.Errorf("from connection: %w", err)
    }

    // Authenticate
    _, err = db.SignIn(ctx, surrealdb.Auth{
        Username: cfg.Username,
        Password: cfg.Password,
    })
    if err != nil {
        conn.Close(ctx)
        return nil, fmt.Errorf("signin: %w", err)
    }

    // Select namespace/database
    if err := db.Use(ctx, cfg.Namespace, cfg.Database); err != nil {
        conn.Close(ctx)
        return nil, fmt.Errorf("use: %w", err)
    }

    return &Client{conn: conn, db: db, cfg: cfg}, nil
}

func (c *Client) Close(ctx context.Context) error {
    return c.conn.Close(ctx)
}

func (c *Client) DB() *surrealdb.DB {
    return c.db
}
```

### Ollama Embedding Client

```go
// Source: https://pkg.go.dev/github.com/ollama/ollama/api
package embedding

import (
    "context"
    "fmt"

    "github.com/ollama/ollama/api"
)

const (
    DefaultModel     = "all-minilm:l6-v2"
    ExpectedDimension = 384
)

type Client struct {
    client *api.Client
    model  string
}

func NewClient(model string) (*Client, error) {
    if model == "" {
        model = DefaultModel
    }

    client, err := api.ClientFromEnvironment()
    if err != nil {
        return nil, fmt.Errorf("create ollama client: %w", err)
    }

    return &Client{client: client, model: model}, nil
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
    resp, err := c.client.Embed(ctx, &api.EmbedRequest{
        Model: c.model,
        Input: text,
    })
    if err != nil {
        return nil, fmt.Errorf("embed: %w", err)
    }

    if len(resp.Embeddings) == 0 {
        return nil, fmt.Errorf("no embeddings returned")
    }

    embedding := resp.Embeddings[0]
    if len(embedding) != ExpectedDimension {
        return nil, fmt.Errorf("unexpected dimension: got %d, want %d", len(embedding), ExpectedDimension)
    }

    return embedding, nil
}

func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
    resp, err := c.client.Embed(ctx, &api.EmbedRequest{
        Model: c.model,
        Input: texts,
    })
    if err != nil {
        return nil, fmt.Errorf("embed batch: %w", err)
    }

    for i, emb := range resp.Embeddings {
        if len(emb) != ExpectedDimension {
            return nil, fmt.Errorf("embedding %d: unexpected dimension: got %d, want %d", i, len(emb), ExpectedDimension)
        }
    }

    return resp.Embeddings, nil
}
```

### Data Models with JSON Tags

```go
// Source: Derived from Python memcp/models.py
package models

import "time"

// Entity represents a memory entity in the knowledge graph
type Entity struct {
    ID             string    `json:"id"`
    Type           string    `json:"type,omitempty"`
    Labels         []string  `json:"labels,omitempty"`
    Content        string    `json:"content"`
    Embedding      []float32 `json:"embedding,omitempty"`
    Confidence     float64   `json:"confidence,omitempty"`
    Source         *string   `json:"source,omitempty"`
    DecayWeight    float64   `json:"decay_weight,omitempty"`
    Context        *string   `json:"context,omitempty"`
    Importance     float64   `json:"importance,omitempty"`
    UserImportance *float64  `json:"user_importance,omitempty"`
    Created        time.Time `json:"created,omitempty"`
    Accessed       time.Time `json:"accessed,omitempty"`
    AccessCount    int       `json:"access_count,omitempty"`
}

// Episode represents an episodic memory (conversation segment)
type Episode struct {
    ID          string         `json:"id"`
    Content     string         `json:"content"`
    Summary     *string        `json:"summary,omitempty"`
    Embedding   []float32      `json:"embedding,omitempty"`
    Metadata    map[string]any `json:"metadata,omitempty"`
    Timestamp   time.Time      `json:"timestamp,omitempty"`
    Context     *string        `json:"context,omitempty"`
    Created     time.Time      `json:"created,omitempty"`
    Accessed    time.Time      `json:"accessed,omitempty"`
    AccessCount int            `json:"access_count,omitempty"`
}

// Procedure represents a procedural memory (workflow/process)
type Procedure struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Steps       []ProcedureStep `json:"steps"`
    Embedding   []float32       `json:"embedding,omitempty"`
    Context     *string         `json:"context,omitempty"`
    Labels      []string        `json:"labels,omitempty"`
    Created     time.Time       `json:"created,omitempty"`
    Accessed    time.Time       `json:"accessed,omitempty"`
    AccessCount int             `json:"access_count,omitempty"`
}

// ProcedureStep represents a single step within a procedure
type ProcedureStep struct {
    Order    int    `json:"order"`
    Content  string `json:"content"`
    Optional bool   `json:"optional,omitempty"`
}

// Relation represents a relationship between entities
type Relation struct {
    From    string    `json:"from"`
    To      string    `json:"to"`
    RelType string    `json:"rel_type"`
    Weight  float64   `json:"weight,omitempty"`
    Created time.Time `json:"created,omitempty"`
}
```

### Environment Configuration

```go
// Source: Matches Python memcp/db.py environment variables
package config

import (
    "log/slog"
    "os"
    "strconv"
)

type Config struct {
    // SurrealDB
    SurrealDBURL       string
    SurrealDBNamespace string
    SurrealDBDatabase  string
    SurrealDBUser      string
    SurrealDBPass      string

    // Ollama
    OllamaHost     string
    EmbeddingModel string

    // Logging
    LogFile  string
    LogLevel slog.Level

    // Context detection
    DefaultContext  string
    ContextFromCWD  bool
}

func Load() Config {
    return Config{
        // SurrealDB (matching Python defaults)
        SurrealDBURL:       getEnv("SURREALDB_URL", "ws://localhost:8000/rpc"),
        SurrealDBNamespace: getEnv("SURREALDB_NAMESPACE", "knowledge"),
        SurrealDBDatabase:  getEnv("SURREALDB_DATABASE", "graph"),
        SurrealDBUser:      getEnv("SURREALDB_USER", "root"),
        SurrealDBPass:      getEnv("SURREALDB_PASS", "root"),

        // Ollama
        OllamaHost:     getEnv("OLLAMA_HOST", "http://localhost:11434"),
        EmbeddingModel: getEnv("MEMCP_EMBEDDING_MODEL", "all-minilm:l6-v2"),

        // Logging
        LogFile:  getEnv("MEMCP_LOG_FILE", "/tmp/memcp.log"),
        LogLevel: parseLogLevel(getEnv("MEMCP_LOG_LEVEL", "INFO")),

        // Context
        DefaultContext: getEnv("MEMCP_DEFAULT_CONTEXT", ""),
        ContextFromCWD: getEnv("MEMCP_CONTEXT_FROM_CWD", "false") == "true",
    }
}

func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func parseLogLevel(s string) slog.Level {
    switch s {
    case "DEBUG":
        return slog.LevelDebug
    case "INFO":
        return slog.LevelInfo
    case "WARN", "WARNING":
        return slog.LevelWarn
    case "ERROR":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| mark3labs/mcp-go | modelcontextprotocol/go-sdk | Dec 2025 | Official SDK now recommended for new projects |
| Direct WebSocket | contrib/rews | v1.2.0 (Jan 2026) | Auto-reconnect with session restoration |
| golang.org/x/exp/slog | log/slog | Go 1.21 (2023) | slog now in stdlib |
| zerolog/zap | slog | Go 1.21+ | slog sufficient, no external dependency |

**Deprecated/outdated:**
- `surrealdb.New()` - use `FromEndpointURLString()` or `FromConnection()`
- `surrealdb.Connect()` - deprecated
- MCP SDK v1.0/v1.1 - v1.2.0 has improved JSON schema handling

## Open Questions

1. **rews package stability**
   - What we know: Located in `contrib/` directory, not covered by backward compatibility guarantee
   - What's unclear: How stable is it for production use?
   - Recommendation: Use it (no alternative for auto-reconnect), pin exact version

2. **Ollama model availability**
   - What we know: `all-minilm:l6-v2` produces 384-dim embeddings
   - What's unclear: Is model pre-pulled on target machines?
   - Recommendation: Document `ollama pull all-minilm:l6-v2` in setup, check model exists at startup

## Sources

### Primary (HIGH confidence)
- [surrealdb.go pkg.go.dev](https://pkg.go.dev/github.com/surrealdb/surrealdb.go) - v1.2.0 API documentation
- [contrib/rews pkg.go.dev](https://pkg.go.dev/github.com/surrealdb/surrealdb.go/contrib/rews) - Auto-reconnect patterns
- [ollama/api pkg.go.dev](https://pkg.go.dev/github.com/ollama/ollama/api) - v0.15.3 Embed API
- [modelcontextprotocol/go-sdk GitHub](https://github.com/modelcontextprotocol/go-sdk) - v1.2.0 AddTool patterns
- [all-MiniLM-L6-v2 HuggingFace](https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2) - 384-dim confirmation

### Secondary (MEDIUM confidence)
- [SurrealDB Golang Connection Engines](https://surrealdb.com/docs/sdk/golang/connection-engines) - WebSocket vs HTTP docs
- [slog-multi GitHub](https://github.com/samber/slog-multi) - Multi-handler logging patterns
- [Go slog Package](https://pkg.go.dev/log/slog) - Stdlib logging

### Tertiary (LOW confidence)
- WebSearch results for ecosystem patterns (verified against primary sources above)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All libraries verified on pkg.go.dev with exact versions
- Architecture: HIGH - Patterns verified against official SDK examples
- Pitfalls: HIGH - Based on official documentation warnings and Python implementation experience

**Research date:** 2026-02-01
**Valid until:** 2026-03-01 (30 days - stable libraries, unlikely to change)
