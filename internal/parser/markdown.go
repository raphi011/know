// Package parser provides Markdown parsing and content extraction.
package parser

import (
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"

	"github.com/raphi011/know/internal/models"
	wlast "go.abhg.dev/goldmark/wikilink"
)

// mentionRegex matches @mentions. Applied only to text nodes (not code blocks).
var mentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)

// MarkdownDoc represents a parsed Markdown document.
type MarkdownDoc struct {
	// Frontmatter metadata (from YAML)
	Frontmatter map[string]any

	// Title extracted from first h1 or frontmatter
	Title string

	// Main content (after frontmatter)
	Content string

	// Structured content by heading
	Sections []Section

	// Tasks extracted from checkbox list items
	Tasks []ExtractedTask

	// Wiki-links extracted from [[link]] syntax
	WikiLinks []string

	// @mentions extracted from text (not code blocks)
	Mentions []string

	// Query blocks from ```know fenced code blocks
	QueryBlocks []QueryBlock

	// Inline #labels found outside code blocks
	InlineLabels []string
}

// Section represents a heading and its content.
type Section struct {
	Level     int    // 1-6 for h1-h6, 0 for preamble
	Heading   string // The heading text
	Path      string // Full path like "## Setup > ### Install"
	Content   string // Content under this heading
	Start     int    // Line number where section starts
	CodeBlock bool   // True if section is primarily a code block (treat as atomic)
}

// ParseMarkdown parses a Markdown document into structured form using a single
// goldmark AST parse and walk. Extracts sections, tasks, wiki-links, mentions,
// query blocks, and inline labels in one pass.
//
// Goldmark's parser always produces an AST (even for malformed input), so this
// function cannot fail. Malformed frontmatter is logged and skipped.
func ParseMarkdown(content string) *MarkdownDoc {
	doc := &MarkdownDoc{
		Frontmatter: make(map[string]any),
	}

	// Extract frontmatter first, then parse the content-after-frontmatter.
	// This ensures all AST line numbers are relative to doc.Content, which is
	// what reconstruct.go expects when splitting doc.Content into lines.
	doc.Content = contentAfterFrontmatter(content)

	// Parse content-after-frontmatter into AST. The frontmatter return is
	// discarded because content has already been stripped of frontmatter.
	root, source, _ := parseAST(doc.Content)

	// Parse frontmatter from the original content using goldmark-meta.
	if strings.HasPrefix(content, "---\n") {
		_, _, fm := parseAST(content)
		if fm != nil {
			doc.Frontmatter = fm
		} else {
			slog.Debug("frontmatter block detected but parsing returned no metadata")
		}
	}

	// Extract title from frontmatter or first h1
	doc.Title = extractTitleFromFrontmatter(doc.Frontmatter)

	// Single-pass AST walk on content-after-frontmatter
	w := &astWalker{source: source}
	w.walk(root)

	// If no frontmatter title, use first h1 from AST
	if doc.Title == "" {
		doc.Title = w.firstH1
	}

	doc.Sections = w.sections
	doc.Tasks = w.tasks
	doc.QueryBlocks = w.queryBlocks

	// Deduplicate wiki-links
	doc.WikiLinks = dedup(w.wikiLinks)

	// Extract mentions and labels from collected text (excludes code blocks)
	doc.Mentions = extractMentions(w.textContent.String())
	doc.InlineLabels = extractInlineLabels(w.textContent.String())

	return doc
}

// contentAfterFrontmatter strips the YAML frontmatter block from content.
// If the opening --- has no matching closing ---, the opener is stripped to
// prevent goldmark-meta from consuming the entire document as a YAML block.
func contentAfterFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	endIdx := strings.Index(content[4:], "\n---")
	if endIdx <= 0 {
		// Unclosed frontmatter: strip the opening "---\n" so goldmark-meta
		// doesn't greedily consume all content as an invalid YAML block.
		slog.Debug("content starts with --- but no closing frontmatter delimiter found")
		return content[4:]
	}
	return strings.TrimPrefix(content[4+endIdx+4:], "\n")
}

// extractTitleFromFrontmatter checks frontmatter for title/name fields.
func extractTitleFromFrontmatter(fm map[string]any) string {
	if title, ok := fm["title"].(string); ok && title != "" {
		return title
	}
	if name, ok := fm["name"].(string); ok && name != "" {
		return name
	}
	return ""
}

