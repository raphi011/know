package llm

import "context"

// MultimodalInput represents a single item to embed multimodally.
type MultimodalInput struct {
	Data     []byte // binary content (image bytes, audio bytes, PDF page bytes)
	MimeType string // MIME type of the data (e.g. "image/png", "audio/mpeg")
	Text     string // optional text context to embed alongside the binary data
}

// MultimodalEmbedder embeds non-text content (images, audio, PDF pages)
// into the same vector space as text embeddings.
// Currently only Gemini Embedding 2 supports this.
type MultimodalEmbedder interface {
	// EmbedMultimodal embeds multiple binary items in a single batch.
	// Returns one embedding vector per input item.
	EmbedMultimodal(ctx context.Context, items []MultimodalInput) ([][]float64, error)

	// SupportsMIME returns true if this embedder can natively embed the given MIME type.
	SupportsMIME(mimeType string) bool
}

// TextExtractor extracts searchable text from binary file data.
// Used to enable BM25 search on non-text files.
type TextExtractor interface {
	// Extract returns extracted text from binary data of the given MIME type.
	Extract(ctx context.Context, data []byte, mimeType string) (string, error)

	// SupportsMIME returns true if this extractor handles the given MIME type.
	SupportsMIME(mimeType string) bool
}
