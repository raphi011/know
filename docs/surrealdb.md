# SurrealDB

Technical learnings about SurrealDB v3.0 patterns and gotchas. Updated for v3.0.2.

## HNSW Vector Index

### Definition

```sql
DEFINE INDEX idx_chunk_embedding ON chunk FIELDS embedding
    HNSW DIMENSION 1024 DIST COSINE TYPE F32 EFC 150 M 12 HASHED_VECTOR;
```

Parameters:
- `DIMENSION` - Must match embedding vector size exactly
- `DIST COSINE` - Cosine similarity (best for normalized embeddings)
- `TYPE F32` - 32-bit floats
- `EFC 150` - Expansion factor at construction (higher = better quality, slower build)
- `M 12` - Max connections per node (higher = better recall, more memory)
- `HASHED_VECTOR` - (v3.0.0+) Uses BLAKE3 hash as storage key instead of full vector, reducing storage overhead

### Gotchas

1. **Dimension changes require fresh DB** - Can't ALTER index dimension
2. **Optional embeddings** - Use `option<array<float>>` for nullable
3. **HNSW rejects NONE values** - Even on `option<array<float>>` fields, the HNSW index cannot index NONE values. Setting `embedding = NONE` in a CREATE statement causes: `Couldn't coerce value for field 'embedding': Expected 'array' but found 'NONE'`. **Fix**: Omit the embedding field entirely from the CREATE statement when no embedding is available. The async embedding worker can UPDATE the field later — UPDATE works fine because it replaces NONE with the actual vector.

```sql
-- BAD: HNSW chokes on NONE
CREATE chunk SET content = $content, embedding = NONE

-- GOOD: omit the field entirely, fill it later via UPDATE
CREATE chunk SET content = $content
-- ...later...
UPDATE chunk:xyz SET embedding = $embedding
```

## Database-Level Strict Mode

In v3.0, `--strict` server flag was replaced by per-database strictness:

```sql
DEFINE DATABASE IF NOT EXISTS mydb STRICT;
```

This is better than server-wide strictness — allows mixing strict and non-strict databases on the same instance. Non-existent tables error in strict mode instead of silently returning empty arrays.

## Async Events

Events can run asynchronously with bounded retries (v3.0.0+):

```sql
DEFINE EVENT cascade_delete ON document
WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
    DELETE FROM chunk WHERE document = $before.id
};
```

- `ASYNC` - Event runs in background after commit (keeps writes fast)
- `RETRY n` - Bounded retries on failure
- Eventually consistent, may be out of order across events
- If all retries fail, the event is silently dropped — orphaned records possible
- Use for cascade deletes and non-critical side effects where eventual consistency is acceptable

## SDK Transactions (v1.3.0+)

Interactive transactions via the Go SDK — replaces multi-statement workarounds with proper ACID guarantees:

```go
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer tx.Cancel(ctx) // safe if already committed

_, err = surrealdb.Query[any](ctx, tx, "DELETE FROM document WHERE vault = $v", vars)
if err != nil {
    return err
}

err = tx.Commit(ctx)
```

