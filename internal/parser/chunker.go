package parser

import (
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
	// Overlap: character overlap between chunks
	Overlap int
}

// DefaultChunkConfig returns sensible defaults.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		Threshold:  1500,
		TargetSize: 750,
		MinSize:    200,
		MaxSize:    1000,
		Overlap:    100,
	}
}

// ShouldChunk returns true if content should be chunked.
func ShouldChunk(content string, config ChunkConfig) bool {
	return len(content) > config.Threshold
}

// ChunkMarkdown splits Markdown content into semantic chunks.
// Prioritizes section boundaries, then paragraph boundaries.
// Returns empty slice if content has no semantic value for embedding.
func ChunkMarkdown(doc *MarkdownDoc, config ChunkConfig) []ChunkResult {
	// If content is short enough, return as single chunk (only if non-empty)
	if !ShouldChunk(doc.Content, config) {
		trimmed := strings.TrimSpace(doc.Content)
		if trimmed == "" {
			return []ChunkResult{} // No content to chunk
		}
		return []ChunkResult{{
			Content:     doc.Content,
			Position:    0,
			HeadingPath: "",
		}}
	}

	// If we have sections, chunk by section first
	if len(doc.Sections) > 0 {
		return chunkBySections(doc.Sections, config)
	}

	// Fallback: chunk by paragraphs
	return chunkByParagraphs(doc.Content, config)
}

// chunkBySections creates chunks from document sections.
// Empty sections are skipped - they have no semantic value for RAG.
func chunkBySections(sections []Section, config ChunkConfig) []ChunkResult {
	var chunks []ChunkResult
	position := 0

	for _, section := range sections {
		// Skip empty sections - they have no semantic value for RAG
		trimmed := strings.TrimSpace(section.Content)
		if trimmed == "" {
			continue
		}

		// If section is small, add as single chunk
		if len(trimmed) <= config.MaxSize {
			if len(trimmed) >= config.MinSize || len(chunks) == 0 {
				chunks = append(chunks, ChunkResult{
					Content:     trimmed,
					Position:    position,
					HeadingPath: section.Path,
				})
				position++
			} else if len(chunks) > 0 {
				// Merge tiny section with previous
				lastChunk := &chunks[len(chunks)-1]
				lastChunk.Content += "\n\n" + trimmed
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

	// Apply overlap
	return applyOverlap(chunks, config.Overlap)
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
	for i := 0; i < len(runes); i++ {
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

// applyOverlap adds overlap between adjacent chunks using semantic boundaries.
// Prefers sentence boundaries (.!?) over word boundaries for better context.
func applyOverlap(chunks []ChunkResult, overlap int) []ChunkResult {
	if overlap <= 0 || len(chunks) <= 1 {
		return chunks
	}

	result := make([]ChunkResult, len(chunks))
	copy(result, chunks)

	for i := 1; i < len(result); i++ {
		prevContent := result[i-1].Content
		if len(prevContent) > overlap {
			// Take last `overlap` characters from previous chunk
			overlapText := prevContent[len(prevContent)-overlap:]

			// Try to find a sentence boundary (. ! ?) followed by space
			bestIdx := -1
			for _, ending := range []string{". ", "! ", "? "} {
				if idx := strings.LastIndex(overlapText, ending); idx > bestIdx {
					bestIdx = idx + len(ending) // Start after the sentence ending
				}
			}

			if bestIdx > 0 && bestIdx < len(overlapText) {
				// Found sentence boundary - use text from there
				overlapText = overlapText[bestIdx:]
			} else {
				// Fallback: find word boundary
				spaceIdx := strings.LastIndex(overlapText, " ")
				if spaceIdx > 0 {
					overlapText = overlapText[spaceIdx+1:]
				}
			}

			if len(overlapText) > 0 {
				result[i].Content = overlapText + " " + result[i].Content
			}
		}
	}

	return result
}
