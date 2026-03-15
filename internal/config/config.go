// Package config handles configuration loading from environment variables.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/parser"
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

	// Multimodal embedding (optional, for native image/audio/PDF embedding)
	MultimodalEmbedProvider LLMProvider // KNOW_MULTIMODAL_EMBED_PROVIDER (default: "none")
	MultimodalEmbedModel    string      // KNOW_MULTIMODAL_EMBED_MODEL

	// Audio chunking
	AudioSegmentSeconds int // max audio segment duration in seconds (default: 60, max 80 for Gemini)

	// LLM configuration (for ask, extract-graph, render)
	LLMProvider      LLMProvider
	LLMModel         string
	LLMContextWindow int // override context window size, takes priority over registry (0 = use registry)

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
	SSHEnabled     bool   // KNOW_SSH_ENABLED (default: false)
	SSHPort        string // KNOW_SSH_PORT (default: "2222")
	SSHHostKeyPath string // KNOW_SSH_HOST_KEY (default: "" = auto-generate)

	// NFS server
	NFSEnabled bool   // KNOW_NFS_ENABLED (default: false)
	NFSPort    string // KNOW_NFS_PORT (default: "2049")

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

	// Embedding input limit
	EmbedMaxInputChars int // max chars per embedding API call, 0 = no limit (KNOW_EMBED_MAX_INPUT_CHARS)

	// Versioning settings
	VersionCoalesceMinutes int // minutes between version snapshots (default: 10)
	VersionRetentionCount  int // max versions per document (default: 50)

	// TLS settings
	TLSSkipVerify bool // skip TLS verification for Bedrock proxy (KNOW_TLS_SKIP_VERIFY)

	// Build info (set by ldflags, passed in by caller)
	Version string
	Commit  string
}

// ChunkConfig returns the raw chunking configuration as a parser.ChunkConfig.
func (c Config) ChunkConfig() parser.ChunkConfig {
	return parser.ChunkConfig{
		Threshold:  c.ChunkThreshold,
		TargetSize: c.ChunkTargetSize,
		MaxSize:    c.ChunkMaxSize,
	}
}

// maxEmbedContextOverhead is the estimated worst-case size of the contextual
// prefix prepended to chunks before embedding ("Document: …\nSection: …\n\n").
const maxEmbedContextOverhead = 250

// EffectiveChunkConfig returns chunk config adjusted for the embedding model's
// input limit. If EmbedMaxInputChars is set, MaxSize, TargetSize, and Threshold
// are capped to leave room for the contextual prefix (doc title + section heading).
//
// The contextual prefix is built by buildEmbeddingContext in document/service.go
// — keep maxEmbedContextOverhead in sync with that format.
func (c Config) EffectiveChunkConfig() parser.ChunkConfig {
	cc := c.ChunkConfig()
	if c.EmbedMaxInputChars > 0 {
		contentBudget := max(c.EmbedMaxInputChars-maxEmbedContextOverhead, 100)
		if cc.MaxSize > contentBudget {
			slog.Info("chunk MaxSize capped by embed input limit",
				"configured", cc.MaxSize, "effective", contentBudget,
				"embed_max_input_chars", c.EmbedMaxInputChars)
			cc.MaxSize = contentBudget
		}
		if cc.Threshold > contentBudget {
			slog.Info("chunk Threshold capped by embed input limit",
				"configured", cc.Threshold, "effective", contentBudget,
				"embed_max_input_chars", c.EmbedMaxInputChars)
			cc.Threshold = contentBudget
		}
		if cc.TargetSize >= cc.MaxSize {
			cc.TargetSize = cc.MaxSize * 3 / 4
		}
	}
	return cc
}

