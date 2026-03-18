package db

import (
	"fmt"
	"testing"

	surrealdb "github.com/surrealdb/surrealdb.go"
)

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "record ID collision",
			err:  &surrealdb.QueryError{Message: "Database record `file:abc123` already exists"},
			want: true,
		},
		{
			name: "unique index violation",
			err:  &surrealdb.QueryError{Message: "Database index `idx_file_vault_path` already contains [vault:default, '/path.md'], with record `file:abc123`"},
			want: true,
		},
		{
			name: "other query error",
			err:  &surrealdb.QueryError{Message: "some other database error"},
			want: false,
		},
		{
			name: "non-query error",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
		{
			name: "wrapped query error with already exists",
			err:  fmt.Errorf("create file: %w", &surrealdb.QueryError{Message: "Database record `file:abc` already exists"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUniqueViolation(tt.err); got != tt.want {
				t.Errorf("isUniqueViolation() = %v, want %v", got, tt.want)
			}
		})
	}
}
