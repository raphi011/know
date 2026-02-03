// Package db provides SurrealDB query functions for entity operations.
package db

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// LabelCount represents a label with its entity count.
type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// TypeCount represents an entity type with its count.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// QueryHybridSearch performs RRF fusion of BM25 + vector search results.
// Returns entities ranked by combined relevance score.
func (c *Client) QueryHybridSearch(
	ctx context.Context,
	query string,
	embedding []float32,
	labels []string,
	limit int,
	contextFilter *string,
) ([]models.Entity, error) {
	// Build dynamic filter clauses
	labelClause := ""
	if len(labels) > 0 {
		labelClause = "AND labels CONTAINSANY $labels"
	}
	contextClause := ""
	if contextFilter != nil {
		contextClause = "AND context = $context"
	}

	// RRF fusion query - combines vector (2x limit for variety) with BM25
	// Vector: HNSW with ef=40 for better recall
	// BM25: full-text search analyzer 0
	// RRF k=60 (standard constant for rank fusion)
	sql := fmt.Sprintf(`
		SELECT * FROM search::rrf([
			(SELECT id, type, labels, content, confidence, source, decay_weight,
					context, importance, accessed, access_count
			 FROM entity
			 WHERE embedding <|%d,40|> $emb %s %s),
			(SELECT id, type, labels, content, confidence, source, decay_weight,
					context, importance, accessed, access_count
			 FROM entity
			 WHERE content @0@ $q %s %s)
		], $limit, 60)
	`, limit*2, labelClause, contextClause, labelClause, contextClause)

	vars := map[string]any{
		"q":      query,
		"emb":    embedding,
		"labels": labels,
		"limit":  limit,
	}
	if contextFilter != nil {
		vars["context"] = *contextFilter
	}

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	// Extract from query result wrapper
	if results != nil && len(*results) > 0 {
		return (*results)[0].Result, nil
	}
	return []models.Entity{}, nil
}

// QueryGetEntity retrieves an entity by ID.
// Returns nil if not found.
func (c *Client) QueryGetEntity(ctx context.Context, id string) (*models.Entity, error) {
	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, `
		SELECT * FROM type::record("entity", $id)
	`, map[string]any{"id": id})

	if err != nil {
		return nil, fmt.Errorf("get entity: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// QueryUpdateAccess updates access tracking for an entity.
// Resets decay_weight to 1.0 to mark as recently accessed.
func (c *Client) QueryUpdateAccess(ctx context.Context, id string) error {
	_, err := surrealdb.Query[any](ctx, c.db, `
		UPDATE type::record("entity", $id) SET
			accessed = time::now(),
			access_count += 1,
			decay_weight = 1.0
	`, map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("update access: %w", err)
	}
	return nil
}

// QueryListLabels returns unique labels with entity counts.
// If contextFilter is non-nil, only counts entities in that context.
func (c *Client) QueryListLabels(ctx context.Context, contextFilter *string) ([]LabelCount, error) {
	// Build context filter
	contextClause := ""
	vars := map[string]any{}
	if contextFilter != nil {
		contextClause = "WHERE context = $context"
		vars["context"] = *contextFilter
	}

	// Get all labels from entities, then group
	// Use array::flatten to combine all label arrays, then group by value
	sql := fmt.Sprintf(`
		SELECT label, count() AS count FROM (
			SELECT array::flatten(labels) AS label FROM entity %s
		) SPLIT label GROUP BY label ORDER BY count DESC
	`, contextClause)

	results, err := surrealdb.Query[[]LabelCount](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []LabelCount{}, nil
	}
	return (*results)[0].Result, nil
}

// QueryListTypes returns entity types with counts.
// If contextFilter is non-nil, only counts entities in that context.
func (c *Client) QueryListTypes(ctx context.Context, contextFilter *string) ([]TypeCount, error) {
	// Build context filter
	contextClause := ""
	vars := map[string]any{}
	if contextFilter != nil {
		contextClause = "WHERE context = $context"
		vars["context"] = *contextFilter
	}

	sql := fmt.Sprintf(`
		SELECT type, count() AS count FROM entity %s GROUP BY type ORDER BY count DESC
	`, contextClause)

	results, err := surrealdb.Query[[]TypeCount](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list types: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []TypeCount{}, nil
	}
	return (*results)[0].Result, nil
}

// QueryUpsertEntity creates or updates an entity by ID.
// Uses SurrealDB UPSERT with array::union for additive label merge.
// Returns (entity, wasCreated, error) where wasCreated indicates if entity was new.
func (c *Client) QueryUpsertEntity(
	ctx context.Context,
	id string,
	entityType string,
	labels []string,
	content string,
	embedding []float32,
	confidence float64,
	source *string,
	entityContext *string,
) (*models.Entity, bool, error) {
	// Ensure labels is not nil
	if labels == nil {
		labels = []string{}
	}

	// Check if entity exists to determine action
	existsSQL := `SELECT count() AS c FROM type::record("entity", $id)`
	existsResult, err := surrealdb.Query[[]struct{ C int }](ctx, c.db, existsSQL, map[string]any{"id": id})
	if err != nil {
		return nil, false, fmt.Errorf("check entity exists: %w", err)
	}

	wasCreated := true
	if existsResult != nil && len(*existsResult) > 0 && len((*existsResult)[0].Result) > 0 {
		wasCreated = (*existsResult)[0].Result[0].C == 0
	}

	// UPSERT with additive label merge
	// Use array::union to merge existing labels with new ones
	// Set importance/access_count only on create, preserve on update
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
			decay_weight = 1.0,
			importance = IF importance THEN importance ELSE 1.0 END,
			access_count = IF access_count THEN access_count ELSE 0 END
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
		"context":    entityContext,
	})
	if err != nil {
		return nil, false, fmt.Errorf("upsert entity: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, false, fmt.Errorf("upsert entity: no result returned")
	}

	return &(*results)[0].Result[0], wasCreated, nil
}

// QueryCreateRelation creates or updates a relation between two entities.
// Uses RELATE which respects the unique_key index for deduplication.
// Returns error if source or target entity doesn't exist.
func (c *Client) QueryCreateRelation(
	ctx context.Context,
	fromID string,
	relType string,
	toID string,
	weight float64,
) error {
	// Verify both entities exist, then create relation
	// SurrealDB RELATE upserts based on unique_key index
	sql := `
		LET $from_exists = (SELECT count() AS c FROM type::record("entity", $from_id)).c > 0;
		LET $to_exists = (SELECT count() AS c FROM type::record("entity", $to_id)).c > 0;

		IF !$from_exists OR !$to_exists {
			THROW "Entity not found"
		};

		RELATE type::record("entity", $from_id)->relates->type::record("entity", $to_id) SET
			rel_type = $rel_type,
			weight = $weight;
	`

	_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
		"from_id":  fromID,
		"to_id":    toID,
		"rel_type": relType,
		"weight":   weight,
	})
	if err != nil {
		return fmt.Errorf("create relation: %w", err)
	}
	return nil
}

