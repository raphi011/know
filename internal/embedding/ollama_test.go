// Package embedding_test contains integration tests for embedding clients.
package embedding_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOllamaClient(t *testing.T) {
	client, err := embedding.NewOllamaClient("", 0)
	require.NoError(t, err, "should create client with default model")
	assert.Equal(t, embedding.DefaultOllamaModel, client.Model())
	assert.Equal(t, embedding.DefaultOllamaDimension, client.Dimension())
}

func TestNewOllamaClientCustomModel(t *testing.T) {
	client, err := embedding.NewOllamaClient("custom-model", 512)
	require.NoError(t, err, "should create client with custom model")
	assert.Equal(t, "custom-model", client.Model())
	assert.Equal(t, 512, client.Dimension())
}

func TestDefaultOllama(t *testing.T) {
	embedder, err := embedding.DefaultOllama()
	require.NoError(t, err, "should create default embedder")
	assert.Equal(t, embedding.DefaultOllamaModel, embedder.Model())
	assert.Equal(t, embedding.DefaultOllamaDimension, embedder.Dimension())
}

func TestEmbedderInterface(t *testing.T) {
	// Verify OllamaClient implements Embedder interface
	var _ embedding.Embedder = (*embedding.OllamaClient)(nil)
}

func TestEmbed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := embedding.NewOllamaClient("", 0)
	require.NoError(t, err, "should create client")

	emb, err := client.Embed(ctx, "This is a test sentence for embedding.")
	require.NoError(t, err, "should generate embedding")

	// CRITICAL: Verify dimension matches expected
	assert.Len(t, emb, client.Dimension(),
		"embedding must be exactly %d dimensions", client.Dimension())

	// Verify values are reasonable (not all zeros, within normal range)
	var sum float32
	for _, v := range emb {
		sum += v * v
	}
	assert.Greater(t, sum, float32(0.1), "embedding should have non-trivial values")
}

func TestEmbedBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := embedding.NewOllamaClient("", 0)
	require.NoError(t, err, "should create client")

	texts := []string{
		"First test sentence.",
		"Second test sentence with different content.",
		"Third sentence about something else entirely.",
	}

	embeddings, err := client.EmbedBatch(ctx, texts)
	require.NoError(t, err, "should generate batch embeddings")

	assert.Len(t, embeddings, len(texts), "should return one embedding per text")

	for i, emb := range embeddings {
		assert.Len(t, emb, client.Dimension(),
			"embedding %d must be exactly %d dimensions", i, client.Dimension())
	}
}

func TestEmbedBatchEmpty(t *testing.T) {
	client, err := embedding.NewOllamaClient("", 0)
	require.NoError(t, err, "should create client")

	ctx := context.Background()
	embeddings, err := client.EmbedBatch(ctx, []string{})
	require.NoError(t, err, "should handle empty batch")
	assert.Len(t, embeddings, 0, "should return empty slice")
}

func TestEmbedSimilarity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := embedding.NewOllamaClient("", 0)
	require.NoError(t, err, "should create client")

	// Similar sentences should have high cosine similarity
	emb1, err := client.Embed(ctx, "The cat sat on the mat.")
	require.NoError(t, err)

	emb2, err := client.Embed(ctx, "A cat was sitting on a mat.")
	require.NoError(t, err)

	// Different sentence
	emb3, err := client.Embed(ctx, "Database query optimization techniques.")
	require.NoError(t, err)

	sim12 := cosineSimilarity(emb1, emb2)
	sim13 := cosineSimilarity(emb1, emb3)

	t.Logf("Similarity (similar sentences): %.4f", sim12)
	t.Logf("Similarity (different topics): %.4f", sim13)

	assert.Greater(t, sim12, sim13, "similar sentences should have higher similarity than different topics")
	assert.Greater(t, sim12, float32(0.7), "similar sentences should have >0.7 similarity")
}

func TestEmbedWithTruncation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := embedding.NewOllamaClient("", 0)
	require.NoError(t, err, "should create client")

	// Create a very long text
	longText := ""
	for i := 0; i < 1000; i++ {
		longText += "This is a repeating sentence to make a very long text. "
	}

	emb, err := client.EmbedWithTruncation(ctx, longText, 512)
	require.NoError(t, err, "should embed with truncation")
	assert.Len(t, emb, client.Dimension())
}

func TestNewEmbedderFactory(t *testing.T) {
	// Test factory with Ollama provider
	embedder, err := embedding.New(embedding.Config{
		Provider: embedding.ProviderOllama,
	})
	require.NoError(t, err, "should create Ollama embedder via factory")
	assert.Equal(t, embedding.DefaultOllamaModel, embedder.Model())
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
