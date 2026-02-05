// Package graph provides GraphQL resolvers for Knowhow.
package graph

import (
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/metrics"
	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/raphaelgruber/memcp-go/internal/service"
)

// entityToGraphQL converts a models.Entity to a GraphQL Entity.
func entityToGraphQL(e *models.Entity) *Entity {
	if e == nil {
		return nil
	}

	idStr, err := models.RecordIDString(e.ID)
	if err != nil {
		idStr = fmt.Sprintf("%v", e.ID.ID)
	}

	return &Entity{
		ID:          idStr,
		Type:        e.Type,
		Name:        e.Name,
		Content:     e.Content,
		Summary:     e.Summary,
		Labels:      e.Labels,
		ContentHash: e.ContentHash,
		Verified:    e.Verified,
		Confidence:  e.Confidence,
		Source:      string(e.Source),
		SourcePath:  e.SourcePath,
		Metadata:    e.Metadata,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		AccessedAt:  e.Accessed,
		AccessCount: e.AccessCount,
		Relations:   []Relation{}, // Relations loaded separately if needed
	}
}

// templateToGraphQL converts a models.Template to a GraphQL Template.
func templateToGraphQL(t *models.Template) *Template {
	if t == nil {
		return nil
	}

	idStr, err := models.RecordIDString(t.ID)
	if err != nil {
		idStr = fmt.Sprintf("%v", t.ID.ID)
	}

	return &Template{
		ID:          idStr,
		Name:        t.Name,
		Description: t.Description,
		Content:     t.Content,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// searchResultToGraphQL converts a models.EntitySearchResult to a GraphQL EntitySearchResult.
func searchResultToGraphQL(r *models.EntitySearchResult) *EntitySearchResult {
	if r == nil {
		return nil
	}

	entity := entityToGraphQL(&r.Entity)

	chunks := make([]ChunkMatch, len(r.MatchedChunks))
	for i, chunk := range r.MatchedChunks {
		chunks[i] = ChunkMatch{
			Content:     chunk.Content,
			HeadingPath: chunk.HeadingPath,
			Position:    chunk.Position,
		}
	}

	return &EntitySearchResult{
		Entity:        *entity,
		MatchedChunks: chunks,
		Score:         r.Score,
	}
}

// serviceJobToGraphQL converts a service.Job to a GraphQL Job.
func serviceJobToGraphQL(j *service.Job) *Job {
	snapshot := j.Snapshot()
	var errPtr *string
	if snapshot.Error != "" {
		errPtr = &snapshot.Error
	}
	var result *IngestResult
	if snapshot.Result != nil {
		result = &IngestResult{
			FilesProcessed:   snapshot.Result.FilesProcessed,
			FilesSkipped:     snapshot.Result.FilesSkipped,
			EntitiesCreated:  snapshot.Result.EntitiesCreated,
			ChunksCreated:    snapshot.Result.ChunksCreated,
			RelationsCreated: snapshot.Result.RelationsCreated,
			Errors:           snapshot.Result.Errors,
		}
	}

	// Handle persistence fields
	var dirPath *string
	if snapshot.DirPath != "" {
		dirPath = &snapshot.DirPath
	}
	var pendingFiles *int
	if snapshot.PendingFiles > 0 {
		pendingFiles = &snapshot.PendingFiles
	}

	// Handle name (optional)
	var name *string
	if snapshot.Name != "" {
		name = &snapshot.Name
	}

	// Ensure labels is never nil
	labels := snapshot.Labels
	if labels == nil {
		labels = []string{}
	}

	return &Job{
		ID:           snapshot.ID,
		Type:         snapshot.Type,
		Status:       string(snapshot.Status),
		Name:         name,
		Labels:       labels,
		Progress:     snapshot.Progress,
		Total:        snapshot.Total,
		Result:       result,
		Error:        errPtr,
		StartedAt:    snapshot.StartedAt,
		CompletedAt:  snapshot.CompletedAt,
		DirPath:      dirPath,
		PendingFiles: pendingFiles,
	}
}

// dbJobToGraphQL converts a models.IngestJob (from database) to a GraphQL Job.
func dbJobToGraphQL(j *models.IngestJob) *Job {
	if j == nil {
		return nil
	}

	jobID, err := models.RecordIDString(j.ID)
	if err != nil {
		jobID = fmt.Sprintf("%v", j.ID.ID)
	}

	var errPtr *string
	if j.Error != nil {
		errPtr = j.Error
	}

	var result *IngestResult
	if j.Result != nil {
		result = &IngestResult{
			FilesProcessed:   intFromMap(j.Result, "files_processed"),
			EntitiesCreated:  intFromMap(j.Result, "entities_created"),
			ChunksCreated:    intFromMap(j.Result, "chunks_created"),
			RelationsCreated: intFromMap(j.Result, "relations_created"),
			Errors:           stringsFromMap(j.Result, "errors"),
		}
	}

	var dirPath *string
	if j.DirPath != "" {
		dirPath = &j.DirPath
	}

	// Ensure labels is never nil
	labels := j.Labels
	if labels == nil {
		labels = []string{}
	}

	return &Job{
		ID:          jobID,
		Type:        j.JobType,
		Status:      j.Status,
		Name:        j.Name,
		Labels:      labels,
		Progress:    j.Progress,
		Total:       j.Total,
		Result:      result,
		Error:       errPtr,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
		DirPath:     dirPath,
	}
}

// intFromMap extracts an int from a map[string]any.
func intFromMap(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

// stringsFromMap extracts a []string from a map[string]any.
func stringsFromMap(m map[string]any, key string) []string {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]any); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
		if arr, ok := v.([]string); ok {
			return arr
		}
	}
	return []string{}
}

