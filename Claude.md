# MCP SurrealDB Server

An MCP (Model Context Protocol) server in Python that connects to a SurrealDB instance to persist knowledge between agent sessions.

## Purpose

This server enables AI agents to store and retrieve knowledge across sessions, providing a persistent memory layer using SurrealDB as the backend database.

## Tech Stack

- **Language**: Python
- **Protocol**: MCP (Model Context Protocol)
- **Database**: SurrealDB

## Features (Planned)

- Connect to a SurrealDB instance
- Store knowledge/memories from agent sessions
- Retrieve relevant knowledge for new sessions
- Query and search stored information
- Manage knowledge lifecycle (create, read, update, delete)

## Development Workflow

**IMPORTANT**: After making any changes to the codebase, always run the build verification test:

```bash
uv run pytest test_memcp.py -v
```

This ensures the module compiles correctly and can be imported without errors.

## SurrealDB v3.0 Breaking Changes

This project requires **SurrealDB v3.0.0-beta.1+**. Key v3.0 breaking changes applied:

| v2.x (Old) | v3.0 (New) |
|------------|------------|
| `type::thing("table", $id)` | `type::record("table", $id)` |
| `SEARCH ANALYZER name BM25` | `FULLTEXT ANALYZER name BM25` |
| `MTREE DIMENSION 384` | `HNSW DIMENSION 384` |
| `<\|k,ef\|>` (KNN operator) | `<\|k,COSINE\|>` |
| `rand::guid()` | `rand::id()` |

Other v3.0 changes (not used in this project):
- `<future>` type replaced with computed fields
- Angle brackets deprecated for identifier escaping (use backticks)
- `LIKE` operators removed
- FoundationDB storage engine removed

## Development Guidelines

**Always know return types** - Never guess what a query/function returns. Test it first:
```python
result = await db.query("...")
print(f"Type: {type(result)}, Value: {result}")
```

Common patterns:
- `SELECT` returns `list[dict]`
- `RETURN { ... }` returns `dict` directly
- `UPDATE/DELETE` returns `list[dict]` of affected records

## SurrealDB Syntax Learnings

Key syntax rules and gotchas when working with SurrealDB queries:

### Record ID Formatting

**RELATE statements must use direct record ID syntax:**
```surql
✅ CORRECT: RELATE entity:from_id->rel_type->entity:to_id SET weight = $weight
❌ WRONG:   RELATE type::record("entity", $from)->$type->type::record("entity", $to) SET weight = $weight
```
Reason: SurrealDB doesn't support `type::record()` inside RELATE statements. Use string interpolation for IDs and relation types.

**Record ID comparisons need type::record():**
```surql
✅ CORRECT: WHERE id != type::record("entity", $id)
❌ WRONG:   WHERE id != $id  (won't match record IDs properly)
```

### KNN Vector Search Operator

**IMPORTANT: MTREE Index Deprecated in SurrealDB v3.0**

As of SurrealDB v3.0 (November 2025), the MTREE index type has been completely removed and replaced with HNSW (Hierarchical Navigable Small World).

**Timeline:**
- Nov 6, 2025: PR #6553 - MTREE removed from SurrealDB
- Nov 20, 2025: Issue #6598 - Confirmed MTREE no longer works in v3.0
- Current: Use HNSW for all vector operations

**Migration from MTREE to HNSW:**
```surql
-- OLD (v2.x - deprecated):
DEFINE INDEX entity_embedding ON entity FIELDS embedding MTREE DIMENSION 384 DIST COSINE;

-- NEW (v3.0+):
DEFINE INDEX entity_embedding ON entity FIELDS embedding HNSW DIMENSION 384 DIST COSINE;
```

**KNN Operator Syntax (v3.0+):**
```surql
✅ CORRECT: WHERE embedding <|k,COSINE|> $vector
✅ CORRECT: WHERE embedding <|5,EUCLIDEAN|> $vector
❌ OLD:     WHERE embedding <|k,ef|> $vector  (v2.x syntax with "ef" parameter)
```

The second parameter is now the distance metric (COSINE, EUCLIDEAN, etc.), not an "ef" expansion factor.

**Fallback without index:**
```surql
✅ WORKS ON ALL VERSIONS:
SELECT id, content, vector::similarity::cosine(embedding, $emb) AS sim
FROM entity
ORDER BY sim DESC
LIMIT $limit
```
The `vector::similarity::cosine()` function works without any index and is reliable across all versions.

### Parameter Usage

**Literal integers required in KNN operators:**
```surql
❌ WRONG:   <|$limit,100|>   (parameters not allowed)
✅ CORRECT: <|{limit},100|>  (f-string interpolation)
```

**Limit can be parameterized in LIMIT clause:**
```surql
✅ CORRECT: LIMIT $limit  (parameters work here)
```

### DELETE Operations

**Simple deletes are preferred:**
```surql
✅ CORRECT: DELETE type::record("entity", $id)
❌ COMPLEX: DELETE FROM type::record("entity", $id)->?;  (may cause parse errors)
```
Reason: SurrealDB automatically cleans up relations when a record is deleted. Extra DELETE statements for relations are usually unnecessary.

## TODO

### MCP Sampling Support

Claude Code does not currently support MCP sampling (see [Issue #1785](https://github.com/anthropics/claude-code/issues/1785)). Features removed due to this limitation:

- `memorize_file` tool - was using LLM to extract entities from documents
- `auto_tag` parameter in `remember` - was using LLM to generate labels
- `summarize` parameter in `search` - was using LLM to summarize results

**When Claude Code adds sampling support**, re-implement these features. The original implementation used:
```python
async def sample(ctx: Context, prompt: str, max_tokens: int = 1000) -> str:
    result = await ctx.request_context.session.create_message(
        messages=[types.SamplingMessage(role="user", content=types.TextContent(type="text", text=prompt))],
        max_tokens=max_tokens
    )
    return result.content.text
```

**Alternative approach**: Call Anthropic API directly with an API key instead of relying on MCP sampling.
