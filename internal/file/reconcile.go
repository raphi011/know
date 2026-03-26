package file

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

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

// applyTaskChanges takes raw file content and a list of tasks with their
// current DB status, and returns the content with checkboxes updated to
// match the DB state, plus the names of any tasks that could not be matched
// to a line. This is a pure function — no side effects.
func applyTaskChanges(rawContent string, tasks []models.Task) (string, []string) {
	parsed := parser.ParseMarkdown(rawContent)
	contentBody := parsed.Content
	lines := strings.Split(contentBody, "\n")

	var unmatched []string
	for _, task := range tasks {
		toggleLine := findToggleLine(parsed.Tasks, task.ContentHash, task.LineNumber)
		if toggleLine <= 0 || toggleLine > len(lines) {
			unmatched = append(unmatched, task.Text)
			continue
		}

		idx := toggleLine - 1
		line := lines[idx]

		// Determine what the markdown currently shows.
		currentlyDone := strings.Contains(line, "[x]") || strings.Contains(line, "[X]")
		wantDone := task.Status == models.TaskStatusDone

		if currentlyDone != wantDone {
			lines[idx] = flipCheckbox(line)
		}
	}

	newBody := strings.Join(lines, "\n")
	prefix := rawContent[:len(rawContent)-len(contentBody)]
	return prefix + newBody, unmatched
}

// reconcileDirtyTasks loads the file content, applies pending task changes,
// persists the result, and clears the dirty flag. Returns the updated content.
func (s *Service) reconcileDirtyTasks(ctx context.Context, f *models.File) (string, error) {
	logger := logutil.FromCtx(ctx)

	fileID, err := models.RecordIDString(f.ID)
	if err != nil {
		return "", fmt.Errorf("extract file id: %w", err)
	}

	rawContent, err := s.ReadContent(ctx, *f.Hash)
	if err != nil {
		return "", fmt.Errorf("read content for reconciliation: %w", err)
	}

	tasks, err := s.db.GetTasksByFile(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("get tasks for reconciliation: %w", err)
	}

	newContent, unmatched := applyTaskChanges(rawContent, tasks)
	if len(unmatched) > 0 {
		logger.Warn("reconcile: could not find lines for tasks", "file", f.Path, "unmatched", unmatched)
	}

	// If nothing changed, just clear the flag.
	if newContent == rawContent {
		logger.Debug("reconcile: no checkbox changes needed", "file", f.Path)
		if err := s.db.SetFileDirtyTasks(ctx, fileID, false); err != nil {
			return "", fmt.Errorf("clear dirty flag: %w", err)
		}
		return rawContent, nil
	}

	// Persist updated content to blob store.
	newHash := models.ContentHash(newContent)
	if err := s.blobStore.Put(ctx, newHash, strings.NewReader(newContent), int64(len(newContent))); err != nil {
		return "", fmt.Errorf("store reconciled content: %w", err)
	}

	// Atomically update file hash and clear dirty flag.
	if err := s.db.UpdateFileHashAndClearDirty(ctx, fileID, &newHash); err != nil {
		return "", fmt.Errorf("update file hash: %w", err)
	}

	// Update the in-memory file struct so callers see the new hash.
	f.Hash = &newHash
	f.DirtyTasks = false

	logger.Debug("reconcile: applied task changes", "file", f.Path, "new_hash", newHash)
	return newContent, nil
}
