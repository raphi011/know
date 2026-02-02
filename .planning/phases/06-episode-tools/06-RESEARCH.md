# Phase 6: Episode Tools - Research

**Researched:** 2026-02-02
**Domain:** Episodic memory storage/search, SurrealDB hybrid search with temporal filtering, Go tool handlers
**Confidence:** HIGH

## Summary

Phase 6 implements four tools for episodic memory: `add_episode` (store with auto-embedding), `search_episodes` (hybrid search + time filtering), `get_episode` (by ID with optional entity linking), and `delete_episode` (by ID). Episodes are distinct from entities - they represent temporal experiences/conversations vs facts/knowledge.

The codebase already has the Episode model (`internal/models/episode.go`), schema definitions (`internal/db/schema.go`), and the Python implementation provides proven query patterns. The key additions are: episode-specific query functions in `db/queries.go`, new tool handlers in `tools/episode.go`, and registry updates.

Search uses the same hybrid BM25+vector RRF approach as entity search, with added temporal filtering (`timestamp >= $start AND timestamp <= $end`) and recency boosting. The `extracted_from` relation table enables bi-directional entity-episode linking.

**Primary recommendation:** Follow the established query layer pattern - create `QueryCreateEpisode`, `QuerySearchEpisodes`, `QueryGetEpisode`, `QueryUpdateEpisodeAccess`, `QueryDeleteEpisode` in `db/queries.go`, then create tool handlers that mirror the search.go pattern but with episode-specific input/output types.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/surrealdb/surrealdb.go` | v1.2.0 | SurrealDB client with generic Query | Already installed, episode table schema exists |
| `github.com/modelcontextprotocol/go-sdk/mcp` | v1.2.0 | MCP tool handlers | Handler factory pattern established |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `time` | stdlib | Timestamp handling | Auto-generate `created`, time-range parsing |
| `encoding/json` | stdlib | Format episode results | Serialize for TextContent |
| `strings` | stdlib | ID extraction | Handle "episode:xxx" vs "xxx" format |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| ISO 8601 time strings | RFC 3339 | ISO 8601 is more familiar, SurrealDB requires timezone |
| Auto-generated timestamps | User-provided | Simpler, per CONTEXT.md decision |

**Already Installed:**
```bash
# No new dependencies needed
```

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── db/
│   └── queries.go      # (extend) Add QueryCreateEpisode, QuerySearchEpisodes, etc.
├── tools/
│   ├── episode.go      # (NEW) Episode tool handlers
│   └── registry.go     # (update) Register 4 episode tools
└── models/
    └── episode.go      # (existing) Episode struct already defined
```

### Pattern 1: Episode Creation with Auto-Generated ID and Embedding

**What:** `add_episode` stores content with auto-generated timestamp-based ID and embedding.

**When to use:** Every episode creation.

**Example:**
```go
// Source: Derived from Python memcp/servers/episode.py:add_episode
package tools

// AddEpisodeInput defines input for add_episode tool.
type AddEpisodeInput struct {
    Content    string         `json:"content" jsonschema:"required,Full episode content (conversation, notes, etc.)"`
    Summary    string         `json:"summary,omitempty" jsonschema:"Optional brief summary"`
    Metadata   map[string]any `json:"metadata,omitempty" jsonschema:"Flexible metadata (session_id, source, participants)"`
    Context    string         `json:"context,omitempty" jsonschema:"Project namespace (auto-detected if omitted)"`
    EntityIDs  []string       `json:"entity_ids,omitempty" jsonschema:"Entity IDs to link as extracted from this episode"`
}

func NewAddEpisodeHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[AddEpisodeInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input AddEpisodeInput) (
        *mcp.CallToolResult, any, error,
    ) {
        // Validate content
        if strings.TrimSpace(input.Content) == "" {
            return ErrorResult("Episode content cannot be empty", "Provide content to store"), nil, nil
        }

        // Auto-generate timestamp and ID
        now := time.Now().UTC()
        ts := now.Format(time.RFC3339)
        // ID format: ep_2026-02-02T15-04-05Z (safe for SurrealDB)
        episodeID := "ep_" + strings.ReplaceAll(strings.ReplaceAll(ts, ":", "-"), "+", "-")

        // Detect context (same pattern as entities)
        var episodeContext *string
        if input.Context != "" {
            episodeContext = &input.Context
        } else {
            episodeContext = DetectContext(cfg)
        }

        // Generate embedding (truncate for model limits)
        contentForEmbed := input.Content
        if len(contentForEmbed) > 8000 {
            contentForEmbed = contentForEmbed[:8000]
        }
        embedding, err := deps.Embedder.Embed(ctx, contentForEmbed)
        if err != nil {
            return ErrorResult("Failed to generate embedding", "Check Ollama connection"), nil, nil
        }

        // Create episode
        episode, err := deps.DB.QueryCreateEpisode(ctx, episodeID, input.Content, embedding, ts,
            nilIfEmpty(input.Summary), input.Metadata, episodeContext)
        if err != nil {
            return ErrorResult("Failed to store episode", "Database error"), nil, nil
        }

        // Link entities if provided
        linkedCount := 0
        for i, entityID := range input.EntityIDs {
            if err := deps.DB.QueryLinkEntityToEpisode(ctx, entityID, episodeID, i, 1.0); err != nil {
                deps.Logger.Debug("failed to link entity", "entity", entityID, "error", err)
            } else {
                linkedCount++
            }
        }

        // Return truncated content in result
        result := EpisodeResult{
            ID:             "episode:" + episodeID,
            Content:        truncateContent(input.Content, 500),
            Timestamp:      ts,
            Summary:        episode.Summary,
            Metadata:       input.Metadata,
            Context:        episodeContext,
            LinkedEntities: linkedCount,
        }
        jsonBytes, _ := json.MarshalIndent(result, "", "  ")
        return TextResult(string(jsonBytes)), nil, nil
    }
}
```

