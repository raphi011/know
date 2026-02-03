// Package db provides SurrealDB query functions for Knowhow entity operations.
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

// optionalFloat returns models.None for nil pointers, otherwise returns the float value.
func optionalFloat(f *float64) any {
	if f == nil {
		return surrealmodels.None
	}
	return *f
}

// =============================================================================
// ENTITY QUERIES
// =============================================================================

// CreateEntity creates a new entity with a generated or specified ID.
// Returns the created entity.
func (c *Client) CreateEntity(ctx context.Context, input models.EntityInput) (*models.Entity, error) {
	// Generate ID from name if not provided
	id := slugify(input.Name)

	// Ensure labels is not nil
	labels := input.Labels
	if labels == nil {
		labels = []string{}
	}

	// Set defaults
	source := "manual"
	if input.Source != nil {
		source = *input.Source
	}
	confidence := 0.5
	if input.Confidence != nil {
		confidence = *input.Confidence
	}
	verified := false
	if input.Verified != nil {
		verified = *input.Verified
	}

	sql := `
		CREATE type::record("entity", $id) SET
			type = $type,
			name = $name,
			content = $content,
			summary = $summary,
			labels = $labels,
			verified = $verified,
			confidence = $confidence,
			source = $source,
			source_path = $source_path,
			metadata = $metadata,
			embedding = $embedding,
			access_count = 0
		RETURN AFTER
	`

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{
		"id":          id,
		"type":        input.Type,
		"name":        input.Name,
		"content":     optionalString(input.Content),
		"summary":     optionalString(input.Summary),
		"labels":      labels,
		"verified":    verified,
		"confidence":  confidence,
		"source":      source,
		"source_path": optionalString(input.SourcePath),
		"metadata":    input.Metadata,
		"embedding":   input.Embedding,
	})
	if err != nil {
		return nil, fmt.Errorf("create entity: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create entity: no result returned")
	}

	return &(*results)[0].Result[0], nil
}

