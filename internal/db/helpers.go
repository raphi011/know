package db

import surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

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

// optionalEmbedding returns models.None for nil/empty slices, otherwise returns the slice.
func optionalEmbedding(e []float32) any {
	if len(e) == 0 {
		return surrealmodels.None
	}
	return e
}

// optionalRecordID returns models.None for nil record pointers, otherwise returns the record.
func optionalRecordID(r *surrealmodels.RecordID) any {
	if r == nil {
		return surrealmodels.None
	}
	return *r
}
