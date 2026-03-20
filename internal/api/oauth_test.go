package api

import "testing"

func TestIsLoopbackRedirectURI(t *testing.T) {
	tests := []struct {
		uri  string
		want bool
	}{
		{"http://localhost:12345/callback", true},
		{"http://localhost/callback", true},
		{"http://127.0.0.1:8080/callback", true},
		{"http://[::1]:9999/callback", true},
		{"https://localhost:12345/callback", false},
		{"http://evil.com/callback", false},
		{"http://localhost.evil.com/callback", false},
		{"http://example.com", false},
		{"not-a-url", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			if got := isLoopbackRedirectURI(tt.uri); got != tt.want {
				t.Errorf("isLoopbackRedirectURI(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}
