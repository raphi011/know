package oidc

import (
	"testing"
)

func TestProviderName(t *testing.T) {
	tests := []struct {
		issuerURL string
		want      string
	}{
		{"https://github.com", "github"},
		{"https://accounts.google.com", "google"},
		{"https://login.microsoftonline.com/tenant", "login"},
		{"https://auth.example.com", "auth"},
		{"https://www.example.com", "example"},
		{"https://accounts.example.org/path", "example"},
		{"://invalid", "://invalid"}, // url.Parse fails
	}

	for _, tt := range tests {
		t.Run(tt.issuerURL, func(t *testing.T) {
			p := &Provider{issuerURL: tt.issuerURL}
			got := p.ProviderName()
			if got != tt.want {
				t.Errorf("ProviderName(%q) = %q, want %q", tt.issuerURL, got, tt.want)
			}
		})
	}
}
