package document

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

var toggleCheckboxRegex = regexp.MustCompile(`^(\s*- \[)([ xX])(\]\s+)`)

// ToggleTask flips a task's checkbox in the source document and re-ingests it.
// Returns the updated task after re-ingestion.
func (s *Service) ToggleTask(ctx context.Context, taskID string) (*models.Task, error) {
	logger := logutil.FromCtx(ctx)

	task, err := s.db.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	docID, err := models.RecordIDString(task.Document)
	if err != nil {
		return nil, fmt.Errorf("extract doc id: %w", err)
	}
	doc, err := s.db.GetDocumentByID(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("source document not found for task %s", taskID)
	}

	vaultID, err := models.RecordIDString(doc.Vault)
	if err != nil {
		return nil, fmt.Errorf("extract vault id: %w", err)
	}

	// Find and toggle the checkbox line.
	// Task LineNumber is relative to content-after-frontmatter (ContentBody),
	// not the raw Content which includes frontmatter.
	lines := strings.Split(doc.ContentBody, "\n")
	toggled := false

	// Primary: try the recorded line number.
	if task.LineNumber > 0 && task.LineNumber <= len(lines) {
		idx := task.LineNumber - 1
		if isMatchingCheckbox(lines[idx], task.ContentHash) {
			lines[idx] = toggleLine(lines[idx])
			toggled = true
		}
	}

	// Fallback: scan nearby lines (±5). LineNumber is 1-based, so idx = LineNumber-1; start at idx-5 = LineNumber-6.
	if !toggled {
		start := max(0, task.LineNumber-6)
		end := min(len(lines), task.LineNumber+5)
		for i := start; i < end; i++ {
			if isMatchingCheckbox(lines[i], task.ContentHash) {
				lines[i] = toggleLine(lines[i])
				toggled = true
				logger.Info("toggle: task not at expected line, found nearby", "expected", task.LineNumber, "found", i+1, "task_id", taskID)
				break
			}
		}
	}

	// Last resort: full document scan.
	if !toggled {
		for i := range lines {
			if isMatchingCheckbox(lines[i], task.ContentHash) {
				lines[i] = toggleLine(lines[i])
				toggled = true
				logger.Warn("toggle: task not found near expected line, required full document scan", "expected", task.LineNumber, "found", i+1, "task_id", taskID)
				break
			}
		}
	}

	if !toggled {
		return nil, fmt.Errorf("could not find checkbox for task %s in document %s", taskID, doc.Path)
	}

	// Reconstruct full content: ContentBody is always a suffix of Content.
	newBody := strings.Join(lines, "\n")
	prefix := doc.Content[:len(doc.Content)-len(doc.ContentBody)]
	newContent := prefix + newBody
	_, err = s.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    doc.Path,
		Content: newContent,
	})
	if err != nil {
		return nil, fmt.Errorf("write back: %w", err)
	}

	// Process immediately so the caller gets fresh data.
	if err := s.ProcessAllPending(ctx); err != nil {
		return nil, fmt.Errorf("process after toggle: %w", err)
	}

	// Fetch the updated task. Since toggle only changes the checkbox marker (not the
	// cleaned text), the content hash is stable and syncTasks updates the existing
	// record rather than deleting/recreating it. A nil result here would indicate the
	// document was concurrently modified and the task was removed.
	updated, err := s.db.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get updated task: %w", err)
	}
	if updated == nil {
		return nil, fmt.Errorf("task %s not found after toggle — document may have been concurrently modified", taskID)
	}
	return updated, nil
}

// isMatchingCheckbox checks if a line is a checkbox whose cleaned text matches the given content hash.
func isMatchingCheckbox(line string, contentHash string) bool {
	m := toggleCheckboxRegex.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	rest := line[len(m[0]):]
	cleaned := parser.CleanTaskText(rest)
	return models.ContentHash(cleaned) == contentHash
}

// toggleLine flips `- [ ]` to `- [x]` or vice versa.
func toggleLine(line string) string {
	return toggleCheckboxRegex.ReplaceAllStringFunc(line, func(match string) string {
		if strings.Contains(match, "[ ]") {
			return strings.Replace(match, "[ ]", "[x]", 1)
		}
		match = strings.Replace(match, "[x]", "[ ]", 1)
		match = strings.Replace(match, "[X]", "[ ]", 1)
		return match
	})
}