Key points:
- `*Transaction` satisfies the `sendable` constraint (the SDK's interface accepted by `surrealdb.Query[T]()`) — all query calls work unchanged, just pass `tx` instead of `db`
- `tx.Cancel()` returns an error if already committed — safe to ignore in defer
- WebSocket-only (not HTTP connections)
- Transactions inherit auth/namespace/database from the session that started them

### When to use transactions vs multi-statement

- **Transactions**: When atomicity matters (e.g., deleting vault + all its data)
- **Multi-statement** (`sql1; sql2`): Still useful for read-only operations where atomicity doesn't matter but you want fewer round-trips

## SDK Error Types

The Go SDK provides three error types (v1.4.0+):

| Type | When | Use |
|------|------|-----|
| `RPCError` | Transport/protocol failures | Retriable |
| `QueryError` | SurrealQL syntax/type errors | Not retriable |
| `ServerError` | Structured v3 errors with Kind/Details/Cause chain | Branch on `Kind` for specific handling |

```go
var se *surrealdb.ServerError
if errors.As(err, &se) {
    fmt.Println(se.Kind, se.Details) // e.g. "NotFound", "NotAllowed"
}
```

## v3.0 Breaking Changes

### KNN Operator

```sql
-- v2.x (deprecated)
vector::distance::knn(embedding, $query_vec)

-- v3.0 HNSW (requires HNSW index)
embedding <|10,40|> $query_vec

-- v3.0 Brute force (no index, explicit distance metric)
embedding <|10,COSINE|> $query_vec
```

The operator has **two different forms** depending on whether an HNSW index exists:

**HNSW form `<|K,EF|>`** (when an HNSW index is defined on the field):
- K = number of nearest neighbors
- EF = effort / dynamic candidate list size (higher = more accurate, slower). 40 is a good default
- **The EF param is required** — `<|K|>` without EF causes: `KNN operators nested in OR/NOT expressions or mixed with unsupported KNN variants are not supported` (misleading error — the real issue is the missing EF param, not the WHERE conditions)
- Can be combined with AND conditions, including record traversals: `WHERE doc.vault = $v AND embedding <|10,40|> $vec`

**Brute force form `<|K,DIST|>`** (no index):
- K = number of nearest neighbors
- DIST = distance metric (COSINE, EUCLIDEAN, etc.)

In this codebase we use the HNSW form with `EF=40`.

### Fulltext Search

```sql
DEFINE ANALYZER knowhow_analyzer
    TOKENIZERS class
    FILTERS lowercase, ascii, snowball(english);

DEFINE INDEX idx_content_ft ON chunk FIELDS content
    FULLTEXT ANALYZER knowhow_analyzer BM25;

-- Query
SELECT * FROM chunk WHERE content @@ 'search terms';
```

### Other Breaking Changes (v3.0.0)

- `type::thing()` → `type::record()`
- `rand::guid()` → `rand::id()`
- `duration::from::*` → `duration::from_*`
- `string::is::*` → `string::is_*`
- `DEFINE TOKEN` / `DEFINE SCOPE` removed
- `MTREE` index removed (use HNSW)
- `SEARCH ANALYZER` → `FULLTEXT ANALYZER`
- `PARALLEL` clause removed from SELECT
- `LET` keyword now mandatory: `$x = 10` → `LET $x = 10`
- `.id` idiom removed → use `.id()` function
- Similarity operators (`~`, `!~`, `?~`, `*~`) removed → use `string::similarity::*` functions
- Reserved words need backtick escaping (not angle brackets)

### v3.0.2 Critical Fixes

- **RRF scoring bug fixed**: `search::rrf()` returned incorrect results in all pre-v3.0.2 versions (Rust min/max heap mixup)
- **Parameterized BM25 fixed**: `content @0@ $query` with bind parameters was silently broken in v3.0.0–v3.0.1 — only literal strings worked
- **DEFINE FUNCTION parse bug fixed**: String literals containing colons in function bodies were corrupted to record IDs

### New in v3.0.0

- **Streaming execution engine**: Pipelines data as streams instead of loading entire datasets into memory
- **COMPUTED fields**: `DEFINE FIELD chunk_count ON document VALUE (SELECT count() FROM chunk WHERE document = $this GROUP ALL)[0].count ?? 0` — recalculates on every read, cannot be indexed
- **ALTER FIELD**: Modify field definitions without full redefine
- **ALTER COMPACT**: Trigger storage compaction at runtime
- **DEFINE API**: Custom HTTP endpoints inside SurrealDB
- **Duration arithmetic**: `1h * 3` = `3h`, `1h / 2` = `30m`
- **HNSW concurrent writes**: Multiple writers no longer block each other
- **HNSW bulk insert**: Fixed O(N²) memory explosion on bulk insert

## Record ID Handling

SurrealDB returns complex record IDs that need extraction:

```go
// ID can be: surrealdb.RecordID, map[string]any, or string
func RecordIDString(id any) (string, error) {
    switch v := id.(type) {
    case surrealdb.RecordID:
        return v.ID.(string), nil
    case string:
        return v, nil
    case map[string]any:
        if tb, ok := v["tb"].(string); ok {
            if id, ok := v["id"].(string); ok {
                return fmt.Sprintf("%s:%s", tb, id), nil
            }
        }
    }
    return "", fmt.Errorf("unexpected ID type: %T", id)
}
```

### `type::record()` Expects Bare IDs

`type::record("table", $id)` constructs a record ID. If `$id` already has a table prefix (e.g., `"vault:default"`), the result is double-prefixed: `vault:vault:default`. This silently matches no records.

**Fix**: Strip the table prefix at the DB boundary with a `bareID` helper:

```go
func bareID(table, id string) string {
    return strings.TrimPrefix(id, table+":")
}
```

Apply `bareID` to **every** query parameter that feeds into `type::record()`.

### `INSIDE` Requires Typed Record IDs, Not Strings

When using `id INSIDE $ids` to batch-fetch records, the Go SDK's CBOR layer distinguishes record IDs from strings. Passing `[]string{"document:abc123"}` will **silently match nothing**.

**Fix**: Pass `[]surrealmodels.RecordID` so the SDK serializes proper CBOR record IDs:

```go
recordIDs := make([]surrealmodels.RecordID, len(ids))
for i, id := range ids {
    recordIDs[i] = newRecordID("document", bareID("document", id))
}
```

## Hybrid Search Pattern

Combine vector and fulltext search with RRF:

```sql
LET $vec_results = (
    SELECT id, content, vector::similarity::cosine(embedding, $embedding) AS vec_score
    FROM chunk
    WHERE embedding <|20,40|> $embedding
);

LET $ft_results = (
    SELECT id, content, search::score(0) AS ft_score
    FROM chunk
    WHERE content @0@ $query
    LIMIT 20
);
```

RRF fusion is performed in Go code (`search.rrfFusion()`), not in SurrealQL.

**Important**: Must be on v3.0.2+ for correct RRF scoring and parameterized BM25.

## Query Gotchas

### Compound OR in WHERE Clauses

As of v3.0.2, parenthesized OR conditions in WHERE clauses can cause parse errors:

```sql
-- FAILS: parse error
DELETE FROM folder WHERE vault = $v AND (path = $p OR string::starts_with(path, $prefix))

-- WORKAROUND: split into separate queries (use a transaction if atomicity needed)
```

### `type::record()` in RELATE Statements

`type::record()` cannot be used inline in RELATE endpoints — it causes a parse error:

```sql
-- FAILS: Parse error: Unexpected token `::`
RELATE type::record("document", $from)->doc_relation->type::record("document", $to) SET ...

-- WORKS: assign to LET variables first
LET $from = type::record("document", $from_id);
LET $to = type::record("document", $to_id);
RELATE $from->doc_relation->$to SET ...
```

Note: multi-statement queries return one result set per statement. With 2 LETs + RELATE, the RELATE result is at index `[2]`.

### `RETURN` vs `SELECT` for Computed Expressions

Standalone computed expressions (without a table) need `RETURN`, not `SELECT`:

```sql
-- FAILS: Unexpected end of file, expected FROM
SELECT array::distinct(array::flatten(...)) AS labels

-- WORKS: RETURN gives the value directly (no row wrapper)
RETURN array::distinct(array::flatten(...))
```

### Negative Array Indexing

Bracket-based negative indexing (`[-1]`) is unreliable — returns NONE. Use `array::last()` or `array::at()` instead.

### `STARTS WITH` vs `string::starts_with()`

The `STARTS WITH` operator can fail in compound WHERE clauses. Use `string::starts_with()` function instead.

## Performance Patterns

### Batch INSERT to Avoid N+1

```go
rows := make([]map[string]any, len(folders))
for i, f := range folders {
    rows[i] = map[string]any{"vault": vaultRecord, "path": f.Path, "name": f.Name}
}
surrealdb.Query(ctx, db, `INSERT INTO folder $rows ON DUPLICATE KEY UPDATE id = id`, map[string]any{"rows": rows})
```

**Important**: For `record<T>` fields in batch inserts, pass typed `surrealmodels.RecordID` values (not strings). The `type::record()` function doesn't work inside `INSERT ... $rows`.

## Connection Best Practices

- Use `rews` (reconnecting websocket) for production
- Force HTTP/1.1 for WSS to prevent ALPN issues
- Use CBOR codec (`surrealcbor`) for proper type handling
- `surrealdb.New()` and `surrealdb.Connect()` are deprecated — use `surrealdb.FromEndpointURLString()` or `surrealdb.FromConnection()` for custom connections (like rews)
