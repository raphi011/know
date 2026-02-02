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
