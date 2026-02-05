# LLM Integration

Technical learnings about LLM integration patterns.

## Provider Abstraction

Use interface for provider independence:

```go
type LLM interface {
    GenerateWithSystem(ctx context.Context, system, user string) (string, error)
    GenerateWithSystemStream(ctx context.Context, system, user string, onToken func(string) error) error
}
```

## Prompt Patterns

### System + User Pattern

```go
systemPrompt := `You are a knowledge assistant.
Answer based ONLY on the provided context.
If insufficient information, say so.`

userPrompt := fmt.Sprintf(`Context:
%s

Question: %s`, context, query)

response, err := llm.GenerateWithSystem(ctx, systemPrompt, userPrompt)
```

### Entity Extraction

For knowledge graph extraction:

```go
systemPrompt := `Extract entities and relations from text.
Output format (one per line):
ENTITY|name|type|description
RELATION|source|target|relation_type|description`
```

## Error Handling

### Fatal vs Retryable Errors

```go
func isFatalAPIError(err error) bool {
    msg := err.Error()
    // Billing/quota - don't retry
    if strings.Contains(msg, "credit balance") ||
       strings.Contains(msg, "quota exceeded") {
        return true
    }
    // Auth errors - don't retry
    if strings.Contains(msg, "invalid api key") ||
       strings.Contains(msg, "401") {
        return true
    }
    return false
}
```

### Wrapping Fatal Errors

```go
var ErrFatalAPI = errors.New("fatal API error")

func wrapFatalError(err error) error {
    if isFatalAPIError(err) {
        return fmt.Errorf("%w: %v", ErrFatalAPI, err)
    }
    return err
}
```

## Token Counting

### Provider Variations

Different providers return tokens differently:

```go
func extractTokenCounts(info map[string]any) (input, output int64) {
    // OpenAI style
    if v, ok := info["PromptTokens"]; ok {
        input = toInt64(v)
    }
    if v, ok := info["CompletionTokens"]; ok {
        output = toInt64(v)
    }
    // Anthropic style
    if input == 0 {
        if v, ok := info["InputTokens"]; ok {
            input = toInt64(v)
        }
    }
    // Fallback to estimates
    if input == 0 {
        input = len(promptText) / 4
    }
    return
}
```

## Streaming

### Token Callback Pattern

```go
err := llm.GenerateWithSystemStream(ctx, system, user,
    func(token string) error {
        fmt.Print(token) // Or send via SSE
        return nil
    })
```

### Abort via Callback Error

```go
func(token string) error {
    if cancelled {
        return context.Canceled
    }
    return nil
}
```

## Max Tokens

Set appropriate limits for task:

```go
llms.WithMaxTokens(8192) // For long-form answers
llms.WithMaxTokens(1024) // For short extractions
```
