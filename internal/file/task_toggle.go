package file

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

// ToggleTask flips a task's checkbox in the source file and re-ingests it.
// The vaultID parameter ensures the task belongs to the expected vault,
// preventing cross-vault task manipulation.
// Returns the updated task after re-ingestion.
func (s *Service) ToggleTask(ctx context.Context, vaultID, taskID string) (*models.Task, error) {
	logger := logutil.FromCtx(ctx)

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

	// Load content from blob store.
	rawContent, err := s.ReadFileContent(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	// Find and toggle the checkbox line.
	// Task LineNumber is relative to content-after-frontmatter (content body),
	// not the raw content which includes frontmatter.
	parsed := parser.ParseMarkdown(rawContent)
	contentBody := parsed.Content
	lines := strings.Split(contentBody, "\n")

	// Use the parser's extracted tasks to find the matching line. This avoids
	// hash mismatches between AST-derived text (used at extraction time) and
	// raw line text (which retains markdown syntax like [[wiki-links]]).
	toggleLine := findToggleLine(parsed.Tasks, task.ContentHash, task.LineNumber)

	if toggleLine <= 0 || toggleLine > len(lines) {
		return nil, fmt.Errorf("could not find checkbox for task %s in file %s", taskID, doc.Path)
	}

	idx := toggleLine - 1
	if toggleLine != task.LineNumber {
		logger.Info("toggle: task line drifted", "expected", task.LineNumber, "found", toggleLine, "task_id", taskID)
	}
	newLine := flipCheckbox(lines[idx])
	if newLine == lines[idx] {
		return nil, fmt.Errorf("line %d in file %s is not a checkbox", toggleLine, doc.Path)
	}
	lines[idx] = newLine

	// Reconstruct full content: contentBody is always a suffix of rawContent.
	newBody := strings.Join(lines, "\n")
	prefix := rawContent[:len(rawContent)-len(contentBody)]
	newContent := prefix + newBody
	_, err = s.Create(ctx, models.FileInput{
		VaultID: docVaultID,
		Path:    doc.Path,
		Content: newContent,
	})
	if err != nil {
		return nil, fmt.Errorf("write back: %w", err)
	}

	// Process immediately so the caller gets fresh data.
	if err := s.ProcessAllPending(ctx, docVaultID); err != nil {
		return nil, fmt.Errorf("process after toggle: %w", err)
	}

	// Fetch the updated task. Since toggle only changes the checkbox marker (not the
	// cleaned text), the content hash is stable and syncTasks updates the existing
	// record rather than deleting/recreating it. A nil result here would indicate the
	// file was concurrently modified and the task was removed.
	updated, err := s.db.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get updated task: %w", err)
	}
	if updated == nil {
		return nil, fmt.Errorf("task %s not found after toggle — file may have been concurrently modified", taskID)
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
