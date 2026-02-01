# Feature Landscape: MCP Tools in mcp-go

**Domain:** MCP server tool implementation for Go migration
**Researched:** 2026-02-01
**Confidence:** HIGH (verified via official docs, pkg.go.dev, mcp-go.dev)

## Core Tool Patterns in mcp-go

### Tool Definition

Tools are created using the builder pattern with `mcp.NewTool()`:

```go
tool := mcp.NewTool("search",
    mcp.WithDescription("Search your persistent memory"),
    mcp.WithString("query",
        mcp.Required(),
        mcp.Description("The search query"),
    ),
    mcp.WithNumber("limit",
        mcp.Description("Max results (1-100)"),
        mcp.Min(1),
        mcp.Max(100),
        mcp.DefaultNumber(10),
    ),
    mcp.WithArray("labels",
        mcp.Description("Optional label filters"),
        mcp.WithStringItems(),
    ),
)
```

### Handler Registration

```go
s := server.NewMCPServer("memcp", "1.0.0",
    server.WithToolCapabilities(true),
)

s.AddTool(tool, handler)
```

### Handler Signature

```go
func handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
```

## Table Stakes

Features every MCP tool implementation needs.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Parameter extraction | Core requirement | Low | Use `RequireString()`, `GetInt()`, etc. |
| Required vs optional params | Validation | Low | `mcp.Required()` option |
| Error responses | UX | Low | `mcp.NewToolResultError(msg)` |
| Text responses | Basic output | Low | `mcp.NewToolResultText(text)` |
| Context propagation | Cancellation | Low | First param is `context.Context` |
| Tool descriptions | Discoverability | Low | `mcp.WithDescription()` |
| Parameter descriptions | UX | Low | `mcp.Description()` on each param |

## Differentiators

Features that improve DX but aren't strictly required.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Structured I/O with generics | Type safety, auto-schema | Medium | `WithInputSchema[T]()` + `NewStructuredToolHandler()` |
| JSON schema via struct tags | Less boilerplate | Medium | `jsonschema:"required"`, `jsonschema_description:"..."` |
| Tool annotations | Hint behavior | Low | `WithReadOnlyHintAnnotation()`, `WithDestructiveHintAnnotation()` |
| Middleware | Cross-cutting concerns | Medium | `WithToolHandlerMiddleware()` |
| Hooks | Observability | Medium | `server.WithHooks()` for before/after callbacks |
| Recovery | Panic handling | Low | `server.WithRecovery()` |

## Anti-Features

Things to explicitly NOT do.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Direct `request.Params.Arguments` access | Deprecated in v0.29.0, breaks | Use `request.GetArguments()` or `RequireX()` methods |
| Returning `error` for tool failures | Confuses MCP clients | Return `mcp.NewToolResultError()` with nil error |
| Panicking in handlers | Crashes server | Use `server.WithRecovery()` + proper error handling |
| Complex nested object params manually | Error-prone | Use `WithInputSchema[T]()` with structs |
| Ignoring context cancellation | Hung requests | Check `ctx.Done()` in long operations |

---

## Tool Definition Patterns

### Pattern 1: Basic Parameter Definition (Recommended for Simple Tools)

```go
tool := mcp.NewTool("forget",
    mcp.WithDescription("Delete information from persistent memory"),
    mcp.WithString("entity_id",
        mcp.Required(),
        mcp.Description("ID of entity to delete"),
    ),
    mcp.WithDestructiveHintAnnotation(true),
)
```

### Pattern 2: Structured Input Schema (Recommended for Complex Tools)

Define input as a Go struct with JSON schema tags:

```go
type RememberInput struct {
    Entities []EntityInput `json:"entities" jsonschema_description:"Entities to store"`
    Relations []RelationInput `json:"relations,omitempty" jsonschema_description:"Relations to create"`
    DetectContradictions bool `json:"detect_contradictions,omitempty" jsonschema_description:"Check for contradictions"`
    Context string `json:"context,omitempty" jsonschema_description:"Project namespace"`
}

type EntityInput struct {
    ID string `json:"id" jsonschema:"required" jsonschema_description:"Unique identifier"`
    Content string `json:"content" jsonschema:"required" jsonschema_description:"Text content"`
    Type string `json:"type,omitempty" jsonschema_description:"Entity type" jsonschema:"default=concept"`
    Labels []string `json:"labels,omitempty" jsonschema_description:"Category tags"`
    Confidence float64 `json:"confidence,omitempty" jsonschema_description:"Confidence score 0-1" jsonschema:"minimum=0,maximum=1"`
}

tool := mcp.NewTool("remember",
    mcp.WithDescription("Store information in persistent memory"),
    mcp.WithInputSchema[RememberInput](),
)
```

