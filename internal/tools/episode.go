package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

// AddEpisodeInput defines the input schema for the add_episode tool.
type AddEpisodeInput struct {
	Content   string         `json:"content" jsonschema:"required,Full episode content (conversation, notes, etc.)"`
	Summary   string         `json:"summary,omitempty" jsonschema:"Optional brief summary"`
	Metadata  map[string]any `json:"metadata,omitempty" jsonschema:"Flexible metadata (session_id, source, participants)"`
	Context   string         `json:"context,omitempty" jsonschema:"Project namespace (auto-detected if omitted)"`
	EntityIDs []string       `json:"entity_ids,omitempty" jsonschema:"Entity IDs to link as extracted from this episode"`
}

// GetEpisodeInput defines the input schema for the get_episode tool.
type GetEpisodeInput struct {
	ID              string `json:"id" jsonschema:"required,Episode ID (with or without 'episode:' prefix)"`
	IncludeEntities bool   `json:"include_entities,omitempty" jsonschema:"Include linked entities in response"`
}

// DeleteEpisodeInput defines the input schema for the delete_episode tool.
type DeleteEpisodeInput struct {
	ID string `json:"id" jsonschema:"required,Episode ID to delete"`
}

// AddEpisodeResult is the response from add_episode.
type AddEpisodeResult struct {
	ID             string    `json:"id"`
	ContentPreview string    `json:"content_preview"`
	Timestamp      time.Time `json:"timestamp"`
	LinkedEntities int       `json:"linked_entities"`
	Context        *string   `json:"context,omitempty"`
}

// GetEpisodeResult is the response from get_episode.
type GetEpisodeResult struct {
	Episode  *models.Episode  `json:"episode"`
	Entities []models.Entity `json:"entities,omitempty"`
}

// DeleteEpisodeResult is the response from delete_episode.
type DeleteEpisodeResult struct {
	Deleted int    `json:"deleted"`
	Message string `json:"message"`
}

// SearchEpisodesInput defines the input schema for the search_episodes tool.
type SearchEpisodesInput struct {
	Query     string `json:"query" jsonschema:"required,Semantic search query"`
	TimeStart string `json:"time_start,omitempty" jsonschema:"Filter episodes after this time (ISO 8601, e.g. 2026-01-15T00:00:00Z)"`
	TimeEnd   string `json:"time_end,omitempty" jsonschema:"Filter episodes before this time (ISO 8601)"`
	Context   string `json:"context,omitempty" jsonschema:"Project namespace filter (auto-detected if omitted)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results 1-50 (default 10)"`
}

// EpisodeSearchResult is the response from search_episodes.
type EpisodeSearchResult struct {
	Episodes []EpisodeResult `json:"episodes"`
	Count    int             `json:"count"`
}

