# Phase 7: Procedure Tools - Research

**Researched:** 2026-02-03
**Domain:** Procedural memory storage/search, SurrealDB hybrid search, Go tool handlers
**Confidence:** HIGH

## Summary

Phase 7 implements five tools for procedural memory: `create_procedure` (store with auto-embedding), `get_procedure` (by ID), `delete_procedure` (by ID), `search_procedures` (hybrid search + label filtering), and `list_procedures` (all procedures with optional context filter). Procedures represent step-by-step knowledge (workflows, processes, how-to guides) distinct from entities (facts) and episodes (experiences).

The codebase already has the Procedure model (`internal/models/procedure.go`), schema definitions (`internal/db/schema.go`), and both Python and Go patterns to follow. The key additions are: procedure-specific query functions in `db/queries.go`, new tool handlers in `tools/procedure.go`, and registry updates.

Search uses the same hybrid BM25+vector RRF approach as entities and episodes. BM25 searches both `name` (index 0) and `description` (index 1) fields. The schema includes label filtering via `CONTAINSANY` and context filtering.

**Primary recommendation:** Follow the established episode pattern exactly - create `QueryCreateProcedure`, `QueryGetProcedure`, `QuerySearchProcedures`, `QueryListProcedures`, `QueryUpdateProcedureAccess`, `QueryDeleteProcedure` in `db/queries.go`, then create tool handlers that mirror episode.go but with procedure-specific input/output types.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/surrealdb/surrealdb.go` | v1.2.0 | SurrealDB client with generic Query | Already installed, procedure table schema exists |
| `github.com/modelcontextprotocol/go-sdk/mcp` | v1.2.0 | MCP tool handlers | Handler factory pattern established |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `strings` | stdlib | ID generation/extraction | Handle "procedure:xxx" vs "xxx" format |
| `encoding/json` | stdlib | Format procedure results | Serialize for TextContent |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Name-based ID generation | UUID | Name-based is human-readable, matches Python |
| Struct steps | Map steps | Struct provides type safety, clear schema |

**Already Installed:**
```bash
# No new dependencies needed
```

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── db/
│   └── queries.go      # (extend) Add QueryCreateProcedure, QuerySearchProcedures, etc.
├── tools/
│   ├── procedure.go    # (NEW) Procedure tool handlers
│   └── registry.go     # (update) Register 5 procedure tools
└── models/
    └── procedure.go    # (existing) Procedure, ProcedureStep structs already defined
```

### Pattern 1: Procedure Creation with Name-Based ID

**What:** `create_procedure` stores a procedure with auto-generated embedding from name+description+steps.

**When to use:** Every procedure creation.

