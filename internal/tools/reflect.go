package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

// ReflectInput defines the input schema for the reflect tool.
type ReflectInput struct {
	Action              string  `json:"action" jsonschema:"required,enum=decay|similar,Maintenance action: 'decay' reduces scores for stale entities, 'similar' finds potential duplicates"`
	DryRun              bool    `json:"dry_run,omitempty" jsonschema:"Preview changes without applying (default: false)"`
	Context             string  `json:"context,omitempty" jsonschema:"Project namespace filter (auto-detected if omitted)"`
	Global              bool    `json:"global,omitempty" jsonschema:"Apply to all contexts (default: false)"`
	DecayDays           int     `json:"decay_days,omitempty" jsonschema:"Days of inactivity before decay applies (default: 30)"`
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty" jsonschema:"Minimum similarity score 0.0-1.0 (default: 0.85)"`
	Limit               int     `json:"limit,omitempty" jsonschema:"Max similar pairs to return (default: 10)"`
}

// ReflectResult wraps the action-specific result.
type ReflectResult struct {
	Action  string                    `json:"action"`
	Decay   *models.DecayResult       `json:"decay,omitempty"`
	Similar *models.SimilarPairsResult `json:"similar,omitempty"`
}

// NewReflectHandler creates the reflect tool handler.
// Supports maintenance actions: decay (reduce stale entity scores) and similar (find duplicates).
func NewReflectHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[ReflectInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ReflectInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate action
		if input.Action != "decay" && input.Action != "similar" {
			return ErrorResult("Invalid action", "Use 'decay' or 'similar'"), nil, nil
		}

		// Detect context: explicit > config
		var contextFilter *string
		if input.Context != "" {
			contextFilter = &input.Context
		} else if !input.Global {
			contextFilter = DetectContext(cfg)
		}

		switch input.Action {
		case "decay":
			return handleDecay(ctx, deps, input, contextFilter)
		case "similar":
			return handleSimilar(ctx, deps, input, contextFilter)
		default:
			return ErrorResult("Unknown action", "Use 'decay' or 'similar'"), nil, nil
		}
	}
}

// handleDecay applies decay to stale entities.
func handleDecay(
	ctx context.Context,
	deps *Dependencies,
	input ReflectInput,
	contextFilter *string,
) (*mcp.CallToolResult, any, error) {
	// Set defaults
	decayDays := input.DecayDays
	if decayDays <= 0 {
		decayDays = 30
	}

	// Query decay
	entities, err := deps.DB.QueryApplyDecay(ctx, decayDays, contextFilter, input.Global, input.DryRun)
	if err != nil {
		deps.Logger.Error("reflect decay failed", "error", err)
		return ErrorResult("Failed to apply decay", "Database may be unavailable"), nil, nil
	}

	// Build result
	decayResult := &models.DecayResult{
		Affected: len(entities),
		DryRun:   input.DryRun,
		Entities: entities,
	}

	result := ReflectResult{
		Action: "decay",
		Decay:  decayResult,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")

	action := "applied"
	if input.DryRun {
		action = "previewed"
	}
	deps.Logger.Info("reflect decay completed", "action", action, "affected", len(entities), "decay_days", decayDays)
	return TextResult(string(jsonBytes)), nil, nil
}

// handleSimilar finds similar entity pairs.
func handleSimilar(
	ctx context.Context,
	deps *Dependencies,
	input ReflectInput,
	contextFilter *string,
) (*mcp.CallToolResult, any, error) {
	// Set defaults
	threshold := input.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.85
	}
	if threshold > 1.0 {
		threshold = 1.0
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// Query similar pairs
	pairs, err := deps.DB.QueryFindSimilarPairs(ctx, threshold, limit, contextFilter, input.Global)
	if err != nil {
		deps.Logger.Error("reflect similar failed", "error", err)
		return ErrorResult("Failed to find similar pairs", "Database may be unavailable"), nil, nil
	}

	// Build result (always dry_run=true since similar is identify-only)
	similarResult := &models.SimilarPairsResult{
		Pairs:  pairs,
		Count:  len(pairs),
		DryRun: true,
	}

	result := ReflectResult{
		Action:  "similar",
		Similar: similarResult,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")

	deps.Logger.Info("reflect similar completed", "pairs_found", len(pairs), "threshold", threshold)
	return TextResult(string(jsonBytes)), nil, nil
}
