package record

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0:00"},
		{59 * time.Second, "0:59"},
		{60 * time.Second, "1:00"},
		{61 * time.Second, "1:01"},
		{5*time.Minute + 30*time.Second, "5:30"},
		{10*time.Minute + 5*time.Second, "10:05"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatDuration(tt.input)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