**Example:**
```go
// Source: Derived from Python memcp/servers/procedure.py:add_procedure
package tools

// CreateProcedureInput defines input for create_procedure tool.
type CreateProcedureInput struct {
    Name        string              `json:"name" jsonschema:"required,Short name for the procedure (e.g. Deploy to production)"`
    Description string              `json:"description" jsonschema:"required,Brief description of what this procedure does"`
    Steps       []ProcedureStepInput `json:"steps" jsonschema:"required,List of steps with content and optional flag"`
    Context     string              `json:"context,omitempty" jsonschema:"Project namespace (auto-detected if omitted)"`
    Labels      []string            `json:"labels,omitempty" jsonschema:"Tags for categorization (e.g. deployment devops)"`
}

type ProcedureStepInput struct {
    Content  string `json:"content" jsonschema:"required,Step description"`
    Optional bool   `json:"optional,omitempty" jsonschema:"Whether step can be skipped (default false)"`
}

func NewCreateProcedureHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[CreateProcedureInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input CreateProcedureInput) (
        *mcp.CallToolResult, any, error,
    ) {
        // Validate required fields
        if strings.TrimSpace(input.Name) == "" {
            return ErrorResult("Name cannot be empty", "Provide a procedure name"), nil, nil
        }
        if strings.TrimSpace(input.Description) == "" {
            return ErrorResult("Description cannot be empty", "Provide a description"), nil, nil
        }
        if len(input.Steps) == 0 {
            return ErrorResult("Steps cannot be empty", "Provide at least one step"), nil, nil
        }

        // Generate ID from name (sanitize for SurrealDB)
        procID := generateProcedureID(input.Name)

        // Detect context: explicit > config > nil
        var procContext *string
        if input.Context != "" {
            procContext = &input.Context
        } else {
            procContext = DetectContext(cfg)
        }

        // Convert input steps to model steps with auto-numbering
        steps := make([]models.ProcedureStep, len(input.Steps))
        var stepText strings.Builder
        for i, s := range input.Steps {
            steps[i] = models.ProcedureStep{
                Order:    i + 1,
                Content:  s.Content,
                Optional: s.Optional,
            }
            stepText.WriteString(s.Content)
            stepText.WriteString(" ")
        }

        // Generate embedding from name + description + step contents
        embedText := fmt.Sprintf("%s. %s. %s", input.Name, input.Description, stepText.String())
        if len(embedText) > 8000 {
            embedText = embedText[:8000]
        }
        embedding, err := deps.Embedder.Embed(ctx, embedText)
        if err != nil {
            return ErrorResult("Failed to generate embedding", "Check Ollama connection"), nil, nil
        }

        // Create procedure
        proc, err := deps.DB.QueryCreateProcedure(
            ctx, procID, input.Name, input.Description, steps,
            embedding, procContext, input.Labels,
        )
        if err != nil {
            return ErrorResult("Failed to create procedure", "Database error"), nil, nil
        }

        // Build result
        result := CreateProcedureResult{
            ID:          proc.ID,
            Name:        proc.Name,
            Description: proc.Description,
            StepCount:   len(steps),
            Context:     procContext,
            Labels:      input.Labels,
        }

        jsonBytes, _ := json.MarshalIndent(result, "", "  ")
        deps.Logger.Info("create_procedure completed", "id", procID, "steps", len(steps))
        return TextResult(string(jsonBytes)), nil, nil
    }
}

// generateProcedureID creates a sanitized ID from procedure name.
// Converts to lowercase, replaces spaces/underscores with hyphens, removes non-alphanumeric.
func generateProcedureID(name string) string {
    id := strings.ToLower(name)
    id = strings.ReplaceAll(id, " ", "-")
    id = strings.ReplaceAll(id, "_", "-")
    // Keep only alphanumeric and hyphens
    var result strings.Builder
    for _, c := range id {
        if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
            result.WriteRune(c)
        }
    }
    return result.String()
}
```

### Pattern 2: Procedure Search with Hybrid BM25+Vector

**What:** Search procedures using RRF fusion of name/description BM25 and vector similarity.

**When to use:** `search_procedures` tool.

**Example:**
```go
// Source: Derived from Python memcp/db.py:query_search_procedures
func (c *Client) QuerySearchProcedures(
    ctx context.Context,
    query string,
    embedding []float32,
    contextFilter *string,
    labels []string,
    limit int,
) ([]models.Procedure, error) {
    // Build dynamic filter clauses
    filterClause := ""
    if contextFilter != nil {
        filterClause += " AND context = $context"
    }
    if len(labels) > 0 {
        filterClause += " AND labels CONTAINSANY $labels"
    }

    // RRF fusion: vector search + BM25 on name (@0@) and description (@1@)
    // Vector gets 2x limit for diversity, RRF k=60
    sql := fmt.Sprintf(`
        SELECT * FROM search::rrf([
            (SELECT id, name, description, steps, context, labels
             FROM procedure
             WHERE embedding <|%d,40|> $emb %s),
            (SELECT id, name, description, steps, context, labels
             FROM procedure
             WHERE name @0@ $q OR description @1@ $q %s)
        ], $limit, 60)
    `, limit*2, filterClause, filterClause)

    vars := map[string]any{
        "q":      query,
        "emb":    embedding,
        "limit":  limit,
    }
    if contextFilter != nil {
        vars["context"] = *contextFilter
    }
    if len(labels) > 0 {
        vars["labels"] = labels
    }

    results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, vars)
    if err != nil {
        return nil, fmt.Errorf("search procedures: %w", err)
    }

    if results != nil && len(*results) > 0 {
        return (*results)[0].Result, nil
    }
    return []models.Procedure{}, nil
}
```

