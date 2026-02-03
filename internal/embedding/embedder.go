// Package embedding provides text embedding generation with multiple backend support.
package embedding

import (
	"context"
	"fmt"
)

// Embedder defines the interface for text embedding providers.
// Implementations include Ollama (local) and Anthropic/Claude (API).
type Embedder interface {
	// Embed generates an embedding vector for a single text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts.
	// More efficient than multiple Embed calls for bulk operations.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Model returns the name of the embedding model being used.
	Model() string

	// Dimension returns the embedding vector dimension.
	// CRITICAL: Must match HNSW index dimension in SurrealDB schema.
	Dimension() int
}

// ProviderType identifies the embedding provider.
type ProviderType string

const (
	// ProviderOllama uses local Ollama server for embeddings.
	ProviderOllama ProviderType = "ollama"

	// ProviderAnthropic uses Anthropic API for embeddings (Claude).
	ProviderAnthropic ProviderType = "anthropic"
)

// Config holds configuration for creating an Embedder.
type Config struct {
	// Provider specifies which embedding backend to use.
	Provider ProviderType

	// Model is the embedding model name (provider-specific).
	// Ollama: "all-minilm:l6-v2" (384-dim), "nomic-embed-text" (768-dim)
	// Anthropic: "claude-3-haiku-20240307" (future embedding support)
	Model string

	// ExpectedDimension is the required output dimension.
	// Set to 0 to use provider's default.
	ExpectedDimension int

	// Anthropic-specific
	AnthropicAPIKey string

	// Ollama-specific (uses OLLAMA_HOST env var if empty)
	OllamaHost string
}

// New creates an Embedder based on the provided configuration.
func New(cfg Config) (Embedder, error) {
	switch cfg.Provider {
	case ProviderOllama, "":
		// Default to Ollama
		return NewOllamaClient(cfg.Model, cfg.ExpectedDimension)

	case ProviderAnthropic:
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("anthropic provider requires API key")
		}
		return NewAnthropicClient(cfg.AnthropicAPIKey, cfg.Model, cfg.ExpectedDimension)

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}

// DefaultOllama returns the default Ollama embedder (all-minilm:l6-v2, 384-dim).
func DefaultOllama() (Embedder, error) {
	return NewOllamaClient("", 0)
}
