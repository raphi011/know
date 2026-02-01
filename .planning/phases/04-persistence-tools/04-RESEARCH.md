# Phase 4: Persistence Tools - Research

**Researched:** 2026-02-02
**Domain:** SurrealDB persistence operations, MCP tool handlers, entity/relation upsert patterns
**Confidence:** HIGH

## Summary

This phase implements the `remember` and `forget` MCP tools for persisting entities and relations. The core challenge is implementing upsert semantics with context-scoped uniqueness for entity names, additive label merging on updates, and handling relation deduplication via the existing unique index.

The established patterns from Phase 3 (handler factory, query function layer, context detection) apply directly. New patterns needed: composite ID generation for context-scoped uniqueness, label array merging in SurrealQL, and embedding generation before storage.

**Primary recommendation:** Use composite IDs (slugified `context:name`) for context-scoped entity uniqueness, additive label merge via `array::union`, and rely on existing `unique_key` index for relation deduplication.

## Standard Stack

The persistence tools use the same stack as Phase 3, with no additional libraries required.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/modelcontextprotocol/go-sdk | v1.2.0 | MCP tool handlers | Official SDK, typed handlers |
| github.com/surrealdb/surrealdb.go | v1.2.0 | Database operations | Official SurrealDB Go client |
| embedding.Embedder | (local) | Generate entity embeddings | Already in codebase, 384-dim vectors |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| encoding/json | stdlib | Response formatting | Format tool responses |
| strings | stdlib | ID normalization | Slugify names for composite IDs |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Composite IDs | UPSERT WHERE name=$name AND context=$context | Composite IDs avoid table scan, unique index pattern already works |
| Label merge in Go | array::union in SurrealQL | DB-side merge is atomic, no fetch-merge-write cycle needed |

**Installation:**
No new dependencies required - all packages already in go.mod.

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── db/
│   └── queries.go        # Add: QueryUpsertEntity, QueryCreateRelation, QueryDeleteEntity
├── tools/
│   ├── remember.go       # New: remember tool handler
│   └── forget.go         # New: forget tool handler
└── models/
    ├── entity.go         # Add: Name field for input
    └── relation.go       # Existing: unchanged
```

### Pattern 1: Composite ID for Context-Scoped Uniqueness
**What:** Generate entity ID from context + name to ensure same name can exist in different contexts
**When to use:** Always when creating/updating entities
**Example:**
```go
// Source: Project decision (CONTEXT.md)
func generateEntityID(name, context string) string {
    // Normalize: lowercase, replace spaces with hyphens
    slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
    if context == "" {
        return slug
    }
    return context + ":" + slug
}

// entity:myproject:user-preferences vs entity:otherproject:user-preferences
```

### Pattern 2: Additive Label Merge via SurrealQL
**What:** Use SurrealDB's array::union to merge labels without fetching existing entity
**When to use:** Entity upsert operations
**Example:**
```sql
-- Source: SurrealDB docs array functions
UPSERT type::record("entity", $id) SET
    labels = array::union(labels ?? [], $new_labels),
    content = $content,
    -- other fields...
