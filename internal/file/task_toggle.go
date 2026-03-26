package file

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"
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
		return nil, fmt.Errorf("task %s raw_line is not a valid checkbox: %q", taskID, task.RawLine)
	}

	// Mark file dirty FIRST — if this fails, nothing has changed yet.
	// If UpdateTask then fails, the file is dirty unnecessarily but the
	// next reconciliation finds no diff and clears it (self-healing).
	if err := s.db.SetFileDirtyTasks(ctx, fileID, true); err != nil {
		return nil, fmt.Errorf("set dirty tasks: %w", err)
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

	// Return the updated task without re-fetching — only Status and RawLine changed.
	task.Status = newStatus
	task.RawLine = newRawLine
	return task, nil
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