// GetEntity retrieves an entity by ID.
// Returns nil if not found.
func (c *Client) GetEntity(ctx context.Context, id string) (*models.Entity, error) {
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

// GetEntityByName retrieves an entity by name (case-insensitive).
// Returns nil if not found.
func (c *Client) GetEntityByName(ctx context.Context, name string) (*models.Entity, error) {
	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, `
		SELECT * FROM entity WHERE string::lowercase(name) = string::lowercase($name) LIMIT 1
	`, map[string]any{"name": name})

	if err != nil {
		return nil, fmt.Errorf("get entity by name: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// UpdateEntity updates an entity with partial data.
// Only non-nil fields in the update are changed.
func (c *Client) UpdateEntity(ctx context.Context, id string, update models.EntityUpdate) (*models.Entity, error) {
	// Build dynamic SET clause
	setClauses := []string{}
	vars := map[string]any{"id": id}

	if update.Name != nil {
		setClauses = append(setClauses, "name = $name")
		vars["name"] = *update.Name
	}
	if update.Content != nil {
		setClauses = append(setClauses, "content = $content")
		vars["content"] = *update.Content
	}
	if update.Summary != nil {
		setClauses = append(setClauses, "summary = $summary")
		vars["summary"] = *update.Summary
	}
	if update.Labels != nil {
		setClauses = append(setClauses, "labels = $labels")
		vars["labels"] = update.Labels
	}
	if len(update.AddLabels) > 0 {
		setClauses = append(setClauses, "labels = array::union(labels, $add_labels)")
		vars["add_labels"] = update.AddLabels
	}
	if len(update.DelLabels) > 0 {
		setClauses = append(setClauses, "labels = array::difference(labels, $del_labels)")
		vars["del_labels"] = update.DelLabels
	}
	if update.Verified != nil {
		setClauses = append(setClauses, "verified = $verified")
		vars["verified"] = *update.Verified
	}
	if update.Confidence != nil {
		setClauses = append(setClauses, "confidence = $confidence")
		vars["confidence"] = *update.Confidence
	}
	if update.Metadata != nil {
		setClauses = append(setClauses, "metadata = $metadata")
		vars["metadata"] = update.Metadata
	}
	if update.Embedding != nil {
		setClauses = append(setClauses, "embedding = $embedding")
		vars["embedding"] = update.Embedding
	}

	// Always update accessed time
	setClauses = append(setClauses, "accessed = time::now()")

	if len(setClauses) == 1 { // Only accessed time update
		// No real updates, just touch the record
	}

	sql := fmt.Sprintf(`
		UPDATE type::record("entity", $id) SET %s RETURN AFTER
	`, strings.Join(setClauses, ", "))

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("update entity: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("entity not found")
	}

	return &(*results)[0].Result[0], nil
}

// DeleteEntity deletes an entity by ID.
// Cascade delete of chunks and relations is handled by SurrealDB events.
// Returns true if entity was deleted.
func (c *Client) DeleteEntity(ctx context.Context, id string) (bool, error) {
	sql := `DELETE type::record("entity", $id) RETURN BEFORE`

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, map[string]any{"id": id})
	if err != nil {
		return false, fmt.Errorf("delete entity: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return false, nil
	}
	return true, nil
}

// UpdateEntityAccess updates access tracking for an entity.
func (c *Client) UpdateEntityAccess(ctx context.Context, id string) error {
	_, err := surrealdb.Query[any](ctx, c.db, `
		UPDATE type::record("entity", $id) SET
			accessed = time::now(),
			access_count += 1
	`, map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("update entity access: %w", err)
	}
	return nil
}

// =============================================================================
// SEARCH QUERIES
// =============================================================================

// SearchOptions configures entity search behavior.
type SearchOptions struct {
	Query        string    // Search query text
	Embedding    []float32 // Query embedding for vector search
	Labels       []string  // Filter by labels (CONTAINSANY)
	Types        []string  // Filter by entity types
	VerifiedOnly bool      // Only return verified entities
	Limit        int       // Max results (default 10)
}

// HybridSearch performs RRF fusion of BM25 + vector search results.
// Returns entities ranked by combined relevance score.
func (c *Client) HybridSearch(ctx context.Context, opts SearchOptions) ([]models.Entity, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Build dynamic filter clauses
	filterClauses := []string{}
	vars := map[string]any{
		"q":     opts.Query,
		"emb":   opts.Embedding,
		"limit": limit,
	}

	if len(opts.Labels) > 0 {
		filterClauses = append(filterClauses, "labels CONTAINSANY $labels")
		vars["labels"] = opts.Labels
	}
	if len(opts.Types) > 0 {
		filterClauses = append(filterClauses, "type IN $types")
		vars["types"] = opts.Types
	}
	if opts.VerifiedOnly {
		filterClauses = append(filterClauses, "verified = true")
	}

	filterClause := ""
	if len(filterClauses) > 0 {
		filterClause = "AND " + strings.Join(filterClauses, " AND ")
	}

	// RRF fusion query - combines vector (2x limit for variety) with BM25
	sql := fmt.Sprintf(`
		SELECT * FROM search::rrf([
			(SELECT * FROM entity
			 WHERE embedding <|%d,60|> $emb %s),
			(SELECT * FROM entity
			 WHERE content @0@ $q OR name @1@ $q %s)
		], $limit, 60)
	`, limit*2, filterClause, filterClause)

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	if results != nil && len(*results) > 0 {
		return (*results)[0].Result, nil
	}
	return []models.Entity{}, nil
}

// SearchWithChunks performs hybrid search including chunk matches.
// Returns entities with their matching chunks for RAG context.
func (c *Client) SearchWithChunks(ctx context.Context, opts SearchOptions) ([]models.EntitySearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Build filter clause
	filterClauses := []string{}
	vars := map[string]any{
		"q":     opts.Query,
		"emb":   opts.Embedding,
		"limit": limit,
	}

	if len(opts.Labels) > 0 {
		filterClauses = append(filterClauses, "labels CONTAINSANY $labels")
		vars["labels"] = opts.Labels
	}
	if len(opts.Types) > 0 {
		filterClauses = append(filterClauses, "type IN $types")
		vars["types"] = opts.Types
	}
	if opts.VerifiedOnly {
		filterClauses = append(filterClauses, "verified = true")
	}

	filterClause := ""
	chunkFilterClause := ""
	if len(filterClauses) > 0 {
		filterClause = "AND " + strings.Join(filterClauses, " AND ")
		chunkFilterClause = "AND " + strings.Join(filterClauses, " AND ")
	}

	// Search entities and chunks, then aggregate by entity
	sql := fmt.Sprintf(`
		LET $entity_hits = (
			SELECT *, [] AS matched_chunks FROM search::rrf([
				(SELECT * FROM entity WHERE embedding <|%d,60|> $emb %s),
				(SELECT * FROM entity WHERE content @0@ $q OR name @1@ $q %s)
			], %d, 60)
		);

		LET $chunk_hits = (
			SELECT entity.* AS entity,
				   [{ content: content, heading_path: heading_path, position: position }] AS matched_chunks
			FROM chunk
			WHERE embedding <|%d,60|> $emb %s
		);

		-- Merge entity hits with chunk hits
		RETURN array::distinct(array::concat($entity_hits, $chunk_hits.map(|$c| {
			...$c.entity,
			matched_chunks: $c.matched_chunks
		}))).slice(0, $limit)
	`, limit*2, filterClause, filterClause, limit*2, limit*3, chunkFilterClause)

	results, err := surrealdb.Query[[]models.EntitySearchResult](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("search with chunks: %w", err)
	}

	// Result is in the last query result (RETURN statement)
	if results != nil && len(*results) > 0 {
		lastIdx := len(*results) - 1
		return (*results)[lastIdx].Result, nil
	}
	return []models.EntitySearchResult{}, nil
}

// =============================================================================
// CHUNK QUERIES
// =============================================================================

// CreateChunks creates multiple chunks for an entity.
func (c *Client) CreateChunks(ctx context.Context, entityID string, chunks []models.ChunkInput) error {
	if len(chunks) == 0 {
		return nil
	}

	for _, chunk := range chunks {
		sql := `
			CREATE chunk SET
				entity = type::record("entity", $entity_id),
				content = $content,
				position = $position,
				heading_path = $heading_path,
				labels = $labels,
				embedding = $embedding
		`
		_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
			"entity_id":    entityID,
			"content":      chunk.Content,
			"position":     chunk.Position,
			"heading_path": optionalString(chunk.HeadingPath),
			"labels":       chunk.Labels,
			"embedding":    chunk.Embedding,
		})
		if err != nil {
			return fmt.Errorf("create chunk %d: %w", chunk.Position, err)
		}
	}

	return nil
}

// DeleteChunks deletes all chunks for an entity.
func (c *Client) DeleteChunks(ctx context.Context, entityID string) error {
	_, err := surrealdb.Query[any](ctx, c.db, `
		DELETE chunk WHERE entity = type::record("entity", $entity_id)
	`, map[string]any{"entity_id": entityID})
	if err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}

// GetChunks retrieves all chunks for an entity, ordered by position.
func (c *Client) GetChunks(ctx context.Context, entityID string) ([]models.Chunk, error) {
	results, err := surrealdb.Query[[]models.Chunk](ctx, c.db, `
		SELECT * FROM chunk
		WHERE entity = type::record("entity", $entity_id)
		ORDER BY position ASC
	`, map[string]any{"entity_id": entityID})

	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []models.Chunk{}, nil
	}
	return (*results)[0].Result, nil
}

