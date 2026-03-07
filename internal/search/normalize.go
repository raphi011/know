package search

import "strings"

// NormalizeQuery normalizes a search query for embedding cache lookup.
// Lowercases, collapses whitespace, trims. "  Claude   Sandbox  " -> "claude sandbox".
func NormalizeQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(query), " "))
}
