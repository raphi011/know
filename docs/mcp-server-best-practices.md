# MCP Server Best Practices

Best practices for building MCP (Model Context Protocol) servers, with specific recommendations for the Knowhow RAG + document editing use case.

## Tool Design

### Design for Agent Outcomes, Not API Coverage

MCP tools are a UI for AI agents. Don't map internal functions 1:1 to tools. Consolidate multi-step workflows into single tools that handle orchestration server-side.

```
Bad:  get_document() → parse_sections() → edit_section()  (3 calls)
Good: edit_document_section(path, heading, content)        (1 call)
```

Knowhow already does this well — `edit_document_section` combines read + parse + edit into one atomic tool.

### Tool Count

Keep **5-15 tools per server**. More tools = more schema tokens in every request. Knowhow has 10 tools — right in the sweet spot.

If tools grow beyond 15, consider **dynamic toolsets**: expose `search_tools` → `describe_tools` → `execute_tool` meta-tools. The agent discovers tools on demand (96% token reduction for large tool sets, but adds 2-3 extra round trips).

### Naming Conventions

Use `{action}_{resource}` format. Names should be 1-128 chars, `[A-Za-z0-9_-.]` only.

```
Good: search_documents, create_memory, edit_document_section
Bad:  docSearch, createNewMem, editSection
```

### Flatten Arguments

Never use nested dicts/objects as input. Use top-level primitives with enum types for constrained values and sensible defaults.

```go
// Good — flat, typed, defaults
type searchInput struct {
    Query   string   `json:"query"`
    Labels  []string `json:"labels,omitempty"`
    Limit   *int     `json:"limit,omitempty"`    // default 20
}

// Bad — opaque nested object
type searchInput struct {
    Filters map[string]any `json:"filters"`
}
```

### Tool Annotations

Use MCP spec annotations to help clients categorize tool behavior:

```go
&mcp.Tool{
    Name: "edit_document",
    Annotations: &mcp.ToolAnnotations{
        ReadOnlyHint:    boolPtr(false),
        DestructiveHint: boolPtr(false),  // edits are versioned
        IdempotentHint:  boolPtr(true),   // same content = same result
    },
}
```

- `readOnlyHint`: safe to call without side effects
- `destructiveHint`: may delete or irreversibly modify data
- `idempotentHint`: safe to retry, same input = same result
- `openWorldHint`: interacts with external systems

## Tool Descriptions

### Structure

1-2 sentences, verb-first. Front-load critical info since agents may truncate.

**What goes where:**
- **Tool description**: Primary action, resource, workflow dependencies, scope limitations
- **Parameter descriptions**: Type, format, constraints, examples
- **Schema annotations**: Auth requirements, pagination, rate limits

### Patterns

```
Simple:     "Search documents using full-text and semantic search."
With deps:  "Edit a section by heading. Call get_document with sections=true first."
With scope: "List immediate children only. Use list_folders for full tree."
```

### Actionable Error Messages

Error messages should help the agent decide what to do next:

```
Bad:  "Not found"
Good: "Document not found at /guides/setup.md. Use search_documents to find it."

Bad:  "Invalid operation"
Good: "Invalid operation 'update'. Use one of: replace, insert_after, insert_before, delete, append"
```

## Token Efficiency

Tool schemas are loaded into every conversation and can consume **500-1000 tokens per tool**. With 10 tools, that's 5-10K tokens before the agent does anything.

### Minimize Schema Size

- Keep parameter descriptions under 10 words
- Use `omitempty` on optional fields (avoids `"required"` array bloat)
- Avoid deeply nested schemas
- Use `enum`/`Literal` types instead of free-text with "one of X, Y, Z" in description

### Minimize Response Size

- Return only what the agent needs to make its next decision
- Paginate large result sets (`limit` default 20, return `has_more`/`total_count`)
- Offer a `detail_level` parameter: `summary` vs `full`
- Filter server-side before returning (10K rows → 5 relevant rows)

### Response Format for RAG

For search results, structure output to minimize tokens while maximizing utility:

```
## /path/to/doc.md (score: 0.85)
> matching snippet with context...

## /other/doc.md (score: 0.72)
> another snippet...
```

vs. verbose JSON:

```json
{"results": [{"path": "/path/to/doc.md", "score": 0.85, "title": "...", ...}]}
```

Text format is ~40% fewer tokens than equivalent JSON for the same information.

### Progressive Disclosure for Documents

Instead of always returning full document content:

1. **Search** → titles, paths, scores, snippets (minimal)
2. **Get document** → full content + metadata (on demand)
3. **Get document with sections=true** → adds section outline (only when editing)

Knowhow already implements this pattern well.

## Performance

### Connection Management

- Reuse DB connections (pool_size=20, timeout=30s)
- Pre-warm connections on server start
- Create external service connections per tool call, not on startup (lets `tools/list` work even if services are down)

### Caching

For a RAG knowledge base:

- **Label/folder cache**: Cache label list and folder tree with short TTL (30-60s). These change infrequently but are queried often for discovery.
- **Search result cache**: Don't cache — queries are unique and results change with document updates
- **Document content cache**: Consider caching hot documents if DB latency is an issue

### Stateless Design

