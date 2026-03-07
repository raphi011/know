package search

import "testing"

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Claude Sandbox", "claude sandbox"},
		{"  claude   sandbox  ", "claude sandbox"},
		{" HELLO   World ", "hello world"},
		{"single", "single"},
		{"", ""},
		{"  ", ""},
		{"already normalized", "already normalized"},
		{"\t tabs \n newlines ", "tabs newlines"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeQuery(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
