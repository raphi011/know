// Package config handles configuration loading from environment variables.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// LLMProvider identifies the LLM provider.
type LLMProvider string

const (
	ProviderOllama    LLMProvider = "ollama"
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic"
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

	// Embedding configuration
	EmbedProvider   LLMProvider
	EmbedModel      string
	EmbedDimension  int

	// LLM configuration (for ask, extract-graph, render)
	LLMProvider LLMProvider
	LLMModel    string

	// Provider-specific settings
	OllamaHost     string
	OpenAIAPIKey   string
	AnthropicAPIKey string

	// Logging
	LogFile  string
	LogLevel slog.Level
}

// Load reads configuration from environment variables.
func Load() Config {
	return Config{
		// SurrealDB
		SurrealDBURL:       getEnv("SURREALDB_URL", "ws://localhost:8000/rpc"),
		SurrealDBNamespace: getEnv("SURREALDB_NAMESPACE", "knowledge"),
		SurrealDBDatabase:  getEnv("SURREALDB_DATABASE", "graph"),
		SurrealDBUser:      getEnv("SURREALDB_USER", "root"),
		SurrealDBPass:      getEnv("SURREALDB_PASS", "root"),
		SurrealDBAuthLevel: getEnv("SURREALDB_AUTH_LEVEL", "root"),

		// Embedding (default to local Ollama)
		EmbedProvider:  LLMProvider(getEnv("KNOWHOW_EMBED_PROVIDER", "ollama")),
		EmbedModel:     getEnv("KNOWHOW_EMBED_MODEL", "all-minilm:l6-v2"),
		EmbedDimension: getEnvInt("KNOWHOW_EMBED_DIMENSION", 384),

		// LLM (default to local Ollama)
		LLMProvider: LLMProvider(getEnv("KNOWHOW_LLM_PROVIDER", "ollama")),
		LLMModel:    getEnv("KNOWHOW_LLM_MODEL", "llama3.2"),

		// Provider hosts/keys
		OllamaHost:      getEnv("OLLAMA_HOST", "http://localhost:11434"),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),

		// Logging
		LogFile:  getEnv("KNOWHOW_LOG_FILE", "/tmp/knowhow.log"),
		LogLevel: parseLogLevel(getEnv("KNOWHOW_LOG_LEVEL", "INFO")),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
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
