// Package db provides SurrealDB query functions for entity operations.
package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// optionalString returns models.None for nil pointers, otherwise returns the string value.
// SurrealDB v3 strict mode requires NONE instead of NULL for option<string> fields.
func optionalString(s *string) any {
	if s == nil {
		return surrealmodels.None
	}
	return *s
}

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

	// SurrealDB v3: SPLIT on array field doesn't work with array::flatten in subquery
	// Instead, use array operations to flatten all labels, then count occurrences
	// LET $all_labels = collect all label arrays → flatten → group and count
	sql := fmt.Sprintf(`
		LET $all_labels = (SELECT labels FROM entity %s);
		LET $flattened = array::flatten($all_labels.labels);
		LET $unique = array::distinct($flattened);
		RETURN $unique.map(|$label| {
			label: $label,
			count: $flattened.filter(|$l| $l == $label).len()
		}).sort(|$a, $b| IF $a.count > $b.count THEN -1 ELSE IF $a.count < $b.count THEN 1 ELSE 0 END)
	`, contextClause)

	results, err := surrealdb.Query[[]LabelCount](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}

	// RETURN statement puts result in last query result
	if results == nil || len(*results) == 0 {
		return []LabelCount{}, nil
	}
	lastIdx := len(*results) - 1
	return (*results)[lastIdx].Result, nil
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
	// SurrealDB v3: SELECT on non-existent record returns empty array, not [{c:0}]
	existsSQL := `SELECT * FROM type::record("entity", $id)`
	existsResult, err := surrealdb.Query[[]models.Entity](ctx, c.db, existsSQL, map[string]any{"id": id})
	if err != nil {
		return nil, false, fmt.Errorf("check entity exists: %w", err)
	}

	exists := existsResult != nil && len(*existsResult) > 0 && len((*existsResult)[0].Result) > 0

	var sql string
	if exists {
		// SurrealDB v3: UPSERT SET doesn't read existing field values during SET clause
		// Use UPDATE for existing entities to properly merge labels
		sql = `
			UPDATE type::record("entity", $id) SET
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
	} else {
		// CREATE for new entities with initial values
		sql = `
			CREATE type::record("entity", $id) SET
				type = $type,
				labels = $labels,
				content = $content,
				embedding = $embedding,
				confidence = $confidence,
				source = $source,
				context = $context,
				accessed = time::now(),
				decay_weight = 1.0,
				importance = 1.0,
				access_count = 0
			RETURN AFTER
		`
	}

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
		"id":         id,
		"type":       entityType,
		"labels":     labels,
		"content":    content,
		"embedding":  embedding,
		"confidence": confidence,
		"source":     optionalString(source),
		"context":    optionalString(entityContext),
	})
	if err != nil {
		return nil, false, fmt.Errorf("upsert entity: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, false, fmt.Errorf("upsert entity: no result returned")
	}

	return &(*results)[0].Result[0], !exists, nil
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
	// Verify both entities exist first using array::len (v3 compatible)
	checkSQL := `
		LET $from_exists = array::len(SELECT * FROM type::record("entity", $from_id)) > 0;
		LET $to_exists = array::len(SELECT * FROM type::record("entity", $to_id)) > 0;
		RETURN { from_exists: $from_exists, to_exists: $to_exists };
	`
	checkResult, err := surrealdb.Query[map[string]any](ctx, c.db, checkSQL, map[string]any{
		"from_id": fromID,
		"to_id":   toID,
	})
	if err != nil {
		return fmt.Errorf("check entities: %w", err)
	}

	// Parse existence check result - RETURN puts result in last query result
	if checkResult != nil && len(*checkResult) > 0 {
		lastIdx := len(*checkResult) - 1
		result := (*checkResult)[lastIdx].Result
		fromExists, _ := result["from_exists"].(bool)
		toExists, _ := result["to_exists"].(bool)
		if !fromExists || !toExists {
			return fmt.Errorf("entity not found")
		}
	}

	// SurrealDB v3: RELATE doesn't upsert - unique_key constraint prevents duplicates
	// Use UPSERT pattern: compute unique_key and update if exists, else create
	// unique_key = array::sort([in, out]) + rel_type
	relateSQL := fmt.Sprintf(`
		LET $from_rec = type::record("entity", $from_id);
		LET $to_rec = type::record("entity", $to_id);
		LET $sorted = array::sort([<string>$from_rec, <string>$to_rec]);
		LET $unique = string::concat($sorted, $rel_type);
		LET $existing = (SELECT * FROM relates WHERE unique_key = $unique);
		IF array::len($existing) > 0 THEN
			UPDATE $existing[0].id SET weight = $weight
		ELSE
			RELATE $from_rec->relates->$to_rec SET rel_type = $rel_type, weight = $weight
		END
	`)
	_, err = surrealdb.Query[any](ctx, c.db, relateSQL, map[string]any{
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
//
// WARNING: Go SDK v1.2.0 cannot decode SurrealDB v3 graph traversal results
// due to CBOR range type incompatibility. The query syntax '->relates..{depth}->entity'
// returns range bounds that cause CBOR decode panics. This will be fixed in a future SDK release.
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
//
// WARNING: Go SDK v1.2.0 cannot decode SurrealDB v3 graph traversal results
// due to CBOR range type incompatibility. The query syntax '->relates..{depth}->entity'
// returns range bounds that cause CBOR decode panics. This will be fixed in a future SDK release.
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

	// SurrealDB v3: WHERE id IN array requires record IDs with type::record()
	// Build array of type::record() calls inline since parameters don't work in array construction
	recordRefs := make([]string, len(ids))
	for i, id := range ids {
		recordRefs[i] = fmt.Sprintf(`type::record("entity", "%s")`, id)
	}

	// Use inline array construction instead of parameterized array
	sql := fmt.Sprintf(`DELETE entity WHERE id IN [%s] RETURN BEFORE`, strings.Join(recordRefs, ", "))

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, nil)
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

	// SurrealDB v3: UPSERT RETURN AFTER returns record with id field as RecordID type
	// Since Episode.ID is string, explicitly cast to string in SELECT after UPSERT
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
			access_count = IF access_count THEN access_count ELSE 0 END;
		SELECT <string>id AS id, content, summary, embedding, metadata, timestamp,
			context, created, accessed, access_count
		FROM type::record("episode", $id)
	`

	results, err := surrealdb.Query[[]models.Episode](ctx, c.db, sql, map[string]any{
		"id":        episodeID,
		"content":   content,
		"embedding": embedding,
		"timestamp": timestamp,
		"summary":   optionalString(summary),
		"metadata":  metadata,
		"context":   optionalString(episodeContext),
	})
	if err != nil {
		return nil, fmt.Errorf("create episode: %w", err)
	}

	// SELECT is the second statement, so result is at index 1
	if results == nil || len(*results) < 2 || len((*results)[1].Result) == 0 {
		return nil, fmt.Errorf("create episode: no result returned")
	}

	return &(*results)[1].Result[0], nil
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
	// SurrealDB v3 RELATE requires direct record notation, not type::record()
	sql := fmt.Sprintf(`RELATE entity:%s->extracted_from->episode:%s SET position = $position, confidence = $confidence`, entityID, episodeID)

	_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
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
	// SurrealDB v3: Use graph traversal instead of subquery for better compatibility
	// Get entities by traversing backwards from episode via extracted_from relation
	sql := `
		SELECT * FROM type::record("episode", $episode_id)<-extracted_from<-entity
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

	// SurrealDB v3: UPSERT RETURN AFTER returns record with id field as RecordID type
	// Since Procedure.ID is string, explicitly cast to string in SELECT after UPSERT
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
			access_count = IF access_count THEN access_count ELSE 0 END;
		SELECT <string>id AS id, name, description, steps, embedding, labels,
			context, created, accessed, access_count
		FROM type::record("procedure", $id)
	`

	results, err := surrealdb.Query[[]models.Procedure](ctx, c.db, sql, map[string]any{
		"id":          procedureID,
		"name":        name,
		"description": description,
		"steps":       steps,
		"embedding":   embedding,
		"labels":      labels,
		"context":     optionalString(procedureContext),
	})
	if err != nil {
		return nil, fmt.Errorf("create procedure: %w", err)
	}

	// SELECT is the second statement, so result is at index 1
	if results == nil || len(*results) < 2 || len((*results)[1].Result) == 0 {
		return nil, fmt.Errorf("create procedure: no result returned")
	}

	return &(*results)[1].Result[0], nil
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
			math::max([decay_weight * %f, 0.1]) AS new_decay_weight,
			importance AS old_importance,
			math::max([importance * %f, 0.1]) AS new_importance
		FROM entity
		WHERE accessed < time::now() - duration::from_days($decay_days)
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
			decay_weight = math::max([decay_weight * %f, 0.1]),
			importance = math::max([importance * %f, 0.1])
		WHERE accessed < time::now() - duration::from_days($decay_days)
			AND decay_weight > 0.1
			%s
	`, decayFactor, decayFactor, contextClause)

	_, err = surrealdb.Query[any](ctx, c.db, updateSQL, vars)
	if err != nil {
		return nil, fmt.Errorf("apply decay: %w", err)
	}

	return entities, nil
}

// QueryFindSimilarPairs finds entity pairs with embedding similarity above threshold.
// Uses vector::similarity::cosine for comparison.
// Deduplicates pairs (returns A-B but not B-A).
// Returns at most `limit` pairs.
func (c *Client) QueryFindSimilarPairs(
	ctx context.Context,
	threshold float64,
	limit int,
	contextFilter *string,
	global bool,
) ([]models.SimilarPair, error) {
	// Build context filter
	vars := map[string]any{
		"threshold": threshold,
		"limit":     limit,
	}

	// Context filter for entity selection
	outerWhere := ""
	if !global && contextFilter != nil {
		outerWhere = "WHERE context = $context"
		vars["context"] = *contextFilter
	}

	// Find similar pairs using cosine similarity
	// v3.0 doesn't support cross-join in FROM clause, use LET + array operations
	// Sort pair IDs to deduplicate (id > $parent.id ensures A-B but not B-A)
	sql := fmt.Sprintf(`
		LET $all_entities = (SELECT id, content, embedding FROM entity %s);
		LET $pairs = array::flatten(
			$all_entities.map(|$e1| (
				$all_entities
					.filter(|$e2| $e2.id > $e1.id)
					.map(|$e2| {
						entity1_id: string::concat($e1.id),
						entity1_name: $e1.content,
						entity2_id: string::concat($e2.id),
						entity2_name: $e2.content,
						similarity: vector::similarity::cosine($e1.embedding, $e2.embedding)
					})
					.filter(|$p| $p.similarity >= $threshold)
			))
		);
		RETURN $pairs
			.sort(|$a, $b| IF $a.similarity > $b.similarity THEN -1 ELSE IF $a.similarity < $b.similarity THEN 1 ELSE 0 END)
			.slice(0, $limit);
	`, outerWhere)

	results, err := surrealdb.Query[[]models.SimilarPair](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("find similar pairs: %w", err)
	}

	if results == nil {
		return []models.SimilarPair{}, nil
	}

	// Since we use RETURN, the last result contains the pairs
	if len(*results) == 0 {
		return []models.SimilarPair{}, nil
	}

	// Get the last result (the RETURN statement)
	lastIdx := len(*results) - 1
	return (*results)[lastIdx].Result, nil
}
