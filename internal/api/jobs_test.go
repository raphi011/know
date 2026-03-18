package api

import (
	"testing"
	"time"
)

func TestParseSinceDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"1h", 1 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{" 24h ", 24 * time.Hour, false},

		// Invalid inputs
		{"abc", 0, true},
		{"7x", 0, true},
		{"0h", 0, true},
		{"0d", 0, true},
		{"-1h", 0, true},
		{"-5d", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseSinceDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSinceDuration(%q): err = %v, wantErr = %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseSinceDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