// Load reads configuration from environment variables.
func Load() Config {
	embedMaxInputChars := getEnvInt("KNOW_EMBED_MAX_INPUT_CHARS", 0)
	if embedMaxInputChars < 0 {
		slog.Warn("KNOW_EMBED_MAX_INPUT_CHARS is negative, treating as 0 (no limit)",
			"configured", embedMaxInputChars)
		embedMaxInputChars = 0
	}

	llmContextWindow := getEnvInt("KNOW_LLM_CONTEXT_WINDOW", 0)
	if llmContextWindow < 0 {
		slog.Warn("KNOW_LLM_CONTEXT_WINDOW is negative, treating as 0 (use registry default)",
			"configured", llmContextWindow)
		llmContextWindow = 0
	}

	return Config{
		// SurrealDB
		SurrealDBURL:       getEnv("SURREALDB_URL", "ws://localhost:4002/rpc"),
		SurrealDBNamespace: getEnv("SURREALDB_NAMESPACE", "knowledge"),
		SurrealDBDatabase:  getEnv("SURREALDB_DATABASE", "graph"),
		SurrealDBUser:      getEnv("SURREALDB_USER", "root"),
		SurrealDBPass:      getEnv("SURREALDB_PASS", "root"),
		SurrealDBAuthLevel: getEnv("SURREALDB_AUTH_LEVEL", "root"),

		// Embedding (default to none — configure per instance)
		EmbedProvider:             LLMProvider(getEnv("KNOW_EMBED_PROVIDER", "none")),
		EmbedModel:                getEnv("KNOW_EMBED_MODEL", ""),
		EmbedDimension:            getEnvInt("KNOW_EMBED_DIMENSION", 768),
		BedrockEmbedModelProvider: getEnv("KNOW_BEDROCK_EMBED_MODEL_PROVIDER", ""),

		// Multimodal embedding
		MultimodalEmbedProvider: LLMProvider(getEnv("KNOW_MULTIMODAL_EMBED_PROVIDER", "none")),
		MultimodalEmbedModel:    getEnv("KNOW_MULTIMODAL_EMBED_MODEL", ""),

		// Audio chunking
		AudioSegmentSeconds: getEnvInt("KNOW_AUDIO_SEGMENT_SECONDS", 60),

		// LLM (default to Anthropic)
		LLMProvider:      LLMProvider(getEnv("KNOW_LLM_PROVIDER", "anthropic")),
		LLMModel:         getEnv("KNOW_LLM_MODEL", "claude-sonnet-4-20250514"),
		LLMContextWindow: llmContextWindow,

		// Provider hosts/keys
		OllamaHost:      getEnv("OLLAMA_HOST", "http://localhost:11434"),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		GoogleAIAPIKey:  getEnv("GOOGLE_AI_API_KEY", ""),
		TavilyAPIKey:    getEnv("TAVILY_API_KEY", ""),

		// Logging
		LogFile:  getEnv("KNOW_LOG_FILE", ""),
		LogLevel: parseLogLevel(getEnv("KNOW_LOG_LEVEL", "INFO")),

		// Server settings
		IngestConcurrency: getEnvInt("KNOW_INGEST_CONCURRENCY", 4),
		NoAuth:            getEnvBool("KNOW_NO_AUTH", false),
		MCPEnabled:        getEnvBool("KNOW_MCP_ENABLED", true),
		SSHEnabled:        getEnvBool("KNOW_SSH_ENABLED", false),
		SSHPort:           getEnv("KNOW_SSH_PORT", "2222"),
		SSHHostKeyPath:    getEnv("KNOW_SSH_HOST_KEY", ""),
		NFSEnabled:        getEnvBool("KNOW_NFS_ENABLED", false),
		NFSPort:           getEnv("KNOW_NFS_PORT", "2049"),

		// Embedding worker
		EmbedWorkerInterval: getEnvInt("KNOW_EMBED_WORKER_INTERVAL", 5),
		EmbedWorkerBatch:    getEnvInt("KNOW_EMBED_WORKER_BATCH", 10),

		// Processing worker
		ProcessingWorkerInterval: getEnvInt("KNOW_PROCESSING_WORKER_INTERVAL", 2),
		ProcessingWorkerBatch:    getEnvInt("KNOW_PROCESSING_WORKER_BATCH", 20),

		// Chunking
		ChunkThreshold:  getEnvInt("KNOW_CHUNK_THRESHOLD", 6000),
		ChunkTargetSize: getEnvInt("KNOW_CHUNK_TARGET_SIZE", 3000),
		ChunkMaxSize:    getEnvInt("KNOW_CHUNK_MAX_SIZE", 4000),

		// Embedding input limit (0 = no limit; Cohere Embed v3 on Bedrock: 2048)
		EmbedMaxInputChars: embedMaxInputChars,

		// Versioning
		VersionCoalesceMinutes: getEnvInt("KNOW_VERSION_COALESCE_MINUTES", 10),
		VersionRetentionCount:  getEnvInt("KNOW_VERSION_RETENTION", 50),

		// TLS
		TLSSkipVerify: getEnvBool("KNOW_TLS_SKIP_VERIFY", false),
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
