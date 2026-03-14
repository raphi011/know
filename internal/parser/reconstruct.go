package parser

import (
	"fmt"
	"strings"
)

// SectionOperation defines the type of targeted edit.
type SectionOperation string

const (
	OpReplace      SectionOperation = "replace"
	OpInsertAfter  SectionOperation = "insert_after"
	OpInsertBefore SectionOperation = "insert_before"
	OpDelete       SectionOperation = "delete"
	OpAppend       SectionOperation = "append"
)

// SectionAddress identifies a section by heading text and a disambiguation
// index (0-based) for when multiple sections share the same heading.
type SectionAddress struct {
	Heading  string // "" targets the preamble (level-0 section)
	Position int    // 0 = first occurrence (default)
}

// SectionEdit describes a targeted edit to apply to a document.
type SectionEdit struct {
	Operation  SectionOperation
	Target     *SectionAddress // nil only for OpAppend
	Content    string          // new body content (required for replace/insert/append)
	NewHeading string          // heading text for insert/append
	NewLevel   int             // heading level 1-6 for insert/append
}

// SectionInfo describes a section for the outline view.
type SectionInfo struct {
	Index    int
	Level    int
	Heading  string
	Position int // disambiguation index among same-heading sections
}

// SectionEditArgs holds the raw JSON-deserialized arguments for building a
// SectionEdit. Used by both the executor and agent service to avoid duplication.
type SectionEditArgs struct {
	Operation  string
	Heading    *string
	Position   *int
	Content    *string
	NewHeading *string
	NewLevel   *int
}

// BuildSectionEdit converts raw JSON arguments into a validated SectionEdit.
func BuildSectionEdit(args SectionEditArgs) SectionEdit {
	edit := SectionEdit{
		Operation: SectionOperation(args.Operation),
	}

	if args.Content != nil {
		edit.Content = *args.Content
	}
	if args.NewHeading != nil {
		edit.NewHeading = *args.NewHeading
	}
	if args.NewLevel != nil {
		edit.NewLevel = *args.NewLevel
	} else if edit.Operation == OpInsertAfter || edit.Operation == OpInsertBefore || edit.Operation == OpAppend {
		edit.NewLevel = 2
	}

	if args.Heading != nil {
		pos := 0
		if args.Position != nil {
			pos = *args.Position
		}
		edit.Target = &SectionAddress{
			Heading:  *args.Heading,
			Position: pos,
		}
	}

	return edit
}

// SectionOutline returns a list of sections with disambiguation positions.
func SectionOutline(doc *MarkdownDoc) []SectionInfo {
	counts := map[string]int{}
	infos := make([]SectionInfo, len(doc.Sections))

	for i, s := range doc.Sections {
		key := s.Heading // "" for preamble
		pos := counts[key]
		counts[key]++

		infos[i] = SectionInfo{
			Index:    i,
			Level:    s.Level,
			Heading:  s.Heading,
			Position: pos,
		}
	}

	return infos
}

// ApplySectionEdit applies a targeted edit to raw markdown content (including
// frontmatter) and returns the new full document content.
func ApplySectionEdit(rawContent string, edit SectionEdit) (string, error) {
	if err := validateEdit(edit); err != nil {
		return "", err
	}

	// Parse to get sections with line numbers
	doc, err := ParseMarkdown(rawContent)
	if err != nil {
		return "", fmt.Errorf("parse document: %w", err)
	}

	// Determine the frontmatter prefix. doc.Content is the content after
	// frontmatter. We find where it starts in rawContent to extract the prefix.
	fmPrefix, err := extractFrontmatterPrefix(rawContent, doc.Content)
	if err != nil {
		return "", fmt.Errorf("extract frontmatter: %w", err)
	}

	// Work with lines of the content-after-frontmatter
	lines := strings.Split(doc.Content, "\n")

	// Compute line ranges for each section
	ranges := sectionLineRanges(lines, doc.Sections)

	switch edit.Operation {
	case OpAppend:
		return applyAppend(fmPrefix, lines, edit), nil
	case OpReplace:
		return applyReplace(fmPrefix, lines, ranges, doc.Sections, edit)
	case OpDelete:
		return applyDelete(fmPrefix, lines, ranges, doc.Sections, edit)
	case OpInsertAfter:
		return applyInsert(fmPrefix, lines, ranges, doc.Sections, edit, false)
	case OpInsertBefore:
		return applyInsert(fmPrefix, lines, ranges, doc.Sections, edit, true)
	default:
		return "", fmt.Errorf("unknown operation: %s", edit.Operation)
	}
}

