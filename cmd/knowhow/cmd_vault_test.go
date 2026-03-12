package main

import "testing"

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{1500, "1,500"},
		{999_999, "999,999"},
		{1_000_000, "1,000,000"},
		{1_234_567, "1,234,567"},
		{999_999_999, "999,999,999"},
		{1_000_000_000, "1,000,000,000"},
		{1_234_567_890, "1,234,567,890"},
		{-1, "-1"},
		{-1500, "-1,500"},
	}
	for _, tt := range tests {
		got := formatNumber(tt.input)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