### Pattern 3: Structured Output Schema

```go
type SearchResponse struct {
    Entities []EntityResult `json:"entities" jsonschema_description:"Search results"`
    Count int `json:"count" jsonschema_description:"Total results"`
}

tool := mcp.NewTool("search",
    mcp.WithDescription("Search persistent memory"),
    mcp.WithInputSchema[SearchInput](),
    mcp.WithOutputSchema[SearchResponse](),
)
```

---

## Handler Patterns

### Pattern 1: Basic Handler with Manual Extraction

```go
func handleForget(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    entityID, err := req.RequireString("entity_id")
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }

    if err := db.DeleteEntity(ctx, entityID); err != nil {
        return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
    }

    return mcp.NewToolResultText(fmt.Sprintf("Removed %s", entityID)), nil
}
```

### Pattern 2: Structured Handler with Type Safety

```go
func handleSearch(ctx context.Context, req mcp.CallToolRequest, args SearchInput) (SearchResponse, error) {
    // args is already parsed and validated
    limit := args.Limit
    if limit == 0 {
        limit = 10
    }

    results, err := db.Search(ctx, args.Query, args.Labels, limit)
    if err != nil {
        return SearchResponse{}, err // returned as error result
    }

    return SearchResponse{
        Entities: results,
        Count:    len(results),
    }, nil
}

// Registration
s.AddTool(searchTool, mcp.NewStructuredToolHandler(handleSearch))
```

### Pattern 3: Handler with External Service Integration

```go
type MemcpHandler struct {
    db      *surrealdb.DB
    embedder *ollama.Client
}

func (h *MemcpHandler) HandleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    query, err := req.RequireString("query")
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }

    // Generate embedding via Ollama
    embedding, err := h.embedder.Embed(ctx, query)
    if err != nil {
        return mcp.NewToolResultError(fmt.Sprintf("embedding failed: %v", err)), nil
    }

    // Query SurrealDB with embedding
    results, err := h.db.Query(ctx, hybridSearchQuery, map[string]any{
        "embedding": embedding,
        "query":     query,
        "limit":     req.GetInt("limit", 10),
    })
    if err != nil {
        return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
    }

    // Return JSON response
    jsonData, _ := json.Marshal(results)
    return mcp.NewToolResultText(string(jsonData)), nil
}
```

---

## Response Patterns

### Text Response (Most Common)

```go
return mcp.NewToolResultText("Operation completed"), nil
```

### JSON Response

```go
result := map[string]any{
    "count": len(items),
    "items": items,
}
jsonData, _ := json.Marshal(result)
return mcp.NewToolResultText(string(jsonData)), nil
```

### Structured Response (Type-Safe)

```go
return mcp.NewToolResultStructured(response, "Fallback text description"), nil
```

### Error Response

```go
// Tool-level error (tool ran but operation failed)
return mcp.NewToolResultError("Entity not found"), nil

// System-level error (tool couldn't run)
return nil, fmt.Errorf("database connection lost: %w", err)
```

### Multiple Content Items

```go
return &mcp.CallToolResult{
    Content: []mcp.Content{
        mcp.NewTextContent("Found 3 results:"),
        mcp.NewTextContent(resultJSON),
    },
}, nil
```

---

## Parameter Extraction Methods

### Required Parameters (Error if Missing)

```go
str, err := req.RequireString("name")
num, err := req.RequireFloat("value")
intVal, err := req.RequireInt("count")
boolVal, err := req.RequireBool("enabled")
strSlice, err := req.RequireStringSlice("tags")
```

### Optional Parameters (Default if Missing)

```go
str := req.GetString("name", "default")
num := req.GetFloat("value", 0.0)
intVal := req.GetInt("count", 10)
boolVal := req.GetBool("enabled", false)
strSlice := req.GetStringSlice("tags", nil)
```

### Struct Binding

