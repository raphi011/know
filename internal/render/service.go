// Package render transforms raw markdown content into enhanced markdown
// by resolving wiki-links and executing query blocks. Used for API and
// agent-facing reads; editors (WebDAV/NFS/SFTP) get raw content.
package render

import (
	"context"
	"fmt"
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

// Enhance transforms raw markdown content into enhanced markdown.
// fileID is needed to look up this file's resolved wiki-links.
// Returns the original content unchanged if no transformations apply.
func (s *Service) Enhance(ctx context.Context, vaultID, fileID, content string) (string, error) {
	if content == "" {
		return content, nil
	}

	logger := logutil.FromCtx(ctx)

	// Collect all replacements (offset-based) to apply in one pass.
	type replacement struct {
		start int
		end   int
		text  string
	}
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

	// 2. Execute query blocks.
	queryBlocks := parser.ExtractQueryBlocks(content)
	for _, qb := range queryBlocks {
		if qb.Error != "" {
			continue // parse error, leave block as-is
		}

		rendered, err := s.executeQueryBlock(ctx, vaultID, qb)
		if err != nil {
			logger.Warn("render: query block execution failed", "error", err)
			continue
		}

		// Find the end of the fenced code block (closing ```).
		blockEnd := findFencedBlockEnd(content, qb.Index)
		if blockEnd < 0 {
			continue
		}

		replacements = append(replacements, replacement{
			start: qb.Index,
			end:   blockEnd,
			text:  rendered,
		})
	}

	if len(replacements) == 0 {
		return content, nil
	}

	// Sort replacements by start offset descending (apply back-to-front
	// to preserve earlier offsets).
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

	return result, nil
}

// executeQueryBlock runs a parsed query block against the DB and renders
// results as markdown.
func (s *Service) executeQueryBlock(ctx context.Context, vaultID string, qb parser.QueryBlock) (string, error) {
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

	// Apply WHERE conditions.
	for _, cond := range qb.Conditions {
		switch {
		case cond.Field == "labels" && cond.Op == parser.OpContain:
			filter.Labels = append(filter.Labels, cond.Value)
		case cond.Field == "doc_type" && cond.Op == parser.OpEqual:
			filter.DocType = &cond.Value
		case cond.Field == "mime_type" && cond.Op == parser.OpEqual:
			filter.MimeType = &cond.Value
		}
		// Other conditions (CONTAINS on arbitrary fields) are not supported
		// by ListFilesFilter — we'd need post-filtering. Skip for now.
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
		return renderTable(files, qb.ShowFields), nil
	}
	return renderList(files, qb.ShowFields), nil
}

// renderList renders files as a markdown bullet list.
func renderList(files []models.File, showFields []string) string {
	var sb strings.Builder
	for _, f := range files {
		title := fileFieldValue(&f, "title")
		path := fileFieldValue(&f, "path")
		sb.WriteString(fmt.Sprintf("- [%s](%s)\n", title, path))
	}
	return sb.String()
}

// renderTable renders files as a markdown table.
func renderTable(files []models.File, showFields []string) string {
	var sb strings.Builder

	// Header
	sb.WriteString("|")
	for _, field := range showFields {
		sb.WriteString(fmt.Sprintf(" %s |", field))
	}
	sb.WriteString("\n|")
	for range showFields {
		sb.WriteString("---|")
	}
	sb.WriteString("\n")

	// Rows
	for _, f := range files {
		sb.WriteString("|")
		for _, field := range showFields {
			sb.WriteString(fmt.Sprintf(" %s |", fileFieldValue(&f, field)))
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
		return db.OrderByPathAsc // no created_at ASC, fallback
	default:
		return db.OrderByPathAsc
	}
}

// findFencedBlockEnd finds the byte offset past the closing ``` line
// of a fenced code block starting at startOffset.
func findFencedBlockEnd(content string, startOffset int) int {
	// Skip the opening ``` line.
	rest := content[startOffset:]
	firstNewline := strings.IndexByte(rest, '\n')
	if firstNewline < 0 {
		return -1
	}

	// Find the closing ``` line.
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

		if strings.TrimSpace(line) == "```" {
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
