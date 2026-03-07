# RAG Architecture

Technical learnings about Retrieval-Augmented Generation patterns.

## Chunking Strategy

### Markdown-Aware Chunking (One Chunk Per Heading)

Chunk documents at heading boundaries — each heading section becomes its own chunk with no cross-heading merging. Large sections exceeding `MaxSize` are split at paragraph boundaries.

```go
// Each heading = one chunk (or multiple if large)
sections := splitByHeaders(content)
for _, section := range sections {
    if len(section.Content) > maxChunkSize {
        subChunks := splitAtParagraphs(section.Content, maxChunkSize)
    }
}
```

Benefits:
- **Accurate heading paths**: Each chunk's `heading_path` exactly matches its content
- **Precise `#hash` navigation**: Search results link to the correct anchor
- **No diluted embeddings**: Small chunks get context via contextual retrieval prefix

### Contextual Retrieval (Embedding-Time Context)

Before embedding, each chunk gets a context prefix prepended (not stored):

```
Document: Getting Started Guide
Section: Setup > Installation

<actual chunk content>
```

This compensates for small chunks that would otherwise lack semantic context. BM25 search uses raw content (no prefix), so keyword matching is unaffected.

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

## Anthropic Contextual Retrieval Research

Source: https://www.anthropic.com/news/contextual-retrieval

### Key Findings

- **Hybrid BM25 + embeddings + reranking** reduces retrieval failures by 67% vs. embeddings alone
- **Full chunks (~800 tokens / 3-4K chars)** should be returned to the LLM, not truncated snippets — agents need enough context to reason about the content
- **Top-20 results** is the recommended retrieval depth (not top-5)
- Contextual retrieval (prepending document context to chunks before embedding) reduces top-20 retrieval failure by 35%

### Chunk Size

- 3-4K character chunks are a good default (this project uses heading-based chunking with similar bounds: threshold 6K, target 3K, max 4K)
- Smaller chunks benefit from contextual retrieval prefixes (already implemented in `internal/document/service.go`)

### Recommendations Applied

- MCP agent gets full chunk content via `FullContent` flag on `SearchInput` (no 200-char truncation)
- Agent retrieves top-20 results (was top-5)
- Web UI uses BM25-only search (`BM25Only` flag) — fast, no embedding API cost, sufficient for interactive keyword search
- Hybrid search reserved for agent use where semantic matching matters