// =============================================================================
// RELATION QUERIES
// =============================================================================

// CreateRelation creates a relation between two entities.
// If a relation of the same type already exists, updates its strength.
func (c *Client) CreateRelation(ctx context.Context, input models.RelationInput) error {
	strength := 1.0
	if input.Strength != nil {
		strength = *input.Strength
	}
	source := "manual"
	if input.Source != nil {
		source = *input.Source
	}

	// Use UPSERT pattern based on unique_key
	sql := `
		LET $from_rec = type::record("entity", $from_id);
		LET $to_rec = type::record("entity", $to_id);
		LET $sorted = array::sort([<string>$from_rec, <string>$to_rec]);
		LET $unique = string::concat($sorted, $rel_type);
		LET $existing = (SELECT * FROM relates_to WHERE unique_key = $unique);
		IF array::len($existing) > 0 THEN
			UPDATE $existing[0].id SET strength = $strength, metadata = $metadata
		ELSE
			RELATE $from_rec->relates_to->$to_rec SET
				rel_type = $rel_type,
				strength = $strength,
				source = $source,
				metadata = $metadata
		END
	`

	_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
		"from_id":  input.FromID,
		"to_id":    input.ToID,
		"rel_type": input.RelType,
		"strength": strength,
		"source":   source,
		"metadata": input.Metadata,
	})
	if err != nil {
		return fmt.Errorf("create relation: %w", err)
	}
	return nil
}