// astWalker collects data from a single pass over the goldmark AST.
type astWalker struct {
	source []byte

	// Section tracking
	sections       []Section
	currentPath    []string
	currentLevels  []int
	contentBuilder strings.Builder
	currentSection *Section
	codeBlockChars int

	// Task tracking
	tasks        []ExtractedTask
	headingStack []taskHeading
	headingPath  string

	// Title
	firstH1 string

	// Wiki-links
	wikiLinks []string

	// Query blocks
	queryBlocks []QueryBlock

	// Text content outside code blocks (for mention/label extraction)
	textContent strings.Builder
}

type taskHeading struct {
	level int
	text  string
}

func (w *astWalker) walk(root ast.Node) {
	// Start with a preamble section to capture pre-heading content
	w.currentSection = &Section{Level: 0}

	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		w.visitBlock(node)
	}

	// Flush last section
	w.flushSection()
}

func (w *astWalker) flushSection() {
	if w.currentSection == nil {
		return
	}
	w.currentSection.Content = strings.TrimSpace(w.contentBuilder.String())
	trimmedLen := len(w.currentSection.Content)
	if trimmedLen > 0 && w.codeBlockChars > trimmedLen/2 {
		w.currentSection.CodeBlock = true
	}
	w.sections = append(w.sections, *w.currentSection)
	w.contentBuilder.Reset()
	w.codeBlockChars = 0
}

func (w *astWalker) visitBlock(node ast.Node) {
	switch n := node.(type) {
	case *ast.Heading:
		w.visitHeading(n)

	case *ast.List:
		w.visitList(n)
		// Also collect list content for sections
		nodeText := nodeContent(w.source, node)
		w.appendSectionContent(nodeText)

	case *ast.FencedCodeBlock:
		w.visitFencedCodeBlock(n)

	case *ast.CodeBlock:
		nodeText := codeBlockContent(w.source, n)
		w.appendSectionContent(nodeText)
		w.codeBlockChars += len(nodeText)

	default:
		nodeText := nodeContent(w.source, node)
		w.appendSectionContent(nodeText)
		// Collect text for mention/label extraction (not code blocks)
		w.collectText(node)
	}
}

func (w *astWalker) appendSectionContent(text string) {
	if text == "" {
		return
	}
	if w.contentBuilder.Len() > 0 {
		w.contentBuilder.WriteString("\n\n")
	}
	w.contentBuilder.WriteString(text)
}

func (w *astWalker) visitHeading(n *ast.Heading) {
	w.flushSection()

	heading := inlineText(w.source, n)
	level := n.Level

	// Track first h1 for title
	if w.firstH1 == "" && level == 1 {
		w.firstH1 = heading
	}

	// Update section path hierarchy
	for len(w.currentLevels) > 0 && w.currentLevels[len(w.currentLevels)-1] >= level {
		w.currentPath = w.currentPath[:len(w.currentPath)-1]
		w.currentLevels = w.currentLevels[:len(w.currentLevels)-1]
	}
	w.currentPath = append(w.currentPath, strings.Repeat("#", level)+" "+heading)
	w.currentLevels = append(w.currentLevels, level)

	var startLine int
	if lines := n.Lines(); lines.Len() > 0 {
		startLine = lineNumber(w.source, lines.At(0).Start)
	}

	w.currentSection = &Section{
		Level:   level,
		Heading: heading,
		Path:    strings.Join(w.currentPath, " > "),
		Start:   startLine,
	}

	// Update task heading stack
	for len(w.headingStack) > 0 && w.headingStack[len(w.headingStack)-1].level >= level {
		w.headingStack = w.headingStack[:len(w.headingStack)-1]
	}
	w.headingStack = append(w.headingStack, taskHeading{level: level, text: heading})

	parts := make([]string, len(w.headingStack))
	for i, h := range w.headingStack {
		parts[i] = h.text
	}
	w.headingPath = strings.Join(parts, " > ")

	// Collect heading text for mentions/labels and wiki-links
	w.collectText(n)
}

func (w *astWalker) visitList(n *ast.List) {
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		listItem, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}

		// Check for task checkbox
		checkbox := findCheckbox(listItem)
		if checkbox != nil {
			w.extractTask(listItem, checkbox)
		}

		// Collect text from list items (for mentions/labels)
		w.collectText(listItem)

		// Recurse into nested lists
		for child := listItem.FirstChild(); child != nil; child = child.NextSibling() {
			if nestedList, ok := child.(*ast.List); ok {
				w.visitList(nestedList)
			}
		}
	}
}