Keep tool execution stateless for horizontal scaling:
- No in-memory state between tool calls
- Vault resolution happens per-request via auth context
- Use idempotency for write operations

## RAG-Specific Patterns

### Search Tool Design

1. **Hybrid search** (BM25 + vector) with RRF fusion — Knowhow does this
2. Include **snippets with context** — shows agents why a result matched
3. Support **filter composition** — labels, doc_type, folder can combine
4. Return **relevance scores** — helps agents decide if results are useful
5. Set a sensible **default limit** (10-20) — prevents overwhelming context

### Memory Tools

Knowhow's `create_memory` pattern (date-prefixed, auto-labeled) is good. Consider also:

- **Scoped retrieval**: `search_documents` with `labels=["memory"]` already works
- **Memory deduplication**: Check if similar memory exists before creating
- **Memory linking**: Auto-link memories to documents they reference via wiki-links

### Embedding Strategy

- Enrich chunk embeddings with document-level context (title, path, labels)
- Use cross-encoder reranking for top results if precision matters
- Consider contextual retrieval: prepend a summary to each chunk before embedding

## Document Editing Patterns

### Section-Based Editing

Knowhow's `edit_document_section` is a strong pattern — it avoids sending full document content for small edits. Key design considerations:

1. **Heading-based addressing** is intuitive for markdown (vs line numbers which shift)
2. **Position disambiguation** handles duplicate headings
3. **Operation enum** (`replace`, `insert_after`, etc.) is clearer than separate tools
4. **Read before write** pattern prevents blind overwrites

### Content Hashing / Optimistic Locking

To prevent edit conflicts (especially with WebDAV + MCP concurrent access):

- Return a `content_hash` with `get_document`
- Accept optional `expected_hash` on `edit_document`
- Reject edit if hash doesn't match (document changed since read)

This is cheap to implement and prevents silent data loss.

### Versioning

Exposing `get_document_versions` is valuable for recovery. Consider adding:

- `restore_document_version` — revert to a previous version
- Version diffs — show what changed between versions

## Error Handling

### Two Error Categories (MCP Spec)

1. **Protocol errors** (JSON-RPC): malformed request, unknown tool. Models can't self-correct.
2. **Tool errors** (`isError: true` in result): validation failures, not-found, permission denied. Should be actionable.

### Return Errors as Results, Not Exceptions

For tool-level errors (not-found, validation), prefer returning a `CallToolResult` with `IsError: true` rather than a Go error. This gives the agent context to try again:

```go
// Good — agent sees the error and can self-correct
return &mcp.CallToolResult{
    Content: []mcp.Content{&mcp.TextContent{
        Text: "Document not found at /wrong/path.md. Use search_documents to find the correct path.",
    }},
    IsError: true,
}, nil, nil

// OK but less informative — treated as infrastructure failure
return nil, nil, fmt.Errorf("document not found: %s", path)
```

### Validation Errors Should Guide

```go
// Good
"operation must be one of: replace, insert_after, insert_before, delete, append"

// Bad
"invalid operation"
```

## Knowhow-Specific Improvements to Consider

### 1. Use `IsError` for Tool-Level Errors

Currently, validation errors (empty path, invalid limit) return Go errors. Consider using `CallToolResult.IsError = true` instead, so the agent sees them as actionable tool feedback rather than infrastructure failures.

### 2. Add Tool Annotations

The 10 tools don't use `ToolAnnotations`. Adding `readOnlyHint`, `destructiveHint`, and `idempotentHint` helps MCP clients (like Claude Code) make better UI decisions.

### 3. Actionable Not-Found Messages

```go
// Current
return textResult(fmt.Sprintf("Document not found: %s", input.Path)), nil, nil

// Improved
return textResult(fmt.Sprintf("Document not found: %s. Use search_documents to find it, or list_folder_contents to browse.", input.Path)), nil, nil
```

### 4. Cache Discovery Tools

`list_labels` and `list_folders` rarely change. Adding a per-request or short-TTL cache avoids repeated DB queries when agents call these during exploration.

### 5. Content Hash for Edit Safety

Add `content_hash` to `get_document` response and `expected_hash` to `edit_document` input. Prevents silent overwrites from concurrent MCP + WebDAV access.

### 6. Structured Output for Search

Consider adding `outputSchema` to `search_documents` for typed results. This enables MCP clients to render results in richer UIs and lets agents parse results programmatically.

## References

- [MCP Specification (2025-11-25)](https://modelcontextprotocol.io/specification/2025-11-25)
- [Anthropic Engineering: Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp)
- [Docker: MCP Server Best Practices](https://www.docker.com/blog/mcp-server-best-practices/)
- [Philipp Schmid: MCP Best Practices](https://www.philschmid.de/mcp-best-practices)
- [Speakeasy: 100x Token Reduction with Dynamic Toolsets](https://www.speakeasy.com/blog/how-we-reduced-token-usage-by-100x-dynamic-toolsets-v2)
- [Merge.dev: MCP Tool Descriptions](https://www.merge.dev/blog/mcp-tool-description)
- [Peter Steinberger: MCP Best Practices](https://steipete.me/posts/2025/mcp-best-practices)
- [MCP Best Practice Guide](https://mcp-best-practice.github.io/mcp-best-practice/best-practice/)