// TraverseResult contains an entity with its connected neighbors.
type TraverseResult struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Labels     []string        `json:"labels"`
	Content    string          `json:"content"`
	Confidence float64         `json:"confidence"`
	Context    string          `json:"context"`
	Connected  []models.Entity `json:"connected"`
}

// QueryTraverse performs bidirectional graph traversal from a starting entity.
// Returns the starting entity with connected neighbors at each depth level.
// If relationTypes is provided, only traverses via those relation types.
func (c *Client) QueryTraverse(
	ctx context.Context,
	startID string,
	depth int,
	relationTypes []string,
) ([]TraverseResult, error) {
	var sql string
	vars := map[string]any{"id": startID}

	if len(relationTypes) > 0 {
		// Filter by rel_type field within the relates table
		sql = fmt.Sprintf(`
			SELECT *, ->(SELECT * FROM relates WHERE rel_type IN $types)..%d->entity AS connected
			FROM type::record("entity", $id)
		`, depth)
		vars["types"] = relationTypes
	} else {
		// Traverse all relation types
		sql = fmt.Sprintf(`
			SELECT *, ->relates..%d->entity AS connected
			FROM type::record("entity", $id)
		`, depth)
	}

	results, err := surrealdb.Query[[]TraverseResult](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("traverse: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []TraverseResult{}, nil
	}
	return (*results)[0].Result, nil
}

// QueryFindPath finds the shortest path between two entities via the relates table.
// Returns path as slice of entities (intermediate nodes) or nil if no path found.
// MaxDepth limits path length (1-20).
func (c *Client) QueryFindPath(
	ctx context.Context,
	fromID string,
	toID string,
	maxDepth int,
) ([]models.Entity, error) {
	// SurrealDB path traversal: from->relates..{depth}->entity WHERE id = to
	// Depth must be literal (cannot parameterize), so use fmt.Sprintf
	sql := fmt.Sprintf(`
		SELECT * FROM type::record("entity", $from)->relates..%d->entity
		WHERE id = type::record("entity", $to) LIMIT 1
	`, maxDepth)

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
		"from": fromID,
		"to":   toID,
	})
	if err != nil {
		return nil, fmt.Errorf("find_path: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// QueryDeleteEntity deletes entities by ID.
// TYPE RELATION in schema auto-cascades relation deletion.
// Returns count of deleted entities (0 if none found - idempotent).
func (c *Client) QueryDeleteEntity(ctx context.Context, ids ...string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	// Build record IDs for batch delete
	// Delete with RETURN BEFORE to count actual deletions
	sql := `DELETE entity WHERE id IN $ids RETURN BEFORE`

	// Convert string IDs to record format
	recordIDs := make([]string, len(ids))
	for i, id := range ids {
		recordIDs[i] = "entity:" + id
	}

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
		"ids": recordIDs,
	})
	if err != nil {
		return 0, fmt.Errorf("delete entity: %w", err)
	}

	// Count deleted (RETURN BEFORE returns deleted records)
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// QueryCreateEpisode creates or updates an episode by ID.
// Uses SurrealDB UPSERT to handle insert vs update semantics.
// Returns the created/updated episode.
func (c *Client) QueryCreateEpisode(
	ctx context.Context,
	episodeID string,
	content string,
	embedding []float32,
	timestamp string,
	summary *string,
	metadata map[string]any,
	episodeContext *string,
) (*models.Episode, error) {
	// Ensure metadata is not nil
	if metadata == nil {
		metadata = map[string]any{}
	}

	// UPSERT with conditional created field (only set on insert)
	sql := `
		UPSERT type::record("episode", $id) SET
			content = $content,
			embedding = $embedding,
			timestamp = type::datetime($timestamp),
			summary = $summary,
			metadata = $metadata,
			context = $context,
			accessed = time::now(),
			created = IF created THEN created ELSE time::now() END,
			access_count = IF access_count THEN access_count ELSE 0 END
		RETURN AFTER
	`

	results, err := surrealdb.Query[[]models.Episode](ctx, c.db, sql, map[string]any{
		"id":        episodeID,
		"content":   content,
		"embedding": embedding,
		"timestamp": timestamp,
		"summary":   summary,
		"metadata":  metadata,
		"context":   episodeContext,
	})
	if err != nil {
		return nil, fmt.Errorf("create episode: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create episode: no result returned")
	}

	return &(*results)[0].Result[0], nil
}

// QueryGetEpisode retrieves an episode by ID.
// Returns nil if not found.
func (c *Client) QueryGetEpisode(ctx context.Context, id string) (*models.Episode, error) {
	results, err := surrealdb.Query[[]models.Episode](ctx, c.db, `
		SELECT * FROM type::record("episode", $id)
	`, map[string]any{"id": id})

	if err != nil {
		return nil, fmt.Errorf("get episode: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// QueryUpdateEpisodeAccess updates access tracking for an episode.
func (c *Client) QueryUpdateEpisodeAccess(ctx context.Context, id string) error {
	_, err := surrealdb.Query[any](ctx, c.db, `
		UPDATE type::record("episode", $id) SET
			accessed = time::now(),
			access_count += 1
	`, map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("update episode access: %w", err)
	}
	return nil
}

// QueryDeleteEpisode deletes an episode by ID.
// Returns count of deleted (0 if not found - idempotent).
func (c *Client) QueryDeleteEpisode(ctx context.Context, id string) (int, error) {
	sql := `DELETE type::record("episode", $id) RETURN BEFORE`

	results, err := surrealdb.Query[[]models.Episode](ctx, c.db, sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return 0, fmt.Errorf("delete episode: %w", err)
	}

	// Count deleted (RETURN BEFORE returns deleted records)
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// QueryLinkEntityToEpisode creates an extracted_from relation from entity to episode.
// Used to track which entities were mentioned/extracted from an episode.
func (c *Client) QueryLinkEntityToEpisode(
	ctx context.Context,
	entityID string,
	episodeID string,
	position int,
	confidence float64,
) error {
	sql := `
		RELATE type::record("entity", $entity_id)->extracted_from->type::record("episode", $episode_id) SET
			position = $position,
			confidence = $confidence
	`

	_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
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

// QueryGetLinkedEntities retrieves entities linked to an episode via extracted_from.
func (c *Client) QueryGetLinkedEntities(ctx context.Context, episodeID string) ([]models.Entity, error) {
	sql := `
		SELECT * FROM entity WHERE id IN (
			SELECT in FROM extracted_from WHERE out = type::record("episode", $episode_id)
		)
	`

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
		"episode_id": episodeID,
	})
	if err != nil {
		return nil, fmt.Errorf("get linked entities: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []models.Entity{}, nil
	}
	return (*results)[0].Result, nil
}

// QuerySearchEpisodes performs hybrid BM25+vector search on episodes.
// Supports optional time range filtering and context filter.
// Returns episodes ranked by RRF fusion with recency consideration.
func (c *Client) QuerySearchEpisodes(
	ctx context.Context,
	query string,
	embedding []float32,
	timeStart *string,
	timeEnd *string,
	contextFilter *string,
	limit int,
) ([]models.Episode, error) {
	// Build dynamic filter clauses
	filterClause := ""
	if timeStart != nil {
		filterClause += " AND timestamp >= <datetime>$time_start"
	}
	if timeEnd != nil {
		filterClause += " AND timestamp <= <datetime>$time_end"
	}
	if contextFilter != nil {
		filterClause += " AND context = $context"
	}

	// RRF fusion query - combines vector (2x limit for variety) with BM25
	// Vector: HNSW with ef=40 for better recall
	// BM25: full-text search analyzer 0
	// RRF k=60 (standard constant for rank fusion)
	// ORDER BY timestamp DESC within subqueries for recency consideration
	sql := fmt.Sprintf(`
		SELECT * FROM search::rrf([
			(SELECT id, content, summary, timestamp, metadata, context
			 FROM episode
			 WHERE embedding <|%d,40|> $emb %s
			 ORDER BY timestamp DESC),
			(SELECT id, content, summary, timestamp, metadata, context
			 FROM episode
			 WHERE content @0@ $q %s
			 ORDER BY timestamp DESC)
		], $limit, 60)
	`, limit*2, filterClause, filterClause)

	vars := map[string]any{
		"q":     query,
		"emb":   embedding,
		"limit": limit,
	}
	if timeStart != nil {
		vars["time_start"] = *timeStart
	}
	if timeEnd != nil {
		vars["time_end"] = *timeEnd
	}
	if contextFilter != nil {
		vars["context"] = *contextFilter
	}

	results, err := surrealdb.Query[[]models.Episode](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}

	// Extract from query result wrapper
	if results != nil && len(*results) > 0 {
		return (*results)[0].Result, nil
	}
	return []models.Episode{}, nil
}

// QueryCreateProcedure creates or updates a procedure by ID.
// Uses SurrealDB UPSERT to handle insert vs update semantics.
// Returns the created/updated procedure.
func (c *Client) QueryCreateProcedure(
	ctx context.Context,
	procedureID string,
	name string,
	description string,
	steps []models.ProcedureStep,
	embedding []float32,
	labels []string,
	procedureContext *string,
) (*models.Procedure, error) {
	// Ensure labels and steps are not nil
	if labels == nil {
		labels = []string{}
	}
	if steps == nil {
		steps = []models.ProcedureStep{}
	}

	// UPSERT with conditional created field (only set on insert)
	sql := `
		UPSERT type::record("procedure", $id) SET
			name = $name,
			description = $description,
			steps = $steps,
			embedding = $embedding,
			labels = $labels,
			context = $context,
			accessed = time::now(),
			created = IF created THEN created ELSE time::now() END,
			access_count = IF access_count THEN access_count ELSE 0 END
		RETURN AFTER
	`

	results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, map[string]any{
		"id":          procedureID,
		"name":        name,
		"description": description,
		"steps":       steps,
		"embedding":   embedding,
		"labels":      labels,
		"context":     procedureContext,
	})
	if err != nil {
		return nil, fmt.Errorf("create procedure: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create procedure: no result returned")
	}

	return &(*results)[0].Result[0], nil
}

// QueryGetProcedure retrieves a procedure by ID.
// Returns nil if not found.
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

// QueryUpdateProcedureAccess updates access tracking for a procedure.
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

// QueryDeleteProcedure deletes a procedure by ID.
// Returns count of deleted (0 if not found - idempotent).
func (c *Client) QueryDeleteProcedure(ctx context.Context, id string) (int, error) {
	sql := `DELETE type::record("procedure", $id) RETURN BEFORE`

	results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return 0, fmt.Errorf("delete procedure: %w", err)
	}

	// Count deleted (RETURN BEFORE returns deleted records)
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// QuerySearchProcedures performs hybrid BM25+vector search on procedures.
// Searches name and description fields. Supports label and context filtering.
// Returns procedures ranked by RRF fusion.
func (c *Client) QuerySearchProcedures(
	ctx context.Context,
	query string,
	embedding []float32,
	labels []string,
	contextFilter *string,
	limit int,
) ([]models.Procedure, error) {
	// Build dynamic filter clauses
	labelClause := ""
	if len(labels) > 0 {
		labelClause = "AND labels CONTAINSANY $labels"
	}
	contextClause := ""
	if contextFilter != nil {
		contextClause = "AND context = $context"
	}

	// RRF fusion query - combines vector (2x limit for variety) with BM25
	// Vector: HNSW with ef=40 for better recall
	// BM25: full-text search on name (analyzer 0) and description (analyzer 1)
	// RRF k=60 (standard constant for rank fusion)
	sql := fmt.Sprintf(`
		SELECT * FROM search::rrf([
			(SELECT id, name, description, steps, labels, context, accessed, access_count
			 FROM procedure
			 WHERE embedding <|%d,40|> $emb %s %s),
			(SELECT id, name, description, steps, labels, context, accessed, access_count
			 FROM procedure
			 WHERE name @0@ $q OR description @1@ $q %s %s)
		], $limit, 60)
	`, limit*2, labelClause, contextClause, labelClause, contextClause)

	vars := map[string]any{
		"q":      query,
		"emb":    embedding,
		"labels": labels,
		"limit":  limit,
	}
	if contextFilter != nil {
		vars["context"] = *contextFilter
	}

	results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("search procedures: %w", err)
	}

	// Extract from query result wrapper
	if results != nil && len(*results) > 0 {
		return (*results)[0].Result, nil
	}
	return []models.Procedure{}, nil
}

// QueryListProcedures returns all procedures with optional context filtering.
// Returns procedures ordered by last access time (most recent first).
func (c *Client) QueryListProcedures(
	ctx context.Context,
	contextFilter *string,
	limit int,
) ([]models.Procedure, error) {
	// Build context filter
	contextClause := ""
	vars := map[string]any{"limit": limit}
	if contextFilter != nil {
		contextClause = "WHERE context = $context"
		vars["context"] = *contextFilter
	}

	sql := fmt.Sprintf(`
		SELECT id, name, description, steps, labels, context, accessed, access_count
		FROM procedure %s
		ORDER BY accessed DESC
		LIMIT $limit
	`, contextClause)

	results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list procedures: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []models.Procedure{}, nil
	}
	return (*results)[0].Result, nil
}

// QueryApplyDecay reduces decay_weight and importance for entities not accessed
// within the specified number of days.
// Floor: decay_weight > 0.1 prevents complete decay.
// Returns entities affected with before/after values.
// Uses two-step approach: SELECT to capture old values, then UPDATE.
func (c *Client) QueryApplyDecay(
	ctx context.Context,
	decayDays int,
	contextFilter *string,
	global bool,
	dryRun bool,
) ([]models.DecayedEntity, error) {
	// Build context filter
	contextClause := ""
	vars := map[string]any{"decay_days": decayDays}

	if !global && contextFilter != nil {
		contextClause = "AND context = $context"
		vars["context"] = *contextFilter
	}

	// Decay factor: multiply by 0.9 (10% reduction each run)
	// Floor at 0.1 to prevent complete decay
	decayFactor := 0.9

	// Step 1: SELECT entities that would be affected (with computed new values)
	// This works for both dry_run (preview only) and apply (capture before updating)
	selectSQL := fmt.Sprintf(`
		SELECT
			id,
			content AS name,
			decay_weight AS old_decay_weight,
			math::max(decay_weight * %f, 0.1) AS new_decay_weight,
			importance AS old_importance,
			math::max(importance * %f, 0.1) AS new_importance
		FROM entity
		WHERE accessed < time::now() - duration::from::days($decay_days)
			AND decay_weight > 0.1
			%s
	`, decayFactor, decayFactor, contextClause)

	results, err := surrealdb.Query[[]models.DecayedEntity](ctx, c.db, selectSQL, vars)
	if err != nil {
		return nil, fmt.Errorf("decay select: %w", err)
	}

	var entities []models.DecayedEntity
	if results != nil && len(*results) > 0 {
		entities = (*results)[0].Result
	}

	// If dry_run, return preview without applying
	if dryRun {
		return entities, nil
	}

	// If no entities to update, return empty
	if len(entities) == 0 {
		return []models.DecayedEntity{}, nil
	}

	// Step 2: Apply UPDATE to affected entities
	updateSQL := fmt.Sprintf(`
		UPDATE entity SET
			decay_weight = math::max(decay_weight * %f, 0.1),
			importance = math::max(importance * %f, 0.1)
		WHERE accessed < time::now() - duration::from::days($decay_days)
			AND decay_weight > 0.1
			%s
	`, decayFactor, decayFactor, contextClause)

	_, err = surrealdb.Query[any](ctx, c.db, updateSQL, vars)
	if err != nil {
		return nil, fmt.Errorf("apply decay: %w", err)
	}

	return entities, nil
}
