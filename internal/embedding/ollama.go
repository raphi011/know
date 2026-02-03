package embedding

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

const (
	// DefaultOllamaModel is the embedding model that produces 384-dimensional vectors.
	DefaultOllamaModel = "all-minilm:l6-v2"

	// DefaultOllamaDimension is the dimension for all-minilm:l6-v2.
	// CRITICAL: This MUST match the HNSW index dimension in SurrealDB schema.
	DefaultOllamaDimension = 384
)

// OllamaClient implements Embedder using local Ollama server.
type OllamaClient struct {
	client    *api.Client
	model     string
	dimension int
}

// Compile-time check that OllamaClient implements Embedder.
var _ Embedder = (*OllamaClient)(nil)

// NewOllamaClient creates a new Ollama embedding client.
// If model is empty, uses DefaultOllamaModel (all-minilm:l6-v2).
// If expectedDimension is 0, uses DefaultOllamaDimension (384).
// Uses OLLAMA_HOST environment variable for server URL (defaults to http://localhost:11434).
func NewOllamaClient(model string, expectedDimension int) (*OllamaClient, error) {
	if model == "" {
		model = DefaultOllamaModel
	}
	if expectedDimension == 0 {
		expectedDimension = DefaultOllamaDimension
	}

	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("create ollama client: %w", err)
	}

	return &OllamaClient{
		client:    client,
		model:     model,
		dimension: expectedDimension,
	}, nil
}

// Model returns the configured embedding model name.
func (c *OllamaClient) Model() string {
	return c.model
}

// Dimension returns the expected embedding dimension.
func (c *OllamaClient) Dimension() int {
	return c.dimension
}

// Embed generates an embedding vector for the given text.
// Returns exactly dimension-sized float32 vector or error if dimension mismatch.
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.Embed(ctx, &api.EmbedRequest{
		Model: c.model,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	embedding := resp.Embeddings[0]
	if len(embedding) != c.dimension {
		return nil, fmt.Errorf("dimension mismatch: got %d, want %d (model: %s)",
			len(embedding), c.dimension, c.model)
	}

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts in a single request.
// More efficient than multiple Embed calls for bulk operations.
// All embeddings are verified to match the expected dimension.
func (c *OllamaClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	resp, err := c.client.Embed(ctx, &api.EmbedRequest{
		Model: c.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("embed batch: %w", err)
	}

	if len(resp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d",
			len(resp.Embeddings), len(texts))
	}

	// Verify all dimensions
	for i, emb := range resp.Embeddings {
		if len(emb) != c.dimension {
			return nil, fmt.Errorf("embedding %d dimension mismatch: got %d, want %d",
				i, len(emb), c.dimension)
		}
	}

	return resp.Embeddings, nil
}

// EmbedWithTruncation embeds text, truncating if necessary for very long inputs.
// Ollama models have context limits; this provides a safe wrapper.
func (c *OllamaClient) EmbedWithTruncation(ctx context.Context, text string, maxTokens int) ([]float32, error) {
	// Simple truncation: estimate ~4 chars per token
	maxChars := maxTokens * 4
	if len(text) > maxChars {
		text = text[:maxChars]
	}
	return c.Embed(ctx, text)
}
