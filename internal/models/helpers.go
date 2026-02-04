// Package models defines data structures for the Knowhow knowledge database.
package models

import (
	"fmt"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// RecordIDString safely extracts the string ID from a SurrealDB RecordID.
// Returns an error if the ID is not a string type.
func RecordIDString(id surrealmodels.RecordID) (string, error) {
	s, ok := id.ID.(string)
	if !ok {
		return "", fmt.Errorf("unexpected ID type: %T (expected string)", id.ID)
	}
	return s, nil
}

// MustRecordIDString extracts the string ID, panicking if not a string.
// Use only when you're certain the ID is a string (e.g., after DB operations that return strings).
func MustRecordIDString(id surrealmodels.RecordID) string {
	s, err := RecordIDString(id)
	if err != nil {
		panic(err)
	}
	return s
}
