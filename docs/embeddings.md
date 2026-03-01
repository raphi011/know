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
| bge-m3 | Ollama | 1024 | Local, good for dev |
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
