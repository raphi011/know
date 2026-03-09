// Package db provides error types for database operations.
package db

import (
	"errors"
	"strings"

	surrealdb "github.com/surrealdb/surrealdb.go"
)

// Sentinel errors for database operations.
// Use errors.Is() to check for these errors in calling code.
var (
	// ErrEntityAlreadyExists indicates an entity with the same ID or name already exists.
	// This can occur during CREATE operations when the entity was previously created
	// or during concurrent operations.
	ErrEntityAlreadyExists = errors.New("entity already exists")

	// ErrTransactionConflict indicates a SurrealDB transaction conflict.
	// This occurs when multiple concurrent operations attempt to modify the same records.
	// Callers should typically retry or skip the operation.
	ErrTransactionConflict = errors.New("transaction conflict")

	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("entity not found")
)

// isUniqueViolation checks if an error is a SurrealDB unique index constraint violation.
// Query-based operations return QueryError with a message like "Database record `X` already exists".
// We use errors.As to extract the QueryError, then check the message for the violation pattern.
func isUniqueViolation(err error) bool {
	var qe *surrealdb.QueryError
	if errors.As(err, &qe) {
		return strings.Contains(qe.Message, "already exists")
	}
	return false
}
