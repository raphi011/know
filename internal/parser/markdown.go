// Package parser provides Markdown parsing and content extraction.
package parser

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

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

// ParseMarkdown parses a Markdown document into structured form.
func ParseMarkdown(content string) (*MarkdownDoc, error) {
	doc := &MarkdownDoc{
		Frontmatter: make(map[string]any),
	}

	// Parse frontmatter if present
	remaining := content
	if strings.HasPrefix(content, "---\n") {
		endIdx := strings.Index(content[4:], "\n---")
		if endIdx > 0 {
			frontmatterYAML := content[4 : 4+endIdx]
			remaining = strings.TrimPrefix(content[4+endIdx+4:], "\n")

			if err := yaml.Unmarshal([]byte(frontmatterYAML), &doc.Frontmatter); err != nil {
				// Log YAML parse error but continue with empty frontmatter
				slog.Warn("failed to parse frontmatter YAML", "error", err)
				doc.Frontmatter = make(map[string]any)
			}
		}
	}

	doc.Content = remaining

	// Extract title
	doc.Title = extractTitle(doc.Frontmatter, remaining)

	// Parse sections
	doc.Sections = parseSections(remaining)

	return doc, nil
}

// extractTitle gets title from frontmatter or first h1.
func extractTitle(fm map[string]any, content string) string {
	// Check frontmatter
	if title, ok := fm["title"].(string); ok && title != "" {
		return title
	}
	if name, ok := fm["name"].(string); ok && name != "" {
		return name
	}

	// Find first h1
	h1Regex := regexp.MustCompile(`(?m)^#\s+(.+)$`)
	if match := h1Regex.FindStringSubmatch(content); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	return ""
}

// parseSections extracts sections from Markdown content using goldmark's AST.
// This correctly handles code fences (# inside code blocks won't be treated as headings)
// and captures pre-heading content as a preamble section with Level=0.
func parseSections(content string) []Section {
	source := []byte(content)
	md := goldmark.New()
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	var sections []Section
	var currentPath []string
	var currentLevels []int
	var contentBuilder strings.Builder
	var currentSection *Section
	var codeBlockChars int

	flushSection := func() {
		if currentSection == nil {
			return
		}
		currentSection.Content = strings.TrimSpace(contentBuilder.String())
		// Mark as code-block-dominated if >50% of section content (by char count) is from code blocks
		trimmedLen := len(currentSection.Content)
		if trimmedLen > 0 && codeBlockChars > trimmedLen/2 {
			currentSection.CodeBlock = true
		}
		sections = append(sections, *currentSection)
		contentBuilder.Reset()
		codeBlockChars = 0
	}

	// Start with a preamble section to capture pre-heading content
	currentSection = &Section{Level: 0}

	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *ast.Heading:
			flushSection()

			heading := inlineText(source, n)
			level := n.Level

			// Update path hierarchy based on heading level
			for len(currentLevels) > 0 && currentLevels[len(currentLevels)-1] >= level {
				currentPath = currentPath[:len(currentPath)-1]
				currentLevels = currentLevels[:len(currentLevels)-1]
			}
			currentPath = append(currentPath, strings.Repeat("#", level)+" "+heading)
			currentLevels = append(currentLevels, level)

			var startLine int
			if lines := n.Lines(); lines.Len() > 0 {
				startLine = lineNumber(source, lines.At(0).Start)
			}

			currentSection = &Section{
				Level:   level,
				Heading: heading,
				Path:    strings.Join(currentPath, " > "),
				Start:   startLine,
			}

		default:
			// Collect content for the current section
			nodeText := nodeContent(source, node)
			if nodeText != "" {
				if contentBuilder.Len() > 0 {
					contentBuilder.WriteString("\n\n")
				}
				contentBuilder.WriteString(nodeText)

				// Track code block content size (fenced and indented)
				switch node.(type) {
				case *ast.FencedCodeBlock, *ast.CodeBlock:
					codeBlockChars += len(nodeText)
				}
			}
		}
	}

	// Flush last section
	flushSection()

	return sections
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
		// For other block nodes, extract all lines
		return blockLines(source, node)
	}
}

// fencedCodeBlockContent reconstructs a fenced code block with its delimiters.
func fencedCodeBlockContent(source []byte, n *ast.FencedCodeBlock) string {
	var sb strings.Builder

	// Opening fence with optional language
	fence := "```"
	if lang := n.Language(source); len(lang) > 0 {
		fence += string(lang)
	}
	sb.WriteString(fence)
	sb.WriteByte('\n')

	// Code lines
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
			// Recurse into inline containers (emphasis, strong, etc.)
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

// ExtractWikiLinks finds [[wiki-style]] links in content.
func ExtractWikiLinks(content string) []string {
	linkRegex := regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	matches := linkRegex.FindAllStringSubmatch(content, -1)

	links := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		link := strings.TrimSpace(match[1])
		if !seen[link] {
			links = append(links, link)
			seen[link] = true
		}
	}
	return links
}

// ExtractMentions finds @mentions in content.
func ExtractMentions(content string) []string {
	mentionRegex := regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)
	matches := mentionRegex.FindAllStringSubmatch(content, -1)

	mentions := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		mention := strings.ToLower(match[1])
		if !seen[mention] {
			mentions = append(mentions, mention)
			seen[mention] = true
		}
	}
	return mentions
}
