package parser

import (
	"regexp"
	"strings"

	"github.com/raphi011/know/internal/models"
)

// CleanTaskText removes due:YYYY-MM-DD dates and #labels from raw task text,
// then collapses whitespace. Used for content-hash-based identity.
func CleanTaskText(rawText string) string {
	cleaned := dueDateRegex.ReplaceAllString(rawText, "")
	cleaned = taskLabelRegex.ReplaceAllString(cleaned, "")
	return strings.Join(strings.Fields(cleaned), " ")
}

var (
	// dueDateRegex extracts `due:YYYY-MM-DD` from task text.
	dueDateRegex = regexp.MustCompile(`\bdue:(\d{4}-\d{2}-\d{2})\b`)

	// taskLabelRegex matches inline #labels in task text.
	taskLabelRegex = regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_-]*)`)
)

// ExtractedTask represents a single task extracted from markdown content.
type ExtractedTask struct {
	Status      models.TaskStatus // "open" or "done"
	RawLine     string            // original line (trimmed)
	Text        string            // cleaned text (no checkbox marker, no due:, no #labels)
	Labels      []string          // inline #labels
	DueDate     *string           // from due:YYYY-MM-DD
	LineNumber  int               // 1-based
	HeadingPath string            // section context, e.g. "Daily Note > Tasks"
	ContentHash string            // SHA256 of cleaned text for stable identity
}
