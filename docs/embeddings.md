# Vector Embeddings

Technical learnings about vector embeddings for semantic search.

## Embedding Models

### Dimension Comparison

| Model | Provider | Dimensions | Notes |
|-------|----------|------------|-------|
| gemini-embedding-001 | Google AI | 768 | Default 768, supports up to 3072 |
| voyage-3-large | Anthropic/Voyage | 1024 | High quality, multilingual |
| voyage-3 | Anthropic/Voyage | 1024 | Balanced quality/cost |
| text-embedding-3-small | OpenAI | 1536 | Cost-effective |
| amazon.titan-embed-text-v1 | Bedrock | 1536 | AWS native, max 8k tokens |
| amazon.titan-embed-text-v2 | Bedrock | 1024 | Improved, configurable dim |
| cohere.embed-english-v3 | Bedrock | 1024 | English-optimized |
| cohere.embed-multilingual-v3 | Bedrock | 1024 | 108 languages |
| mxbai-embed-large | Ollama | 1024 | Recommended local dev model |
| bge-m3 | Ollama | 1024 | Crashes on texts >~2400 chars (Ollama 0.13.1) |
| all-minilm:l6-v2 | Ollama | 384 | Fast, good for dev |

### Dimension Selection

- **Higher dimensions** = more semantic nuance, higher storage/compute
- **768–1024** is a good balance for most RAG applications
- HNSW index must match embedding dimension exactly
- Changing dimensions requires fresh database (rebuild indexes)

## langchaingo Embedding Patterns

### Two approaches to embeddings:

1. **LLM-based embedders**: Wrap LLM with `embeddings.NewEmbedder(llm)`
   - Works with: Google AI, OpenAI, Ollama
   - Requires LLM to implement `CreateEmbedding`

2. **Dedicated embedders**: Use specialized package
   - `embeddings/bedrock.NewBedrock()` for AWS Bedrock
   - Required because `llms/bedrock` doesn't implement `CreateEmbedding`

3. **OpenAI-compatible endpoints**: Use `openai.New()` with custom base URL
   - Works with: Anthropic/Voyage AI (`https://api.voyageai.com/v1`)
   - Any provider with OpenAI-compatible embedding API

```go
// Google AI pattern
llm, _ := googleai.New(ctx, googleai.WithAPIKey(key),
    googleai.WithDefaultEmbeddingModel("gemini-embedding-001"))
embedder, _ := embeddings.NewEmbedder(llm)

// Anthropic/Voyage pattern (OpenAI-compatible)
llm, _ := openai.New(openai.WithToken(anthropicKey),
    openai.WithBaseURL("https://api.voyageai.com/v1"),
    openai.WithEmbeddingModel("voyage-3-large"))
embedder, _ := embeddings.NewEmbedder(llm)

// Bedrock pattern (different!)
embedder, _ := bedrockembed.NewBedrock(
    bedrockembed.WithModel("amazon.titan-embed-text-v2"),
)
```

## Dimension Validation

Always validate embedding dimensions match the HNSW index:

```go
if len(embedding) != expectedDimension {
    return fmt.Errorf("dimension mismatch: got %d, want %d",
        len(embedding), expectedDimension)
}
```

Mismatched dimensions cause SurrealDB errors on insert/search.

## Batching

- Batch embeddings when possible to reduce API calls
- Most providers support batch embedding via `EmbedDocuments()`
- Monitor batch sizes - some providers limit batch size

## Chunking Strategy

### Chunk Size Recommendations

Research consensus (Pinecone, LlamaIndex, OpenAI cookbook):
- **512–1024 tokens** (~2000–4000 chars) is the sweet spot for embedding quality
- Too small (<200 tokens): insufficient context for meaningful embeddings
- Too large (>2048 tokens): semantic signal diluted, retrieval precision drops
- Match chunk size to embedding model's training data (most trained on ~512 token passages)

Our defaults are in `DefaultChunkConfig()` in `internal/parser/chunker.go`.

### AST-Based Markdown Chunking

We use **goldmark** (Go markdown parser) to build an AST before chunking. This solves critical issues with regex-based heading detection:

1. **Code fence awareness**: `#` inside fenced code blocks is correctly parsed as code, not a heading
2. **Pre-heading content**: Text before the first heading is captured as a "preamble" section (Level=0)
3. **Proper block boundaries**: Paragraphs, lists, and code blocks have clean boundaries from the AST

The AST walker iterates top-level block nodes, grouping them under their parent heading. Each heading node creates a new `Section` with a hierarchical path (e.g., `## Setup > ### Install`).

### Why No Overlap

We previously used 100-char overlap between chunks. This was removed because:

1. **Cascade re-embedding**: Editing section N changes section N+1's stored content (due to overlap prefix), triggering unnecessary re-embedding
2. **Heading paths provide context**: Each chunk carries its heading path, giving the embedding model sufficient context without overlap
3. **Search navigates to headings**: Results link to the heading path, so users navigate to the section — the overlap doesn't help with navigation

### Code Block Handling

- Code blocks (`FencedCodeBlock` AST nodes) are treated as **atomic units**
- Sections dominated by code (majority of content) are marked `CodeBlock=true`
- Code-block sections are kept atomic up to `maxAtomicCodeBlockSize` (8000 chars)
- Beyond that limit, they fall through to standard paragraph/sentence splitting

### Hierarchical Merge

When a section is below `MinSize`:
1. **Try parent merge**: Merge into the parent heading's chunk (e.g., `### Subsection` merges into `## Parent`)
2. **Overflow check**: Only merge if combined size stays under `MaxSize`
3. **Fallback**: If no parent or parent is full, merge with previous chunk or create standalone

This preserves semantic relationships — a small subsection belongs with its parent heading, not with an unrelated preceding section.

### Chunk Change Detection

`syncChunks()` in `document/service.go` uses **exact content matching** to diff new chunks against existing ones. Unchanged chunks keep their embeddings. Only new/modified chunks are scheduled for embedding via `embed_at`. This makes document updates cheap when only parts of the content change.