// EpisodeResult represents a single episode in search results.
type EpisodeResult struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"` // Full content, NOT truncated
	Timestamp string         `json:"timestamp"`
	Summary   *string        `json:"summary,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Context   *string        `json:"context,omitempty"`
}

// extractEpisodeID removes "episode:" prefix if present.
func extractEpisodeID(id string) string {
	return strings.TrimPrefix(id, "episode:")
}

// generateEpisodeID creates a timestamp-based episode ID.
// Format: ep_2024-01-15T14-30-45Z (RFC3339 with colons replaced).
func generateEpisodeID() string {
	ts := time.Now().UTC().Format(time.RFC3339)
	// Replace colons with hyphens for valid ID
	ts = strings.ReplaceAll(ts, ":", "-")
	return "ep_" + ts
}

// truncateContent truncates a string to max length with ellipsis.
func truncateContent(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// NewAddEpisodeHandler creates the add_episode tool handler.
// Stores episodic memories with auto-generated timestamp and embedding.
func NewAddEpisodeHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[AddEpisodeInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input AddEpisodeInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate content
		if input.Content == "" {
			return ErrorResult("Content cannot be empty", "Provide episode content"), nil, nil
		}

		// Generate timestamp-based ID
		episodeID := generateEpisodeID()
		timestamp := time.Now().UTC()

		// Detect context: explicit > config > nil
		var episodeContext *string
		if input.Context != "" {
			episodeContext = &input.Context
		} else {
			episodeContext = DetectContext(cfg)
		}

		// Truncate content for embedding (8000 chars max)
		embeddingContent := truncateContent(input.Content, 8000)

		// Generate embedding
		embedding, err := deps.Embedder.Embed(ctx, embeddingContent)
		if err != nil {
			deps.Logger.Error("embedding failed", "error", err)
			return ErrorResult("Failed to generate embedding", "Check Ollama connection"), nil, nil
		}

		// Prepare optional summary
		var summary *string
		if input.Summary != "" {
			summary = &input.Summary
		}

		// Create episode
		episode, err := deps.DB.QueryCreateEpisode(
			ctx,
			episodeID,
			input.Content,
			embedding,
			timestamp.Format(time.RFC3339),
			summary,
			input.Metadata,
			episodeContext,
		)
		if err != nil {
			deps.Logger.Error("create episode failed", "id", episodeID, "error", err)
			return ErrorResult("Failed to create episode", "Database may be unavailable"), nil, nil
		}

		// Link entities (log failures, don't fail operation)
		linkedCount := 0
		for i, entityID := range input.EntityIDs {
			// Strip entity: prefix if present
			bareID := strings.TrimPrefix(entityID, "entity:")
			if err := deps.DB.QueryLinkEntityToEpisode(ctx, bareID, episodeID, i, 1.0); err != nil {
				deps.Logger.Warn("failed to link entity", "entity", bareID, "episode", episodeID, "error", err)
			} else {
				linkedCount++
			}
		}

		// Build result
		result := AddEpisodeResult{
			ID:             episode.ID,
			ContentPreview: truncateContent(input.Content, 500),
			Timestamp:      timestamp,
			LinkedEntities: linkedCount,
			Context:        episodeContext,
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("add_episode completed", "id", episodeID, "linked", linkedCount)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// NewGetEpisodeHandler creates the get_episode tool handler.
// Retrieves an episode by ID with full content.
func NewGetEpisodeHandler(deps *Dependencies) mcp.ToolHandlerFor[GetEpisodeInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetEpisodeInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate ID
		if input.ID == "" {
			return ErrorResult("ID cannot be empty", "Provide an episode ID"), nil, nil
		}

		// Extract bare ID
		id := extractEpisodeID(input.ID)

		// Query episode
		episode, err := deps.DB.QueryGetEpisode(ctx, id)
		if err != nil {
			deps.Logger.Error("get_episode failed", "id", id, "error", err)
			return ErrorResult("Failed to retrieve episode", "Database may be unavailable"), nil, nil
		}

		// Handle not found
		if episode == nil {
			return ErrorResult(fmt.Sprintf("Episode not found: %s", id), "Use add_episode to create episodes"), nil, nil
		}

		// Update access tracking
		if updateErr := deps.DB.QueryUpdateEpisodeAccess(ctx, id); updateErr != nil {
			deps.Logger.Warn("failed to update episode access", "id", id, "error", updateErr)
		}

		// Build result
		result := GetEpisodeResult{
			Episode: episode,
		}

		// Optionally include linked entities
		if input.IncludeEntities {
			entities, err := deps.DB.QueryGetLinkedEntities(ctx, id)
			if err != nil {
				deps.Logger.Warn("failed to get linked entities", "id", id, "error", err)
			} else {
				result.Entities = entities
			}
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("get_episode completed", "id", id)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// NewDeleteEpisodeHandler creates the delete_episode tool handler.
// Deletes an episode by ID. Idempotent - non-existent IDs silently succeed.
func NewDeleteEpisodeHandler(deps *Dependencies) mcp.ToolHandlerFor[DeleteEpisodeInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input DeleteEpisodeInput) (
		*mcp.CallToolResult, any, error,
	) {
		// Validate ID
		if input.ID == "" {
			return ErrorResult("ID cannot be empty", "Provide an episode ID to delete"), nil, nil
		}

		// Extract bare ID
		id := extractEpisodeID(input.ID)

		// Delete episode
		deleted, err := deps.DB.QueryDeleteEpisode(ctx, id)
		if err != nil {
			deps.Logger.Error("delete_episode failed", "id", id, "error", err)
			return ErrorResult("Failed to delete episode", "Database may be unavailable"), nil, nil
		}

		// Build result
		result := DeleteEpisodeResult{
			Deleted: deleted,
			Message: fmt.Sprintf("Deleted %d episode(s)", deleted),
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		deps.Logger.Info("delete_episode completed", "id", id, "deleted", deleted)
		return TextResult(string(jsonBytes)), nil, nil
	}
}

// ensureTimezone appends Z if timestamp lacks timezone indicator.
func ensureTimezone(ts string) string {
	if ts == "" || len(ts) < 6 {
		return ts
	}
	// Check for Z suffix
	if strings.HasSuffix(ts, "Z") {
		return ts
	}
	// Check for timezone offset patterns (+HH:MM or -HH:MM) in last 6 chars
	tail := ts[len(ts)-6:]
	if strings.Contains(tail, "+") || strings.Contains(tail, "-") {
		return ts
	}
	return ts + "Z"
}

// NewSearchEpisodesHandler creates the search_episodes tool handler.
// Searches episodic memories using hybrid BM25+vector search with optional time filtering.
func NewSearchEpisodesHandler(deps *Dependencies, cfg *config.Config) mcp.ToolHandlerFor[SearchEpisodesInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SearchEpisodesInput) (
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

		// Prepare time filters (optional)
		var timeStart, timeEnd *string
		if input.TimeStart != "" {
			ts := ensureTimezone(input.TimeStart)
			timeStart = &ts
		}
		if input.TimeEnd != "" {
			te := ensureTimezone(input.TimeEnd)
			timeEnd = &te
		}

		// Detect context: explicit > config
		var contextFilter *string
		if input.Context != "" {
			contextFilter = &input.Context
		} else {
			contextFilter = DetectContext(cfg)
		}

		// Query episodes
		episodes, err := deps.DB.QuerySearchEpisodes(ctx, input.Query, embedding, timeStart, timeEnd, contextFilter, limit)
		if err != nil {
			deps.Logger.Error("search_episodes failed", "error", err)
			return ErrorResult("Failed to search episodes", "Database may be unavailable"), nil, nil
		}

		// Update access tracking for each result (fire-and-forget)
		for _, ep := range episodes {
			go func(id string) {
				_ = deps.DB.QueryUpdateEpisodeAccess(context.Background(), id)
			}(extractEpisodeID(ep.ID))
		}

		// Build result with FULL content (user decision: no truncation)
		results := make([]EpisodeResult, len(episodes))
		for i, ep := range episodes {
			results[i] = EpisodeResult{
				ID:        ep.ID,
				Content:   ep.Content, // Full content, NOT truncated
				Timestamp: ep.Timestamp.Format(time.RFC3339),
				Summary:   ep.Summary,
				Metadata:  ep.Metadata,
				Context:   ep.Context,
			}
		}

		result := EpisodeSearchResult{
			Episodes: results,
			Count:    len(results),
		}

		jsonBytes, _ := json.MarshalIndent(result, "", "  ")

		// Log query (truncated) and result count
		queryLog := truncateContent(input.Query, 100)
		deps.Logger.Info("search_episodes completed", "query", queryLog, "results", len(results))
		return TextResult(string(jsonBytes)), nil, nil
	}
}