func (w *astWalker) extractTask(listItem *ast.ListItem, checkbox *east.TaskCheckBox) {
	// Get the line number from the list item
	var lineNum int
	if lines := listItem.Lines(); lines.Len() > 0 {
		lineNum = lineNumber(w.source, lines.At(0).Start)
	} else if listItem.ChildCount() > 0 {
		// ListItem may not have direct lines; check first child (paragraph)
		firstChild := listItem.FirstChild()
		if lines := firstChild.Lines(); lines.Len() > 0 {
			lineNum = lineNumber(w.source, lines.At(0).Start)
		}
	}

	// Reconstruct the raw line from source
	rawLine := extractRawLine(w.source, lineNum)

	// Get text after the checkbox (the task description)
	rawText := listItemText(w.source, listItem)

	status := models.TaskStatusOpen
	if checkbox.IsChecked {
		status = models.TaskStatusDone
	}

	// Extract due date
	var dueDate *string
	if dm := dueDateRegex.FindStringSubmatch(rawText); dm != nil {
		if _, err := time.Parse("2006-01-02", dm[1]); err == nil {
			dueDate = &dm[1]
		} else {
			slog.Debug("task has invalid due date", "date", dm[1], "error", err)
		}
	}

	// Extract inline labels
	var labels []string
	seen := make(map[string]bool)
	for _, lm := range taskLabelRegex.FindAllStringSubmatch(rawText, -1) {
		lower := strings.ToLower(lm[1])
		if !seen[lower] {
			seen[lower] = true
			labels = append(labels, lower)
		}
	}

	cleaned := CleanTaskText(rawText)

	w.tasks = append(w.tasks, ExtractedTask{
		Status:      status,
		RawLine:     strings.TrimSpace(rawLine),
		Text:        cleaned,
		Labels:      labels,
		DueDate:     dueDate,
		LineNumber:  lineNum,
		HeadingPath: w.headingPath,
		ContentHash: models.ContentHash(cleaned),
	})
}

func (w *astWalker) visitFencedCodeBlock(n *ast.FencedCodeBlock) {
	lang := string(n.Language(w.source))
	nodeText := fencedCodeBlockContent(w.source, n)

	// Track for sections
	w.appendSectionContent(nodeText)
	w.codeBlockChars += len(nodeText)

	// Check for ```know query blocks
	if lang == "know" {
		var rawQuery strings.Builder
		lines := n.Lines()
		for i := range lines.Len() {
			line := lines.At(i)
			rawQuery.Write(line.Value(w.source))
		}

		// Compute byte offset of the opening ``` in source
		var offset int
		if lines.Len() > 0 {
			// The fence line starts before the first content line.
			// Walk backwards from the first line's start to find the fence.
			firstLineStart := lines.At(0).Start
			offset = findFenceStart(w.source, firstLineStart)
		}

		block := parseQueryBlock(rawQuery.String())
		block.Index = offset
		block.RawQuery = rawQuery.String()
		w.queryBlocks = append(w.queryBlocks, block)
	}
}

// collectText extracts text and wiki-links from non-code-block nodes.
// Skips nested lists to avoid double-counting when visitList recurses.
func (w *astWalker) collectText(node ast.Node) {
	// ast.Walk only returns errors from the callback; ours always returns nil.
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.List:
			// Skip nested lists — visitList handles recursion separately
			// to avoid double-counting text from nested list items.
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			w.textContent.Write(t.Value(w.source))
			w.textContent.WriteByte(' ')
		case *wlast.Node:
			target := strings.TrimSpace(string(t.Target))
			if target != "" {
				w.wikiLinks = append(w.wikiLinks, target)
			}
		}
		return ast.WalkContinue, nil
	})
}

// findCheckbox finds a TaskCheckBox inline node within a list item.
func findCheckbox(listItem *ast.ListItem) *east.TaskCheckBox {
	var checkbox *east.TaskCheckBox
	// ast.Walk only returns errors from the callback; ours always returns nil.
	_ = ast.Walk(listItem, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if cb, ok := n.(*east.TaskCheckBox); ok {
			checkbox = cb
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return checkbox
}

// listItemText extracts the text content of a list item (after the checkbox marker).
func listItemText(source []byte, listItem *ast.ListItem) string {
	var sb strings.Builder
	for child := listItem.FirstChild(); child != nil; child = child.NextSibling() {
		// Skip nested lists
		if _, ok := child.(*ast.List); ok {
			continue
		}
		collectInlineText(source, child, &sb)
	}
	return strings.TrimSpace(sb.String())
}

// collectInlineText recursively collects text from inline nodes, skipping TaskCheckBox.
func collectInlineText(source []byte, node ast.Node, sb *strings.Builder) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *east.TaskCheckBox:
			continue
		case *wlast.Node:
			// Wiki-link: use the target as text
			sb.Write(n.Target)
		case *ast.Text:
			sb.Write(n.Value(source))
			if n.SoftLineBreak() {
				sb.WriteByte(' ')
			}
		default:
			collectInlineText(source, child, sb)
		}
	}
}

