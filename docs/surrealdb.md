# SurrealDB

Technical learnings about SurrealDB v3.0 patterns and gotchas.

## HNSW Vector Index

### Definition

```sql
DEFINE INDEX idx_entity_embedding ON entity FIELDS embedding
    HNSW DIMENSION 1024 DIST COSINE TYPE F32 EFC 150 M 12;
```

Parameters:
- `DIMENSION` - Must match embedding vector size exactly
- `DIST COSINE` - Cosine similarity (best for normalized embeddings)
- `TYPE F32` - 32-bit floats
- `EFC 150` - Expansion factor at construction (higher = better quality, slower build)
- `M 12` - Max connections per node (higher = better recall, more memory)

### Gotchas

1. **Dimension changes require fresh DB** - Can't ALTER index dimension
2. **Optional embeddings** - Use `option<array<float>>` for nullable
3. **Batch inserts** - HNSW builds during insert, can be slow for large batches
4. **HNSW rejects NONE values** - Even on `option<array<float>>` fields, the HNSW index cannot index NONE values. Setting `embedding = NONE` in a CREATE statement causes: `Couldn't coerce value for field 'embedding': Expected 'array' but found 'NONE'`. **Fix**: Omit the embedding field entirely from the CREATE statement when no embedding is available. The async embedding worker can UPDATE the field later — UPDATE works fine because it replaces NONE with the actual vector.

```sql
-- BAD: HNSW chokes on NONE
CREATE chunk SET content = $content, embedding = NONE

-- GOOD: omit the field entirely, fill it later via UPDATE
CREATE chunk SET content = $content
-- ...later...
UPDATE chunk:xyz SET embedding = $embedding
```

## v3.0 Breaking Changes

### KNN Operator

```sql
-- v2.x (deprecated)
vector::distance::knn(embedding, $query_vec)

-- v3.0 (new)
embedding <|10,COSINE|> $query_vec
```

The `<|K,DIST|>` operator:
- K = number of nearest neighbors
- DIST = distance metric (COSINE, EUCLIDEAN, etc.)
- Returns pre-filtered results from HNSW index

### Fulltext Search

```sql
-- BM25 scoring with analyzer
DEFINE ANALYZER entity_analyzer
    TOKENIZERS class
    FILTERS lowercase, ascii, snowball(english);

DEFINE INDEX idx_content_ft ON entity FIELDS content
    FULLTEXT ANALYZER entity_analyzer BM25;

-- Query
SELECT * FROM entity WHERE content @@ 'search terms';
```

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
// bareID strips the "table:" prefix if present.
// Callers can pass either "default" or "vault:default" — both work.
func bareID(table, id string) string {
    return strings.TrimPrefix(id, table+":")
}

// Usage in queries
vars := map[string]any{
    "vault_id": bareID("vault", vaultID),
}
```

Apply `bareID` to **every** query parameter that feeds into `type::record()`. Missing even one causes silent empty results that are hard to debug.

### `INSIDE` Requires Typed Record IDs, Not Strings

When using `id INSIDE $ids` to batch-fetch records, the Go SDK's CBOR layer distinguishes record IDs from strings. Passing `[]string{"document:abc123"}` will **silently match nothing** — SurrealDB compares a `record<document>` field against plain strings and they never equal.

**Fix**: Pass `[]surrealmodels.RecordID` so the SDK serializes proper CBOR record IDs:

```go
// BAD: strings look like record IDs but aren't — silent empty results
recordIDs := make([]string, len(ids))
for i, id := range ids {
    recordIDs[i] = "document:" + id
}

// GOOD: typed record IDs via SDK
recordIDs := make([]surrealmodels.RecordID, len(ids))
for i, id := range ids {
    recordIDs[i] = newRecordID("document", bareID("document", id))
}
```

This applies to any query that passes an array of IDs for `INSIDE`, `CONTAINSANY`, or similar set operations.

## Hybrid Search Pattern

Combine vector and fulltext search with RRF:

```sql
LET $vec_results = (
    SELECT id, name, content, labels,
           embedding <|20,COSINE|> $embedding AS vec_score
    FROM entity
    WHERE embedding <|20,COSINE|> $embedding
);

LET $ft_results = (
    SELECT id, name, content, labels,
           search::score(0) AS ft_score
    FROM entity
    WHERE content @0@ $query
    LIMIT 20
);

-- RRF fusion
SELECT id, name, content, labels,
       math::sum(1.0 / (60 + vec_rank), 1.0 / (60 + ft_rank)) AS rrf_score
