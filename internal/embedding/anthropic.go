package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// DefaultAnthropicModel is the default model for Anthropic embeddings.
	// Note: Anthropic doesn't have native embedding API yet.
	// This uses Voyage AI (Anthropic's recommended embedding partner).
	// See: https://docs.anthropic.com/en/docs/build-with-claude/embeddings
	DefaultAnthropicModel = "voyage-3"

	// DefaultAnthropicDimension is the dimension for voyage-3.
	DefaultAnthropicDimension = 1024

	// VoyageAPIEndpoint is the Voyage AI API endpoint.
	VoyageAPIEndpoint = "https://api.voyageai.com/v1/embeddings"
)

// AnthropicClient implements Embedder using Voyage AI (Anthropic's recommended provider).
// Note: When Anthropic releases native embedding support, this can be updated.
type AnthropicClient struct {
	apiKey    string
	model     string
	dimension int
	client    *http.Client
}

// Compile-time check that AnthropicClient implements Embedder.
var _ Embedder = (*AnthropicClient)(nil)

// NewAnthropicClient creates a new embedding client using Voyage AI.
// apiKey should be a Voyage AI API key (not Anthropic key).
// If model is empty, uses DefaultAnthropicModel (voyage-3).
// If expectedDimension is 0, uses DefaultAnthropicDimension (1024).
func NewAnthropicClient(apiKey, model string, expectedDimension int) (*AnthropicClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required for Anthropic/Voyage embeddings")
	}
	if model == "" {
		model = DefaultAnthropicModel
	}
	if expectedDimension == 0 {
		expectedDimension = DefaultAnthropicDimension
	}

	return &AnthropicClient{
		apiKey:    apiKey,
		model:     model,
		dimension: expectedDimension,
		client:    &http.Client{},
	}, nil
}

// Model returns the configured embedding model name.
func (c *AnthropicClient) Model() string {
	return c.model
}

// Dimension returns the expected embedding dimension.
func (c *AnthropicClient) Dimension() int {
	return c.dimension
}

// voyageRequest is the request format for Voyage AI API.
type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"`
}

// voyageResponse is the response format from Voyage AI API.
type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed generates an embedding vector for the given text.
func (c *AnthropicClient) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (c *AnthropicClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	reqBody := voyageRequest{
		Input: texts,
		Model: c.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", VoyageAPIEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var voyageResp voyageResponse
	if err := json.NewDecoder(resp.Body).Decode(&voyageResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(voyageResp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d",
			len(voyageResp.Data), len(texts))
	}

	// Sort by index and extract embeddings
	embeddings := make([][]float32, len(texts))
	for _, d := range voyageResp.Data {
		if d.Index >= len(embeddings) {
			return nil, fmt.Errorf("invalid embedding index: %d", d.Index)
		}
		if len(d.Embedding) != c.dimension {
			return nil, fmt.Errorf("embedding %d dimension mismatch: got %d, want %d",
				d.Index, len(d.Embedding), c.dimension)
		}
		embeddings[d.Index] = d.Embedding
	}

	return embeddings, nil
}
