package db

import (
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

	"github.com/raphi011/knowhow/internal/models"
)

// bareID is a package-local alias for models.BareID.
var bareID = models.BareID

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
