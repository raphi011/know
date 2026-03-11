package tui

import (
	"strings"
	"testing"
)

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{1500, "1.5k"},
		{999_999, "1000.0k"},
		{1_000_000, "1.0M"},
		{1_500_000, "1.5M"},
		{10_000_000, "10.0M"},
	}

	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestRenderStatusBar(t *testing.T) {
	t.Run("zero tokens shows vault only", func(t *testing.T) {
		got := renderStatusBar(0, 0, "default")
		if !strings.Contains(got, "vault: default") {
			t.Errorf("expected vault info, got %q", got)
		}
		if strings.Contains(got, "tokens:") {
			t.Errorf("expected no token info for zero usage, got %q", got)
		}
	})

	t.Run("nonzero tokens shows summary", func(t *testing.T) {
		got := renderStatusBar(1500, 300, "myvault")
		if !strings.Contains(got, "1.5k in") {
			t.Errorf("expected '1.5k in', got %q", got)
		}
		if !strings.Contains(got, "300 out") {
			t.Errorf("expected '300 out', got %q", got)
		}
		if !strings.Contains(got, "vault: myvault") {
			t.Errorf("expected vault info, got %q", got)
		}
	})
}
