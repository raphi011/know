package document

import (
	"context"
	"fmt"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

// syncTasks syncs pre-extracted checkbox tasks with the DB.
// Uses content-hash-based diffing to preserve task identity across re-ingestion.
func (s *Service) syncTasks(ctx context.Context, docID, vaultID string, extracted []parser.ExtractedTask) error {
	logger := logutil.FromCtx(ctx)

	// Fetch existing tasks from DB.
	existing, err := s.db.GetTasksByDocument(ctx, docID)
	if err != nil {
		return fmt.Errorf("fetch existing tasks: %w", err)
	}

	// Build lookup of existing tasks by content hash.
	// For duplicate hashes (same text appears multiple times), group them.
	type existingEntry struct {
		task    models.Task
		matched bool
	}
	byHash := map[string][]*existingEntry{}
	for _, t := range existing {
		id := t.ContentHash
		byHash[id] = append(byHash[id], &existingEntry{task: t})
	}

	// Match extracted tasks against existing ones.
	for _, ext := range extracted {
		entries := byHash[ext.ContentHash]

		// Find first unmatched entry with this hash.
		var match *existingEntry
		for _, e := range entries {
			if !e.matched {
				match = e
				break
			}
		}

		if match != nil {
			// Update existing task (line number, status, metadata may have changed).
			match.matched = true
			taskID, idErr := models.RecordIDString(match.task.ID)
			if idErr != nil {
				return fmt.Errorf("corrupt task record id: %w", idErr)
			}
			if err := s.db.UpdateTask(ctx, taskID, db.TaskUpdate{
				Status:      ext.Status,
				RawLine:     ext.RawLine,
				Text:        ext.Text,
				Labels:      ext.Labels,
				DueDate:     ext.DueDate,
				LineNumber:  ext.LineNumber,
				HeadingPath: headingPtr(ext.HeadingPath),
			}); err != nil {
				return fmt.Errorf("update task: %w", err)
			}
		} else {
			// Create new task.
			_, err := s.db.CreateTask(ctx, models.TaskInput{
				DocumentID:  docID,
				VaultID:     vaultID,
				Status:      ext.Status,
				RawLine:     ext.RawLine,
				Text:        ext.Text,
				Labels:      ext.Labels,
				DueDate:     ext.DueDate,
				LineNumber:  ext.LineNumber,
				HeadingPath: headingPtr(ext.HeadingPath),
				ContentHash: ext.ContentHash,
			})
			if err != nil {
				return fmt.Errorf("create task: %w", err)
			}
		}
	}

	// Delete tasks that no longer exist in the document.
	for _, entries := range byHash {
		for _, e := range entries {
			if e.matched {
				continue
			}
			taskID, idErr := models.RecordIDString(e.task.ID)
			if idErr != nil {
				return fmt.Errorf("corrupt task record id for delete: %w", idErr)
			}
			if err := s.db.DeleteTask(ctx, taskID); err != nil {
				return fmt.Errorf("delete removed task: %w", err)
			}
		}
	}

	logger.Debug("synced tasks", "doc", docID, "extracted", len(extracted), "existing", len(existing))
	return nil
}

func headingPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
