package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"google.golang.org/genai"
)

// geminiEmbedBatchLimit is the maximum number of items per EmbedContent call.
// Gemini limits multimodal embedding requests to 6 items per batch.
const geminiEmbedBatchLimit = 6

// GeminiMultimodalEmbedder embeds images, audio, PDFs and text
// using the Gemini embedding API with native multimodal support.
type GeminiMultimodalEmbedder struct {
	client    *genai.Client
	modelName string
	dimension int
}

// NewGeminiMultimodalEmbedder creates a multimodal embedder backed by Gemini.
func NewGeminiMultimodalEmbedder(ctx context.Context, apiKey, modelName string, dimension int) (*GeminiMultimodalEmbedder, error) {
	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}
	return &GeminiMultimodalEmbedder{
		client:    client,
		modelName: modelName,
		dimension: dimension,
	}, nil
}

// EmbedMultimodal embeds a batch of multimodal items using the Gemini embedding API.
// Items are split into sub-batches of at most geminiEmbedBatchLimit items each.
func (g *GeminiMultimodalEmbedder) EmbedMultimodal(ctx context.Context, items []MultimodalInput) ([][]float64, error) {
	if len(items) == 0 {
		return [][]float64{}, nil
	}

	logger := logutil.FromCtx(ctx).With("model", g.modelName)
	logger.Debug("multimodal embedding starting", "batch_size", len(items), "dimension", g.dimension)

	start := time.Now()
	result := make([][]float64, 0, len(items))

	for offset := 0; offset < len(items); offset += geminiEmbedBatchLimit {
		end := min(offset+geminiEmbedBatchLimit, len(items))
		sub := items[offset:end]

		vecs, err := g.embedBatch(ctx, sub)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", offset, end, err)
		}
		result = append(result, vecs...)
	}

	logger.Debug("multimodal embedding complete",
		"batch_size", len(items),
		"duration_ms", time.Since(start).Milliseconds(),
		"dimension", g.dimension,
	)

	return result, nil
}

// embedBatch sends a single sub-batch (≤ geminiEmbedBatchLimit items) to the API.
func (g *GeminiMultimodalEmbedder) embedBatch(ctx context.Context, items []MultimodalInput) ([][]float64, error) {
	contents := make([]*genai.Content, len(items))
	for i, item := range items {
		parts := make([]*genai.Part, 0, 2)
		parts = append(parts, genai.NewPartFromBytes(item.Data, item.MimeType))
		if item.Text != "" {
			parts = append(parts, genai.NewPartFromText(item.Text))
		}
		contents[i] = genai.NewContentFromParts(parts, genai.RoleUser)
	}

	cfg := &genai.EmbedContentConfig{
		TaskType: "RETRIEVAL_DOCUMENT",
	}
	if g.dimension > 0 {
		dim := int32(g.dimension)
		cfg.OutputDimensionality = &dim
	}

	resp, err := g.client.Models.EmbedContent(ctx, g.modelName, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini embed content: %w", err)
	}

	if len(resp.Embeddings) != len(items) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d", len(resp.Embeddings), len(items))
	}

	result := make([][]float64, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		result[i] = float32sToFloat64s(emb.Values)
	}
	return result, nil
}

// SupportsMIME returns true if Gemini can natively embed the given MIME type.
func (g *GeminiMultimodalEmbedder) SupportsMIME(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "application/pdf":
		return true
	}
	return strings.HasPrefix(mimeType, "audio/")
}
