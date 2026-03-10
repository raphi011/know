// Package llm provides LLM and embedding services using eino.
package llm

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/cloudwego/eino/components/embedding"
	geminiembed "github.com/cloudwego/eino-ext/components/embedding/gemini"
	ollamaembed "github.com/cloudwego/eino-ext/components/embedding/ollama"
	openaiembed "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/metrics"
	bedrockembed "github.com/tmc/langchaingo/embeddings/bedrock"
	"google.golang.org/genai"
)

// einoEmbedder matches eino's embedding.Embedder interface.
type einoEmbedder interface {
	EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error)
}

// Embedder wraps eino embeddings with dimension validation.
type Embedder struct {
	model     einoEmbedder
	dimension int
	modelName string
	metrics   *metrics.Collector
}

// NewEmbedder creates an embedder based on configuration.
// If mc is nil, metrics recording is disabled.
func NewEmbedder(ctx context.Context, cfg config.Config, mc *metrics.Collector) (*Embedder, error) {
	var model einoEmbedder
	var err error

	switch cfg.EmbedProvider {
	case config.ProviderOllama:
		model, err = ollamaembed.NewEmbedder(ctx, &ollamaembed.EmbeddingConfig{
			BaseURL: cfg.OllamaHost,
			Model:   cfg.EmbedModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create ollama embedder: %w", err)
		}

	case config.ProviderGoogleAI:
		if cfg.GoogleAIAPIKey == "" {
			return nil, fmt.Errorf("google AI API key required (GOOGLE_AI_API_KEY)")
		}
		client, clientErr := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  cfg.GoogleAIAPIKey,
			Backend: genai.BackendGeminiAPI,
		})
		if clientErr != nil {
			return nil, fmt.Errorf("create google ai client: %w", clientErr)
		}
		dim := int32(cfg.EmbedDimension)
		model, err = geminiembed.NewEmbedder(ctx, &geminiembed.EmbeddingConfig{
			Client:               client,
			Model:                cfg.EmbedModel,
			OutputDimensionality: &dim,
		})
		if err != nil {
			return nil, fmt.Errorf("create google ai embedder: %w", err)
		}

	case config.ProviderOpenAI:
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("openAI API key required")
		}
		dim := cfg.EmbedDimension
		model, err = openaiembed.NewEmbedder(ctx, &openaiembed.EmbeddingConfig{
			APIKey:     cfg.OpenAIAPIKey,
			Model:      cfg.EmbedModel,
			Dimensions: &dim,
		})
		if err != nil {
			return nil, fmt.Errorf("create openai embedder: %w", err)
		}

	case config.ProviderAnthropic:
		// Anthropic embeddings via Voyage AI (OpenAI-compatible API).
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("anthropic API key required (used for Voyage AI embeddings)")
		}
		model, err = openaiembed.NewEmbedder(ctx, &openaiembed.EmbeddingConfig{
			APIKey:  cfg.AnthropicAPIKey,
			BaseURL: "https://api.voyageai.com/v1",
			Model:   cfg.EmbedModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create anthropic/voyage embedder: %w", err)
		}

	case config.ProviderBedrock:
		model, err = newBedrockEmbedder(ctx, cfg.EmbedModel, cfg.BedrockEmbedModelProvider, cfg.TLSSkipVerify)
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

// float64sToFloat32s converts a float64 slice to float32.
func float64sToFloat32s(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

// Embed generates an embedding vector for text.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	textLen := len(text)
	slog.Debug("embedding text", "model", e.modelName, "text_len", textLen)

	start := time.Now()
	vectors, err := e.model.EmbedStrings(ctx, []string{text})
	duration := time.Since(start)

	if err != nil {
		slog.Warn("embedding failed", "model", e.modelName, "text_len", textLen, "duration_ms", duration.Milliseconds(), "error", err)
		return nil, fmt.Errorf("embed: %w", err)
	}

	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	vec := vectors[0]
	if len(vec) != e.dimension {
		return nil, fmt.Errorf("dimension mismatch: got %d, want %d", len(vec), e.dimension)
	}

	slog.Debug("embedding complete", "model", e.modelName, "text_len", textLen, "duration_ms", duration.Milliseconds())

	if e.metrics != nil {
		e.metrics.RecordTiming(metrics.OpEmbedding, duration)
	}

	return float64sToFloat32s(vec), nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	start := time.Now()
	vectors, err := e.model.EmbedStrings(ctx, texts)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("embed batch: %w", err)
	}

	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("count mismatch: got %d, want %d", len(vectors), len(texts))
	}

	result := make([][]float32, len(vectors))
	for i, v := range vectors {
		if len(v) != e.dimension {
			return nil, fmt.Errorf("embedding %d dimension mismatch: got %d, want %d", i, len(v), e.dimension)
		}
		result[i] = float64sToFloat32s(v)
	}

	if e.metrics != nil {
		e.metrics.RecordTiming(metrics.OpEmbedding, duration)
	}

	return result, nil
}

// Model returns the embedding model name.
func (e *Embedder) Model() string {
	return e.modelName
}

// Dimension returns the expected embedding dimension.
func (e *Embedder) Dimension() int {
	return e.dimension
}

// bedrockEmbedder wraps langchaingo's bedrock embedder with ARN support,
// adapted to return [][]float64 for the einoEmbedder interface.
type bedrockEmbedder struct {
	client   *bedrockruntime.Client
	modelID  string
	provider string // "amazon" or "cohere"
}

// newBedrockEmbedder creates a Bedrock embedder that supports inference profile ARNs.
// If providerHint is empty and modelID is an ARN, returns error.
func newBedrockEmbedder(ctx context.Context, modelID, providerHint string, tlsSkipVerify bool) (*bedrockEmbedder, error) {
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

	// Create AWS client, optionally skipping TLS verification for local proxy.
	// See .env.example for required AWS env vars.
	var opts []func(*awsconfig.LoadOptions) error
	if tlsSkipVerify {
		slog.Warn("bedrock embedder: skipping TLS verification for local proxy")
		opts = append(opts, awsconfig.WithHTTPClient(
			awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
				if t.TLSClientConfig == nil {
					t.TLSClientConfig = &tls.Config{}
				}
				t.TLSClientConfig.InsecureSkipVerify = true
			}),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
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

// float32sToFloat64s converts a float32 slice to float64.
func float32sToFloat64s(in []float32) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

// EmbedStrings implements einoEmbedder.
func (b *bedrockEmbedder) EmbedStrings(ctx context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
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
		return nil, fmt.Errorf("bedrock %s embedding: %w", b.provider, err)
	}
	slog.Debug("bedrock embedding complete", "provider", b.provider, "texts", len(texts), "duration_ms", duration.Milliseconds())

	// Convert float32→float64 for einoEmbedder interface. The Embedder wrapper
	// converts back to float32 for storage. This round-trip is lossless but
	// required because langchaingo bedrock returns float32 while eino uses float64.
	result := make([][]float64, len(vecs))
	for i, v := range vecs {
		result[i] = float32sToFloat64s(v)
	}
	return result, nil
}
