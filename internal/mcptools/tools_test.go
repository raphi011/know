package mcptools

import (
	"fmt"
	"testing"

	"github.com/raphi011/know/internal/tools"
)

func TestIsToolLevelError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ToolError — matches",
			err:  &tools.ToolError{Message: "document not found: /foo"},
			want: true,
		},
		{
			name: "wrapped ToolError — matches",
			err:  fmt.Errorf("apply section edit: %w", &tools.ToolError{Message: "heading not found"}),
			want: true,
		},
		{
			name: "plain error — no match",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
		{
			name: "wrapped plain error — no match",
			err:  fmt.Errorf("apply section edit: %w", fmt.Errorf("connection refused")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isToolLevelError(tt.err); got != tt.want {
				t.Errorf("isToolLevelError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorResult(t *testing.T) {
	result := errorResult("something went wrong")
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
}

func TestTextResult(t *testing.T) {
	result := textResult("hello")
	if result.IsError {
		t.Fatal("expected IsError to be false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
}