### Pattern 3: List Procedures with Context Filter

**What:** List all procedures with optional context filtering, ordered by last access.

**When to use:** `list_procedures` tool.

**Example:**
```go
// Source: Derived from Python memcp/db.py:query_list_procedures
func (c *Client) QueryListProcedures(
    ctx context.Context,
    contextFilter *string,
    limit int,
) ([]models.Procedure, error) {
    filterClause := ""
    vars := map[string]any{"limit": limit}
    if contextFilter != nil {
        filterClause = "WHERE context = $context"
        vars["context"] = *contextFilter
    }

    // Return name, description, step count (not full steps) for list view
    sql := fmt.Sprintf(`
        SELECT id, name, description, array::len(steps) AS step_count, context, labels, accessed
        FROM procedure %s
        ORDER BY accessed DESC
        LIMIT $limit
    `, filterClause)

    results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, vars)
    if err != nil {
        return nil, fmt.Errorf("list procedures: %w", err)
    }

    if results != nil && len(*results) > 0 {
        return (*results)[0].Result, nil
    }
    return []models.Procedure{}, nil
}
```

### Pattern 4: Procedure ID Extraction

**What:** Extract bare ID from "procedure:xxx" format.

**When to use:** get_procedure, delete_procedure handlers.

**Example:**
```go
// extractProcedureID removes "procedure:" prefix if present.
func extractProcedureID(id string) string {
    return strings.TrimPrefix(id, "procedure:")
}
```

### Anti-Patterns to Avoid

- **Empty steps array:** Validate at least one step is provided
- **Missing step content:** Each step must have non-empty content
- **Searching only name OR description:** BM25 should search both fields
- **Returning full steps in list:** list_procedures should return step_count, not full steps
- **Mutable step order:** Steps should maintain order from creation (no step-level operations per CONTEXT.md)

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Hybrid search ranking | Custom scoring | `search::rrf()` | RRF handles BM25 vs vector scale differences |
| ID sanitization | Regex patterns | Simple char filter | Predictable, no regex dependency |
| Step ordering | Manual indexing | Array position on input | Simpler, matches Python |
| Label deduplication | Manual loop | `array::distinct()` in SurrealDB | Database-level optimization |

**Key insight:** The procedure schema already exists with HNSW vector index, BM25 on name+description, and label index. Use existing infrastructure; follow episode pattern.

## Common Pitfalls

### Pitfall 1: Name-Based ID Collisions

**What goes wrong:** Two procedures with similar names get same ID (e.g., "Deploy App" and "Deploy APP").

**Why it happens:** ID generation lowercases and strips characters.

**How to avoid:**
- Use UPSERT semantics (same as Python) - duplicate name overwrites
- Document this behavior in tool description
- Consider adding context to ID for uniqueness within project

**Warning signs:** Procedure content unexpectedly changes.

### Pitfall 2: BM25 Index Selection

**What goes wrong:** Search only matches name, not description.

**Why it happens:** Using wrong analyzer index number (@0@ vs @1@).

**How to avoid:**
- Schema defines `procedure_name_ft` first (index 0), `procedure_desc_ft` second (index 1)
- Query both: `name @0@ $q OR description @1@ $q`
- Test search for terms appearing only in description

**Warning signs:** Searches miss procedures that should match by description.

### Pitfall 3: Steps Array Serialization

**What goes wrong:** Steps stored as empty array or wrong format.

**Why it happens:** Go struct serialization vs SurrealDB FLEXIBLE object handling.

**How to avoid:**
- Explicitly convert `[]ProcedureStepInput` to `[]models.ProcedureStep` with order
- Ensure each step has order, content, optional fields
- Test round-trip: create, get, verify steps match

**Warning signs:** Empty steps in get_procedure response.

### Pitfall 4: List vs Search Result Format

**What goes wrong:** list_procedures returns full steps, making response too large.

**Why it happens:** Using same result type as get_procedure.

**How to avoid:**
- list_procedures: Return only id, name, description, step_count, context, labels
- get_procedure: Return full steps array
- Create separate ListProcedureResult type if needed

