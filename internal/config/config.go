// Package config handles configuration loading from environment variables.
package config

import (
	"fmt"
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

	// PDF processing
	TextExtractorModel string // KNOW_TEXT_EXTRACTOR_MODEL (default: "gemini-2.0-flash")
	PDFRenderDPI       int    // KNOW_PDF_RENDER_DPI (default: 300)

	// Audio chunking
	AudioSegmentSeconds int // max audio segment duration in seconds (default: 60, max 80 for Gemini)

	// Speech-to-text configuration
	STTProvider LLMProvider // KNOW_STT_PROVIDER (default: "none")
	STTModel    string      // KNOW_STT_MODEL (default: "gpt-4o-transcribe")
	STTBaseURL  string      // KNOW_STT_BASE_URL (default: "" = OpenAI API)

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

	// Auth settings
	TokenMaxLifetimeDays int // KNOW_TOKEN_MAX_LIFETIME_DAYS (default: 90, 0 = no limit)

	// OIDC authentication
	OIDCEnabled       bool   // KNOW_OIDC_ENABLED (default: false)
	OIDCIssuerURL     string // KNOW_OIDC_ISSUER_URL
	OIDCClientID      string // KNOW_OIDC_CLIENT_ID
	OIDCClientSecret  string // KNOW_OIDC_CLIENT_SECRET
	OIDCRedirectURL   string // KNOW_OIDC_REDIRECT_URL
	OIDCProviderName  string // KNOW_OIDC_PROVIDER_NAME (default: derived from issuer URL)
	SelfSignupEnabled bool   // KNOW_SELF_SIGNUP_ENABLED (default: false)

	// Server settings
	Environment       string // KNOW_ENVIRONMENT ("development" or "production", default: "development")
	IngestConcurrency int
	NoAuth            bool   // bypass token auth (for local/Docker use)
	MCPEnabled        bool   // serve MCP endpoint at /mcp (default: true)
	ProtocolPort      string // KNOW_PROTOCOL_PORT — separate port for WebDAV + MCP (default: "4002")
	MetricsPort       string // KNOW_METRICS_PORT — separate port for /metrics (default: "" = disabled)

	// SSH/SFTP server
	SSHEnabled     bool   // KNOW_SSH_ENABLED (default: false)
	SSHPort        string // KNOW_SSH_PORT (default: "2222")
	SSHHostKeyPath string // KNOW_SSH_HOST_KEY (default: "" = auto-generate)

	// NFS server
	NFSEnabled bool   // KNOW_NFS_ENABLED (default: false)
	NFSPort    string // KNOW_NFS_PORT (default: "2049")

	// Pipeline worker settings
	PipelineWorkerInterval int // seconds between worker ticks (KNOW_PIPELINE_WORKER_INTERVAL, default: 5)
	PipelineWorkerBatch    int // max jobs per tick (KNOW_PIPELINE_WORKER_BATCH, default: 10)

	// Chunking settings
	ChunkThreshold  int // only chunk if content exceeds this length (default: 6000)
	ChunkTargetSize int // ideal chunk size in chars (default: 3000)
	ChunkMaxSize    int // maximum chunk size in chars (default: 4000)

	// Embedding input limit
	EmbedMaxInputChars int // max chars per embedding API call, 0 = no limit (KNOW_EMBED_MAX_INPUT_CHARS)

	// Versioning settings
	VersionCoalesceMinutes int // minutes between version snapshots (default: 10)
	VersionRetentionCount  int // max versions per document (default: 50)

	// Apify
	ApifyToken string // KNOW_APIFY_TOKEN — enables YouTube transcript tool

	// Jina Reader
	JinaAPIKey string // KNOW_JINA_API_KEY — optional, enables higher rate limits for web clipping

	// TLS settings
	TLSSkipVerify bool // skip TLS verification for Bedrock proxy (KNOW_TLS_SKIP_VERIFY)

	// Blob storage
	BlobStore      string // KNOW_BLOB_STORE (default: "fs")
	BlobDir        string // KNOW_BLOB_DIR (default: "/data/blobs")
	BlobS3Bucket   string // KNOW_BLOB_S3_BUCKET
	BlobS3Prefix   string // KNOW_BLOB_S3_PREFIX (default: "blobs")
	BlobS3Endpoint string // KNOW_BLOB_S3_ENDPOINT
	BlobS3Region   string // KNOW_BLOB_S3_REGION (default: "us-east-1")

	// Build info (set by ldflags, passed in by caller)
	Version string
	Commit  string
}

// ValidateOIDC checks that OIDC configuration is consistent.
// Returns an error if OIDC is enabled but required fields are missing.
func (c Config) ValidateOIDC() error {
	if !c.OIDCEnabled {
		return nil
	}
	if c.OIDCIssuerURL == "" {
		return fmt.Errorf("oidc config: KNOW_OIDC_ISSUER_URL is required when OIDC is enabled")
	}
	if c.OIDCClientID == "" {
		return fmt.Errorf("oidc config: KNOW_OIDC_CLIENT_ID is required when OIDC is enabled")
	}
	if c.OIDCRedirectURL == "" {
		return fmt.Errorf("oidc config: KNOW_OIDC_REDIRECT_URL is required when OIDC is enabled")
	}
	// OIDCClientSecret is not validated — public OIDC clients using PKCE may not have one.
	return nil
}