```

### Pattern 3: Handler Factory with Config
**What:** Follow established Phase 3 pattern for tool handlers
**When to use:** All new tool handlers
**Example:**
```go
// Source: internal/tools/search.go (Phase 3 pattern)
func NewRememberHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[RememberInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input RememberInput) (
        *mcp.CallToolResult, any, error,
    ) {
        // Validate input
        // Generate embedding
        // Execute query
        // Return result
    }
}
```

### Anti-Patterns to Avoid
- **Fetch-merge-write for labels:** Don't fetch entity, merge labels in Go, then write. Use array::union in SQL.
- **Global unique constraint on name:** Don't add UNIQUE index on name alone. Must scope by context.
- **Hardcoded context:** Don't hardcode context. Use DetectContext for fallback.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Label merging | Go-side array merge | `array::union(labels ?? [], $new)` | Atomic, handles nil, no race conditions |
| Relation deduplication | Check-then-insert | Existing `unique_key` index | Schema already has UNIQUE constraint |
| Context detection | Hardcoded string | `DetectContext(cfg)` | Already handles git origin, cwd, config |
| ID normalization | Custom sanitization | Simple slug: lowercase + hyphenate | SurrealDB IDs support most chars |
| Embedding generation | Manual HTTP calls | `deps.Embedder.Embed(ctx, content)` | Embedder interface abstracts provider |

**Key insight:** SurrealDB's UPSERT with computed fields and array functions handles most complexity at the database level. Avoid reimplementing merge logic in Go.

## Common Pitfalls

### Pitfall 1: Forgetting to Generate Embedding Before Store
**What goes wrong:** Entity stored without embedding, breaks vector search
**Why it happens:** Embedding is async, easy to forget await/check
**How to avoid:** Always generate embedding immediately after input validation
**Warning signs:** Entities with nil/empty embedding in database

### Pitfall 2: Using RecordID for Entity Instead of String
**What goes wrong:** SurrealDB Go SDK has quirks with RecordID type for entities
**Why it happens:** Relations use RecordID, entities might seem similar
**How to avoid:** Use `type::record("entity", $id)` in SQL, pass id as string
**Warning signs:** "invalid record id" errors

### Pitfall 3: Not Handling Empty Context in Composite ID
**What goes wrong:** Composite ID becomes `:name` instead of just `name`
**Why it happens:** Context can be nil/empty, string concat adds colon anyway
**How to avoid:** Check context before adding colon separator
**Warning signs:** Entity IDs starting with colon

### Pitfall 4: Relation With Missing Entity Returns Silent Success
**What goes wrong:** RELATE succeeds even if source/target entity doesn't exist
**Why it happens:** SurrealDB creates records on RELATE by default
**How to avoid:** Decision needed (CONTEXT.md "Claude's Discretion"): error vs auto-create
**Warning signs:** Orphan entities appearing in graph

### Pitfall 5: Assuming Forget Cascades Relations
**What goes wrong:** Relations left pointing to deleted entity
**Why it happens:** SurrealDB doesn't auto-cascade by default
**How to avoid:** Schema uses TYPE RELATION which auto-cleans on source/target delete
**Warning signs:** "dead" relations in database

## Code Examples

Verified patterns from official sources and existing codebase:

### Entity Upsert with Label Merge
```go
// Source: Existing db/queries.go pattern + SurrealDB docs
func (c *Client) QueryUpsertEntity(
    ctx context.Context,
    id string,
    entityType string,
    labels []string,
    content string,
    embedding []float32,
    confidence float64,
    source *string,
    context *string,
) (*models.Entity, error) {
    // Use array::union for additive label merge
    // Use ?? [] to handle nil existing labels
    sql := `
        UPSERT type::record("entity", $id) SET
            type = $type,
            labels = array::union(labels ?? [], $labels),
            content = $content,
            embedding = $embedding,
            confidence = $confidence,
            source = $source,
            context = $context,
            accessed = time::now(),
            decay_weight = 1.0
        RETURN AFTER
    `

    results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
        "id":         id,
        "type":       entityType,
        "labels":     labels,
        "content":    content,
        "embedding":  embedding,
        "confidence": confidence,
        "source":     source,
        "context":    context,
    })

    if err != nil {
        return nil, fmt.Errorf("upsert entity: %w", err)
    }

    if results != nil && len(*results) > 0 && len((*results)[0].Result) > 0 {
        return &(*results)[0].Result[0], nil
    }
    return nil, nil
}
```

### Create/Update Relation
```go
// Source: Existing Python db.py + SurrealDB RELATE docs
func (c *Client) QueryCreateRelation(
    ctx context.Context,
    fromID string,
    relType string,
    toID string,
    weight float64,
) error {
    // RELATE creates/updates based on unique_key index
    // Existing schema: unique_key = concat(sort([in, out]), rel_type)
    sql := `
        RELATE $from_rec->relates->$to_rec SET
            rel_type = $rel_type,
            weight = $weight
    `

    _, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
        "from_rec": surrealdb.RecordID{Table: "entity", ID: fromID},
        "to_rec":   surrealdb.RecordID{Table: "entity", ID: toID},
        "rel_type": relType,
        "weight":   weight,
    })

    if err != nil {
        return fmt.Errorf("create relation: %w", err)
    }
    return nil
}
```

### Delete Entity
```go
// Source: Existing Python db.py
func (c *Client) QueryDeleteEntity(ctx context.Context, id string) error {
    // TYPE RELATION in schema auto-cleans relations when entity deleted
    _, err := surrealdb.Query[any](ctx, c.db, `
        DELETE type::record("entity", $id)
    `, map[string]any{"id": id})

    if err != nil {
        return fmt.Errorf("delete entity: %w", err)
    }
    return nil
}
```

### Remember Handler Structure
```go
// Source: Phase 3 search.go pattern
type RememberInput struct {
    Entities []EntityInput `json:"entities,omitempty" jsonschema:"Entities to store"`
    Relations []RelationInput `json:"relations,omitempty" jsonschema:"Relations to create"`
    Context string `json:"context,omitempty" jsonschema:"Project namespace (auto-detected if omitted)"`
}

