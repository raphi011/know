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
Chunk sizes are configurable via environment variables:

| Env Var | Default | Description |
|---------|---------|-------------|
| `KNOWHOW_CHUNK_THRESHOLD` | 6000 | Only chunk if content exceeds this length |
| `KNOWHOW_CHUNK_TARGET_SIZE` | 3000 | Ideal chunk size in chars |
| `KNOWHOW_CHUNK_MAX_SIZE` | 4000 | Maximum chunk size (larger chunks get split) |

### Model-Specific Chunk Sizes

For `mxbai-embed-large` via Ollama (512 token max sequence length ≈ 2048 chars, with headroom):

```bash
KNOWHOW_CHUNK_THRESHOLD=1200
KNOWHOW_CHUNK_TARGET_SIZE=1000
KNOWHOW_CHUNK_MAX_SIZE=1500
```

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

### One Chunk Per Heading (No Cross-Heading Merging)

Each heading section becomes its own chunk, regardless of size. No small sections are merged into parent or sibling chunks. This gives:

1. **Accurate heading paths**: Each chunk's `heading_path` exactly matches its content
2. **Precise hash navigation**: Search results link to the correct `#heading` anchor
3. **Better embeddings**: Small chunks get meaningful embeddings via contextual retrieval (see below)

### Contextual Retrieval

Small chunks (e.g., a short `### Install` subsection) would normally produce poor embeddings due to insufficient context. We compensate by prepending document/section context **only at embedding time**:

```
Document: Getting Started Guide
Section: Setup > Installation

<actual chunk content>
```

This context prefix is **not stored** in the chunk — only used when calling the embedding model. BM25 fulltext search operates on raw `chunk.content` without the prefix, avoiding keyword noise from the context.

Based on [Anthropic's contextual retrieval research (2024)](https://www.anthropic.com/news/contextual-retrieval), this technique reduces retrieval failures by ~49% when combined with BM25.

Implementation: `buildEmbeddingContext()` in `internal/document/service.go`.

### Chunk Change Detection

`syncChunks()` in `document/service.go` uses **exact content matching** to diff new chunks against existing ones. Unchanged chunks keep their embeddings. Only new/modified chunks are scheduled for embedding via `embed_at`. This makes document updates cheap when only parts of the content change.
