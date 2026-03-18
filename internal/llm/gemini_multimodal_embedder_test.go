package llm

import (
	"context"
	"os"
	"testing"
)

func TestGeminiMultimodalEmbedder_SupportsMIME(t *testing.T) {
	g := &GeminiMultimodalEmbedder{}

	tests := []struct {
		mimeType string
		want     bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"application/pdf", true},
		{"audio/mpeg", true},
		{"audio/wav", true},
		{"audio/ogg", true},
		{"audio/", true},
		{"image/gif", false},
		{"image/webp", false},
		{"video/mp4", false},
		{"text/plain", false},
		{"application/json", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := g.SupportsMIME(tt.mimeType)
			if got != tt.want {
				t.Errorf("SupportsMIME(%q) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestGeminiMultimodalEmbedder_EmptyInput(t *testing.T) {
	g := &GeminiMultimodalEmbedder{}

	vecs, err := g.EmbedMultimodal(context.Background(), nil)
	if err != nil {
		t.Fatalf("EmbedMultimodal(nil) returned error: %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("expected empty result for nil input, got %d vectors", len(vecs))
	}

	vecs, err = g.EmbedMultimodal(context.Background(), []MultimodalInput{})
	if err != nil {
		t.Fatalf("EmbedMultimodal([]) returned error: %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("expected empty result for empty input, got %d vectors", len(vecs))
	}
}

func TestGeminiEmbedBatchLimit(t *testing.T) {
	// Verify the batch limit constant hasn't accidentally changed.
	if geminiEmbedBatchLimit != 6 {
		t.Fatalf("expected geminiEmbedBatchLimit == 6, got %d", geminiEmbedBatchLimit)
	}
}

func TestGeminiMultimodalEmbedder_Integration(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_AI_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_AI_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	embedder, err := NewGeminiMultimodalEmbedder(ctx, apiKey, "gemini-embedding-001", 768)
	if err != nil {
		t.Fatalf("NewGeminiMultimodalEmbedder: %v", err)
	}

	if !embedder.SupportsMIME("image/png") {
		t.Error("expected SupportsMIME(image/png) == true")
	}

	// Minimal 1x1 white PNG.
	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}

	items := []MultimodalInput{
		{Data: pngBytes, MimeType: "image/png", Text: "a white pixel"},
	}

	vecs, err := embedder.EmbedMultimodal(ctx, items)
	if err != nil {
		t.Fatalf("EmbedMultimodal: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vecs))
	}
	if len(vecs[0]) != 768 {
		t.Errorf("expected dimension 768, got %d", len(vecs[0]))
	}
}