FROM (
    SELECT *, array::find_index($vec_results.id, id) AS vec_rank
    FROM $vec_results
) UNION ALL (
    SELECT *, array::find_index($ft_results.id, id) AS ft_rank
    FROM $ft_results
)
GROUP BY id
ORDER BY rrf_score DESC
LIMIT $limit;
```

## Query Gotchas

### Compound OR in WHERE Clauses

As of v3.0.2, parenthesized OR conditions in WHERE clauses can cause parse errors:

```sql
-- FAILS: parse error "Unexpected token `an identifier` expected delimiter `)`"
DELETE FROM folder WHERE vault = type::record("vault", $id)
  AND (path = $path OR string::starts_with(path, $prefix))
```

**Workaround**: Split into two separate queries:

```sql
DELETE FROM folder WHERE vault = type::record("vault", $id) AND path = $path;
DELETE FROM folder WHERE vault = type::record("vault", $id) AND string::starts_with(path, $prefix);
```

### Negative Array Indexing

Bracket-based negative indexing (`[-1]`) is unreliable in SurrealQL — it returns NONE instead of the last element. Use `array::last()` or `array::at()` instead:

```sql
-- BAD: returns NONE
string::split("/a/b/c", "/")[-1]

-- GOOD: returns "c"
array::last(string::split("/a/b/c", "/"))
```

### `STARTS WITH` Operator vs `string::starts_with()`

The `STARTS WITH` operator can fail in compound WHERE clauses. Use the `string::starts_with()` function instead:

```sql
-- May fail in compound WHERE
SELECT * FROM folder WHERE vault = $v AND path STARTS WITH $prefix

-- Reliable
SELECT * FROM folder WHERE vault = $v AND string::starts_with(path, $prefix)
```

## Performance Patterns

### Batch INSERT to Avoid N+1

Instead of looping over rows in Go and sending one INSERT per iteration, use `INSERT INTO table $rows` with an array parameter. This sends a single query regardless of how many rows are being inserted:

```go
// BAD: N round-trips for N folders
for _, f := range folders {
    surrealdb.Query(ctx, db, `INSERT INTO folder { vault: $v, path: $p, name: $n }`, ...)
}

// GOOD: 1 round-trip for N folders
rows := make([]map[string]any, len(folders))
for i, f := range folders {
    rows[i] = map[string]any{"vault": vaultRecord, "path": f.Path, "name": f.Name}
}
surrealdb.Query(ctx, db, `INSERT INTO folder $rows ON DUPLICATE KEY UPDATE id = id`, map[string]any{"rows": rows})
```

**Important**: For `record<T>` fields in batch inserts, pass typed `surrealmodels.RecordID` values (not strings). The `type::record()` function doesn't work inside `INSERT ... $rows` — the rows are treated as literal objects.

This also works with `ON DUPLICATE KEY UPDATE id = id` for idempotent upserts.

### Multi-Statement Queries to Reduce Round-Trips

Send multiple statements separated by `;` in a single `surrealdb.Query` call. Each statement returns its own result set in the response array. All statements share the same parameter map:

```go
// BAD: 4 round-trips
surrealdb.Query(ctx, db, `DELETE FROM document WHERE vault = $v`, vars)
surrealdb.Query(ctx, db, `DELETE FROM folder WHERE vault = $v`, vars)
surrealdb.Query(ctx, db, `DELETE FROM template WHERE vault = $v`, vars)
surrealdb.Query(ctx, db, `DELETE type::record("vault", $id)`, vars)

// GOOD: 1 round-trip, 4 result sets
sql := `DELETE FROM document WHERE vault = $v;` +
    `DELETE FROM folder WHERE vault = $v;` +
    `DELETE FROM template WHERE vault = $v;` +
    `DELETE type::record("vault", $id)`
surrealdb.Query(ctx, db, sql, vars)
```

**Counting results across statements**: Use `RETURN BEFORE` and iterate all result sets:

```go
results, err := surrealdb.Query[[]MyType](ctx, db, sql, vars)
count := 0
if results != nil {
    for _, r := range *results {
        count += len(r.Result)
    }
}
```

**Note**: Multi-statement queries are NOT transactions — each statement commits independently. Use `BEGIN TRANSACTION; ...; COMMIT TRANSACTION;` if atomicity is needed. However, multi-statement is still useful for reducing network latency when atomicity isn't critical.

## Connection Best Practices

- Use `rews` (reconnecting websocket) for production
- Force HTTP/1.1 for WSS to prevent ALPN issues
- Use CBOR codec (`surrealcbor`) for proper type handling