```go
var args MyInputType
if err := req.BindArguments(&args); err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}
```

### Raw Access

```go
argsMap := req.GetArguments() // map[string]any
rawArgs := req.GetRawArguments() // any
```

---

## Tool Annotations

Match Python's `ToolAnnotations`:

```go
// Read-only tool (won't modify state)
mcp.WithReadOnlyHintAnnotation(true)

// Destructive tool (deletes data)
mcp.WithDestructiveHintAnnotation(true)

// Idempotent tool (safe to retry)
mcp.WithIdempotentHintAnnotation(true)
```

---

## Migration Mapping: Python to Go

| Python (FastMCP) | Go (mcp-go) |
|------------------|-------------|
| `@server.tool()` | `s.AddTool(tool, handler)` |
| `ToolAnnotations(readOnlyHint=True)` | `mcp.WithReadOnlyHintAnnotation(true)` |
| `ToolAnnotations(destructiveHint=True)` | `mcp.WithDestructiveHintAnnotation(true)` |
| `raise ToolError("msg")` | `return mcp.NewToolResultError("msg"), nil` |
| `ctx: Context` parameter | Use closure or handler struct for DB access |
| `return SearchResult(...)` | `return mcp.NewToolResultText(json.Marshal(...))` or `NewStructuredToolHandler` |
| Pydantic model return | Go struct with json tags |
| `list[str] \| None` param | `mcp.WithArray("name", mcp.WithStringItems())` |

---

## Known Limitations

### Issue: Empty Parameters Cause Failures (Issue #690)
- Tools with all optional parameters may fail when invoked with empty input
- **Mitigation:** Always have at least one parameter, or handle empty args explicitly

### Issue: Tool Schema Serialization (Issue #671)
- Some edge cases in schema serialization
- **Mitigation:** Test tool definitions with `claude_desktop_config.json` locally

### Issue: Array Type Complexity
- Complex nested arrays need `Items()` option
- Simple arrays work with `WithStringItems()`, `WithNumberItems()`
- **Mitigation:** For complex arrays, use `WithInputSchema[T]()` with struct

### Breaking Change: v0.29.0 Argument Access
- Direct `request.Params.Arguments` deprecated
- **Mitigation:** Always use `GetArguments()` or `RequireX()` methods

---

## Recommended Architecture for memcp-go

```go
// internal/tools/search.go
type SearchHandler struct {
    db      *surrealdb.DB
    embedder embedding.Embedder
}

func NewSearchHandler(db *surrealdb.DB, embedder embedding.Embedder) *SearchHandler {
    return &SearchHandler{db: db, embedder: embedder}
}

func (h *SearchHandler) Tool() mcp.Tool {
    return mcp.NewTool("search",
        mcp.WithDescription("Search persistent memory..."),
        mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
        mcp.WithNumber("limit", mcp.Description("Max results"), mcp.Min(1), mcp.Max(100), mcp.DefaultNumber(10)),
        mcp.WithArray("labels", mcp.Description("Filter labels"), mcp.WithStringItems()),
        mcp.WithString("context", mcp.Description("Project namespace")),
        mcp.WithReadOnlyHintAnnotation(true),
    )
}

func (h *SearchHandler) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Implementation
}

// cmd/memcp/main.go
func main() {
    db := connectSurrealDB()
    embedder := connectOllama()

    s := server.NewMCPServer("memcp", "1.0.0",
        server.WithToolCapabilities(true),
        server.WithRecovery(),
    )

    searchHandler := tools.NewSearchHandler(db, embedder)
    s.AddTool(searchHandler.Tool(), searchHandler.Handle)

    // ... more tools

    server.ServeStdio(s)
}
```

---

## Sources

- [mcp-go GitHub Repository](https://github.com/mark3labs/mcp-go) - HIGH confidence
- [mcp-go Official Documentation](https://mcp-go.dev/servers/tools/) - HIGH confidence
- [pkg.go.dev mcp package](https://pkg.go.dev/github.com/mark3labs/mcp-go/mcp) - HIGH confidence
- [pkg.go.dev server package](https://pkg.go.dev/github.com/mark3labs/mcp-go/server) - HIGH confidence
- [mcp-go GitHub Issues](https://github.com/mark3labs/mcp-go/issues) - MEDIUM confidence (for known issues)
