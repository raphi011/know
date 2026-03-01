package db

import (
	"strings"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// bareID strips a "table:" prefix from a record ID string so it can be
// used safely with type::record("table", id). Accepts both "default"
// and "vault:default" — returns "default" in both cases.
func bareID(table, id string) string {
	return strings.TrimPrefix(id, table+":")
}

// optionalString returns models.None for nil pointers, otherwise returns the string value.
func optionalString(s *string) any {
	if s == nil {
		return surrealmodels.None
	}
	return *s
}

// optionalObject returns models.None for nil maps, otherwise returns the map.
func optionalObject(m map[string]any) any {
	if m == nil {
		return surrealmodels.None
	}
	return m
}

