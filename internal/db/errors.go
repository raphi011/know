// Package db provides error types for database operations.
package db

import (
	"errors"
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
