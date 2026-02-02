package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
)

// RememberInput defines the input schema for the remember tool.
type RememberInput struct {
	Entities []EntityInput   `json:"entities,omitempty" jsonschema:"Entities to store"`
	Relations []RelationInput `json:"relations,omitempty" jsonschema:"Relations to create (Phase 04-02)"`
	Context   string          `json:"context,omitempty" jsonschema:"Project namespace (auto-detected if omitted)"`
}

// EntityInput defines an entity to be stored.
type EntityInput struct {
	Name       string   `json:"name" jsonschema:"required,Unique name within context"`
	Content    string   `json:"content" jsonschema:"required,Description or value"`
	Type       string   `json:"type,omitempty" jsonschema:"Entity type (concept, fact, preference)"`
	Labels     []string `json:"labels,omitempty" jsonschema:"Tags for categorization"`
	Confidence float64  `json:"confidence,omitempty" jsonschema:"Confidence score 0-1"`
	Source     string   `json:"source,omitempty" jsonschema:"Where this info came from"`
}

// RelationInput placeholder for Phase 04-02.
type RelationInput struct {
	From   string  `json:"from" jsonschema:"required,Source entity name"`
	To     string  `json:"to" jsonschema:"required,Target entity name"`
	Type   string  `json:"type" jsonschema:"required,Relation type"`
	Weight float64 `json:"weight,omitempty" jsonschema:"Relation strength 0-1"`
}

// EntityResult represents a stored entity in the response (without embedding).
type EntityResult struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Content    string   `json:"content"`
	Confidence float64  `json:"confidence,omitempty"`
	Source     *string  `json:"source,omitempty"`
	Context    *string  `json:"context,omitempty"`
	Action     string   `json:"action"` // "created" or "updated"
}

// RememberResult is the response from the remember tool.
type RememberResult struct {
	Entities []EntityResult `json:"entities"`
	Created  int            `json:"created"`
	Updated  int            `json:"updated"`
}

// slugify normalizes a name for use in entity IDs.
func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	// Remove non-alphanumeric except hyphens and colons (for context separator)
	reg := regexp.MustCompile(`[^a-z0-9\-:]`)
	s = reg.ReplaceAllString(s, "")
	return s
}

// generateEntityID creates a composite ID from context and name.
func generateEntityID(name, ctx string) string {
	slug := slugify(name)
	if ctx == "" {
		return slug
	}
	return ctx + ":" + slug
}

// NewRememberHandler creates the remember tool handler.
// Stores entities with auto-generated embeddings.
func NewRememberHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[RememberInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input RememberInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate: at least one entity required
		if len(input.Entities) == 0 {
			return ErrorResult("At least one entity is required", "Provide entities array with name and content"), nil, nil
		}

		// Validate each entity
		for i, e := range input.Entities {
			if e.Name == "" {
				return ErrorResult("Entity name is required", "All entities must have a name"), nil, nil
			}
			if e.Content == "" {
				return ErrorResult("Entity content is required", "Entity "+e.Name+" at index "+string(rune('0'+i))+" needs content"), nil, nil
			}
		}

		// Detect context: explicit > config > git origin > cwd
		var entityContext *string
		if input.Context != "" {
			entityContext = &input.Context
		} else {
			entityContext = DetectContext(cfg)
		}

		// Process each entity
		result := RememberResult{
			Entities: make([]EntityResult, 0, len(input.Entities)),
		}

		for _, e := range input.Entities {
			// Generate embedding
			embedding, err := deps.Embedder.Embed(ctx, e.Content)
			if err != nil {
				deps.Logger.Error("embedding failed", "name", e.Name, "error", err)
				return ErrorResult("Failed to generate embedding for "+e.Name, "Check Ollama connection"), nil, nil
			}

			// Generate composite ID
			ctxStr := ""
			if entityContext != nil {
				ctxStr = *entityContext
			}
			id := generateEntityID(e.Name, ctxStr)

			// Set defaults
			entityType := e.Type
			if entityType == "" {
				entityType = "concept"
			}
			confidence := e.Confidence
			if confidence <= 0 {
				confidence = 1.0
			}

			// Prepare optional source
			var source *string
			if e.Source != "" {
				source = &e.Source
			}

			// Upsert entity
			entity, wasCreated, err := deps.DB.QueryUpsertEntity(
				ctx,
				id,
				entityType,
				e.Labels,
				e.Content,
				embedding,
				confidence,
				source,
				entityContext,
			)
			if err != nil {
				deps.Logger.Error("upsert failed", "id", id, "error", err)
				return ErrorResult("Failed to store "+e.Name, "Database may be unavailable"), nil, nil
			}

			// Build result without embedding
			action := "updated"
			if wasCreated {
				action = "created"
				result.Created++
			} else {
				result.Updated++
			}

			result.Entities = append(result.Entities, EntityResult{
				ID:         entity.ID,
				Name:       e.Name,
				Type:       entity.Type,
				Labels:     entity.Labels,
				Content:    entity.Content,
				Confidence: entity.Confidence,
				Source:     entity.Source,
				Context:    entity.Context,
				Action:     action,
			})
		}

		// Format response as JSON
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("remember completed", "created", result.Created, "updated", result.Updated)
		return TextResult(string(jsonBytes)), nil, nil
	}
}