### Pattern 2: Episode Search with Time Range Filtering

**What:** Hybrid search with optional time-range filters and recency boosting.

**When to use:** `search_episodes` tool.

**Example:**
```go
// Source: Derived from Python memcp/db.py:799 query_search_episodes
func (c *Client) QuerySearchEpisodes(
    ctx context.Context,
    query string,
    embedding []float32,
    timeStart *string,  // ISO 8601 format
    timeEnd *string,    // ISO 8601 format
    contextFilter *string,
    limit int,
) ([]models.Episode, error) {
    // Build time filter clauses
    timeFilter := ""
    if timeStart != nil {
        timeFilter += " AND timestamp >= <datetime>$time_start"
    }
    if timeEnd != nil {
        timeFilter += " AND timestamp <= <datetime>$time_end"
    }
    contextClause := ""
    if contextFilter != nil {
        contextClause = " AND context = $context"
    }

    // RRF hybrid search (same k=60 as entities)
    // Vector gets 2x limit for diversity
    sql := fmt.Sprintf(`
        SELECT * FROM search::rrf([
            (SELECT id, content, summary, timestamp, metadata, context
             FROM episode
             WHERE embedding <|%d,40|> $emb %s %s),
            (SELECT id, content, summary, timestamp, metadata, context
             FROM episode
             WHERE content @0@ $q %s %s)
        ], $limit, 60)
    `, limit*2, timeFilter, contextClause, timeFilter, contextClause)

    results, err := surrealdb.Query[[]models.Episode](ctx, c.db, sql, map[string]any{
        "q":          query,
        "emb":        embedding,
        "time_start": timeStart,
        "time_end":   timeEnd,
        "context":    contextFilter,
        "limit":      limit,
    })
    if err != nil {
        return nil, fmt.Errorf("search episodes: %w", err)
    }

    if results == nil || len(*results) == 0 {
        return []models.Episode{}, nil
    }
    return (*results)[0].Result, nil
}
```

### Pattern 3: Get Episode with Optional Entity Linking

**What:** Retrieve episode by ID, optionally with linked entities.

**When to use:** `get_episode` tool with `include_entities` flag.

**Example:**
```go
// GetEpisodeInput for episode retrieval.
type GetEpisodeInput struct {
    ID              string `json:"id" jsonschema:"required,Episode ID (with or without 'episode:' prefix)"`
    IncludeEntities bool   `json:"include_entities,omitempty" jsonschema:"Include linked entities in response"`
}

// Query to get linked entities via extracted_from relation
func (c *Client) QueryGetEpisodeEntities(ctx context.Context, episodeID string) ([]models.Entity, error) {
    // Reverse traversal: episode <- extracted_from <- entity
    results, err := surrealdb.Query[[]struct{ Entities []models.Entity }](ctx, c.db, `
        SELECT <-extracted_from<-entity.* AS entities FROM type::record("episode", $id)
    `, map[string]any{"id": episodeID})

    if err != nil {
        return nil, fmt.Errorf("get episode entities: %w", err)
    }

    if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
        return []models.Entity{}, nil
    }
    return (*results)[0].Result[0].Entities, nil
}
```

### Pattern 4: Episode ID Generation

**What:** Timestamp-based episode IDs that are URL-safe for SurrealDB records.

