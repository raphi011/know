package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// ChunkResult represents a chunk of content.
type ChunkResult struct {
	Content     string
	Position    int
	HeadingPath string // Section context
}

// ChunkConfig defines chunking parameters.
type ChunkConfig struct {
	// Threshold: only chunk if content exceeds this length
	Threshold int
	// TargetSize: ideal chunk size
	TargetSize int
	// MaxSize: maximum chunk size (larger chunks split at sentences)
	MaxSize int
}

// DefaultChunkConfig returns sensible defaults for embedding-quality chunk sizes.
// TargetSize ~750 tokens, MaxSize ~1000 tokens (at ~4 chars/token for English prose).
// These are typically overridden by KNOW_CHUNK_* environment variables (see justfile).
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		Threshold:  6000,
		TargetSize: 3000,
		MaxSize:    4000,
	}
}

// Validate checks that chunk config values form a coherent configuration.
func (c ChunkConfig) Validate() error {
	if c.TargetSize <= 0 || c.MaxSize <= 0 || c.Threshold <= 0 {
		return fmt.Errorf("all chunk config values must be positive: target=%d max=%d threshold=%d",
			c.TargetSize, c.MaxSize, c.Threshold)
	}
	if c.TargetSize >= c.MaxSize {
		return fmt.Errorf("chunk TargetSize (%d) must be less than MaxSize (%d)", c.TargetSize, c.MaxSize)
	}
	return nil
}

// ExceedsThreshold returns true if content length exceeds the chunking threshold.
func ExceedsThreshold(content string, config ChunkConfig) bool {
	return len(content) > config.Threshold
}

// ChunkMarkdown splits Markdown content into semantic chunks.
// Prioritizes section boundaries, then paragraph boundaries.
// Returns empty slice if content has no semantic value for embedding.
func ChunkMarkdown(doc *MarkdownDoc, config ChunkConfig) []ChunkResult {
	// If content is below threshold, check whether it also fits within MaxSize
	if !ExceedsThreshold(doc.Content, config) {
		trimmed := strings.TrimSpace(doc.Content)
		if trimmed == "" {
			return []ChunkResult{}
		}
		// Even below Threshold, if content exceeds MaxSize it must be chunked
		// to avoid embedding context length errors
		if len(doc.Content) <= config.MaxSize {
			return []ChunkResult{{
				Content:     doc.Content,
				Position:    0,
				HeadingPath: "",
			}}
		}
	}

	// If we have sections, chunk by section first
	if len(doc.Sections) > 0 {
		return chunkBySections(doc.Sections, config)
	}

	// Fallback: chunk by paragraphs
	return chunkByParagraphs(doc.Content, config)
}

// maxAtomicCodeBlockSize is the hard size limit for keeping code blocks atomic.
// Code blocks exceeding this fall through to standard paragraph/sentence splitting.
const maxAtomicCodeBlockSize = 8000

// chunkBySections creates one chunk per heading section (no cross-heading merging).
// Empty sections are skipped. Each heading with content becomes its own chunk.
// Code-block-dominated sections are treated as atomic up to maxAtomicCodeBlockSize.
func chunkBySections(sections []Section, config ChunkConfig) []ChunkResult {
	var chunks []ChunkResult
	position := 0

	for _, section := range sections {
		trimmed := strings.TrimSpace(section.Content)
		if trimmed == "" {
			continue
		}

		// Keep section as a single chunk if it fits: code blocks get a higher
		// size ceiling (maxAtomicCodeBlockSize), capped by MaxSize so that
		// small-context embedding models still produce valid chunks.
		codeBlockLimit := min(maxAtomicCodeBlockSize, config.MaxSize)
		if (section.CodeBlock && len(trimmed) <= codeBlockLimit) || len(trimmed) <= config.MaxSize {
			chunks = append(chunks, ChunkResult{
				Content:     trimmed,
				Position:    position,
				HeadingPath: section.Path,
			})
			position++
			continue
		}

		// Large section: split into paragraphs
		paragraphChunks := chunkByParagraphs(section.Content, config)
		for _, pc := range paragraphChunks {
			chunks = append(chunks, ChunkResult{
				Content:     pc.Content,
				Position:    position,
				HeadingPath: section.Path,
			})
			position++
		}
	}

	return chunks
}

// chunkByParagraphs splits content by paragraph boundaries.
func chunkByParagraphs(content string, config ChunkConfig) []ChunkResult {
	// Split on double newlines (paragraphs)
	paragraphs := strings.Split(content, "\n\n")

	var chunks []ChunkResult
	var currentChunk strings.Builder
	position := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If adding this paragraph would exceed max, flush current chunk
		if currentChunk.Len()+len(para) > config.MaxSize && currentChunk.Len() > 0 {
			chunks = append(chunks, ChunkResult{
				Content:  strings.TrimSpace(currentChunk.String()),
				Position: position,
			})
			position++
			currentChunk.Reset()
		}

		// If single paragraph exceeds max, split by sentences
		if len(para) > config.MaxSize {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, ChunkResult{
					Content:  strings.TrimSpace(currentChunk.String()),
					Position: position,
				})
				position++
				currentChunk.Reset()
			}

			sentenceChunks := chunkBySentences(para, config)
			for _, sc := range sentenceChunks {
				chunks = append(chunks, ChunkResult{
					Content:  sc,
					Position: position,
				})
				position++
			}
			continue
		}

		// Add paragraph to current chunk
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// Flush remaining
	if currentChunk.Len() > 0 {
		chunks = append(chunks, ChunkResult{
			Content:  strings.TrimSpace(currentChunk.String()),
			Position: position,
		})
	}

	return chunks
}

// chunkBySentences splits text by sentence boundaries.
func chunkBySentences(text string, config ChunkConfig) []string {
	sentences := splitSentences(text)

	var chunks []string
	var currentChunk strings.Builder

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// If adding would exceed target, start new chunk
		if currentChunk.Len()+len(sentence) > config.TargetSize && currentChunk.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
			currentChunk.Reset()
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(sentence)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// splitSentences splits text into sentences.
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i := range runes {
		r := runes[i]
		current.WriteRune(r)

		// Check for sentence ending
		if r == '.' || r == '!' || r == '?' {
			// Look ahead for space or end
			if i+1 >= len(runes) || unicode.IsSpace(runes[i+1]) {
				// Not an abbreviation (simple heuristic)
				if i > 1 && unicode.IsUpper(runes[i-1]) {
					continue // Likely abbreviation like "Dr."
				}
				sentences = append(sentences, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}

	return sentences
}
