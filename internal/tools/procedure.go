package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

// ProcedureStepInput represents a step in the input.
type ProcedureStepInput struct {
	Content  string `json:"content" jsonschema:"required,Step instruction/description"`
	Optional bool   `json:"optional,omitempty" jsonschema:"Whether step can be skipped"`
}

// CreateProcedureInput defines the input schema for the create_procedure tool.
type CreateProcedureInput struct {
	Name        string               `json:"name" jsonschema:"required,Unique procedure name"`
	Description string               `json:"description" jsonschema:"required,What this procedure accomplishes"`
	Steps       []ProcedureStepInput `json:"steps" jsonschema:"required,Ordered list of steps (minimum 1)"`
	Labels      []string             `json:"labels,omitempty" jsonschema:"Tags for categorization"`
	Context     string               `json:"context,omitempty" jsonschema:"Project namespace (auto-detected if omitted)"`
}

// GetProcedureInput defines the input schema for the get_procedure tool.
type GetProcedureInput struct {
	ID string `json:"id" jsonschema:"required,Procedure ID (with or without 'procedure:' prefix)"`
}

// DeleteProcedureInput defines the input schema for the delete_procedure tool.
type DeleteProcedureInput struct {
	ID string `json:"id" jsonschema:"required,Procedure ID to delete"`
}

// CreateProcedureResult is the response from create_procedure.
type CreateProcedureResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	StepCount   int      `json:"step_count"`
	Labels      []string `json:"labels,omitempty"`
	Context     *string  `json:"context,omitempty"`
	Action      string   `json:"action"` // "created" or "updated"
}

// GetProcedureResult is the response from get_procedure.
type GetProcedureResult struct {
	Procedure *models.Procedure `json:"procedure"`
}

// DeleteProcedureResult is the response from delete_procedure.
type DeleteProcedureResult struct {
	Deleted int    `json:"deleted"`
	Message string `json:"message"`
}

// extractProcedureID removes "procedure:" prefix if present.
func extractProcedureID(id string) string {
	return strings.TrimPrefix(id, "procedure:")
}

// generateProcedureID creates a composite ID from context and name.
func generateProcedureID(name, ctx string) string {
	slug := slugify(name)
	if ctx == "" {
		return slug
	}
	return ctx + ":" + slug
}