// GetRelations retrieves all relations for an entity (both directions).
func (c *Client) GetRelations(ctx context.Context, entityID string) ([]models.Relation, error) {
	sql := `
		SELECT * FROM relates_to
		WHERE in = type::record("entity", $id) OR out = type::record("entity", $id)
	`
	results, err := surrealdb.Query[[]models.Relation](ctx, c.db, sql, map[string]any{"id": entityID})
	if err != nil {
		return nil, fmt.Errorf("get relations: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []models.Relation{}, nil
	}
	return (*results)[0].Result, nil
}

// DeleteRelation deletes a specific relation by from, to, and type.
func (c *Client) DeleteRelation(ctx context.Context, fromID, toID, relType string) error {
	sql := `
		DELETE relates_to WHERE
			(in = type::record("entity", $from_id) AND out = type::record("entity", $to_id) AND rel_type = $rel_type)
			OR
			(in = type::record("entity", $to_id) AND out = type::record("entity", $from_id) AND rel_type = $rel_type)
	`
	_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
		"from_id":  fromID,
		"to_id":    toID,
		"rel_type": relType,
	})
	if err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	return nil
}

// =============================================================================
// TEMPLATE QUERIES
// =============================================================================

// CreateTemplate creates a new template.
func (c *Client) CreateTemplate(ctx context.Context, input models.TemplateInput) (*models.Template, error) {
	id := slugify(input.Name)

	sql := `
		CREATE type::record("template", $id) SET
			name = $name,
			description = $description,
			content = $content
		RETURN AFTER
	`

	results, err := surrealdb.Query[[]models.Template](ctx, c.db, sql, map[string]any{
		"id":          id,
		"name":        input.Name,
		"description": optionalString(input.Description),
		"content":     input.Content,
	})
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create template: no result returned")
	}

	return &(*results)[0].Result[0], nil
}

// GetTemplate retrieves a template by name.
func (c *Client) GetTemplate(ctx context.Context, name string) (*models.Template, error) {
	results, err := surrealdb.Query[[]models.Template](ctx, c.db, `
		SELECT * FROM template WHERE name = $name LIMIT 1
	`, map[string]any{"name": name})

	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// ListTemplates returns all templates.
func (c *Client) ListTemplates(ctx context.Context) ([]models.Template, error) {
	results, err := surrealdb.Query[[]models.Template](ctx, c.db, `
		SELECT * FROM template ORDER BY name ASC
	`, nil)

	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []models.Template{}, nil
	}
	return (*results)[0].Result, nil
}

