package llm

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsFatalAPIError(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		fatal bool
	}{
		{"nil error", nil, false},
		{"generic error", errors.New("connection reset"), false},
		{"credit balance", errors.New("insufficient credit balance"), true},
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"quota exceeded", errors.New("quota exceeded for model"), true},
		{"billing issue", errors.New("billing account inactive"), true},
		{"invalid api key", errors.New("invalid api key"), true},
		{"authentication failed", errors.New("authentication failed"), true},
		{"unauthorized", errors.New("unauthorized request"), true},
		{"401 status", errors.New("HTTP 401: not allowed"), true},
		{"403 status", errors.New("HTTP 403: forbidden"), true},
		{"wrapped error", fmt.Errorf("embed: %w", errors.New("credit balance too low")), true},
		{"404 not fatal", errors.New("HTTP 404: not found"), false},
		{"timeout not fatal", errors.New("context deadline exceeded"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFatalAPIError(tt.err)
			if got != tt.fatal {
				t.Errorf("isFatalAPIError(%v) = %v, want %v", tt.err, got, tt.fatal)
			}
		})
	}
}

func TestWrapFatalError(t *testing.T) {
	t.Run("wraps fatal error", func(t *testing.T) {
		err := errors.New("invalid api key provided")
		wrapped := wrapFatalError(err)
		if !errors.Is(wrapped, ErrFatalAPI) {
			t.Errorf("expected wrapped error to match ErrFatalAPI")
		}
	})

	t.Run("passes through non-fatal error", func(t *testing.T) {
		err := errors.New("network timeout")
		result := wrapFatalError(err)
		if errors.Is(result, ErrFatalAPI) {
			t.Errorf("non-fatal error should not be wrapped with ErrFatalAPI")
		}
		if result != err {
			t.Errorf("expected original error returned, got %v", result)
		}
	})

	t.Run("nil error", func(t *testing.T) {
		result := wrapFatalError(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}
