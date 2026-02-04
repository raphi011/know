// Package llm provides LLM and embedding services using langchaingo.
package llm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// Embedder wraps langchaingo embeddings with dimension validation.
type Embedder struct {
	model     embeddings.Embedder
	dimension int
	modelName string
}

// NewEmbedder creates an embedder based on configuration.
func NewEmbedder(cfg config.Config) (*Embedder, error) {
	var model embeddings.Embedder
	var err error

	switch cfg.EmbedProvider {
	case config.ProviderOllama:
		llm, ollamaErr := ollama.New(
			ollama.WithModel(cfg.EmbedModel),
			ollama.WithServerURL(cfg.OllamaHost),
		)
		if ollamaErr != nil {
			return nil, fmt.Errorf("create ollama client: %w", ollamaErr)
		}
		model, err = embeddings.NewEmbedder(llm)
		if err != nil {
			return nil, fmt.Errorf("create ollama embedder: %w", err)
		}

	case config.ProviderOpenAI:
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OpenAI API key required")
		}
		llm, openaiErr := openai.New(
			openai.WithToken(cfg.OpenAIAPIKey),
			openai.WithEmbeddingModel(cfg.EmbedModel),
		)
		if openaiErr != nil {
			return nil, fmt.Errorf("create openai client: %w", openaiErr)
		}
		model, err = embeddings.NewEmbedder(llm)
		if err != nil {
			return nil, fmt.Errorf("create openai embedder: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.EmbedProvider)
	}

	return &Embedder{
		model:     model,
		dimension: cfg.EmbedDimension,
		modelName: cfg.EmbedModel,
	}, nil
}

// Embed generates an embedding vector for text.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	textLen := len(text)
	slog.Debug("embedding text", "model", e.modelName, "text_len", textLen)

	start := time.Now()
	vectors, err := e.model.EmbedDocuments(ctx, []string{text})
	duration := time.Since(start)

	if err != nil {
		slog.Warn("embedding failed", "model", e.modelName, "text_len", textLen, "duration_ms", duration.Milliseconds(), "error", err)
		return nil, fmt.Errorf("embed: %w", err)
	}

	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	embedding := vectors[0]
	if len(embedding) != e.dimension {
		return nil, fmt.Errorf("dimension mismatch: got %d, want %d", len(embedding), e.dimension)
	}

	slog.Debug("embedding complete", "model", e.modelName, "text_len", textLen, "duration_ms", duration.Milliseconds())
	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	vectors, err := e.model.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed batch: %w", err)
	}

	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("count mismatch: got %d, want %d", len(vectors), len(texts))
	}

	// Validate dimensions
	for i, v := range vectors {
		if len(v) != e.dimension {
			return nil, fmt.Errorf("embedding %d dimension mismatch: got %d, want %d", i, len(v), e.dimension)
		}
	}

	return vectors, nil
}

// Model returns the embedding model name.
func (e *Embedder) Model() string {
	return e.modelName
}

// Dimension returns the expected embedding dimension.
func (e *Embedder) Dimension() int {
	return e.dimension
}
