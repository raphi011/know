// Package embedding provides text embedding generation using Ollama.
package embedding

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

const (
	// DefaultModel is the embedding model that produces 384-dimensional vectors.
	// CRITICAL: This MUST match the HNSW index dimension in SurrealDB schema.
	DefaultModel = "all-minilm:l6-v2"

	// ExpectedDimension is the required embedding dimension.
	// Mismatch will cause HNSW index queries to fail.
	ExpectedDimension = 384
)

// Client wraps the Ollama API for embedding generation.
type Client struct {
	client *api.Client
	model  string
}

// NewClient creates a new Ollama embedding client.
// If model is empty, uses DefaultModel (all-minilm:l6-v2).
// Uses OLLAMA_HOST environment variable for server URL (defaults to http://localhost:11434).
func NewClient(model string) (*Client, error) {
	if model == "" {
		model = DefaultModel
	}

	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("create ollama client: %w", err)
	}

	return &Client{client: client, model: model}, nil
}

// NewClientWithHost creates a client with explicit host (for testing).
func NewClientWithHost(host, model string) (*Client, error) {
	if model == "" {
		model = DefaultModel
	}

	client := api.NewClient(nil, nil)
	// Note: api.NewClient uses OLLAMA_HOST env var, to override we'd need custom http.Client
	// For now, rely on environment variable

	return &Client{client: client, model: model}, nil
}

// Model returns the configured embedding model name.
func (c *Client) Model() string {
	return c.model
}

// Embed generates an embedding vector for the given text.
// Returns exactly 384-dimensional float32 vector or error if dimension mismatch.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
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
	if len(embedding) != ExpectedDimension {
		return nil, fmt.Errorf("dimension mismatch: got %d, want %d (model: %s)",
			len(embedding), ExpectedDimension, c.model)
	}

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts in a single request.
// More efficient than multiple Embed calls for bulk operations.
// All embeddings are verified to be exactly 384 dimensions.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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
		if len(emb) != ExpectedDimension {
			return nil, fmt.Errorf("embedding %d dimension mismatch: got %d, want %d",
				i, len(emb), ExpectedDimension)
		}
	}

	return resp.Embeddings, nil
}

// EmbedWithTruncation embeds text, truncating if necessary for very long inputs.
// Ollama models have context limits; this provides a safe wrapper.
func (c *Client) EmbedWithTruncation(ctx context.Context, text string, maxTokens int) ([]float32, error) {
	// Simple truncation: estimate ~4 chars per token
	maxChars := maxTokens * 4
	if len(text) > maxChars {
		text = text[:maxChars]
	}
	return c.Embed(ctx, text)
}