// NewCreateProcedureHandler creates the create_procedure tool handler.
// Stores procedural memories with auto-generated embedding.
func NewCreateProcedureHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[CreateProcedureInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CreateProcedureInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate required fields
		if input.Name == "" {
			return ErrorResult("Name cannot be empty", "Provide a procedure name"), nil, nil
		}
		if input.Description == "" {
			return ErrorResult("Description cannot be empty", "Describe what this procedure accomplishes"), nil, nil
		}
		if len(input.Steps) == 0 {
			return ErrorResult("Steps cannot be empty", "Provide at least one step"), nil, nil
		}

		// Validate each step has content
		for i, step := range input.Steps {
			if step.Content == "" {
				return ErrorResult(
					fmt.Sprintf("Step %d content cannot be empty", i+1),
					"Each step must have content",
				), nil, nil
			}
		}

		// Detect context: explicit > config
		var procedureContext *string
		if input.Context != "" {
			procedureContext = &input.Context
		} else {
			procedureContext = DetectContext(cfg)
		}

		// Get context string for ID generation
		ctxStr := ""
		if procedureContext != nil {
			ctxStr = *procedureContext
		}

		// Generate ID using slugified name + context prefix
		procedureID := generateProcedureID(input.Name, ctxStr)

		// Convert input steps to model steps with order index
		modelSteps := make([]models.ProcedureStep, len(input.Steps))
		stepContents := make([]string, len(input.Steps))
		for i, step := range input.Steps {
			modelSteps[i] = models.ProcedureStep{
				Order:    i + 1,
				Content:  step.Content,
				Optional: step.Optional,
			}
			stepContents[i] = step.Content
		}

		// Generate embedding from combined text: name + description + steps
		embeddingText := input.Name + " " + input.Description + " " + strings.Join(stepContents, " ")
		embedding, err := deps.Embedder.Embed(ctx, embeddingText)
		if err != nil {
			deps.Logger.Error("embedding failed", "name", input.Name, "error", err)
			return ErrorResult("Failed to generate embedding", "Check Ollama connection"), nil, nil
		}

		// Check if procedure exists (to determine action)
		existing, err := deps.DB.QueryGetProcedure(ctx, procedureID)
		if err != nil {
			deps.Logger.Error("check procedure exists failed", "id", procedureID, "error", err)
			return ErrorResult("Failed to check existing procedure", "Database may be unavailable"), nil, nil
		}
		action := "created"
		if existing != nil {
			action = "updated"
		}

		// Create/update procedure
		procedure, err := deps.DB.QueryCreateProcedure(
			ctx,
			procedureID,
			input.Name,
			input.Description,
			modelSteps,
			embedding,
			input.Labels,
			procedureContext,
		)
		if err != nil {
			deps.Logger.Error("create procedure failed", "id", procedureID, "error", err)
			return ErrorResult("Failed to create procedure", "Database may be unavailable"), nil, nil
		}

		// Build result
		result := CreateProcedureResult{
			ID:          procedure.ID,
			Name:        procedure.Name,
			Description: procedure.Description,
			StepCount:   len(procedure.Steps),
			Labels:      procedure.Labels,
			Context:     procedureContext,
			Action:      action,
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("create_procedure completed", "id", procedureID, "steps", len(modelSteps), "action", action)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// NewGetProcedureHandler creates the get_procedure tool handler.
// Retrieves a procedure by ID with full steps.
func NewGetProcedureHandler(deps *Dependencies) mcp.ToolHandlerFor[GetProcedureInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetProcedureInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate ID
		if input.ID == "" {
			return ErrorResult("ID cannot be empty", "Provide a procedure ID"), nil, nil
		}

		// Extract bare ID
		id := extractProcedureID(input.ID)

		// Query procedure
		procedure, err := deps.DB.QueryGetProcedure(ctx, id)
		if err != nil {
			deps.Logger.Error("get_procedure failed", "id", id, "error", err)
			return ErrorResult("Failed to retrieve procedure", "Database may be unavailable"), nil, nil
		}

		// Handle not found
		if procedure == nil {
			return ErrorResult(
				fmt.Sprintf("Procedure not found: %s", id),
				"Use create_procedure to create procedures",
			), nil, nil
		}

		// Update access tracking
		if updateErr := deps.DB.QueryUpdateProcedureAccess(ctx, id); updateErr != nil {
			deps.Logger.Warn("failed to update procedure access", "id", id, "error", updateErr)
		}

		// Build result
		result := GetProcedureResult{
			Procedure: procedure,
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("get_procedure completed", "id", id)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// NewDeleteProcedureHandler creates the delete_procedure tool handler.
// Deletes a procedure by ID. Idempotent - non-existent IDs silently succeed.
func NewDeleteProcedureHandler(deps *Dependencies) mcp.ToolHandlerFor[DeleteProcedureInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProcedureInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate ID
		if input.ID == "" {
			return ErrorResult("ID cannot be empty", "Provide a procedure ID to delete"), nil, nil
		}

		// Extract bare ID
		id := extractProcedureID(input.ID)

		// Delete procedure
		deleted, err := deps.DB.QueryDeleteProcedure(ctx, id)
		if err != nil {
			deps.Logger.Error("delete_procedure failed", "id", id, "error", err)
			return ErrorResult("Failed to delete procedure", "Database may be unavailable"), nil, nil
		}

		// Build result
		result := DeleteProcedureResult{
			Deleted: deleted,
			Message: fmt.Sprintf("Deleted %d procedure(s)", deleted),
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("delete_procedure completed", "id", id, "deleted", deleted)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// SearchProceduresInput defines the input schema for the search_procedures tool.
type SearchProceduresInput struct {
	Query   string   `json:"query" jsonschema:"required,Semantic search query"`
	Labels  []string `json:"labels,omitempty" jsonschema:"Filter by labels (matches ANY)"`
	Context string   `json:"context,omitempty" jsonschema:"Project namespace filter (auto-detected if omitted)"`
	Limit   int      `json:"limit,omitempty" jsonschema:"Max results 1-50 (default 10)"`
}

// ListProceduresInput defines the input schema for the list_procedures tool.
type ListProceduresInput struct {
	Context string `json:"context,omitempty" jsonschema:"Project namespace filter (auto-detected if omitted)"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Max results 1-100 (default 50)"`
}

// ProcedureSearchResult is the response from search_procedures.
type ProcedureSearchResult struct {
	Procedures []ProcedureSummary `json:"procedures"`
	Count      int                `json:"count"`
}

// ProcedureSummary is a lightweight procedure representation for search/list results.
type ProcedureSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	StepCount   int      `json:"step_count"`
	Labels      []string `json:"labels,omitempty"`
	Context     *string  `json:"context,omitempty"`
}

// NewSearchProceduresHandler creates the search_procedures tool handler.
// Searches procedural memories using hybrid BM25+vector search with optional filtering.
func NewSearchProceduresHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[SearchProceduresInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SearchProceduresInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate query
		if input.Query == "" {
			return ErrorResult("Query cannot be empty", "Provide a search query"), nil, nil
		}

		// Set limit defaults and validate
		limit := input.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}

		// Generate embedding
		embedding, err := deps.Embedder.Embed(ctx, input.Query)
		if err != nil {
			deps.Logger.Error("embedding failed", "error", err)
			return ErrorResult("Failed to generate embedding", "Check Ollama connection"), nil, nil
		}

		// Detect context: explicit > config
		var contextFilter *string
		if input.Context != "" {
			contextFilter = &input.Context
		} else {
			contextFilter = DetectContext(cfg)
		}

		// Query procedures
		procedures, err := deps.DB.QuerySearchProcedures(ctx, input.Query, embedding, input.Labels, contextFilter, limit)
		if err != nil {
			deps.Logger.Error("search_procedures failed", "error", err)
			return ErrorResult("Failed to search procedures", "Database may be unavailable"), nil, nil
		}

		// Update access tracking for each result (fire-and-forget)
		for _, proc := range procedures {
			go func(id string) {
				_ = deps.DB.QueryUpdateProcedureAccess(context.Background(), id)
			}(extractProcedureID(proc.ID))
		}

		// Build result with summaries (no full steps)
		summaries := make([]ProcedureSummary, len(procedures))
		for i, proc := range procedures {
			summaries[i] = ProcedureSummary{
				ID:          proc.ID,
				Name:        proc.Name,
				Description: proc.Description,
				StepCount:   len(proc.Steps),
				Labels:      proc.Labels,
				Context:     proc.Context,
			}
		}

		result := ProcedureSearchResult{
			Procedures: summaries,
			Count:      len(summaries),
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		// Log query (truncated) and result count
		queryLog := input.Query
		if len(queryLog) > 100 {
			queryLog = queryLog[:100] + "..."
		}
		deps.Logger.Info("search_procedures completed", "query", queryLog, "results", len(summaries))
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// NewListProceduresHandler creates the list_procedures tool handler.
// Lists all procedures with optional context filtering.
func NewListProceduresHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[ListProceduresInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ListProceduresInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Set limit defaults and validate
		limit := input.Limit
		if limit <= 0 {
			limit = 50
		}
		if limit > 100 {
			limit = 100
		}

		// Detect context: explicit > config
		var contextFilter *string
		if input.Context != "" {
			contextFilter = &input.Context
		} else {
			contextFilter = DetectContext(cfg)
		}

		// Query procedures
		procedures, err := deps.DB.QueryListProcedures(ctx, contextFilter, limit)
		if err != nil {
			deps.Logger.Error("list_procedures failed", "error", err)
			return ErrorResult("Failed to list procedures", "Database may be unavailable"), nil, nil
		}

		// Build result with summaries (no full steps)
		summaries := make([]ProcedureSummary, len(procedures))
		for i, proc := range procedures {
			summaries[i] = ProcedureSummary{
				ID:          proc.ID,
				Name:        proc.Name,
				Description: proc.Description,
				StepCount:   len(proc.Steps),
				Labels:      proc.Labels,
				Context:     proc.Context,
			}
		}

		result := ProcedureSearchResult{
			Procedures: summaries,
			Count:      len(summaries),
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("list_procedures completed", "results", len(summaries))
		return TextResult(string(jsonBytes)), nil, nil
	}
}