func validateEdit(edit SectionEdit) error {
	switch edit.Operation {
	case OpReplace:
		if edit.Target == nil {
			return fmt.Errorf("target is required for replace")
		}
	case OpInsertAfter, OpInsertBefore:
		if edit.Target == nil {
			return fmt.Errorf("target is required for %s", edit.Operation)
		}
		if edit.NewLevel < 1 || edit.NewLevel > 6 {
			return fmt.Errorf("new_level must be 1-6, got %d", edit.NewLevel)
		}
		if edit.NewHeading == "" {
			return fmt.Errorf("new_heading is required for %s", edit.Operation)
		}
		if edit.Content == "" {
			return fmt.Errorf("content is required for %s", edit.Operation)
		}
	case OpDelete:
		if edit.Target == nil {
			return fmt.Errorf("target is required for delete")
		}
	case OpAppend:
		if edit.NewLevel < 1 || edit.NewLevel > 6 {
			return fmt.Errorf("new_level must be 1-6, got %d", edit.NewLevel)
		}
		if edit.NewHeading == "" {
			return fmt.Errorf("new_heading is required for append")
		}
		if edit.Content == "" {
			return fmt.Errorf("content is required for append")
		}
	default:
		return fmt.Errorf("unknown operation: %s", edit.Operation)
	}
	return nil
}

// extractFrontmatterPrefix returns the frontmatter portion of rawContent
// (everything before doc.Content), preserving exact formatting.
// contentAfterFM is always a suffix of rawContent, so we slice by length.
func extractFrontmatterPrefix(rawContent, contentAfterFM string) (string, error) {
	if len(contentAfterFM) >= len(rawContent) {
		return "", nil
	}
	prefix := rawContent[:len(rawContent)-len(contentAfterFM)]
	// Verify the invariant: rawContent = prefix + contentAfterFM
	if !strings.HasSuffix(rawContent, contentAfterFM) {
		return "", fmt.Errorf("content is not a suffix of raw content; frontmatter would be lost")
	}
	return prefix, nil
}

// lineRange is an inclusive-start, exclusive-end range of 0-based line indices.
type lineRange struct {
	start int
	end   int
}

// sectionLineRanges computes the [start, end) line range for each section.
// Section.Start is 1-based (relative to content-after-frontmatter).
// The preamble (Level=0) has Start=0, which we treat as starting at line 0.
func sectionLineRanges(lines []string, sections []Section) []lineRange {
	totalLines := len(lines)
	ranges := make([]lineRange, len(sections))

	for i, s := range sections {
		startLine := 0
		if s.Level > 0 && s.Start > 0 {
			startLine = s.Start - 1 // convert 1-based to 0-based
		} else if i > 0 {
			// Non-first preamble shouldn't happen, but handle gracefully
			startLine = ranges[i-1].end
		}

		endLine := totalLines
		if i+1 < len(sections) {
			next := sections[i+1]
			if next.Level > 0 && next.Start > 0 {
				endLine = next.Start - 1
			}
		}

		ranges[i] = lineRange{start: startLine, end: endLine}
	}

	return ranges
}

// deepSectionEnd returns the end line (exclusive) of a section including all
// its child sections (subsections with strictly higher level numbers).
// For the preamble (level 0), returns the shallow range since all headings
// are children of the preamble in a markdown sense but we want to only
// target the preamble text itself.
func deepSectionEnd(sections []Section, ranges []lineRange, idx int) int {
	level := sections[idx].Level
	if level == 0 {
		return ranges[idx].end
	}
	for j := idx + 1; j < len(sections); j++ {
		if sections[j].Level > 0 && sections[j].Level <= level {
			return ranges[j].start
		}
	}
	return ranges[len(ranges)-1].end
}

