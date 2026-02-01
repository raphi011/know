# Phase 2: MCP Server - Research

**Researched:** 2026-02-01
**Domain:** MCP Go SDK server setup, stdio transport, tool handler patterns
**Confidence:** HIGH

## Summary

Phase 2 establishes the MCP server skeleton that all tools will plug into. The official `modelcontextprotocol/go-sdk` provides a clean, type-safe API for server creation, tool registration, and handler implementation. The server uses stdio transport where stdout is reserved exclusively for JSON-RPC messages and stderr is available for logging.

The SDK's `AddTool[In, Out]` generic function automatically generates JSON schemas from Go structs, matching the Python implementation's pattern. Tool handlers receive typed, validated arguments and return structured results. Dependencies like the database client and embedder can be injected via handler factory functions (closures) without requiring DI frameworks.

**Primary recommendation:** Use handler factory functions that capture dependencies (db, embedder, logger) and return typed `ToolHandlerFor[In, Out]` handlers. Wrap all handlers with a common middleware for logging and error handling.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) | v1.2.0 | MCP protocol, stdio transport, tool registration | Official SDK, `AddTool[In,Out]` with auto-schema generation |
| `log/slog` | stdlib | Structured logging | Go 1.21+ stdlib, JSON/text handlers |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| [samber/slog-multi](https://github.com/samber/slog-multi) | v1.3+ | Multi-handler logging | Dual output (stderr + file) |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Handler factory closures | DI framework (Wire, Dig) | Frameworks add complexity; closures are idiomatic Go |
| Custom middleware | mark3labs/mcp-go middleware | Official SDK has `AddReceivingMiddleware`, sufficient for logging |

**Installation:**
```bash
# Already installed in Phase 1
go get github.com/modelcontextprotocol/go-sdk@v1.2.0
```

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── server/
│   ├── server.go         # Server initialization, lifecycle
│   └── middleware.go     # Logging/error middleware
├── tools/
│   ├── registry.go       # Tool registration (calls AddTool for each tool)
│   ├── search.go         # Search tool handlers (Phase 3)
│   ├── persist.go        # Persistence handlers (Phase 4)
│   └── ...               # Additional tool files
├── db/                   # (from Phase 1)
├── embedding/            # (from Phase 1)
├── config/               # (from Phase 1)
└── models/               # (from Phase 1)
cmd/
└── memcp/
    └── main.go           # Composition root, wires everything
```

### Pattern 1: Server Initialization

**What:** Create MCP server with implementation info and run on stdio transport.

**When to use:** Application entry point.

**Example:**
```go
// Source: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
package server

import (
    "context"
    "log/slog"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps MCP server with dependencies
type Server struct {
    mcp    *mcp.Server
    logger *slog.Logger
}

// New creates a new MCP server
func New(version string, logger *slog.Logger) *Server {
    impl := &mcp.Implementation{
        Name:    "memcp",
        Version: version,
    }

    mcpServer := mcp.NewServer(impl, nil)

    return &Server{
        mcp:    mcpServer,
        logger: logger,
    }
}

// Run starts the server on stdio transport (blocks until disconnect)
func (s *Server) Run(ctx context.Context) error {
    s.logger.Info("starting MCP server", "transport", "stdio")
    return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// MCPServer returns underlying server for tool registration
func (s *Server) MCPServer() *mcp.Server {
    return s.mcp
}
```

### Pattern 2: Handler Factory with Dependency Injection

**What:** Create handler factories that capture dependencies and return typed handlers.

**When to use:** Every tool handler that needs DB, embedder, or other services.

**Example:**
```go
// Source: Derived from official SDK examples
package tools

import (
    "context"
    "log/slog"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "memcp-go/internal/db"
    "memcp-go/internal/embedding"
)

// Dependencies holds shared services for tool handlers
type Dependencies struct {
    DB       *db.Client
    Embedder embedding.Embedder
    Logger   *slog.Logger
}

// SearchInput defines the input schema for the search tool
type SearchInput struct {
    Query   string   `json:"query" jsonschema:"required,description=The search query"`
    Labels  []string `json:"labels,omitempty" jsonschema:"description=Filter by labels"`
    Limit   int      `json:"limit,omitempty" jsonschema:"description=Max results (default 10)"`
}

// NewSearchHandler creates a search tool handler with injected dependencies
func NewSearchHandler(deps *Dependencies) mcp.ToolHandlerFor[SearchInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (
        *mcp.CallToolResult, any, error,
    ) {
        // Handler implementation uses deps.DB, deps.Embedder, deps.Logger
        deps.Logger.Debug("search called", "query", input.Query)

        // ... business logic ...

        return &mcp.CallToolResult{
            Content: []mcp.Content{
                &mcp.TextContent{Text: "Search results..."},
            },
        }, nil, nil
    }
}
```

### Pattern 3: Tool Registration

**What:** Register all tools with the server using `mcp.AddTool`.

**When to use:** After server creation, before running.

**Example:**
```go
// Source: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#AddTool
package tools

import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAll registers all tools with the server
func RegisterAll(server *mcp.Server, deps *Dependencies) {
    // Search tool
    mcp.AddTool(server, &mcp.Tool{
        Name:        "search",
        Description: "Search entities using hybrid BM25 + vector search",
    }, NewSearchHandler(deps))

    // Get entity tool
    mcp.AddTool(server, &mcp.Tool{
        Name:        "get_entity",
        Description: "Retrieve an entity by ID",
    }, NewGetEntityHandler(deps))

    // ... more tools ...
}
```

### Pattern 4: Common Handler Wrapper for Logging and Error Handling

**What:** Wrap all handlers with timing, logging, and error formatting.

**When to use:** Apply to every tool handler via middleware.

**Example:**
```go
// Source: Derived from SDK middleware pattern
package server

import (
    "context"
    "log/slog"
    "time"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// loggingMiddleware logs all tool calls with timing
func loggingMiddleware(logger *slog.Logger) mcp.Middleware {
    return func(ctx context.Context, req mcp.Request, next mcp.MethodHandler) (mcp.Result, error) {
        start := time.Now()

        // Extract tool name from params if this is a tool call
        params := req.GetParams()

        result, err := next(ctx, req)

        duration := time.Since(start)

        // Log based on result type and error
        if err != nil {
            logger.Error("request failed",
                "duration_ms", duration.Milliseconds(),
                "error", err.Error(),
            )
        } else {
            logger.Info("request completed",
                "duration_ms", duration.Milliseconds(),
            )
        }

        return result, err
    }
}

// Setup adds middleware to the server
func (s *Server) Setup() {
    s.mcp.AddReceivingMiddleware(loggingMiddleware(s.logger))
}
```

### Pattern 5: Error Handling - Tool Errors vs Protocol Errors

**What:** Return tool errors in Content with IsError=true; protocol errors as error return value.

**When to use:** All tool handlers.

**Example:**
```go
// Source: https://modelcontextprotocol.io/ error handling docs
package tools

import (
    "context"
    "fmt"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewGetEntityHandler(deps *Dependencies) mcp.ToolHandlerFor[GetEntityInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input GetEntityInput) (
        *mcp.CallToolResult, any, error,
    ) {
        entity, err := deps.DB.GetEntity(ctx, input.ID)
        if err != nil {
            // Tool error: entity not found - return in Content with IsError
            // This allows LLM to see the error and self-correct
            return &mcp.CallToolResult{
                Content: []mcp.Content{
                    &mcp.TextContent{Text: fmt.Sprintf("Entity not found: %s. Try search first.", input.ID)},
                },
                IsError: true,
            }, nil, nil
        }

        // Success
        return &mcp.CallToolResult{
            Content: []mcp.Content{
                &mcp.TextContent{Text: formatEntity(entity)},
            },
        }, nil, nil
    }
}

// For protocol-level errors (shouldn't happen in normal operation):
func protocolErrorExample(ctx context.Context, req *mcp.CallToolRequest, input SomeInput) (
    *mcp.CallToolResult, any, error,
) {
    // Return error directly - this is a protocol-level failure
    return nil, nil, fmt.Errorf("internal server error")
}
```

### Pattern 6: Graceful Shutdown

**What:** Handle SIGTERM/SIGINT for graceful shutdown.

**When to use:** Main entry point.

**Example:**
```go
// Source: Go signal handling pattern
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "memcp-go/internal/config"
    "memcp-go/internal/db"
    "memcp-go/internal/server"
)

func main() {
    cfg := config.Load()
    logger := config.SetupLogger(cfg.LogFile, cfg.LogLevel)

    // Log startup info
    logger.Info("memcp starting",
        "version", cfg.Version,
        "db_url", cfg.SurrealDBURL,
    )

    // Setup context with signal handling
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
    go func() {
        sig := <-sigCh
        logger.Info("received signal, shutting down", "signal", sig)
        cancel()
    }()

    // Connect to database
    dbClient, err := db.NewClient(ctx, cfg.DB, logger)
    if err != nil {
        logger.Error("failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer func() {
        logger.Info("closing database connection")
        _ = dbClient.Close(ctx)
    }()

    // Create and run server
    srv := server.New(cfg.Version, logger)

    if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
        logger.Error("server error", "error", err)
        os.Exit(1)
    }

    logger.Info("memcp shutdown complete")
}
```

### Anti-Patterns to Avoid

- **Logging to stdout:** MCP stdio transport reserves stdout for JSON-RPC. Log to stderr or file only.
- **Global dependencies:** Don't use package-level `var db *db.Client`. Pass via factory functions.
- **Protocol errors for tool failures:** Return tool errors in Content with IsError=true so LLM can see them.
- **Missing recovery:** Always catch panics in handlers to prevent server crash.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON schema from structs | Manual schema definition | `AddTool[In,Out]` with struct tags | SDK auto-generates from `json` and `jsonschema` tags |
| Stdio transport | Custom stdin/stdout handling | `mcp.StdioTransport{}` | SDK handles JSON-RPC framing, newline delimiters |
| Request validation | Manual type assertions | Typed handlers `ToolHandlerFor[In,Out]` | SDK validates and unmarshals before handler |
| Middleware chain | Custom wrapper functions | `AddReceivingMiddleware` | SDK provides proper middleware chaining |

**Key insight:** The SDK handles all protocol concerns - JSON-RPC framing, schema generation, request validation, transport management. Focus on business logic in handlers.

## Common Pitfalls

### Pitfall 1: Logging to stdout

**What goes wrong:** Server output corrupts JSON-RPC stream, client can't parse responses.

**Why it happens:** Default `slog` logs to stdout; stdio transport uses stdout for messages.

**How to avoid:**
- Configure slog to write to stderr: `slog.NewTextHandler(os.Stderr, opts)`
- Or write to file only (safer for production)
- NEVER use `fmt.Println()` or `log.Println()` in server code

**Warning signs:** Client reports "invalid JSON" errors, connection drops randomly.

### Pitfall 2: Returning errors instead of tool results

**What goes wrong:** LLM can't see error message, can't self-correct.

**Why it happens:** Intuition says return `error` for failures.

**How to avoid:**
- For tool failures (entity not found, validation error): return `CallToolResult` with `IsError: true`
- For protocol failures (should never happen): return `error`
- Include recovery hints in error messages: "Entity not found. Try search first."

**Warning signs:** LLM keeps retrying same failing tool without understanding why.

### Pitfall 3: Missing input validation

**What goes wrong:** Handler panics on nil/empty values, server crashes.

**Why it happens:** Trusting SDK validation for all cases.

**How to avoid:**
- SDK validates required fields, but add business logic validation
- Check zero values: `if input.Query == "" { return error result }`
- Use `jsonschema:"required"` for mandatory fields

**Warning signs:** Panics in production logs, sporadic tool failures.

### Pitfall 4: Not handling context cancellation

**What goes wrong:** Long-running operations continue after client disconnects.

**Why it happens:** Ignoring `ctx.Done()` in handlers.

**How to avoid:**
- Check `ctx.Err()` in long operations
- Pass context to all database/network calls
- Return early if context cancelled

**Warning signs:** Zombie goroutines, resource leaks after disconnects.

### Pitfall 5: Slow logging blocking handlers

**What goes wrong:** File writes block tool responses, degraded latency.

**Why it happens:** Synchronous file logging in hot path.

**How to avoid:**
- Use async slog handler or buffer writes
- For slow queries (>100ms as per CONTEXT.md), log asynchronously
- Consider structured logging to stderr (fast) + async file

**Warning signs:** Tool response times spike during logging bursts.

## Code Examples

### Complete Server Setup

```go
// Source: Derived from official SDK patterns
// cmd/memcp/main.go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "memcp-go/internal/config"
    "memcp-go/internal/db"
    "memcp-go/internal/embedding"
    "memcp-go/internal/server"
    "memcp-go/internal/tools"
)

const version = "0.1.0"

func main() {
    cfg := config.Load()
    logger := config.SetupLogger(cfg.LogFile, cfg.LogLevel)

    // Startup logging
    logger.Info("memcp starting",
        "version", version,
        "surrealdb_url", cfg.SurrealDBURL,
        "embedding_model", cfg.EmbeddingModel,
    )

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
    go func() {
        <-sigCh
        logger.Info("shutdown signal received")
        cancel()
    }()

    // Connect database
    dbClient, err := db.NewClient(ctx, cfg.DBConfig(), logger)
    if err != nil {
        logger.Error("database connection failed", "error", err)
        os.Exit(1)
    }
    defer dbClient.Close(ctx)

    // Init schema
    if err := dbClient.InitSchema(ctx); err != nil {
        logger.Error("schema init failed", "error", err)
        os.Exit(1)
    }

    // Create embedder
    embedder, err := embedding.DefaultOllama()
    if err != nil {
        logger.Error("embedder creation failed", "error", err)
        os.Exit(1)
    }

    logger.Info("dependencies initialized",
        "db_connected", true,
        "embedder_model", embedder.Model(),
    )

    // Create server
    srv := server.New(version, logger)

    // Register tools
    deps := &tools.Dependencies{
        DB:       dbClient,
        Embedder: embedder,
        Logger:   logger,
    }
    tools.RegisterAll(srv.MCPServer(), deps)

    logger.Info("server ready, awaiting connections")

    // Run (blocks)
    if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
        logger.Error("server failed", "error", err)
        os.Exit(1)
    }

    logger.Info("memcp shutdown complete")
}
```

### Tool Input with JSON Schema Tags

```go
// Source: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
package tools

// SearchInput demonstrates JSON schema generation from struct tags
type SearchInput struct {
    Query   string   `json:"query" jsonschema:"required,description=The search query text"`
    Labels  []string `json:"labels,omitempty" jsonschema:"description=Filter results to entities with these labels"`
    Types   []string `json:"types,omitempty" jsonschema:"description=Filter results to these entity types"`
    Context string   `json:"context,omitempty" jsonschema:"description=Project context to search within"`
    Limit   int      `json:"limit,omitempty" jsonschema:"description=Maximum number of results (default 10)"`
}

// GetEntityInput for single entity retrieval
type GetEntityInput struct {
    ID string `json:"id" jsonschema:"required,description=The entity ID to retrieve"`
}
```

### Error Response with Recovery Hint

```go
// Source: MCP error handling best practices
package tools

import (
    "fmt"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// errorResult creates a tool error result with recovery hint
func errorResult(msg string, hint string) *mcp.CallToolResult {
    text := msg
    if hint != "" {
        text = fmt.Sprintf("%s. %s", msg, hint)
    }
    return &mcp.CallToolResult{
        Content: []mcp.Content{
            &mcp.TextContent{Text: text},
        },
        IsError: true,
    }
}

// Usage examples:
// errorResult("Entity not found", "Try search first")
// errorResult("Invalid query", "Query must not be empty")
// errorResult("Database error", "Please retry")
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| mark3labs/mcp-go | modelcontextprotocol/go-sdk | Dec 2025 | Official SDK is now recommended |
| Manual JSON schema | `AddTool[In,Out]` generics | SDK v1.0 | Auto-generated from struct tags |
| Custom transport | `StdioTransport{}` | SDK v1.0 | Built-in, handles all framing |

**Deprecated/outdated:**
- Custom JSON-RPC handling: SDK handles protocol layer completely
- Manual middleware chains: Use `AddReceivingMiddleware` method

## Open Questions

1. **Middleware access to tool name**
   - What we know: Middleware receives `Request` interface with `GetParams()`
   - What's unclear: How to extract tool name for per-tool logging
   - Recommendation: Log at handler level via wrapper function, or inspect params type

2. **Async logging performance**
   - What we know: CONTEXT.md requires logging all tool calls with timing
   - What's unclear: Whether sync slog to file will bottleneck
   - Recommendation: Start with sync (simpler), measure, add async if needed

## Sources

### Primary (HIGH confidence)
- [modelcontextprotocol/go-sdk mcp package](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp) - Server API, AddTool, Middleware
- [MCP SDK GitHub](https://github.com/modelcontextprotocol/go-sdk) - Examples, version info
- [MCP Specification - Transports](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) - Stdio logging constraints

### Secondary (MEDIUM confidence)
- [Writing MCP Server with Go SDK](https://medium.com/@xcoulon/writing-your-first-mcp-server-with-the-go-sdk-62fada87e5eb) - Practical patterns
- [mcp-go error handling](https://deepwiki.com/grafana/mcp-go/7-error-handling) - Tool vs protocol errors

### Tertiary (LOW confidence)
- WebSearch results for middleware patterns (verified against official docs above)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Official SDK verified on pkg.go.dev
- Architecture patterns: HIGH - Derived from official examples and SDK API
- Pitfalls: HIGH - Based on MCP specification and SDK documentation

**Research date:** 2026-02-01
**Valid until:** 2026-03-01 (30 days - stable SDK)
