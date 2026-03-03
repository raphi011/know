package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		path := filepath.Join(dir, "valid.toml")
		os.WriteFile(path, []byte(`
port = 9090

[[instance]]
name = "private"
url = "http://localhost:8484"
token = "kh_abc"

[[instance]]
name = "work"
url = "http://work:8484"
token = "kh_xyz"
`), 0644)

		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != 9090 {
			t.Errorf("port = %d, want 9090", cfg.Port)
		}
		if len(cfg.Instances) != 2 {
			t.Fatalf("instances = %d, want 2", len(cfg.Instances))
		}
		if cfg.Instances[0].Name != "private" {
			t.Errorf("instance[0].name = %q, want %q", cfg.Instances[0].Name, "private")
		}
	})

	t.Run("default port", func(t *testing.T) {
		path := filepath.Join(dir, "default_port.toml")
		os.WriteFile(path, []byte(`
[[instance]]
name = "test"
url = "http://localhost:8484"
token = "kh_test"
`), 0644)

		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != 8585 {
			t.Errorf("port = %d, want 8585", cfg.Port)
		}
	})

	t.Run("no instances", func(t *testing.T) {
		path := filepath.Join(dir, "empty.toml")
		os.WriteFile(path, []byte(`port = 8585`), 0644)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for empty instances")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		path := filepath.Join(dir, "no_name.toml")
		os.WriteFile(path, []byte(`
[[instance]]
url = "http://localhost:8484"
token = "kh_test"
`), 0644)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("missing url", func(t *testing.T) {
		path := filepath.Join(dir, "no_url.toml")
		os.WriteFile(path, []byte(`
[[instance]]
name = "test"
token = "kh_test"
`), 0644)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for missing url")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		path := filepath.Join(dir, "no_token.toml")
		os.WriteFile(path, []byte(`
[[instance]]
name = "test"
url = "http://localhost:8484"
`), 0644)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for missing token")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := loadConfig(filepath.Join(dir, "nonexistent.toml"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}
