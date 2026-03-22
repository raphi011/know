package api

import (
	"testing"
)

func TestIsRegisteredRedirectURI(t *testing.T) {
	registered := []string{
		"http://localhost:1234/callback",
		"http://127.0.0.1:5678/callback",
	}

	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"exact match", "http://localhost:1234/callback", true},
		{"different port same host", "http://localhost:9999/callback", true},
		{"different loopback host", "http://127.0.0.1:5678/callback", true},
		{"different port on 127", "http://127.0.0.1:1111/callback", true},
		{"unregistered host", "http://[::1]:1234/callback", false},
		{"different path", "http://localhost:1234/other", false},
		{"https scheme", "https://localhost:1234/callback", false},
		{"not loopback", "http://evil.com:1234/callback", false},
		{"empty", "", false},
		{"malformed", "not-a-url", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRegisteredRedirectURI(tt.uri, registered); got != tt.want {
				t.Errorf("isRegisteredRedirectURI(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

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