**When to use:** Every `add_episode` call.

**Example:**
```go
// generateEpisodeID creates timestamp-based ID safe for SurrealDB.
// Format: ep_2026-02-02T15-04-05Z (colons replaced with hyphens)
func generateEpisodeID() string {
    ts := time.Now().UTC().Format(time.RFC3339)
    // Replace characters that may cause issues in record IDs
    safe := strings.ReplaceAll(ts, ":", "-")
    safe = strings.ReplaceAll(safe, "+", "-")
    return "ep_" + safe
}
```

### Anti-Patterns to Avoid

- **User-provided timestamps:** Per CONTEXT.md, timestamps are auto-generated
- **Missing timezone in timestamps:** SurrealDB requires `2026-02-02T15:04:05Z` format
- **Blocking on entity linking failures:** Link errors should be logged, not fail the operation
- **Returning full content in search results:** Truncate to 500 chars for readability
- **Mutable episodes without updated_at:** Recommend making episodes immutable (no update tool)

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Hybrid search ranking | Custom scoring | `search::rrf()` | RRF handles BM25 vs vector scale differences |
| Timestamp parsing | Manual parsing | `time.Parse(time.RFC3339, ...)` | Handles ISO 8601 with timezone |
| Entity linking | Direct INSERT | `extracted_from` relation table | Schema defines proper relation with position/confidence |
| Recency boosting | Post-query sort | Add ORDER BY recency in RRF subqueries | Database-level optimization |

**Key insight:** The episode schema already exists with all necessary indexes (HNSW, BM25, timestamp). Use the existing infrastructure; don't re-implement search logic.

## Common Pitfalls

### Pitfall 1: Timestamp Format Without Timezone

**What goes wrong:** SurrealDB rejects timestamps without timezone indicator.

**Why it happens:** `2026-02-02T15:04:05` is invalid; needs `2026-02-02T15:04:05Z`.

**How to avoid:**
- Always use `time.RFC3339` format (includes Z or offset)
- Add `Z` suffix if timestamp lacks timezone
- Test with various timezone formats

**Warning signs:** "invalid datetime" errors on episode creation.

### Pitfall 2: Episode ID Characters

**What goes wrong:** SurrealDB record IDs fail with certain characters.

**Why it happens:** Colons in timestamp (15:04:05) may conflict with record ID parsing.

**How to avoid:**
- Replace `:` with `-` in episode IDs
- Use format: `ep_2026-02-02T15-04-05Z`
- Test ID creation with edge cases

**Warning signs:** "record not found" when ID looks correct.

### Pitfall 3: Entity Linking Silently Fails

**What goes wrong:** `entity_ids` in add_episode never get linked.

**Why it happens:** Entity doesn't exist, or ID format mismatch.

