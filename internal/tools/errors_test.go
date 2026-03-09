package tools

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestToolError(t *testing.T) {
	err := &ToolError{Message: "document not found: /foo"}
	if err.Error() != "document not found: /foo" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestToolError_SurvivesWrapping(t *testing.T) {
	inner := &ToolError{Message: "document not found: /test"}
	wrapped := fmt.Errorf("apply section edit: %w", inner)

	var toolErr *ToolError
	if !errors.As(wrapped, &toolErr) {
		t.Fatal("errors.As should find ToolError through wrapping")
	}
	if toolErr.Message != "document not found: /test" {
		t.Fatalf("unexpected message: %s", toolErr.Message)
	}
}

func TestToolError_NotConfusedWithPlainError(t *testing.T) {
	plain := fmt.Errorf("connection refused")
	wrapped := fmt.Errorf("apply section edit: %w", plain)

	var toolErr *ToolError
	if errors.As(wrapped, &toolErr) {
		t.Fatal("plain error should not match ToolError")
	}
}

func TestCheckContentHash(t *testing.T) {
	ptr := func(s string) *string { return &s }

	tests := []struct {
		name         string
		expectedHash *string
		currentHash  *string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "no expected hash — skip check",
			expectedHash: nil,
			currentHash:  ptr("abc"),
		},
		{
			name:         "both nil — skip check",
			expectedHash: nil,
			currentHash:  nil,
		},
		{
			name:         "matching hashes — pass",
			expectedHash: ptr("abc"),
			currentHash:  ptr("abc"),
		},
		{
			name:         "mismatching hashes — fail",
			expectedHash: ptr("abc"),
			currentHash:  ptr("def"),
			wantErr:      true,
			errContains:  "document changed",
		},
		{
			name:         "expected hash but document has no hash — fail",
			expectedHash: ptr("abc"),
			currentHash:  nil,
			wantErr:      true,
			errContains:  "no content hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkContentHash(tt.expectedHash, tt.currentHash)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var toolErr *ToolError
				if !errors.As(err, &toolErr) {
					t.Fatalf("expected ToolError, got %T: %v", err, err)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