// extractRawLine extracts the raw source line at the given 1-based line number.
func extractRawLine(source []byte, lineNum int) string {
	if lineNum <= 0 {
		return ""
	}
	current := 1
	start := -1
	for i, b := range source {
		if current == lineNum {
			start = i
			break
		}
		if b == '\n' {
			current++
		}
	}
	if start < 0 {
		// lineNum exceeds total lines in source
		return ""
	}
	end := start
	for end < len(source) && source[end] != '\n' {
		end++
	}
	return string(source[start:end])
}

// findFenceStart walks backwards from a content line to find the opening ``` offset.
func findFenceStart(source []byte, contentStart int) int {
	// Go back to find the start of the fence line (```know\n)
	i := contentStart - 1
	// Skip past the \n
	for i >= 0 && source[i] == '\n' {
		i--
	}
	// Find the start of that line
	for i > 0 && source[i-1] != '\n' {
		i--
	}
	if i < 0 {
		i = 0
	}
	return i
}

// extractMentions finds @mentions in text content.
func extractMentions(text string) []string {
	matches := mentionRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var mentions []string
	for _, match := range matches {
		mention := strings.ToLower(match[1])
		if !seen[mention] {
			seen[mention] = true
			mentions = append(mentions, mention)
		}
	}
	return mentions
}

// extractInlineLabels finds #labels in text content.
func extractInlineLabels(text string) []string {
	matches := taskLabelRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var labels []string
	for _, match := range matches {
		lower := strings.ToLower(match[1])
		if !seen[lower] {
			seen[lower] = true
			labels = append(labels, lower)
		}
	}
	return labels
}

func dedup(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// nodeContent extracts the source text for an AST node.
// For code blocks, it reconstructs the block with delimiters.
// For other block nodes, it extracts direct line content.
func nodeContent(source []byte, node ast.Node) string {
	switch n := node.(type) {
	case *ast.FencedCodeBlock:
		return fencedCodeBlockContent(source, n)
	case *ast.CodeBlock:
		return codeBlockContent(source, n)
	default:
		return blockLines(source, node)
	}
}

// fencedCodeBlockContent reconstructs a fenced code block with its delimiters.
func fencedCodeBlockContent(source []byte, n *ast.FencedCodeBlock) string {
	var sb strings.Builder

	fence := "```"
	if lang := n.Language(source); len(lang) > 0 {
		fence += string(lang)
	}
	sb.WriteString(fence)
	sb.WriteByte('\n')

	lines := n.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		sb.Write(line.Value(source))
	}

	sb.WriteString("```")
	return sb.String()
}

// codeBlockContent extracts indented code block content.
func codeBlockContent(source []byte, n *ast.CodeBlock) string {
	var sb strings.Builder
	lines := n.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		sb.Write(line.Value(source))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// blockLines extracts all text lines from a generic block node.
func blockLines(source []byte, node ast.Node) string {
	var sb strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		sb.Write(line.Value(source))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// inlineText extracts the full text content from a node's inline children
// (e.g. Text, CodeSpan, Emphasis children of a Heading).
func inlineText(source []byte, node ast.Node) string {
	var sb strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			sb.Write(t.Value(source))
		} else {
			sb.WriteString(inlineText(source, child))
		}
	}
	return sb.String()
}

// lineNumber returns the 1-based line number for a byte offset in source.
func lineNumber(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	line := 1
	for i := range offset {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

// GetFrontmatterString extracts a string from frontmatter.
func (d *MarkdownDoc) GetFrontmatterString(key string) string {
	if v, ok := d.Frontmatter[key].(string); ok {
		return v
	}
	return ""
}

// GetFrontmatterStringSlice extracts a string slice from frontmatter.
func (d *MarkdownDoc) GetFrontmatterStringSlice(key string) []string {
	switch v := d.Frontmatter[key].(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

// ExtractWikiLinks finds [[wiki-style]] links in content using AST parsing.
func ExtractWikiLinks(content string) []string {
	return ParseMarkdown(content).WikiLinks
}

// ExtractMentions finds @mentions in content using AST parsing.
func ExtractMentions(content string) []string {
	return ParseMarkdown(content).Mentions
}

// ExtractTasks extracts checkbox tasks from markdown content using AST parsing.
func ExtractTasks(content string) []ExtractedTask {
	return ParseMarkdown(content).Tasks
}

// ExtractQueryBlocks finds all ```know blocks in content using AST parsing.
func ExtractQueryBlocks(content string) []QueryBlock {
	return ParseMarkdown(content).QueryBlocks
}
