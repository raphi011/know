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
		got := renderStatusBar(0, 0, "default", 0, 0)
		if !strings.Contains(got, "vault: default") {
			t.Errorf("expected vault info, got %q", got)
		}
		if strings.Contains(got, "tokens:") {
			t.Errorf("expected no token info for zero usage, got %q", got)
		}
	})

	t.Run("nonzero tokens shows summary", func(t *testing.T) {
		got := renderStatusBar(1500, 300, "myvault", 0, 0)
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

	t.Run("with context window shows bar", func(t *testing.T) {
		got := renderStatusBar(1500, 300, "myvault", 200_000, 120_000)
		if !strings.Contains(got, "60%") {
			t.Errorf("expected context percentage, got %q", got)
		}
		if !strings.Contains(got, "█") {
			t.Errorf("expected filled bar chars, got %q", got)
		}
	})
}

func TestRenderContextBar(t *testing.T) {
	t.Run("empty when no data", func(t *testing.T) {
		if got := renderContextBar(0, 0); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
		if got := renderContextBar(200_000, 0); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
		if got := renderContextBar(0, 100); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("low usage green", func(t *testing.T) {
		got := renderContextBar(200_000, 50_000) // 25%
		if !strings.Contains(got, "25%") {
			t.Errorf("expected 25%%, got %q", got)
		}
		// ANSI RGB for #10B981 = 16;185;129
		if !strings.Contains(got, "16;185;129") {
			t.Errorf("expected green color, got %q", got)
		}
	})

	t.Run("medium usage yellow", func(t *testing.T) {
		got := renderContextBar(200_000, 140_000) // 70%
		if !strings.Contains(got, "70%") {
			t.Errorf("expected 70%%, got %q", got)
		}
		// ANSI RGB for #F59E0B = 245;158;11
		if !strings.Contains(got, "245;158;11") {
			t.Errorf("expected yellow color, got %q", got)
		}
	})

	t.Run("high usage red", func(t *testing.T) {
		got := renderContextBar(200_000, 180_000) // 90%
		if !strings.Contains(got, "90%") {
			t.Errorf("expected 90%%, got %q", got)
		}
		// ANSI RGB for #EF4444 = 239;68;68
		if !strings.Contains(got, "239;68;68") {
			t.Errorf("expected red color, got %q", got)
		}
	})

	t.Run("over 100 percent capped", func(t *testing.T) {
		got := renderContextBar(100_000, 150_000) // 150% → capped to 100%
		if !strings.Contains(got, "100%") {
			t.Errorf("expected 100%%, got %q", got)
		}
	})

	t.Run("negative contextUsed returns empty", func(t *testing.T) {
		if got := renderContextBar(200_000, -100); got != "" {
			t.Errorf("expected empty for negative contextUsed, got %q", got)
		}
	})

	t.Run("negative contextMax returns empty", func(t *testing.T) {
		if got := renderContextBar(-1, 100); got != "" {
			t.Errorf("expected empty for negative contextMax, got %q", got)
		}
	})

	t.Run("boundary 59 percent is green", func(t *testing.T) {
		got := renderContextBar(200_000, 119_999) // 59.99%
		// ANSI RGB for #10B981 = 16;185;129
		if !strings.Contains(got, "16;185;129") {
			t.Errorf("expected green color at 59.99%%, got %q", got)
		}
	})

	t.Run("boundary 60 percent is yellow", func(t *testing.T) {
		got := renderContextBar(200_000, 120_000) // 60%
		// ANSI RGB for #F59E0B = 245;158;11
		if !strings.Contains(got, "245;158;11") {
			t.Errorf("expected yellow color at 60%%, got %q", got)
		}
	})

	t.Run("boundary 85 percent is yellow", func(t *testing.T) {
		got := renderContextBar(200_000, 170_000) // 85%
		// ANSI RGB for #F59E0B = 245;158;11
		if !strings.Contains(got, "245;158;11") {
			t.Errorf("expected yellow color at 85%%, got %q", got)
		}
	})

	t.Run("boundary 86 percent is red", func(t *testing.T) {
		got := renderContextBar(200_000, 170_001) // 85.0005%
		// ANSI RGB for #EF4444 = 239;68;68
		if !strings.Contains(got, "239;68;68") {
			t.Errorf("expected red color at >85%%, got %q", got)
		}
	})

	t.Run("bar has correct char count", func(t *testing.T) {
		got := renderContextBar(200_000, 100_000) // 50%
		// Count █ and ░ characters
		filled := strings.Count(got, "█")
		empty := strings.Count(got, "░")
		if filled+empty != 12 {
			t.Errorf("expected 12 bar chars, got %d filled + %d empty = %d", filled, empty, filled+empty)
		}
	})
}
