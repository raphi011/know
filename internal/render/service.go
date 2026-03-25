// Package render transforms raw markdown content into enhanced markdown
// by resolving wiki-links and executing query blocks. Used for API and
// agent-facing reads; editors (WebDAV/NFS/SFTP) get raw content.
package render

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

// Service enhances raw markdown content with resolved wiki-links and
// executed query blocks.
type Service struct {
	db *db.Client
}

// NewService creates a new render service.
func NewService(dbClient *db.Client) *Service {
	return &Service{db: dbClient}
}

// wikiLinkPattern matches [[target]] or [[target|alias]] in markdown.
var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

type replacement struct {
	start int
	end   int
	text  string
}

// Enhance transforms raw markdown content into enhanced markdown.
// fileID is needed to look up this file's resolved wiki-links.
// Returns enhanced content and the resolved wiki-links (so callers don't
// need to fetch them again for the API response).
func (s *Service) Enhance(ctx context.Context, vaultID, fileID, content string) (string, []db.WikiLinkWithTarget, error) {
	if content == "" {
		return content, nil, nil
	}

	logger := logutil.FromCtx(ctx)

	var replacements []replacement

	// 1. Resolve wiki-links.
	wikiLinks, err := s.db.GetWikiLinksWithTargetInfo(ctx, fileID)
	if err != nil {
		logger.Warn("render: failed to get wiki links", "file_id", fileID, "error", err)
	} else if len(wikiLinks) > 0 {
		// Build lookup: raw_target → resolved info.
		linkMap := make(map[string]db.WikiLinkWithTarget, len(wikiLinks))
		for _, wl := range wikiLinks {
			linkMap[wl.RawTarget] = wl
		}

		// Find all [[...]] in content and create replacements.
		matches := wikiLinkPattern.FindAllStringSubmatchIndex(content, -1)
		for _, match := range matches {
			// match[0]:match[1] = full match [[target]] or [[target|alias]]
			// match[2]:match[3] = target group
			// match[4]:match[5] = alias group (-1 if not present)
			fullStart, fullEnd := match[0], match[1]
			target := content[match[2]:match[3]]

			wl, ok := linkMap[strings.TrimSpace(target)]
			if !ok {
				continue // unknown wiki-link, leave as-is
			}

			// Determine display text: alias if present, else resolved title, else target.
			displayText := strings.TrimSpace(target)
			if match[4] >= 0 {
				displayText = content[match[4]:match[5]]
			} else if wl.Title != nil && *wl.Title != "" {
				displayText = *wl.Title
			}

			if wl.Path != nil {
				// Resolved: replace with markdown link.
				replacements = append(replacements, replacement{
					start: fullStart,
					end:   fullEnd,
					text:  fmt.Sprintf("[%s](%s)", displayText, *wl.Path),
				})
			}
			// Dangling links (Path == nil) are left as [[target]].
		}
	}

	// 2. Execute query blocks (skip full parse if no fenced know blocks exist).
	if !strings.Contains(content, "```know") && !strings.Contains(content, "~~~know") {
		if len(replacements) == 0 {
			return content, wikiLinks, nil
		}
		return applyReplacements(content, replacements), wikiLinks, nil
	}
	queryBlocks := parser.ExtractQueryBlocks(content)
	for _, qb := range queryBlocks {
		var rendered string

		if qb.Error != "" {
			logger.Warn("render: query block parse error", "error", qb.Error)
			rendered = fmt.Sprintf("**Query error:** %s\n", qb.Error)
		} else {
			var err error
			rendered, err = s.executeQueryBlock(ctx, vaultID, qb)
			if err != nil {
				logger.Warn("render: query block execution failed", "error", err)
				rendered = fmt.Sprintf("**Query error:** %s\n", err)
			}
		}

		// Find the end of the fenced code block (closing ```).
		blockEnd := findFencedBlockEnd(content, qb.Index)
		if blockEnd < 0 {
			logger.Warn("render: unclosed fenced code block", "offset", qb.Index)
			continue
		}

		replacements = append(replacements, replacement{
			start: qb.Index,
			end:   blockEnd,
			text:  rendered,
		})
	}

	if len(replacements) == 0 {
		return content, wikiLinks, nil
	}

	return applyReplacements(content, replacements), wikiLinks, nil
}

// applyReplacements applies offset-based text replacements back-to-front.
func applyReplacements(content string, replacements []replacement) string {
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].start > replacements[j].start
	})

	result := content
	for _, r := range replacements {
		if r.start < 0 || r.end > len(result) || r.start >= r.end {
			continue
		}
		result = result[:r.start] + r.text + result[r.end:]
	}
	return result
}

