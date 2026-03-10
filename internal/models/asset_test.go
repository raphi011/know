package models

import "testing"

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"/docs/photo.png", true},
		{"/docs/photo.jpg", true},
		{"/docs/photo.jpeg", true},
		{"/docs/photo.gif", true},
		{"/docs/photo.svg", true},
		{"/docs/photo.webp", true},
		// Case insensitive
		{"/docs/photo.PNG", true},
		{"/docs/photo.Jpg", true},
		{"/docs/photo.WEBP", true},
		// Unsupported
		{"/docs/readme.md", false},
		{"/docs/file.txt", false},
		{"/docs/file.pdf", false},
		{"/docs/file.bmp", false},
		{"/docs/file.tiff", false},
		// Edge cases
		{"", false},
		{"noext", false},
		{"/docs/image.backup.png", true},
		{"/docs/.png", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsImageFile(tt.name)
			if got != tt.want {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMimeTypeFromExt(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"/photo.png", "image/png"},
		{"/photo.jpg", "image/jpeg"},
		{"/photo.jpeg", "image/jpeg"},
		{"/photo.gif", "image/gif"},
		{"/photo.svg", "image/svg+xml"},
		{"/photo.webp", "image/webp"},
		// Case insensitive
		{"/photo.PNG", "image/png"},
		{"/photo.JPG", "image/jpeg"},
		// Unsupported returns empty
		{"/file.txt", ""},
		{"/file.md", ""},
		{"", ""},
		{"noext", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MimeTypeFromExt(tt.name)
			if got != tt.want {
				t.Errorf("MimeTypeFromExt(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
