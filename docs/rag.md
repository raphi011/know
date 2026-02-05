# RAG Architecture

Technical learnings about Retrieval-Augmented Generation patterns.

## Chunking Strategy

### Markdown-Aware Chunking

Chunk documents by Markdown structure, not arbitrary byte boundaries:

```go
// Split on headers, preserve hierarchy
sections := splitByHeaders(content)
for _, section := range sections {
    if len(section.Content) > maxChunkSize {
        // Split large sections at paragraph boundaries
        subChunks := splitAtParagraphs(section.Content, maxChunkSize)
    }
}
```

Benefits:
- Preserves semantic coherence
- Heading path provides context ("## Setup > ### Install")
- Better retrieval quality vs fixed-size chunks

### Chunk Metadata

Track chunk provenance for context reconstruction:

```go
type Chunk struct {
    EntityID    string   // Parent document
    Content     string   // Chunk text
    Position    int      // Order within document
    HeadingPath string   // "## Section > ### Subsection"
    Labels      []string // Inherited from parent
    Embedding   []float32
}
```

### Skip Empty Sections

Empty chunks cause embedding failures:

```go
if strings.TrimSpace(section.Content) == "" {
    continue // Skip empty sections
}
```

## Hybrid Search

### Why Hybrid?

- **Vector search**: Semantic similarity, handles paraphrasing
- **Fulltext search**: Exact matches, handles keywords/names
- **Combined**: Best of both for RAG retrieval

### RRF (Reciprocal Rank Fusion)

Merge results from different search methods:

```
RRF_score = sum(1 / (k + rank_i))
```

Where `k` is a constant (typically 60) and `rank_i` is the rank in each result set.

### Label Filtering

Apply label filters before search, not after:
- Pre-filtering: Faster, uses indexes
- Post-filtering: More flexible, but retrieves extra docs

```sql
-- Pre-filter approach
WHERE labels CONTAINSALL $required_labels
  AND embedding <|20,COSINE|> $query_vec
```

## Context Assembly

### Token Budget

Estimate context size to fit LLM limits:

```go
const charsPerToken = 4 // Rough estimate

func estimateTokens(text string) int {
    return len(text) / charsPerToken
}

func assembleContext(chunks []Chunk, maxTokens int) string {
    var context strings.Builder
    tokens := 0
    for _, chunk := range chunks {
        chunkTokens := estimateTokens(chunk.Content)
        if tokens + chunkTokens > maxTokens {
            break
        }
        context.WriteString(chunk.Content)
        context.WriteString("\n\n")
        tokens += chunkTokens
    }
    return context.String()
}
```

### Source Attribution

Include provenance in context for citations:

```go
fmt.Sprintf("From %s (section: %s):\n%s",
    chunk.EntityName,
    chunk.HeadingPath,
    chunk.Content)
```

## Quality Metrics

Track retrieval quality:
- **Recall@K**: % of relevant docs in top K
- **MRR**: Mean Reciprocal Rank of first relevant doc
- **Latency**: Time from query to context assembly