// executeQueryBlock runs a parsed query block against the DB and renders
// results as markdown.
func (s *Service) executeQueryBlock(ctx context.Context, vaultID string, qb parser.QueryBlock) (string, error) {
	if qb.Format == parser.FormatTask {
		return s.executeTaskQuery(ctx, vaultID, qb)
	}
	return s.executeFileQuery(ctx, vaultID, qb)
}

func (s *Service) executeFileQuery(ctx context.Context, vaultID string, qb parser.QueryBlock) (string, error) {
	filter := db.ListFilesFilter{
		VaultID: vaultID,
		Limit:   qb.Limit,
		OrderBy: queryBlockOrderBy(qb),
	}

	if qb.Folder != nil {
		folder := *qb.Folder
		if !strings.HasSuffix(folder, "/") {
			folder += "/"
		}
		filter.Folder = &folder
	}

	for _, cond := range qb.Conditions {
		switch {
		case cond.Field == "labels" && cond.Op == parser.OpContain:
			filter.Labels = append(filter.Labels, cond.Value)
		case cond.Field == "doc_type" && cond.Op == parser.OpEqual:
			filter.DocType = &cond.Value
		case cond.Field == "mime_type" && cond.Op == parser.OpEqual:
			filter.MimeType = &cond.Value
		default:
			return "", fmt.Errorf("unsupported condition: %s %s %q", cond.Field, cond.Op, cond.Value)
		}
	}

	isNotFolder := false
	filter.IsFolder = &isNotFolder

	files, err := s.db.ListFiles(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("execute query: %w", err)
	}

	if len(files) == 0 {
		return "*No results*\n", nil
	}

	if qb.Format == parser.FormatTable {
		return renderTable(files, qb.Fields, qb.WithoutID), nil
	}
	return renderList(files, qb.Fields, qb.WithoutID), nil
}

func (s *Service) executeTaskQuery(ctx context.Context, vaultID string, qb parser.QueryBlock) (string, error) {
	filter := db.TaskFilter{
		VaultID: vaultID,
		Limit:   qb.Limit,
	}

	if qb.Folder != nil {
		filter.Folder = qb.Folder
	}

	for _, cond := range qb.Conditions {
		switch {
		case cond.Field == "status" && cond.Op == parser.OpEqual:
			status := models.TaskStatus(cond.Value)
			filter.Status = &status
		case cond.Field == "labels" && cond.Op == parser.OpContain:
			filter.Labels = append(filter.Labels, cond.Value)
		case cond.Field == "due_before" && cond.Op == parser.OpEqual:
			filter.DueBefore = &cond.Value
		case cond.Field == "due_after" && cond.Op == parser.OpEqual:
			filter.DueAfter = &cond.Value
		default:
			return "", fmt.Errorf("unsupported task condition: %s %s %q", cond.Field, cond.Op, cond.Value)
		}
	}

	tasks, err := s.db.ListTasks(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("execute task query: %w", err)
	}

	if len(tasks) == 0 {
		return "*No results*\n", nil
	}

	return renderTaskList(tasks, qb.WithoutID), nil
}

// renderList renders files as a markdown bullet list.
func renderList(files []models.File, fields []parser.ShowField, withoutID bool) string {
	var sb strings.Builder
	for _, f := range files {
		if withoutID {
			// Plain text, no link.
			text := "title"
			if len(fields) >= 1 {
				text = fields[0].Name
			}
			sb.WriteString(fmt.Sprintf("- %s\n", fileFieldValue(&f, text)))
		} else {
			// Link format: - [title](path)
			textField := "title"
			if len(fields) >= 1 {
				textField = fields[0].Name
			}
			text := fileFieldValue(&f, textField)
			link := fileFieldValue(&f, "path")
			sb.WriteString(fmt.Sprintf("- [%s](%s)\n", text, link))

			// Extra field shown after the link if provided.
			if len(fields) >= 2 {
				extra := fileFieldValue(&f, fields[1].Name)
				if extra != "" {
					sb.WriteString(fmt.Sprintf("  %s\n", extra))
				}
			}
		}
	}
	return sb.String()
}