**How to avoid:**
- Log linking failures (don't fail the whole operation)
- Return `linked_entities` count in response
- Document that entities must exist before linking

**Warning signs:** `linked_entities: 0` when entities were provided.

### Pitfall 4: Time Range Filter with NULL Context

**What goes wrong:** Search returns no results when context is nil but time filters are set.

**Why it happens:** SQL building error when combining optional filters.

**How to avoid:**
- Test all combinations: no filters, time only, context only, both
- Use consistent `AND` placement in query building
- Check nil pointers before adding filter clauses

**Warning signs:** Filters work individually but not combined.

### Pitfall 5: Embedding Too-Long Content

**What goes wrong:** Embedding model rejects content or truncates unexpectedly.

**Why it happens:** Content exceeds model token limit.

**How to avoid:**
- Truncate content to 8000 chars before embedding (match Python)
- Log when truncation happens
- Consider chunking for very long episodes

**Warning signs:** Embedding errors or poor search quality on long episodes.

## Code Examples

### Complete Episode Creation Query

```go
// Source: Derived from Python memcp/db.py:763 query_create_episode
func (c *Client) QueryCreateEpisode(
    ctx context.Context,
    episodeID string,
    content string,
    embedding []float32,
    timestamp string,
    summary *string,
    metadata map[string]any,
    context_ *string,
) (*models.Episode, error) {
    // Ensure metadata is not nil (SurrealDB needs empty object, not null)
    if metadata == nil {
        metadata = map[string]any{}
    }

    results, err := surrealdb.Query[[]models.Episode](ctx, c.db, `
        UPSERT type::record("episode", $id) SET
            content = $content,
            embedding = $embedding,
            timestamp = IF $timestamp THEN <datetime>$timestamp ELSE time::now() END,
            summary = $summary,
            metadata = $metadata,
            context = $context
        RETURN AFTER
    `, map[string]any{
        "id":        episodeID,
        "content":   content,
        "embedding": embedding,
        "timestamp": timestamp,
        "summary":   summary,
        "metadata":  metadata,
        "context":   context_,
    })
    if err != nil {
        return nil, fmt.Errorf("create episode: %w", err)
    }

    if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
        return nil, fmt.Errorf("create episode: no result returned")
    }
    return &(*results)[0].Result[0], nil
}
```

### Episode Search with Time Filters

```go
// SearchEpisodesInput for search_episodes tool.
type SearchEpisodesInput struct {
    Query     string `json:"query" jsonschema:"required,Semantic search query"`
    TimeStart string `json:"time_start,omitempty" jsonschema:"Filter episodes after this time (ISO 8601)"`
    TimeEnd   string `json:"time_end,omitempty" jsonschema:"Filter episodes before this time (ISO 8601)"`
    Context   string `json:"context,omitempty" jsonschema:"Project namespace filter"`
    Limit     int    `json:"limit,omitempty" jsonschema:"Max results 1-50 (default 10)"`
}

func NewSearchEpisodesHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[SearchEpisodesInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input SearchEpisodesInput) (
        *mcp.CallToolResult, any, error,
    ) {
        // Validate
        if strings.TrimSpace(input.Query) == "" {
            return ErrorResult("Query cannot be empty", "Provide a search query"), nil, nil
        }

        limit := input.Limit
        if limit <= 0 {
            limit = 10
        }
        if limit > 50 {
            return ErrorResult("Limit must be 1-50", "Reduce limit value"), nil, nil
        }

        // Generate embedding
        embedding, err := deps.Embedder.Embed(ctx, input.Query)
        if err != nil {
            return ErrorResult("Failed to generate embedding", "Check Ollama"), nil, nil
        }

        // Prepare time filters (ensure timezone)
        var timeStart, timeEnd *string
        if input.TimeStart != "" {
            ts := ensureTimezone(input.TimeStart)
            timeStart = &ts
        }
        if input.TimeEnd != "" {
            ts := ensureTimezone(input.TimeEnd)
            timeEnd = &ts
        }

        // Context detection
        var contextPtr *string
        if input.Context != "" {
            contextPtr = &input.Context
        } else {
            contextPtr = DetectContext(cfg)
        }

        // Execute search
        episodes, err := deps.DB.QuerySearchEpisodes(ctx, input.Query, embedding, timeStart, timeEnd, contextPtr, limit)
        if err != nil {
            return ErrorResult("Search failed", "Database error"), nil, nil
        }

        // Update access tracking
        for _, ep := range episodes {
            _ = deps.DB.QueryUpdateEpisodeAccess(ctx, extractEpisodeID(ep.ID))
        }

        // Build response with truncated content
        results := make([]EpisodeResult, len(episodes))
        for i, ep := range episodes {
            results[i] = EpisodeResult{
                ID:        ep.ID,
                Content:   truncateContent(ep.Content, 500),
                Timestamp: ep.Timestamp.Format(time.RFC3339),
                Summary:   ep.Summary,
                Metadata:  ep.Metadata,
                Context:   ep.Context,
            }
        }

        response := EpisodeSearchResult{
            Episodes: results,
            Count:    len(results),
        }
        jsonBytes, _ := json.MarshalIndent(response, "", "  ")
        return TextResult(string(jsonBytes)), nil, nil
    }
}

// ensureTimezone appends Z if timestamp lacks timezone.
func ensureTimezone(ts string) string {
    if ts == "" {
        return ts
    }
    if !strings.HasSuffix(ts, "Z") && !strings.Contains(ts, "+") && !strings.HasSuffix(ts, "00:00") {
        return ts + "Z"
    }
    return ts
}
```

### Get Episode with Entities

```go
func NewGetEpisodeHandler(deps *Dependencies) mcp.ToolHandlerFor[GetEpisodeInput, any] {
    return func(ctx context.Context, req *mcp.CallToolRequest, input GetEpisodeInput) (
        *mcp.CallToolResult, any, error,
    ) {
        if strings.TrimSpace(input.ID) == "" {
            return ErrorResult("Episode ID cannot be empty", "Provide an episode ID"), nil, nil
        }

        // Extract bare ID
        id := extractEpisodeID(input.ID)

        // Get episode
        episode, err := deps.DB.QueryGetEpisode(ctx, id)
        if err != nil {
            return ErrorResult("Failed to retrieve episode", "Database error"), nil, nil
        }
        if episode == nil {
            return ErrorResult("Episode not found: "+id, "Use search_episodes to find valid IDs"), nil, nil
        }

        // Update access
        _ = deps.DB.QueryUpdateEpisodeAccess(ctx, id)

        // Get linked entities if requested
        var entities []models.Entity
        if input.IncludeEntities {
            entities, _ = deps.DB.QueryGetEpisodeEntities(ctx, id)
        }

        result := EpisodeResult{
            ID:             episode.ID,
            Content:        episode.Content,  // Full content for get
            Timestamp:      episode.Timestamp.Format(time.RFC3339),
            Summary:        episode.Summary,
            Metadata:       episode.Metadata,
            Context:        episode.Context,
            LinkedEntities: len(entities),
            Entities:       entities,
        }
        jsonBytes, _ := json.MarshalIndent(result, "", "  ")
        return TextResult(string(jsonBytes)), nil, nil
    }
}

// extractEpisodeID removes "episode:" prefix if present.
func extractEpisodeID(id string) string {
    return strings.TrimPrefix(id, "episode:")
}
```

### Delete Episode

```go
func (c *Client) QueryDeleteEpisode(ctx context.Context, episodeID string) (int, error) {
    // DELETE with RETURN BEFORE to count deletions
    results, err := surrealdb.Query[[]models.Episode](ctx, c.db, `
        DELETE type::record("episode", $id) RETURN BEFORE
    `, map[string]any{"id": episodeID})
    if err != nil {
        return 0, fmt.Errorf("delete episode: %w", err)
    }

    if results == nil || len(*results) == 0 {
        return 0, nil
    }
    return len((*results)[0].Result), nil
}
```

### Link Entity to Episode

```go
func (c *Client) QueryLinkEntityToEpisode(
    ctx context.Context,
    entityID string,
    episodeID string,
    position int,
    confidence float64,
) error {
    _, err := surrealdb.Query[any](ctx, c.db, `
        RELATE type::record("entity", $entity_id)->extracted_from->type::record("episode", $episode_id)
        SET position = $position, confidence = $confidence
    `, map[string]any{
        "entity_id":  entityID,
        "episode_id": episodeID,
        "position":   position,
        "confidence": confidence,
    })
    if err != nil {
        return fmt.Errorf("link entity to episode: %w", err)
    }
    return nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Separate BM25 + vector | `search::rrf()` | SurrealDB 2.0 | Unified hybrid search |
| User timestamps | Auto-generated | Phase 6 decision | Simpler API |
| Episode updates | Immutable episodes | Phase 6 recommendation | Temporal integrity |

**Deprecated/outdated:**
- User-provided timestamps: Removed per CONTEXT.md decision
- `DISTINCT`: Use `GROUP BY` in SurrealDB v3.0

## Open Questions

1. **Recency boosting implementation**
   - What we know: CONTEXT.md wants "boost recent episodes in ranking"
   - What's unclear: RRF doesn't directly support recency; need custom approach
   - Recommendation: Add `ORDER BY timestamp DESC` within RRF subqueries, or post-process with time-decay multiplier. Start simple (no explicit boosting), add if search quality suffers.

2. **Episode immutability**
   - What we know: CONTEXT.md asks about `updated_at` tracking
   - What's unclear: Should episodes be mutable?
   - Recommendation: Treat episodes as immutable (no update tool). Memories of experiences shouldn't change. If needed, delete and re-add.

3. **Cross-reference in get_entity**
   - What we know: CONTEXT.md mentions "flag to include related episodes when fetching entities"
   - What's unclear: Is this Phase 6 scope or Phase 7?
   - Recommendation: Out of scope for Phase 6 (tools are episode-focused). Document for future enhancement.

## Sources

### Primary (HIGH confidence)
- Python `memcp/servers/episode.py` - Reference implementation (verified)
- Python `memcp/db.py` lines 763-890 - Episode query patterns (verified)
- Go codebase `internal/db/schema.go` - Episode schema already defined
- Go codebase `internal/models/episode.go` - Episode struct exists

### Secondary (MEDIUM confidence)
- SurrealDB v3.0 documentation - Datetime handling, RRF syntax
- Phase 3 RESEARCH.md - Hybrid search patterns (same approach)

### Tertiary (LOW confidence)
- None - all patterns verified against existing code

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - No new dependencies, existing patterns
- Architecture: HIGH - Mirrors entity tools exactly
- Query patterns: HIGH - Python implementation + Go search tools verified
- Pitfalls: MEDIUM - Based on Python experience, Go-specific datetime handling

**Research date:** 2026-02-02
**Valid until:** 2026-03-02 (30 days - stable patterns)
