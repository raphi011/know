package file

import (
	"strings"

	"github.com/raphi011/know/internal/parser"
)

// MergeLabels combines inline labels with other label sources, deduplicating.
func MergeLabels(inlineLabels []string, otherLabels []string) []string {
	seen := make(map[string]bool)
	var labels []string
	for _, l := range otherLabels {
		lower := strings.ToLower(l)
		if !seen[lower] {
			seen[lower] = true
			labels = append(labels, lower)
		}
	}

	for _, l := range inlineLabels {
		lower := strings.ToLower(l)
		if !seen[lower] {
			seen[lower] = true
			labels = append(labels, lower)
		}
	}
	return labels
}

// ExtractInlineLabels finds #tags in content, merging with frontmatter labels.
// Deprecated: Use ParseMarkdown().InlineLabels with MergeLabels instead.
func ExtractInlineLabels(content string, frontmatterLabels []string) []string {
	// Parse the content to get inline labels from AST
	doc := parser.ParseMarkdown(content)
	return MergeLabels(doc.InlineLabels, frontmatterLabels)
}
