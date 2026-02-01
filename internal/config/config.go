package config

import (
	"log/slog"
	"os"
	"strings"
)

// Config holds all configuration values.
type Config struct {
	// SurrealDB connection
	SurrealDBURL       string
	SurrealDBNamespace string
	SurrealDBDatabase  string
	SurrealDBUser      string
	SurrealDBPass      string
	SurrealDBAuthLevel string

	// Ollama embedding
	OllamaHost     string
	EmbeddingModel string

	// Logging
	LogFile  string
	LogLevel slog.Level

	// Context detection
	DefaultContext string
	ContextFromCWD bool
}

// Load reads configuration from environment variables.
// Defaults match Python memcp/db.py exactly.
func Load() Config {
	return Config{
		// SurrealDB (matching Python defaults)
		SurrealDBURL:       getEnv("SURREALDB_URL", "ws://localhost:8000/rpc"),
		SurrealDBNamespace: getEnv("SURREALDB_NAMESPACE", "knowledge"),
		SurrealDBDatabase:  getEnv("SURREALDB_DATABASE", "graph"),
		SurrealDBUser:      getEnv("SURREALDB_USER", "root"),
		SurrealDBPass:      getEnv("SURREALDB_PASS", "root"),
		SurrealDBAuthLevel: getEnv("SURREALDB_AUTH_LEVEL", "root"),

		// Ollama
		OllamaHost:     getEnv("OLLAMA_HOST", "http://localhost:11434"),
		EmbeddingModel: getEnv("MEMCP_EMBEDDING_MODEL", "all-minilm:l6-v2"),

		// Logging
		LogFile:  getEnv("MEMCP_LOG_FILE", "/tmp/memcp.log"),
		LogLevel: parseLogLevel(getEnv("MEMCP_LOG_LEVEL", "INFO")),

		// Context
		DefaultContext: getEnv("MEMCP_DEFAULT_CONTEXT", ""),
		ContextFromCWD: getEnv("MEMCP_CONTEXT_FROM_CWD", "false") == "true",
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