**Warning signs:** list_procedures response is very large with many procedures.

### Pitfall 5: Context Auto-Detection Not Applied

**What goes wrong:** Procedures created without context, then search with auto-detected context finds nothing.

**Why it happens:** Context detection inconsistent between create and search.

**How to avoid:**
- Use same DetectContext(cfg) pattern in all tools
- Apply context detection in create, search, AND list
- Document that context is auto-detected if not provided

**Warning signs:** search_procedures returns nothing after create_procedure.

## Code Examples

### Complete Procedure Creation Query

```go
// Source: Derived from Python memcp/db.py:query_create_procedure
func (c *Client) QueryCreateProcedure(
    ctx context.Context,
    procedureID string,
    name string,
    description string,
    steps []models.ProcedureStep,
    embedding []float32,
    procContext *string,
    labels []string,
) (*models.Procedure, error) {
    // Ensure labels is not nil
    if labels == nil {
        labels = []string{}
    }

    // Convert steps to generic interface for SurrealDB
    stepsData := make([]map[string]any, len(steps))
    for i, s := range steps {
        stepsData[i] = map[string]any{
            "order":    s.Order,
            "content":  s.Content,
            "optional": s.Optional,
        }
    }

    sql := `
        UPSERT type::record("procedure", $id) SET
            name = $name,
            description = $description,
            steps = $steps,
            embedding = $embedding,
            context = $context,
            labels = array::distinct($labels),
            accessed = time::now(),
            created = IF created THEN created ELSE time::now() END,
            access_count = IF access_count THEN access_count ELSE 0 END
        RETURN AFTER
    `

    results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, map[string]any{
        "id":          procedureID,
        "name":        name,
        "description": description,
        "steps":       stepsData,
        "embedding":   embedding,
        "context":     procContext,
        "labels":      labels,
    })
    if err != nil {
        return nil, fmt.Errorf("create procedure: %w", err)
    }

    if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
        return nil, fmt.Errorf("create procedure: no result returned")
    }

    return &(*results)[0].Result[0], nil
}
```

### Get Procedure by ID

```go
func (c *Client) QueryGetProcedure(ctx context.Context, id string) (*models.Procedure, error) {
    results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, `
        SELECT * FROM type::record("procedure", $id)
    `, map[string]any{"id": id})

    if err != nil {
        return nil, fmt.Errorf("get procedure: %w", err)
    }

    if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
        return nil, nil
    }
    return &(*results)[0].Result[0], nil
}
```

### Update Procedure Access Tracking

```go
func (c *Client) QueryUpdateProcedureAccess(ctx context.Context, id string) error {
    _, err := surrealdb.Query[any](ctx, c.db, `
        UPDATE type::record("procedure", $id) SET
            accessed = time::now(),
            access_count += 1
    `, map[string]any{"id": id})
    if err != nil {
        return fmt.Errorf("update procedure access: %w", err)
    }
    return nil
}
```

### Delete Procedure

```go
func (c *Client) QueryDeleteProcedure(ctx context.Context, id string) (int, error) {
    sql := `DELETE type::record("procedure", $id) RETURN BEFORE`

    results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, map[string]any{
        "id": id,
    })
    if err != nil {
        return 0, fmt.Errorf("delete procedure: %w", err)
    }

    if results == nil || len(*results) == 0 {
        return 0, nil
    }
    return len((*results)[0].Result), nil
}
```

### Tool Handler Input/Output Types