// DeleteTemplate deletes a template by name.
func (c *Client) DeleteTemplate(ctx context.Context, name string) (bool, error) {
	sql := `DELETE template WHERE name = $name RETURN BEFORE`

	results, err := surrealdb.Query[[]models.Template](ctx, c.db, sql, map[string]any{"name": name})
	if err != nil {
		return false, fmt.Errorf("delete template: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return false, nil
	}
	return true, nil
}

// =============================================================================
// TOKEN USAGE QUERIES
// =============================================================================

// RecordTokenUsage records LLM token usage.
func (c *Client) RecordTokenUsage(ctx context.Context, input models.TokenUsageInput) error {
	total := input.InputTokens + input.OutputTokens

	sql := `
		CREATE token_usage SET
			operation = $operation,
			model = $model,
			input_tokens = $input_tokens,
			output_tokens = $output_tokens,
			total_tokens = $total_tokens,
			cost_usd = $cost_usd,
			entity_id = $entity_id
	`

	_, err := surrealdb.Query[any](ctx, c.db, sql, map[string]any{
		"operation":     input.Operation,
		"model":         input.Model,
		"input_tokens":  input.InputTokens,
		"output_tokens": input.OutputTokens,
		"total_tokens":  total,
		"cost_usd":      optionalFloat(input.CostUSD),
		"entity_id":     optionalString(input.EntityID),
	})
	if err != nil {
		return fmt.Errorf("record token usage: %w", err)
	}
	return nil
}

// GetTokenUsageSummary returns aggregated token usage statistics.
func (c *Client) GetTokenUsageSummary(ctx context.Context, since string) (*models.TokenUsageSummary, error) {
	sql := `
		LET $usage = (SELECT * FROM token_usage WHERE created_at >= <datetime>$since);
		LET $total = math::sum($usage.total_tokens) ?? 0;
		LET $cost = math::sum($usage.cost_usd) ?? 0.0;
		LET $by_op = $usage.group(|$u| $u.operation).map(|$g| {
			key: $g[0].operation,
			value: math::sum($g.total_tokens)
		});
		LET $by_model = $usage.group(|$u| $u.model).map(|$g| {
			key: $g[0].model,
			value: math::sum($g.total_tokens)
		});
		RETURN {
			total_tokens: $total,
			total_cost_usd: $cost,
			by_operation: object::from_entries($by_op),
			by_model: object::from_entries($by_model)
		}
	`

	results, err := surrealdb.Query[models.TokenUsageSummary](ctx, c.db, sql, map[string]any{
		"since": since,
	})
	if err != nil {
		return nil, fmt.Errorf("get token usage summary: %w", err)
	}

	if results != nil && len(*results) > 0 {
		lastIdx := len(*results) - 1
		return &(*results)[lastIdx].Result, nil
	}
	return &models.TokenUsageSummary{}, nil
}

// =============================================================================
// UTILITY QUERIES
// =============================================================================

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

// ListLabels returns unique labels with entity counts.
func (c *Client) ListLabels(ctx context.Context) ([]LabelCount, error) {
	sql := `
		LET $all_labels = (SELECT labels FROM entity);
		LET $flattened = array::flatten($all_labels.labels);
		LET $unique = array::distinct($flattened);
		RETURN $unique.map(|$label| {
			label: $label,
			count: $flattened.filter(|$l| $l == $label).len()
		}).sort(|$a, $b| IF $a.count > $b.count THEN -1 ELSE IF $a.count < $b.count THEN 1 ELSE 0 END)
	`

	results, err := surrealdb.Query[[]LabelCount](ctx, c.db, sql, nil)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []LabelCount{}, nil
	}
	lastIdx := len(*results) - 1
	return (*results)[lastIdx].Result, nil
}

// ListTypes returns entity types with counts.
func (c *Client) ListTypes(ctx context.Context) ([]TypeCount, error) {
	sql := `
		SELECT type, count() AS count FROM entity GROUP BY type ORDER BY count DESC
	`

	results, err := surrealdb.Query[[]TypeCount](ctx, c.db, sql, nil)
	if err != nil {
		return nil, fmt.Errorf("list types: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []TypeCount{}, nil
	}
	return (*results)[0].Result, nil
}

// ListEntities returns entities with optional filtering.
func (c *Client) ListEntities(ctx context.Context, entityType string, labels []string, limit int) ([]models.Entity, error) {
	if limit <= 0 {
		limit = 50
	}

	filterClauses := []string{}
	vars := map[string]any{"limit": limit}

	if entityType != "" {
		filterClauses = append(filterClauses, "type = $type")
		vars["type"] = entityType
	}
	if len(labels) > 0 {
		filterClauses = append(filterClauses, "labels CONTAINSANY $labels")
		vars["labels"] = labels
	}

	whereClause := ""
	if len(filterClauses) > 0 {
		whereClause = "WHERE " + strings.Join(filterClauses, " AND ")
	}

	sql := fmt.Sprintf(`
		SELECT * FROM entity %s ORDER BY updated_at DESC LIMIT $limit
	`, whereClause)

	results, err := surrealdb.Query[[]models.Entity](ctx, c.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return []models.Entity{}, nil
	}
	return (*results)[0].Result, nil
}

// slugify converts a name to a URL-safe ID.
func slugify(name string) string {
	// Simple slugification: lowercase, replace spaces with hyphens
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// Remove non-alphanumeric except hyphens
	result := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