// IsProduction returns true if the server is running in production mode.
func (c Config) IsProduction() bool {
	return c.Environment == "production"
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

		// PDF processing
		TextExtractorModel: getEnv("KNOW_TEXT_EXTRACTOR_MODEL", "gemini-2.0-flash"),
		PDFRenderDPI:       getEnvInt("KNOW_PDF_RENDER_DPI", 300),

		// Audio chunking
		AudioSegmentSeconds: getEnvInt("KNOW_AUDIO_SEGMENT_SECONDS", 60),

		// Speech-to-text
		STTProvider: LLMProvider(getEnv("KNOW_STT_PROVIDER", "none")),
		STTModel:    getEnv("KNOW_STT_MODEL", "gpt-4o-transcribe"),
		STTBaseURL:  getEnv("KNOW_STT_BASE_URL", ""),

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

		// Auth settings
		TokenMaxLifetimeDays: getEnvInt("KNOW_TOKEN_MAX_LIFETIME_DAYS", 90),

		// OIDC authentication
		OIDCEnabled:       getEnvBool("KNOW_OIDC_ENABLED", false),
		OIDCIssuerURL:     getEnv("KNOW_OIDC_ISSUER_URL", ""),
		OIDCClientID:      getEnv("KNOW_OIDC_CLIENT_ID", ""),
		OIDCClientSecret:  getEnv("KNOW_OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURL:   getEnv("KNOW_OIDC_REDIRECT_URL", ""),
		OIDCProviderName:  getEnv("KNOW_OIDC_PROVIDER_NAME", ""),
		SelfSignupEnabled: getEnvBool("KNOW_SELF_SIGNUP_ENABLED", false),

		// Server settings
		Environment:       getEnv("KNOW_ENVIRONMENT", "development"),
		IngestConcurrency: getEnvInt("KNOW_INGEST_CONCURRENCY", 4),
		NoAuth:            getEnvBool("KNOW_NO_AUTH", false),
		MCPEnabled:        getEnvBool("KNOW_MCP_ENABLED", true),
		ProtocolPort:      getEnv("KNOW_PROTOCOL_PORT", "4002"),
		SSHEnabled:        getEnvBool("KNOW_SSH_ENABLED", false),
		SSHPort:           getEnv("KNOW_SSH_PORT", "2222"),
		SSHHostKeyPath:    getEnv("KNOW_SSH_HOST_KEY", ""),
		MetricsPort:       getEnv("KNOW_METRICS_PORT", ""),
		NFSEnabled:        getEnvBool("KNOW_NFS_ENABLED", false),
		NFSPort:           getEnv("KNOW_NFS_PORT", "2049"),

		// Pipeline worker
		PipelineWorkerInterval: getEnvInt("KNOW_PIPELINE_WORKER_INTERVAL", 5),
		PipelineWorkerBatch:    getEnvInt("KNOW_PIPELINE_WORKER_BATCH", 10),

		// Chunking
		ChunkThreshold:  getEnvInt("KNOW_CHUNK_THRESHOLD", 6000),
		ChunkTargetSize: getEnvInt("KNOW_CHUNK_TARGET_SIZE", 3000),
		ChunkMaxSize:    getEnvInt("KNOW_CHUNK_MAX_SIZE", 4000),

		// Embedding input limit (0 = no limit; Cohere Embed v3 on Bedrock: 2048)
		EmbedMaxInputChars: embedMaxInputChars,

		// Versioning
		VersionCoalesceMinutes: getEnvInt("KNOW_VERSION_COALESCE_MINUTES", 10),
		VersionRetentionCount:  getEnvInt("KNOW_VERSION_RETENTION", 50),

		// Apify
		ApifyToken: getEnv("KNOW_APIFY_TOKEN", ""),

		// Jina Reader
		JinaAPIKey: getEnv("KNOW_JINA_API_KEY", ""),

		// TLS
		TLSSkipVerify: getEnvBool("KNOW_TLS_SKIP_VERIFY", false),

		// Blob storage
		BlobStore:      getEnv("KNOW_BLOB_STORE", "fs"),
		BlobDir:        getEnv("KNOW_BLOB_DIR", "/data/blobs"),
		BlobS3Bucket:   getEnv("KNOW_BLOB_S3_BUCKET", ""),
		BlobS3Prefix:   getEnv("KNOW_BLOB_S3_PREFIX", "blobs"),
		BlobS3Endpoint: getEnv("KNOW_BLOB_S3_ENDPOINT", ""),
		BlobS3Region:   getEnv("KNOW_BLOB_S3_REGION", "us-east-1"),
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
