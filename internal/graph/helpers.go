// Package graph provides GraphQL resolvers for Knowhow.
package graph

import (
	"fmt"

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

	return &Job{
		ID:           snapshot.ID,
		Type:         snapshot.Type,
		Status:       string(snapshot.Status),
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
