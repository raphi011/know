package jina

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://example.com/page", wantErr: false},
		{name: "valid http", url: "http://example.com/page", wantErr: false},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: true},
		{name: "ftp scheme", url: "ftp://example.com/file", wantErr: true},
		{name: "javascript scheme", url: "javascript:alert(1)", wantErr: true},
		{name: "no scheme", url: "example.com", wantErr: true},
		{name: "empty string", url: "", wantErr: true},
		{name: "localhost", url: "http://localhost/secret", wantErr: true},
		{name: "127.0.0.1", url: "http://127.0.0.1/secret", wantErr: true},
		{name: "loopback ipv6", url: "http://[::1]/secret", wantErr: true},
		{name: "private 10.x", url: "http://10.0.0.1/internal", wantErr: true},
		{name: "private 172.16.x", url: "http://172.16.0.1/internal", wantErr: true},
		{name: "private 192.168.x", url: "http://192.168.1.1/internal", wantErr: true},
		{name: "link-local", url: "http://169.254.169.254/latest/meta-data/", wantErr: true},
		// URL obfuscation bypass attempts
		{name: "credentials in URL", url: "http://user@127.0.0.1/", wantErr: true},
		{name: "expanded ipv6 loopback", url: "http://[0:0:0:0:0:0:0:1]/", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
