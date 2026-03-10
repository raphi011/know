// Package config handles configuration loading from environment variables.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/raphi011/knowhow/internal/parser"
)

// LLMProvider identifies the LLM provider.
type LLMProvider string

const (
	ProviderNone      LLMProvider = "none"
	ProviderOllama    LLMProvider = "ollama"
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic"
	ProviderBedrock   LLMProvider = "bedrock"
	ProviderGoogleAI  LLMProvider = "googleai"
)

// Enabled returns true if the provider is configured (not empty or "none").
func (p LLMProvider) Enabled() bool {
	return p != ProviderNone && p != ""
}

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
	EmbedProvider             LLMProvider
	EmbedModel                string
	EmbedDimension            int
	BedrockEmbedModelProvider string // e.g., "amazon" for Titan, "cohere" for Cohere

	// LLM configuration (for ask, extract-graph, render)
	LLMProvider LLMProvider
	LLMModel    string

	// Provider-specific settings
	OllamaHost      string
	OpenAIAPIKey    string
	AnthropicAPIKey string
	GoogleAIAPIKey  string
	TavilyAPIKey    string

	// Logging
	LogFile  string
	LogLevel slog.Level

	// Server settings
	IngestConcurrency int
	NoAuth            bool // bypass token auth (for local/Docker use)
	MCPEnabled        bool // serve MCP endpoint at /mcp (default: true)

	// SSH/SFTP server
	SSHEnabled     bool   // KNOWHOW_SSH_ENABLED (default: false)
	SSHPort        string // KNOWHOW_SSH_PORT (default: "2222")
	SSHHostKeyPath string // KNOWHOW_SSH_HOST_KEY (default: "" = auto-generate)

	// Embedding worker settings
	EmbedWorkerInterval int // seconds between worker ticks (default: 5)
	EmbedWorkerBatch    int // max chunks per tick (default: 10)

	// Processing worker settings
	ProcessingWorkerInterval int // seconds between processing ticks (default: 2)
	ProcessingWorkerBatch    int // max documents per tick (default: 20)

	// Chunking settings
	ChunkThreshold  int // only chunk if content exceeds this length (default: 6000)
	ChunkTargetSize int // ideal chunk size in chars (default: 3000)
	ChunkMaxSize    int // maximum chunk size in chars (default: 4000)

	// Versioning settings
	VersionCoalesceMinutes int // minutes between version snapshots (default: 10)
	VersionRetentionCount  int // max versions per document (default: 50)

	// TLS settings
	TLSSkipVerify bool // skip TLS verification for Bedrock proxy (KNOWHOW_TLS_SKIP_VERIFY)
}

// ChunkConfig returns the chunking configuration as a parser.ChunkConfig.
func (c Config) ChunkConfig() parser.ChunkConfig {
	return parser.ChunkConfig{
		Threshold:  c.ChunkThreshold,
		TargetSize: c.ChunkTargetSize,
		MaxSize:    c.ChunkMaxSize,
	}
}

// Load reads configuration from environment variables.
func Load() Config {
	return Config{
		// SurrealDB
		SurrealDBURL:       getEnv("SURREALDB_URL", "ws://localhost:4002/rpc"),
		SurrealDBNamespace: getEnv("SURREALDB_NAMESPACE", "knowledge"),
		SurrealDBDatabase:  getEnv("SURREALDB_DATABASE", "graph"),
		SurrealDBUser:      getEnv("SURREALDB_USER", "root"),
		SurrealDBPass:      getEnv("SURREALDB_PASS", "root"),
		SurrealDBAuthLevel: getEnv("SURREALDB_AUTH_LEVEL", "root"),

		// Embedding (default to none — configure per instance)
		EmbedProvider:             LLMProvider(getEnv("KNOWHOW_EMBED_PROVIDER", "none")),
		EmbedModel:                getEnv("KNOWHOW_EMBED_MODEL", ""),
		EmbedDimension:            getEnvInt("KNOWHOW_EMBED_DIMENSION", 768),
		BedrockEmbedModelProvider: getEnv("KNOWHOW_BEDROCK_EMBED_MODEL_PROVIDER", ""),

		// LLM (default to Anthropic)
		LLMProvider: LLMProvider(getEnv("KNOWHOW_LLM_PROVIDER", "anthropic")),
		LLMModel:    getEnv("KNOWHOW_LLM_MODEL", "claude-sonnet-4-20250514"),

		// Provider hosts/keys
		OllamaHost:           getEnv("OLLAMA_HOST", "http://localhost:11434"),
		OpenAIAPIKey:         getEnv("OPENAI_API_KEY", ""),
		AnthropicAPIKey:      getEnv("ANTHROPIC_API_KEY", ""),
		GoogleAIAPIKey:       getEnv("GOOGLE_AI_API_KEY", ""),
		TavilyAPIKey: getEnv("TAVILY_API_KEY", ""),

		// Logging
		LogFile:  getEnv("KNOWHOW_LOG_FILE", "/tmp/knowhow.log"),
		LogLevel: parseLogLevel(getEnv("KNOWHOW_LOG_LEVEL", "INFO")),

		// Server settings
		IngestConcurrency: getEnvInt("KNOWHOW_INGEST_CONCURRENCY", 4),
		NoAuth:            getEnvBool("KNOWHOW_NO_AUTH", false),
		MCPEnabled:        getEnvBool("KNOWHOW_MCP_ENABLED", true),
		SSHEnabled:        getEnvBool("KNOWHOW_SSH_ENABLED", false),
		SSHPort:           getEnv("KNOWHOW_SSH_PORT", "2222"),
		SSHHostKeyPath:    getEnv("KNOWHOW_SSH_HOST_KEY", ""),

		// Embedding worker
		EmbedWorkerInterval: getEnvInt("KNOWHOW_EMBED_WORKER_INTERVAL", 5),
		EmbedWorkerBatch:    getEnvInt("KNOWHOW_EMBED_WORKER_BATCH", 10),

		// Processing worker
		ProcessingWorkerInterval: getEnvInt("KNOWHOW_PROCESSING_WORKER_INTERVAL", 2),
		ProcessingWorkerBatch:    getEnvInt("KNOWHOW_PROCESSING_WORKER_BATCH", 20),

		// Chunking
		ChunkThreshold:  getEnvInt("KNOWHOW_CHUNK_THRESHOLD", 6000),
		ChunkTargetSize: getEnvInt("KNOWHOW_CHUNK_TARGET_SIZE", 3000),
		ChunkMaxSize:    getEnvInt("KNOWHOW_CHUNK_MAX_SIZE", 4000),

		// Versioning
		VersionCoalesceMinutes: getEnvInt("KNOWHOW_VERSION_COALESCE_MINUTES", 10),
		VersionRetentionCount:  getEnvInt("KNOWHOW_VERSION_RETENTION", 50),

		// TLS
		TLSSkipVerify: getEnvBool("KNOWHOW_TLS_SKIP_VERIFY", false),
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
		i, err := strconv.Atoi(val)
		if err != nil {
			slog.Warn("invalid integer env var, using default", "key", key, "value", val, "default", defaultVal, "error", err)
			return defaultVal
		}
		return i
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			slog.Warn("invalid boolean env var, using default", "key", key, "value", val, "default", defaultVal, "error", err)
			return defaultVal
		}
		return b
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
