// Package llm provides LLM and embedding services using langchaingo.
package llm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/metrics"
	"github.com/tmc/langchaingo/embeddings"
	bedrockembed "github.com/tmc/langchaingo/embeddings/bedrock"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// Embedder wraps langchaingo embeddings with dimension validation.
type Embedder struct {
	model     embeddings.Embedder
	dimension int
	modelName string
	metrics   *metrics.Collector
}

// NewEmbedder creates an embedder based on configuration.
// If mc is nil, metrics recording is disabled.
func NewEmbedder(ctx context.Context, cfg config.Config, mc *metrics.Collector) (*Embedder, error) {
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

	case config.ProviderBedrock:
		// AWS SDK picks up env vars: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
		// AWS_REGION, HTTPS_PROXY, AWS_CA_BUNDLE
		model, err = newBedrockEmbedder(ctx, cfg.EmbedModel, cfg.BedrockEmbedModelProvider)
		if err != nil {
			return nil, fmt.Errorf("create bedrock embedder: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.EmbedProvider)
	}

	return &Embedder{
		model:     model,
		dimension: cfg.EmbedDimension,
		modelName: cfg.EmbedModel,
		metrics:   mc,
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

	if e.metrics != nil {
		e.metrics.RecordTiming(metrics.OpEmbedding, duration)
	}

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	start := time.Now()
	vectors, err := e.model.EmbedDocuments(ctx, texts)
	duration := time.Since(start)

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

	// Record one timing entry per batch (not per text)
	if e.metrics != nil {
		e.metrics.RecordTiming(metrics.OpEmbedding, duration)
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

// bedrockEmbedder wraps langchaingo's bedrock embedder with ARN support.
// The standard bedrock embedder can't detect provider from ARN-based model IDs.
type bedrockEmbedder struct {
	client   *bedrockruntime.Client
	modelID  string
	provider string // "amazon" or "cohere"
}

// newBedrockEmbedder creates a Bedrock embedder that supports inference profile ARNs.
// If providerHint is empty and modelID is an ARN, returns error.
func newBedrockEmbedder(ctx context.Context, modelID, providerHint string) (embeddings.Embedder, error) {
	// Determine provider
	provider := providerHint
	if provider == "" {
		// Try to detect from model ID (works for standard IDs like "amazon.titan-embed-text-v2")
		if strings.HasPrefix(modelID, "amazon.") {
			provider = "amazon"
		} else if strings.HasPrefix(modelID, "cohere.") {
			provider = "cohere"
		} else if strings.HasPrefix(modelID, "arn:") {
			return nil, fmt.Errorf("KNOWHOW_BEDROCK_EMBED_MODEL_PROVIDER required for ARN-based model: %s", modelID)
		} else {
			provider = strings.Split(modelID, ".")[0]
		}
	}

	if provider != "amazon" && provider != "cohere" {
		return nil, fmt.Errorf("unsupported bedrock embedding provider: %s (use 'amazon' or 'cohere')", provider)
	}

	// Create AWS client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := bedrockruntime.NewFromConfig(awsCfg)

	return &bedrockEmbedder{
		client:   client,
		modelID:  modelID,
		provider: provider,
	}, nil
}

// EmbedDocuments implements embeddings.Embedder.
func (b *bedrockEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	start := time.Now()
	totalChars := 0
	for _, t := range texts {
		totalChars += len(t)
	}
	slog.Debug("bedrock embedding starting", "provider", b.provider, "texts", len(texts), "total_chars", totalChars)

	var vecs [][]float32
	var err error

	switch b.provider {
	case "amazon":
		vecs, err = bedrockembed.FetchAmazonTextEmbeddings(ctx, b.client, b.modelID, texts)
	case "cohere":
		vecs, err = bedrockembed.FetchCohereTextEmbeddings(ctx, b.client, b.modelID, texts, bedrockembed.CohereInputTypeText)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", b.provider)
	}

	duration := time.Since(start)
	if err != nil {
		slog.Warn("bedrock embedding failed", "provider", b.provider, "duration_ms", duration.Milliseconds(), "error", err)
		return nil, err
	}
	slog.Debug("bedrock embedding complete", "provider", b.provider, "texts", len(texts), "duration_ms", duration.Milliseconds())
	return vecs, nil
}

// EmbedQuery implements embeddings.Embedder.
func (b *bedrockEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	var vecs [][]float32
	var err error

	switch b.provider {
	case "amazon":
		vecs, err = bedrockembed.FetchAmazonTextEmbeddings(ctx, b.client, b.modelID, []string{text})
	case "cohere":
		vecs, err = bedrockembed.FetchCohereTextEmbeddings(ctx, b.client, b.modelID, []string{text}, bedrockembed.CohereInputTypeQuery)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", b.provider)
	}

	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vecs[0], nil
}