// intPtr returns a pointer to an int value.
func intPtr(v int64) *int {
	i := int(v)
	return &i
}

// floatPtr returns a pointer to a float64 value.
func floatPtr(v float64) *float64 {
	return &v
}

// operationSnapshotToGraphQL converts a metrics.OperationSnapshot to a GraphQL OperationStats.
func operationSnapshotToGraphQL(s *metrics.OperationSnapshot) *OperationStats {
	if s == nil {
		return nil
	}

	stats := &OperationStats{
		Count:       int(s.Count),
		TotalTimeMs: int(s.TotalTimeMs),
		AvgTimeMs:   s.AvgTimeMs,
		MinTimeMs:   int(s.MinTimeMs),
		MaxTimeMs:   int(s.MaxTimeMs),
	}

	// Add token stats if present
	if s.TotalInputTokens != nil {
		stats.TotalInputTokens = intPtr(*s.TotalInputTokens)
	}
	if s.TotalOutputTokens != nil {
		stats.TotalOutputTokens = intPtr(*s.TotalOutputTokens)
	}
	if s.AvgInputTokens != nil {
		stats.AvgInputTokens = floatPtr(*s.AvgInputTokens)
	}
	if s.AvgOutputTokens != nil {
		stats.AvgOutputTokens = floatPtr(*s.AvgOutputTokens)
	}
	if s.MinInputTokens != nil {
		stats.MinInputTokens = intPtr(*s.MinInputTokens)
	}
	if s.MaxInputTokens != nil {
		stats.MaxInputTokens = intPtr(*s.MaxInputTokens)
	}
	if s.MinOutputTokens != nil {
		stats.MinOutputTokens = intPtr(*s.MinOutputTokens)
	}
	if s.MaxOutputTokens != nil {
		stats.MaxOutputTokens = intPtr(*s.MaxOutputTokens)
	}

	return stats
}

// metricsSnapshotToGraphQL converts a metrics.Snapshot to a GraphQL ServerStats.
func metricsSnapshotToGraphQL(s metrics.Snapshot) *ServerStats {
	return &ServerStats{
		UptimeSeconds: s.UptimeSeconds,
		Embedding:     operationSnapshotToGraphQL(s.Embedding),
		LlmGenerate:   operationSnapshotToGraphQL(s.LLMGenerate),
		LlmStream:     operationSnapshotToGraphQL(s.LLMStream),
		DbQuery:       operationSnapshotToGraphQL(s.DBQuery),
		DbSearch:      operationSnapshotToGraphQL(s.DBSearch),
	}
}