type EntityInput struct {
    Name       string   `json:"name" jsonschema:"required,Unique name for entity within context"`
    Content    string   `json:"content" jsonschema:"required,Description or value of the entity"`
    Type       string   `json:"type,omitempty" jsonschema:"Entity type (concept, fact, preference, etc)"`
    Labels     []string `json:"labels,omitempty" jsonschema:"Tags for categorization"`
    Confidence float64  `json:"confidence,omitempty" jsonschema:"Confidence score 0-1"`
    Source     string   `json:"source,omitempty" jsonschema:"Where this info came from"`
}

type RelationInput struct {
    From   string  `json:"from" jsonschema:"required,Source entity name"`
    To     string  `json:"to" jsonschema:"required,Target entity name"`
    Type   string  `json:"type" jsonschema:"required,Relation type (e.g. 'prefers', 'knows')"`
    Weight float64 `json:"weight,omitempty" jsonschema:"Relation strength 0-1"`
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| SurrealDB UPDATE creates if missing | Must use UPSERT for create-or-update | SurrealDB 2.0 | Use UPSERT statement explicitly |
| Python uses 'id' field | Go uses 'name' field with composite ID | This phase | Better context-scoped uniqueness |

**Deprecated/outdated:**
- `UPDATE entity:$id SET ...` for upsert: Changed in SurrealDB 2.0, use `UPSERT` statement

## Claude's Discretion Recommendations

The CONTEXT.md left several decisions to implementation discretion. Recommendations:

### Self-relations
**Recommendation:** Allow self-relations (entity relating to itself)
**Reason:** Some entities naturally self-reference (e.g., "this concept refers to itself")

### Missing Entity on Relation
**Recommendation:** Error if source/target entity doesn't exist
**Reason:** Prevents orphan data, user should create entities first
```go
// Check entity exists before RELATE
existing, _ := deps.DB.QueryGetEntity(ctx, fromID)
if existing == nil {
    return ErrorResult("Source entity not found: "+fromID, "Create entity first"), nil, nil
}
```

### Response Verbosity
**Recommendation:** Return full entity on remember success (not just ID)
**Reason:** User can verify what was stored, no extra fetch needed

### Upsert Action Indicator
**Recommendation:** Include "action" field indicating "created" vs "updated"
**Reason:** User knows if this was new info or update
```go
type RememberResult struct {
    EntitiesStored   int    `json:"entities_stored"`
    RelationsStored  int    `json:"relations_stored"`
    Action           string `json:"action,omitempty"` // "created" or "updated" for single entity
}
```

### Forget Behavior
**Recommendations:**
- **Cascade vs error:** Let SurrealDB handle (TYPE RELATION auto-cascades)
- **Soft vs hard delete:** Hard delete (match Python, simplicity)
- **Batch delete:** Support array of IDs for convenience
- **Non-existent entity:** Silent success (idempotent delete)
- **Response format:** Return deleted entity summary
```go
type ForgetResult struct {
    Deleted []string `json:"deleted"` // IDs that were deleted
    Count   int      `json:"count"`
}
```

## Open Questions

Things that couldn't be fully resolved:

1. **UPSERT RETURN behavior with array::union**
   - What we know: RETURN AFTER shows merged result
   - What's unclear: Does RETURN indicate created vs updated?
   - Recommendation: Test in integration, may need pre-check query

2. **Composite ID collision handling**
   - What we know: Slug normalization reduces collisions
   - What's unclear: Edge cases with special characters in names
   - Recommendation: Use conservative slug function, document limits

## Sources

### Primary (HIGH confidence)
- [SurrealDB UPSERT docs](https://surrealdb.com/docs/surrealql/statements/upsert) - UPSERT syntax and behavior
- [SurrealDB RELATE docs](https://surrealdb.com/docs/surrealql/statements/relate) - Relation creation
- [SurrealDB Go SDK](https://pkg.go.dev/github.com/surrealdb/surrealdb.go) - Query method signatures
- Existing codebase: `internal/db/queries.go` - Query function pattern
- Existing codebase: `internal/tools/search.go` - Handler factory pattern
- Existing codebase: `internal/db/schema.go` - unique_key constraint for relations

### Secondary (MEDIUM confidence)
- [SurrealDB array functions](https://surrealdb.com/docs/surrealql/functions/array) - array::union for label merge
- Python `memcp/servers/persist.py` - Original implementation for reference

### Tertiary (LOW confidence)
- None - all critical claims verified with official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependencies, verified packages
- Architecture: HIGH - follows established Phase 3 patterns
- Pitfalls: HIGH - based on SurrealDB docs and Python implementation experience
- Claude's Discretion items: MEDIUM - logical recommendations, need user validation

**Research date:** 2026-02-02
**Valid until:** 2026-03-02 (30 days - stable domain, no breaking changes expected)
