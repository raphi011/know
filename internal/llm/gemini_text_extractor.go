package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"google.golang.org/genai"
)

const textExtractorPrompt = `You are a document text extractor. Extract all content from this page image as structured markdown.

Rules:
- Preserve heading hierarchy using ## and ### markdown headings
- Render tables as markdown tables with headers preserved
- Describe figures, charts, and diagrams in [Figure: description] with detail
- Include all visible text exactly as it appears
- If content continues from a previous page, start with [CONTINUES]
- If a table spans this page, preserve column headers
- If there is no readable text, describe the visual content
- Do not add any commentary or meta-text, only the extracted content`

// GeminiTextExtractor extracts structured text from images and PDFs
// using Gemini's vision capabilities.
type GeminiTextExtractor struct {
	client    *genai.Client
	modelName string
}

// NewGeminiTextExtractor creates a text extractor backed by Gemini.
func NewGeminiTextExtractor(ctx context.Context, apiKey, modelName string) (*GeminiTextExtractor, error) {
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
	return &GeminiTextExtractor{
		client:    client,
		modelName: modelName,
	}, nil
}

// Extract sends an image to Gemini and returns structured markdown text.
func (g *GeminiTextExtractor) Extract(ctx context.Context, data []byte, mimeType string) (string, error) {
	logger := logutil.FromCtx(ctx).With("model", g.modelName)
	logger.Debug("text extraction starting", "mime_type", mimeType, "input_bytes", len(data))

	start := time.Now()

	contents := []*genai.Content{
		genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromText(textExtractorPrompt),
			genai.NewPartFromBytes(data, mimeType),
		}, genai.RoleUser),
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.modelName, contents, nil)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned no candidates (possible content safety block or quota issue)")
	}

	var text strings.Builder
	for _, candidate := range resp.Candidates {
		if candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
			}
		}
	}

	result := strings.TrimSpace(text.String())

	logger.Debug("text extraction complete",
		"output_chars", len(result),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return result, nil
}

// SupportsMIME returns true if this extractor can process the given MIME type.
func (g *GeminiTextExtractor) SupportsMIME(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "application/pdf":
		return true
	}
	return false
}
