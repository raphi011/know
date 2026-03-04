package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test config %s: %v", name, err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		path := writeTestConfig(t, dir, "valid.toml", `
port = 9090

[[instance]]
name = "private"
url = "http://localhost:8484"
token = "kh_abc"

[[instance]]
name = "work"
url = "http://work:8484"
token = "kh_xyz"
`)

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
		path := writeTestConfig(t, dir, "default_port.toml", `
[[instance]]
name = "test"
url = "http://localhost:8484"
token = "kh_test"
`)

		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != 8585 {
			t.Errorf("port = %d, want 8585", cfg.Port)
		}
	})

	t.Run("no instances", func(t *testing.T) {
		path := writeTestConfig(t, dir, "empty.toml", `port = 8585`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for empty instances")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		path := writeTestConfig(t, dir, "no_name.toml", `
[[instance]]
url = "http://localhost:8484"
token = "kh_test"
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("missing url", func(t *testing.T) {
		path := writeTestConfig(t, dir, "no_url.toml", `
[[instance]]
name = "test"
token = "kh_test"
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for missing url")
		}
	})

	t.Run("missing token is allowed (no-auth mode)", func(t *testing.T) {
		path := writeTestConfig(t, dir, "no_token.toml", `
[[instance]]
name = "test"
url = "http://localhost:8484"
`)

		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Instances[0].Token != "" {
			t.Fatalf("expected empty token, got %q", cfg.Instances[0].Token)
		}
	})

	t.Run("duplicate instance names", func(t *testing.T) {
		path := writeTestConfig(t, dir, "dup_names.toml", `
[[instance]]
name = "test"
url = "http://localhost:8484"
token = "kh_abc"

[[instance]]
name = "test"
url = "http://other:8484"
token = "kh_xyz"
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for duplicate instance names")
		}
	})

	t.Run("invalid url scheme", func(t *testing.T) {
		path := writeTestConfig(t, dir, "bad_url.toml", `
[[instance]]
name = "test"
url = "ftp://localhost:8484"
token = "kh_test"
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for non-http url")
		}
	})

	t.Run("invalid port too high", func(t *testing.T) {
		path := writeTestConfig(t, dir, "high_port.toml", `
port = 99999

[[instance]]
name = "test"
url = "http://localhost:8484"
token = "kh_test"
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for port out of range")
		}
	})

	t.Run("negative port", func(t *testing.T) {
		path := writeTestConfig(t, dir, "neg_port.toml", `
port = -1

[[instance]]
name = "test"
url = "http://localhost:8484"
token = "kh_test"
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected error for negative port")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := loadConfig(filepath.Join(dir, "nonexistent.toml"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}