// renderTable renders files as a markdown table.
func renderTable(files []models.File, fields []parser.ShowField, withoutID bool) string {
	// Default fields if none specified.
	if len(fields) == 0 {
		fields = []parser.ShowField{
			{Name: "title"},
			{Name: "path"},
		}
	}

	var sb strings.Builder

	// Header: auto-prepend File column unless WITHOUT ID.
	sb.WriteString("|")
	if !withoutID {
		sb.WriteString(" File |")
	}
	for _, field := range fields {
		header := field.Name
		if field.Alias != "" {
			header = field.Alias
		}
		sb.WriteString(fmt.Sprintf(" %s |", header))
	}
	sb.WriteString("\n|")
	if !withoutID {
		sb.WriteString("---|")
	}
	for range fields {
		sb.WriteString("---|")
	}
	sb.WriteString("\n")

	// Rows
	for _, f := range files {
		sb.WriteString("|")
		if !withoutID {
			sb.WriteString(fmt.Sprintf(" [%s](%s) |", f.Title, f.Path))
		}
		for _, field := range fields {
			sb.WriteString(fmt.Sprintf(" %s |", fileFieldValue(&f, field.Name)))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderTaskList renders tasks as markdown checkboxes with embedded task IDs
// as HTML comments for programmatic toggling.
func renderTaskList(tasks []models.TaskWithDoc, withoutID bool) string {
	var sb strings.Builder
	for _, t := range tasks {
		checkbox := "[ ]"
		if t.Status == models.TaskStatusDone {
			checkbox = "[x]"
		}

		sb.WriteString(fmt.Sprintf("- %s %s", checkbox, t.Text))

		if t.DueDate != nil {
			sb.WriteString(fmt.Sprintf(" (due: %s)", *t.DueDate))
		}

		if !withoutID {
			sb.WriteString(fmt.Sprintf(" — *%s*", t.DocPath))
		}

		if taskID, err := models.RecordIDString(t.ID); err != nil {
			// Zero-value RecordID is expected for tasks without a persisted ID.
			// Non-zero IDs that fail extraction indicate a bug in ID handling.
			slog.Debug("render: failed to extract task ID", "error", err)
		} else if taskID != "" {
			sb.WriteString(fmt.Sprintf("<!-- task:%s -->", taskID))
		}

		sb.WriteString("\n")
	}
	return sb.String()
}

func fileFieldValue(f *models.File, field string) string {
	switch field {
	case "title":
		return f.Title
	case "path":
		return f.Path
	case "mime_type":
		return f.MimeType
	case "doc_type":
		if f.DocType != nil {
			return *f.DocType
		}
		return ""
	case "labels":
		return strings.Join(f.Labels, ", ")
	default:
		if f.Metadata != nil {
			if v, ok := f.Metadata[field]; ok {
				return fmt.Sprint(v)
			}
		}
		return ""
	}
}

// queryBlockOrderBy maps query block sort config to a DB order by clause.
func queryBlockOrderBy(qb parser.QueryBlock) db.FileOrderBy {
	switch qb.SortField {
	case "updated_at":
		if qb.SortDesc {
			return db.OrderByUpdatedAtDesc
		}
		return db.OrderByUpdatedAtAsc
	case "created_at":
		if qb.SortDesc {
			return db.OrderByCreatedAtDesc
		}
		return db.OrderByPathAsc // DB only supports created_at DESC; ASC falls back to path ordering
	default:
		return db.OrderByPathAsc
	}
}

// findFencedBlockEnd finds the byte offset past the closing fence line
// of a fenced code block starting at startOffset.
// Supports both ``` and ~~~ fence styles.
func findFencedBlockEnd(content string, startOffset int) int {
	// Skip the opening fence line.
	rest := content[startOffset:]
	firstNewline := strings.IndexByte(rest, '\n')
	if firstNewline < 0 {
		return -1
	}

	// Detect which fence style was used for the opening.
	openingLine := rest[:firstNewline]
	openingFence := "```"
	if strings.Contains(openingLine, "~~~") {
		openingFence = "~~~"
	}

	// Find the matching closing fence line.
	remaining := rest[firstNewline+1:]
	offset := startOffset + firstNewline + 1
	for {
		lineEnd := strings.IndexByte(remaining, '\n')
		var line string
		if lineEnd < 0 {
			line = remaining
		} else {
			line = remaining[:lineEnd]
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == openingFence {
			if lineEnd < 0 {
				return offset + len(line)
			}
			return offset + lineEnd + 1 // include the newline
		}

		if lineEnd < 0 {
			return -1 // no closing fence found
		}
		offset += lineEnd + 1
		remaining = remaining[lineEnd+1:]
	}
}
