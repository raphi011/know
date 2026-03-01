# langchaingo

Technical learnings about the Go LLM library.

## Overview

langchaingo is the Go port of LangChain. Import: `github.com/tmc/langchaingo`

## Provider Setup

### Google AI (Gemini)

```go
import "github.com/tmc/langchaingo/llms/googleai"

llm, err := googleai.New(ctx,
    googleai.WithAPIKey(os.Getenv("GOOGLE_AI_API_KEY")),
    googleai.WithDefaultModel("gemini-2.0-flash"),
)
```

### Anthropic

```go
import "github.com/tmc/langchaingo/llms/anthropic"

llm, err := anthropic.New(
    anthropic.WithToken(os.Getenv("ANTHROPIC_API_KEY")),
    anthropic.WithModel("claude-sonnet-4-20250514"),
)
```

### OpenAI

```go
import "github.com/tmc/langchaingo/llms/openai"

llm, err := openai.New(
    openai.WithToken(os.Getenv("OPENAI_API_KEY")),
    openai.WithModel("gpt-4o"),
)
```

### Ollama (Local)

```go
import "github.com/tmc/langchaingo/llms/ollama"

llm, err := ollama.New(
    ollama.WithModel("gemma2"),
    ollama.WithServerURL("http://localhost:11434"),
)
```

### AWS Bedrock

```go
import "github.com/tmc/langchaingo/llms/bedrock"

// AWS SDK picks up env vars automatically
llm, err := bedrock.New(
    bedrock.WithModel("arn:aws:bedrock:..."),
    bedrock.WithModelProvider("anthropic"), // Required for inference profiles
)
```

## Embeddings

### Via LLM Wrapper

Works for providers that implement `CreateEmbedding`:

```go
import "github.com/tmc/langchaingo/embeddings"

// Google AI
llm, _ := googleai.New(ctx, googleai.WithAPIKey(key),
    googleai.WithDefaultEmbeddingModel("gemini-embedding-001"))
embedder, _ := embeddings.NewEmbedder(llm)

vectors, err := embedder.EmbedDocuments(ctx, []string{"text1", "text2"})
single, err := embedder.EmbedQuery(ctx, "query text")
```

### OpenAI-Compatible (Voyage AI)

Anthropic embeddings via Voyage AI's OpenAI-compatible endpoint:

```go
llm, _ := openai.New(
    openai.WithToken(anthropicKey),
    openai.WithBaseURL("https://api.voyageai.com/v1"),
    openai.WithEmbeddingModel("voyage-3-large"))
embedder, _ := embeddings.NewEmbedder(llm)
```

### Dedicated Embedders

For providers without `CreateEmbedding` on LLM:

```go
import bedrockembed "github.com/tmc/langchaingo/embeddings/bedrock"

embedder, err := bedrockembed.NewBedrock(
    bedrockembed.WithModel("amazon.titan-embed-text-v2"),
)
```

## Generation

### Basic Generation

```go
response, err := llm.GenerateContent(ctx, messages,
    llms.WithMaxTokens(4096),
)
text := response.Choices[0].Content
```

### With System Prompt

```go
messages := []llms.MessageContent{
    llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
    llms.TextParts(llms.ChatMessageTypeHuman, userPrompt),
}
response, err := llm.GenerateContent(ctx, messages)
```

### Streaming

```go
response, err := llm.GenerateContent(ctx, messages,
    llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
        fmt.Print(string(chunk))
        return nil
    }),
)
```

## Token Information

Access via `GenerationInfo`:

```go
choice := response.Choices[0]
info := choice.GenerationInfo // map[string]any

// Keys vary by provider:
// OpenAI: PromptTokens, CompletionTokens
// Anthropic: InputTokens, OutputTokens
```

## Common Gotchas

1. **Bedrock LLM doesn't support embeddings** - Use `embeddings/bedrock` package
2. **Inference profiles need provider hint** - Use `WithModelProvider()` for LLM
3. **Bedrock embeddings don't support ARNs** - The `embeddings/bedrock` package detects provider via `strings.Split(modelID, ".")[0]`, which fails for ARN-based inference profiles. Knowhow implements a custom wrapper using `KNOWHOW_BEDROCK_EMBED_MODEL_PROVIDER`
4. **Streaming callback must not block** - Process quickly or buffer
5. **Token counts may be missing** - Always have fallback estimates
6. **Google AI requires context** - `googleai.New(ctx, ...)` needs a context parameter unlike other providers
