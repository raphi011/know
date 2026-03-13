package llm

import "log/slog"

// contextWindows maps known model names to their context window size in tokens.
var contextWindows = map[string]int{
	// Anthropic
	"claude-opus-4-6":            200_000,
	"claude-sonnet-4-6":          200_000,
	"claude-sonnet-4-5":          200_000,
	"claude-opus-4-20250514":     200_000,
	"claude-sonnet-4-20250514":   200_000,
	"claude-3-7-sonnet-20250219": 200_000,
	"claude-3-5-sonnet-20241022": 200_000,
	"claude-3-5-haiku-20241022":  200_000,

	// Anthropic (Bedrock model IDs)
	"anthropic.claude-opus-4-6-v1:0":            200_000,
	"anthropic.claude-sonnet-4-6-v1:0":          200_000,
	"anthropic.claude-sonnet-4-5-v1:0":          200_000,
	"anthropic.claude-opus-4-20250514-v1:0":     200_000,
	"anthropic.claude-sonnet-4-20250514-v1:0":   200_000,
	"anthropic.claude-3-7-sonnet-20250219-v1:0": 200_000,
	"anthropic.claude-3-5-sonnet-20241022-v2:0": 200_000,
	"anthropic.claude-3-5-haiku-20241022-v1:0":  200_000,

	// OpenAI
	"gpt-5.4":     1_047_576,
	"gpt-5.4-pro": 1_047_576,
	"gpt-5-mini":  400_000,
	"o3":          200_000,
	"o3-mini":     200_000,
	"o3-pro":      200_000,

	// Google
	"gemini-3.1-pro":   1_048_576,
	"gemini-3-flash":   1_048_576,
	"gemini-2.5-pro":   1_048_576,
	"gemini-2.5-flash": 1_048_576,
	"gemini-2.0-flash": 1_048_576,

	// Ollama (common models)
	"llama4:scout":    10_000_000,
	"llama4:maverick": 1_000_000,
	"llama3.3":        128_000,
	"llama3.2":        128_000,
	"llama3.1":        128_000,
	"mistral":         32_768,
	"mixtral":         32_768,
	"qwen2.5":         128_000,
	"qwen3":           256_000,
	"deepseek-r1":     128_000,
}

const defaultContextWindow = 128_000

// ContextWindowSize returns the context window size for a model.
// Priority: envOverride → registry lookup → default (128k).
func ContextWindowSize(modelName string, envOverride int) int {
	if envOverride > 0 {
		return envOverride
	}
	if size, ok := contextWindows[modelName]; ok {
		return size
	}
	slog.Warn("model not in context window registry, using default",
		"model", modelName, "default", defaultContextWindow)
	return defaultContextWindow
}