// findSection resolves a SectionAddress to a section index.
func findSection(sections []Section, addr SectionAddress) (int, error) {
	count := 0
	for i, s := range sections {
		match := false
		if addr.Heading == "" && s.Level == 0 {
			// Skip empty preamble sections — the parser always creates a
			// Level=0 section, but if the doc starts with a heading it has
			// no actual preamble content. Treating it as a match would
			// prepend content before the first heading, corrupting the doc.
			if s.Content == "" {
				continue
			}
			match = true
		} else if s.Heading == addr.Heading {
			match = true
		}

		if match {
			if count == addr.Position {
				return i, nil
			}
			count++
		}
	}

	if count == 0 {
		if addr.Heading == "" {
			return -1, fmt.Errorf("preamble section not found")
		}
		return -1, fmt.Errorf("section not found: %q", addr.Heading)
	}
	return -1, fmt.Errorf("section %q position %d out of range (found %d)", addr.Heading, addr.Position, count)
}

func reassemble(fmPrefix string, lines []string) string {
	content := strings.Join(lines, "\n")
	return fmPrefix + content
}

func buildSectionText(edit SectionEdit) string {
	var sb strings.Builder
	sb.WriteString(strings.Repeat("#", edit.NewLevel))
	sb.WriteString(" ")
	sb.WriteString(edit.NewHeading)
	if edit.Content != "" {
		sb.WriteString("\n\n")
		sb.WriteString(edit.Content)
	}
	return sb.String()
}

func applyAppend(fmPrefix string, lines []string, edit SectionEdit) string {
	newSection := buildSectionText(edit)
	// Ensure blank line before new section
	newLines := make([]string, len(lines))
	copy(newLines, lines)

	// Trim trailing empty lines, then add blank separator
	for len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) == "" {
		newLines = newLines[:len(newLines)-1]
	}
	newLines = append(newLines, "")
	newLines = append(newLines, strings.Split(newSection, "\n")...)
	newLines = append(newLines, "") // trailing newline

	return reassemble(fmPrefix, newLines)
}

func applyReplace(fmPrefix string, lines []string, ranges []lineRange, sections []Section, edit SectionEdit) (string, error) {
	idx, err := findSection(sections, *edit.Target)
	if err != nil {
		return "", err
	}

	r := ranges[idx]
	s := sections[idx]
	// Use deep end to include all child sections in the replacement
	deepEnd := deepSectionEnd(sections, ranges, idx)

	var newLines []string
	// Lines before this section
	newLines = append(newLines, lines[:r.start]...)

	if s.Level > 0 {
		// Preserve the heading line
		newLines = append(newLines, lines[r.start])
		// Add the new content body
		if edit.Content != "" {
			newLines = append(newLines, "")
			newLines = append(newLines, strings.Split(edit.Content, "\n")...)
		}
	} else {
		// Preamble: replace all content
		if edit.Content != "" {
			newLines = append(newLines, strings.Split(edit.Content, "\n")...)
		}
	}

	// Ensure blank line before next section if there is one
	if deepEnd < len(lines) {
		// Add separator blank line if content doesn't end with one
		if len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) != "" {
			newLines = append(newLines, "")
		}
	}

	// Lines after this section (using deep end to skip child sections)
	newLines = append(newLines, lines[deepEnd:]...)

	return reassemble(fmPrefix, newLines), nil
}

func applyDelete(fmPrefix string, lines []string, ranges []lineRange, sections []Section, edit SectionEdit) (string, error) {
	idx, err := findSection(sections, *edit.Target)
	if err != nil {
		return "", err
	}

	r := ranges[idx]

	var newLines []string
	newLines = append(newLines, lines[:r.start]...)
	newLines = append(newLines, lines[r.end:]...)

	return reassemble(fmPrefix, newLines), nil
}

func applyInsert(fmPrefix string, lines []string, ranges []lineRange, sections []Section, edit SectionEdit, before bool) (string, error) {
	idx, err := findSection(sections, *edit.Target)
	if err != nil {
		return "", err
	}

	r := ranges[idx]
	newSection := buildSectionText(edit)
	insertLines := strings.Split(newSection, "\n")

	var newLines []string
	if before {
		newLines = append(newLines, lines[:r.start]...)
		newLines = append(newLines, insertLines...)
		newLines = append(newLines, "")
		newLines = append(newLines, lines[r.start:]...)
	} else {
		newLines = append(newLines, lines[:r.end]...)
		// Add blank separator before new section
		if len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) != "" {
			newLines = append(newLines, "")
		}
		newLines = append(newLines, insertLines...)
		newLines = append(newLines, "")
		newLines = append(newLines, lines[r.end:]...)
	}

	return reassemble(fmPrefix, newLines), nil
}
