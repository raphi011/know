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

**Use Python SDK RecordID for IDs with special characters:**

When entity IDs contain hyphens, dots, or other special characters, string interpolation breaks RELATE statements. Use the SDK's `RecordID` type instead:

```python
from surrealdb import RecordID

# ✅ CORRECT - handles special characters properly
await db.query("""
    RELATE $from_rec->relates_to->$to_rec SET weight = $weight
""", {
    'from_rec': RecordID('entity', 'my-hyphenated-id'),
    'to_rec': RecordID('entity', 'another.dotted.id'),
    'weight': 1.0
})

# ❌ WRONG - breaks on hyphens
await db.query(f"RELATE entity:{from_id}->relates_to->entity:{to_id}")
```

**RecordID also works for comparisons:**
```python
# ✅ CORRECT
await db.query("SELECT * FROM entity WHERE id != $exclude", {
    'exclude': RecordID('entity', 'some-id')
})

# ✅ ALSO WORKS - type::record() in SQL
await db.query("WHERE id != type::record('entity', $id)", {'id': 'some-id'})
```

### KNN Vector Search Operator

**IMPORTANT: Brute Force vs HNSW Index**

The KNN operator `<|k,param|>` behaves differently based on the second parameter:

```surql
-- BRUTE FORCE (exact, slower on large datasets):
WHERE embedding <|5,COSINE|> $vector     -- Uses cosine distance
WHERE embedding <|5,EUCLIDEAN|> $vector  -- Uses euclidean distance

-- HNSW INDEX (approximate, faster):
WHERE embedding <|5,40|> $vector         -- Uses HNSW with ef=40
WHERE embedding <|5,100|> $vector        -- Higher ef = more accurate, slower
```

The numeric second parameter (e.g., 40) is the "ef" (exploration factor) for HNSW search. Higher values improve recall at cost of speed. Typical range: 40-200.

**HNSW Index Definition:**
```surql
-- Recommended production setup:
DEFINE INDEX entity_embedding ON entity FIELDS embedding
    HNSW DIMENSION 384
    DIST COSINE
    TYPE F32      -- F32 saves memory vs default F64
    EFC 150       -- Construction effort (default 150)
    M 12;         -- Max connections per node (default 12)
```

**When to use which:**
- **Brute force** (`<|k,COSINE|>`): Small datasets (<10k), need exact results
- **HNSW index** (`<|k,40|>`): Large datasets, approximate results acceptable

**Fallback without index:**
```surql
-- Always works, no index needed:
SELECT id, content, vector::similarity::cosine(embedding, $emb) AS sim
FROM entity
ORDER BY sim DESC
LIMIT $limit
```

### Hybrid Search with RRF

For combining BM25 full-text and vector search, use `search::rrf()` (Reciprocal Rank Fusion):

```surql
-- Get text matches
LET $text_results = SELECT id FROM entity WHERE content @@ $query;

-- Get vector matches
LET $vector_results = SELECT id FROM entity WHERE embedding <|10,40|> $emb;

-- Combine with RRF
SELECT *, search::rrf() AS score FROM entity
WHERE id IN $text_results OR id IN $vector_results
ORDER BY score DESC;
```

RRF combines rankings without needing to normalize incompatible score scales.

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

### Labels as Sets

Use `set<string>` instead of `array<string>` for fields that should auto-deduplicate:
```surql
-- Better: set auto-deduplicates
DEFINE FIELD labels ON entity TYPE set<string>;

-- Worse: requires manual deduplication
DEFINE FIELD labels ON entity TYPE array<string>;
```

### DELETE Operations

**Simple deletes are preferred:**
```surql
✅ CORRECT: DELETE type::record("entity", $id)
❌ COMPLEX: DELETE FROM type::record("entity", $id)->?;  (may cause parse errors)
```
Reason: SurrealDB automatically cleans up relations when a record is deleted. Extra DELETE statements for relations are usually unnecessary.

### Graph Traversal Idioms