```go
// CreateProcedureInput for create_procedure tool.
type CreateProcedureInput struct {
    Name        string              `json:"name" jsonschema:"required,Short name for the procedure"`
    Description string              `json:"description" jsonschema:"required,Brief description"`
    Steps       []ProcedureStepInput `json:"steps" jsonschema:"required,List of steps"`
    Context     string              `json:"context,omitempty" jsonschema:"Project namespace"`
    Labels      []string            `json:"labels,omitempty" jsonschema:"Tags for categorization"`
}

type ProcedureStepInput struct {
    Content  string `json:"content" jsonschema:"required,Step description"`
    Optional bool   `json:"optional,omitempty" jsonschema:"Can be skipped"`
}

// GetProcedureInput for get_procedure tool.
type GetProcedureInput struct {
    ID string `json:"id" jsonschema:"required,Procedure ID (with or without 'procedure:' prefix)"`
}

// DeleteProcedureInput for delete_procedure tool.
type DeleteProcedureInput struct {
    ID string `json:"id" jsonschema:"required,Procedure ID to delete"`
}

// SearchProceduresInput for search_procedures tool.
type SearchProceduresInput struct {
    Query   string   `json:"query" jsonschema:"required,Semantic search query"`
    Context string   `json:"context,omitempty" jsonschema:"Project namespace filter"`
    Labels  []string `json:"labels,omitempty" jsonschema:"Filter by tags"`
    Limit   int      `json:"limit,omitempty" jsonschema:"Max results 1-50 (default 10)"`
}

// ListProceduresInput for list_procedures tool.
type ListProceduresInput struct {
    Context string `json:"context,omitempty" jsonschema:"Project namespace filter"`
    Limit   int    `json:"limit,omitempty" jsonschema:"Max results 1-100 (default 50)"`
}

// CreateProcedureResult returned from create_procedure.
type CreateProcedureResult struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Description string   `json:"description"`
    StepCount   int      `json:"step_count"`
    Context     *string  `json:"context,omitempty"`
    Labels      []string `json:"labels,omitempty"`
}

// ProcedureSearchResult returned from search_procedures.
type ProcedureSearchResult struct {
    Procedures []ProcedureResult `json:"procedures"`
    Count      int               `json:"count"`
}

// ProcedureResult represents a procedure in search/list results.
type ProcedureResult struct {
    ID          string                 `json:"id"`
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Steps       []models.ProcedureStep `json:"steps,omitempty"` // Only in get, not list
    StepCount   int                    `json:"step_count,omitempty"` // Only in list
    Context     *string                `json:"context,omitempty"`
    Labels      []string               `json:"labels,omitempty"`
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Separate BM25 + vector | `search::rrf()` | SurrealDB 2.0 | Unified hybrid search |
| Dynamic table names | Single table with rel_type | Phase 4 | Simpler relation queries |
| User-provided IDs | Name-based auto-generation | Python impl | Consistent, predictable |

**Deprecated/outdated:**
- `DISTINCT`: Use `GROUP BY` in SurrealDB v3.0
- Step-level operations: Not in scope per CONTEXT.md

## Open Questions

1. **ID uniqueness across contexts**
   - What we know: Python uses name-based IDs, same name = same ID
   - What's unclear: Should ID include context for uniqueness?
   - Recommendation: Keep Python behavior (UPSERT overwrites). Document that same name in same context overwrites. If cross-context uniqueness needed, prefix ID with context hash.

2. **Step validation depth**
   - What we know: CONTEXT.md leaves step structure to discretion
   - What's unclear: Minimum content length? Allow empty optional steps?
   - Recommendation: Require non-empty content for all steps. Empty content is validation error.

3. **List procedure ordering**
   - What we know: CONTEXT.md leaves sort order to discretion
   - What's unclear: Order by accessed (recency) or created (chronological)?
   - Recommendation: Order by accessed DESC (most recently used first) - matches Python.

## Sources

### Primary (HIGH confidence)
- Python `memcp/servers/procedure.py` - Reference implementation (verified)
- Python `memcp/db.py` lines 1107-1210 - Procedure query patterns (verified)
- Go codebase `internal/db/schema.go` - Procedure schema exists (verified)
- Go codebase `internal/models/procedure.go` - Procedure struct exists (verified)
- Go codebase `internal/tools/episode.go` - Handler pattern to follow (verified)

### Secondary (MEDIUM confidence)
- SurrealDB v3.0 documentation - RRF syntax, BM25 index selection
- Phase 6 RESEARCH.md - Episode handler patterns (same approach)

### Tertiary (LOW confidence)
- None - all patterns verified against existing code

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - No new dependencies, existing patterns
- Architecture: HIGH - Mirrors episode tools exactly
- Query patterns: HIGH - Python implementation + Go episode tools verified
- Pitfalls: MEDIUM - Based on Python experience, schema specifics

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (30 days - stable patterns)
