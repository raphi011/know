// Package db provides error types for database operations.
package db

import (
	"errors"
	"fmt"
	"strings"

	"github.com/surrealdb/surrealdb.go"
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

// wrapQueryError inspects a SurrealDB error and wraps it with the appropriate
// sentinel error if it's a known query error type. Returns the original error
// if it's not a QueryError or doesn't match known patterns.
func wrapQueryError(err error) error {
	if err == nil {
		return nil
	}

	// Extract QueryError if present - this is a database-level error
	var queryErr *surrealdb.QueryError
	if errors.As(err, &queryErr) {
		msg := queryErr.Message
		if strings.Contains(msg, "already exists") {
			return fmt.Errorf("%w: %s", ErrEntityAlreadyExists, msg)
		}
		if strings.Contains(msg, "Transaction conflict") {
			return fmt.Errorf("%w: %s", ErrTransactionConflict, msg)
		}
	}

	return err
}