**Arrow syntax for relations:**
```surql
-- Outgoing relations
SELECT ->owns->project FROM person:alice

-- Incoming relations (who owns this?)
SELECT <-owns<-person FROM project:memcp

-- Bidirectional (friends)
SELECT <->friends_with<->person FROM person:alice

-- Wildcard (any relation type)
SELECT ->? AS all_relations FROM person:alice
```

**Recursive depth traversal:**
```surql
-- Exactly 3 levels deep
SELECT @.{3}->relates_to->entity FROM entity:start

-- Range: 1-4 levels
SELECT @.{1..4}->relates_to->entity FROM entity:start

-- Unlimited depth (use with caution + TIMEOUT)
SELECT @.{..}->relates_to->entity FROM entity:start TIMEOUT 5s
```

**Special operators:**
- `@` - Current record in recursion
- `.?` - Optional/safe access (won't error on missing)
- `{..+shortest=id}` - Find shortest path to target ID
- `{..+collect}` - Collect all unique nodes traversed

**Enforce unique relations:**
```surql
-- Prevent duplicate relations
DEFINE FIELD key ON TABLE relates_to VALUE <string>array::sort([in, out]);
DEFINE INDEX unique_relation ON relates_to FIELDS key UNIQUE;
```

### Computed & Calculated Fields

**VALUE** - Calculated on CREATE/UPDATE, stored in DB:
```surql
-- Always lowercase email
DEFINE FIELD email ON user TYPE string VALUE string::lowercase($value);

-- Auto-update timestamp
DEFINE FIELD updated_at ON entity TYPE datetime VALUE time::now();
```

**DEFAULT** - Only on CREATE, static after:
```surql
DEFINE FIELD created_at ON entity TYPE datetime DEFAULT time::now();
```

**COMPUTED** (v3.0+) - Calculated on every SELECT, not stored:
```surql
-- Dynamic age check
DEFINE FIELD can_vote ON person COMPUTED time::now() > birthday + 18y;

-- Always current
DEFINE FIELD accessed_at ON entity COMPUTED time::now();
```

**Field order matters**: DEFINE FIELD statements process alphabetically!

### Events (Triggers)

```surql
-- Audit log on entity changes
DEFINE EVENT entity_audit ON entity WHEN $event IN ["CREATE", "UPDATE", "DELETE"] THEN {
    CREATE audit_log SET
        table = "entity",
        event = $event,
        record_id = $value.id,
        before = $before,
        after = $after,
        timestamp = time::now()
};

-- Cascade updates
DEFINE EVENT update_relations ON entity WHEN $event = "UPDATE" THEN {
    UPDATE relates SET updated = time::now() WHERE in = $value.id OR out = $value.id
};
```

Event parameters: `$event` (CREATE/UPDATE/DELETE), `$before`, `$after`, `$value`

### Change Feeds (CDC)

```surql
-- Enable change tracking with 7-day retention
DEFINE TABLE entity CHANGEFEED 7d INCLUDE ORIGINAL;

-- Replay changes since timestamp
SHOW CHANGES FOR TABLE entity SINCE d"2025-01-01T00:00:00Z" LIMIT 100;
```

### Live Queries (Real-time)

```surql
-- Subscribe to changes (returns UUID)
LIVE SELECT * FROM entity WHERE labels CONTAINS "important";

-- With DIFF mode (only changes, not full records)
LIVE SELECT DIFF FROM entity;

-- Kill subscription
KILL $live_query_uuid;
```

Note: Live queries only work in single-node deployments currently.

### Transactions

```surql
BEGIN TRANSACTION;
    UPDATE account:from SET balance -= $amount;
    UPDATE account:to SET balance += $amount;

    -- Validate
    IF account:from.balance < 0 {
        THROW "Insufficient funds"
    };
COMMIT TRANSACTION;

-- Or rollback
CANCEL TRANSACTION;
```

Each statement runs in its own transaction by default. Use BEGIN/COMMIT for atomicity.

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
