package models

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase", "hello", "hello"},
		{"uppercase", "Hello World", "hello-world"},
		{"underscores", "my_doc_name", "my-doc-name"},
		{"special chars stripped", "Hello, World!", "hello-world"},
		{"numbers preserved", "doc-v2.1", "doc-v21"},
		{"mixed", "My Cool_Doc (v3)", "my-cool-doc-v3"},
		{"empty string", "", ""},
		{"only special chars", "!@#$%", ""},
		{"consecutive spaces", "hello   world", "hello---world"},
		{"unicode stripped", "café résumé", "caf-rsum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.in)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
