package document

import (
	"regexp"
	"strings"
)

var hashTagRegex = regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_-]*)`)

// ExtractInlineLabels finds #tags in content, merging with frontmatter labels.
func ExtractInlineLabels(content string, frontmatterLabels []string) []string {
	seen := make(map[string]bool)
	var labels []string
	for _, l := range frontmatterLabels {
		lower := strings.ToLower(l)
		if !seen[lower] {
			seen[lower] = true
			labels = append(labels, lower)
		}
	}

	matches := hashTagRegex.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		lower := strings.ToLower(m[1])
		if !seen[lower] {
			seen[lower] = true
			labels = append(labels, lower)
		}
	}
	return labels
}
