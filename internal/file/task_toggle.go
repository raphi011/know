package file

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

var toggleCheckboxRegex = regexp.MustCompile(`^(\s*- \[)([ xX])(\]\s+)`)

// ToggleTask flips a task's status in the database and marks the parent file
// as having dirty tasks. The markdown file is not rewritten immediately —
// instead, the next ReadFileContent call will reconcile the checkbox state.
func (s *Service) ToggleTask(ctx context.Context, vaultID, taskID string) (*models.Task, error) {
	task, err := s.db.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	fileID, err := models.RecordIDString(task.File)
	if err != nil {
		return nil, fmt.Errorf("extract file id: %w", err)
	}
	doc, err := s.db.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("source file not found for task %s", taskID)
	}

	docVaultID, err := models.RecordIDString(doc.Vault)
	if err != nil {
		return nil, fmt.Errorf("extract vault id: %w", err)
	}
	if docVaultID != vaultID {
		return nil, fmt.Errorf("task %s does not belong to vault %s", taskID, vaultID)
	}

	// Flip status.
	newStatus := models.TaskStatusDone
	if task.Status == models.TaskStatusDone {
		newStatus = models.TaskStatusOpen
	}

	// Update raw_line checkbox marker to match new status.
	newRawLine := flipCheckbox(task.RawLine)
	if newRawLine == task.RawLine {
		// Fallback: if raw_line doesn't match checkbox regex, just update status.
		newRawLine = task.RawLine
	}

	// Update task status in DB.
	if err := s.db.UpdateTask(ctx, taskID, db.TaskUpdate{
		Status:      newStatus,
		RawLine:     newRawLine,
		Text:        task.Text,
		Labels:      task.Labels,
		DueDate:     task.DueDate,
		LineNumber:  task.LineNumber,
		HeadingPath: task.HeadingPath,
	}); err != nil {
		return nil, fmt.Errorf("update task status: %w", err)
	}

	// Mark file as having dirty tasks.
	if err := s.db.SetFileDirtyTasks(ctx, fileID, true); err != nil {
		return nil, fmt.Errorf("set dirty tasks: %w", err)
	}

	// Return the updated task.
	updated, err := s.db.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get updated task: %w", err)
	}
	if updated == nil {
		return nil, fmt.Errorf("task %s not found after toggle", taskID)
	}
	return updated, nil
}

// findToggleLine returns the 1-based line number of the task to toggle.
// When multiple tasks share the same ContentHash (identical text), it picks
// the one closest to expectedLine. Returns -1 if no match is found.
func findToggleLine(tasks []parser.ExtractedTask, contentHash string, expectedLine int) int {
	best := -1
	bestDist := int(^uint(0) >> 1) // max int
	for _, pt := range tasks {
		if pt.ContentHash != contentHash {
			continue
		}
		dist := pt.LineNumber - expectedLine
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			best = pt.LineNumber
		}
	}
	return best
}

// flipCheckbox flips `- [ ]` to `- [x]` or vice versa.
func flipCheckbox(line string) string {
	return toggleCheckboxRegex.ReplaceAllStringFunc(line, func(match string) string {
		if strings.Contains(match, "[ ]") {
			return strings.Replace(match, "[ ]", "[x]", 1)
		}
		match = strings.Replace(match, "[x]", "[ ]", 1)
		match = strings.Replace(match, "[X]", "[ ]", 1)
		return match
	})
}
