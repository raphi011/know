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
	// MinSize: minimum chunk size (smaller chunks merge with neighbors)
	MinSize int
	// MaxSize: maximum chunk size (larger chunks split at sentences)
	MaxSize int
}

// DefaultChunkConfig returns sensible defaults for embedding-quality chunk sizes.
// TargetSize ~750 tokens, MaxSize ~1000 tokens (at ~4 chars/token for English prose).
// These are typically overridden by KNOWHOW_CHUNK_* environment variables (see justfile).
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		Threshold:  6000,
		TargetSize: 3000,
		MinSize:    800,
		MaxSize:    4000,
	}
}

// Validate checks that chunk config values form a coherent configuration.
func (c ChunkConfig) Validate() error {
	if c.MinSize <= 0 || c.TargetSize <= 0 || c.MaxSize <= 0 || c.Threshold <= 0 {
		return fmt.Errorf("all chunk config values must be positive: min=%d target=%d max=%d threshold=%d",
			c.MinSize, c.TargetSize, c.MaxSize, c.Threshold)
	}
	if c.MinSize >= c.TargetSize {
		return fmt.Errorf("chunk MinSize (%d) must be less than TargetSize (%d)", c.MinSize, c.TargetSize)
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

// chunkBySections creates chunks from document sections using hierarchical merging.
// Empty sections are skipped. Small sections merge into their parent heading's chunk
// rather than the positional predecessor, preserving semantic relationships.
// Code-block-dominated sections are treated as atomic up to maxAtomicCodeBlockSize.
func chunkBySections(sections []Section, config ChunkConfig) []ChunkResult {
	var chunks []ChunkResult
	position := 0

	// parentPath returns the parent heading path for hierarchical merging.
	// e.g. "# A > ## B > ### C" → "# A > ## B"
	parentPath := func(path string) string {
		if idx := strings.LastIndex(path, " > "); idx >= 0 {
			return path[:idx]
		}
		return ""
	}

	// findParentChunk finds the most recent chunk whose heading path matches
	// or is a descendant of the parent path. Returns nil for top-level sections
	// (no parent in the hierarchy).
	findParentChunk := func(path string) *ChunkResult {
		parent := parentPath(path)
		if parent == "" {
			return nil
		}
		for i := len(chunks) - 1; i >= 0; i-- {
			if chunks[i].HeadingPath == parent || strings.HasPrefix(chunks[i].HeadingPath, parent) {
				return &chunks[i]
			}
		}
		return nil
	}

	for _, section := range sections {
		trimmed := strings.TrimSpace(section.Content)
		if trimmed == "" {
			continue
		}

		// Code-block-dominated sections: keep atomic unless they exceed
		// the hard size limit (code blocks beyond this fall through to splitting)
		if section.CodeBlock && len(trimmed) <= maxAtomicCodeBlockSize {
			if len(trimmed) >= config.MinSize {
				chunks = append(chunks, ChunkResult{
					Content:     trimmed,
					Position:    position,
					HeadingPath: section.Path,
				})
				position++
			} else {
				// Small code block: try to merge with parent
				if parent := findParentChunk(section.Path); parent != nil {
					parent.Content += "\n\n" + trimmed
				} else if len(chunks) > 0 {
					chunks[len(chunks)-1].Content += "\n\n" + trimmed
				} else {
					chunks = append(chunks, ChunkResult{
						Content:     trimmed,
						Position:    position,
						HeadingPath: section.Path,
					})
					position++
				}
			}
			continue
		}

		// If section fits in a chunk
		if len(trimmed) <= config.MaxSize {
			if len(trimmed) >= config.MinSize {
				chunks = append(chunks, ChunkResult{
					Content:     trimmed,
					Position:    position,
					HeadingPath: section.Path,
				})
				position++
			} else {
				// Small section: hierarchical merge — try parent first
				if parent := findParentChunk(section.Path); parent != nil {
					merged := parent.Content + "\n\n" + trimmed
					if len(merged) <= config.MaxSize {
						parent.Content = merged
					} else {
						// Parent too full, create standalone chunk
						chunks = append(chunks, ChunkResult{
							Content:     trimmed,
							Position:    position,
							HeadingPath: section.Path,
						})
						position++
					}
				} else if len(chunks) > 0 {
					// No parent found, merge with previous
					merged := chunks[len(chunks)-1].Content + "\n\n" + trimmed
					if len(merged) <= config.MaxSize {
						chunks[len(chunks)-1].Content = merged
					} else {
						chunks = append(chunks, ChunkResult{
							Content:     trimmed,
							Position:    position,
							HeadingPath: section.Path,
						})
						position++
					}
				} else {
					// First chunk
					chunks = append(chunks, ChunkResult{
						Content:     trimmed,
						Position:    position,
						HeadingPath: section.Path,
					})
					position++
				}
			}
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
