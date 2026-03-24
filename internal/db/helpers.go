package db

import (
	"fmt"
	"time"

	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

	"github.com/raphi011/know/internal/models"
)

// bareID is a package-local alias for models.BareID.
var bareID = models.BareID

// newRecordID creates a SurrealDB record ID from a table and bare ID.
func newRecordID(table, id string) surrealmodels.RecordID {
	return surrealmodels.RecordID{Table: table, ID: id}
}

// optionalString returns models.None for nil pointers, otherwise returns the string value.
func optionalString(s *string) any {
	if s == nil {
		return surrealmodels.None
	}
	return *s
}

// optionalTime returns models.None for nil pointers, otherwise returns the time value.
func optionalTime(t *time.Time) any {
	if t == nil {
		return surrealmodels.None
	}
	return *t
}

// optionalObject returns models.None for nil maps, otherwise returns the map.
func optionalObject(m map[string]any) any {
	if m == nil {
		return surrealmodels.None
	}
	return m
}

// firstResult returns the first row from a query result, or an error if no rows were returned.
// Use for CREATE/UPDATE queries that must return exactly one row.
func firstResult[T any](results *[]surrealdb.QueryResult[[]T], op string) (*T, error) {
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("%s: no result returned", op)
	}
	return &(*results)[0].Result[0], nil
}

// firstResultOpt returns the first row from a query result, or nil if no rows were returned.
// Use for SELECT queries where "not found" is a valid outcome.
func firstResultOpt[T any](results *[]surrealdb.QueryResult[[]T]) *T {
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil
	}
	return &(*results)[0].Result[0]
}

// allResults returns all rows from the first statement in a query response, or nil.
// Use for SELECT queries that return a list of rows.
func allResults[T any](results *[]surrealdb.QueryResult[[]T]) []T {
	if results == nil || len(*results) == 0 {
		return nil
	}
	return (*results)[0].Result
}

// countResults returns the number of rows from the first statement in a query response.
func countResults[T any](results *[]surrealdb.QueryResult[[]T]) int {
	if results == nil || len(*results) == 0 {
		return 0
	}
	return len((*results)[0].Result)
}
