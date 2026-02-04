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

## Connection Best Practices

- Use `rews` (reconnecting websocket) for production
- Force HTTP/1.1 for WSS to prevent ALPN issues
- Use CBOR codec (`surrealcbor`) for proper type handling
