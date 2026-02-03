// Package parser provides Markdown parsing and content extraction.
package parser

import (
	"bufio"
	"regexp"
	"strings"

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
	Level   int       // 1-6 for h1-h6
	Heading string    // The heading text
	Path    string    // Full path like "## Setup > ### Install"
	Content string    // Content under this heading
	Start   int       // Line number where section starts
	End     int       // Line number where section ends
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
				// Ignore YAML errors, just use empty frontmatter
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

// parseSections extracts sections from Markdown content.
func parseSections(content string) []Section {
	var sections []Section
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	var currentPath []string
	var currentLevels []int

	var currentSection *Section
	var contentBuilder strings.Builder

	flushSection := func(endLine int) {
		if currentSection != nil {
			currentSection.Content = strings.TrimSpace(contentBuilder.String())
			currentSection.End = endLine
			sections = append(sections, *currentSection)
			contentBuilder.Reset()
		}
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if match := headingRegex.FindStringSubmatch(line); len(match) > 0 {
			// Flush previous section
			flushSection(lineNum - 1)

			level := len(match[1])
			heading := strings.TrimSpace(match[2])

			// Update path based on heading level
			for len(currentLevels) > 0 && currentLevels[len(currentLevels)-1] >= level {
				currentPath = currentPath[:len(currentPath)-1]
				currentLevels = currentLevels[:len(currentLevels)-1]
			}
			currentPath = append(currentPath, match[1]+" "+heading)
			currentLevels = append(currentLevels, level)

			currentSection = &Section{
				Level:   level,
				Heading: heading,
				Path:    strings.Join(currentPath, " > "),
				Start:   lineNum,
			}
		} else if currentSection != nil {
			contentBuilder.WriteString(line)
			contentBuilder.WriteString("\n")
		}
	}

	// Flush last section
	flushSection(lineNum)

	return sections
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
