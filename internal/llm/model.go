package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/metrics"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// ErrFatalAPI indicates a non-recoverable API error (billing, auth, etc.)
// that should stop all further LLM operations.
var ErrFatalAPI = errors.New("fatal API error")

// isFatalAPIError checks if an error indicates a non-recoverable API issue.
func isFatalAPIError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Billing/quota errors
	if strings.Contains(msg, "credit balance") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "quota exceeded") ||
		strings.Contains(msg, "billing") {
		return true
	}
	// Auth errors
	if strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") {
		return true
	}
	return false
}

// wrapFatalError wraps an error with ErrFatalAPI if it's a fatal API error.
func wrapFatalError(err error) error {
	if isFatalAPIError(err) {
		return fmt.Errorf("%w: %v", ErrFatalAPI, err)
	}
	return err
}

// charsPerToken is used to estimate token counts when real counts unavailable.
const charsPerToken = 4

// Model wraps langchaingo LLM for text generation.
type Model struct {
	llm       llms.Model
	modelName string
	metrics   *metrics.Collector
}

// extractTokenCounts gets input/output token counts from GenerationInfo.
// Returns actual counts from API response, or estimates if unavailable.
// Provider key names vary: OpenAI uses PromptTokens/CompletionTokens,
// Anthropic uses InputTokens/OutputTokens.
func extractTokenCounts(info map[string]any, inputChars, outputChars int) (input, output int64) {
	// Try OpenAI-style keys first
	if v, ok := info["PromptTokens"]; ok {
		if i, ok := toInt64(v); ok {
			input = i
		}
	}
	if v, ok := info["CompletionTokens"]; ok {
		if i, ok := toInt64(v); ok {
			output = i
		}
	}

	// Try Anthropic-style keys
	if input == 0 {
		if v, ok := info["InputTokens"]; ok {
			if i, ok := toInt64(v); ok {
				input = i
			}
		}
	}
	if output == 0 {
		if v, ok := info["OutputTokens"]; ok {
			if i, ok := toInt64(v); ok {
				output = i
			}
		}
	}

	// Fall back to estimates if API didn't provide counts
	if input == 0 {
		input = int64(inputChars / charsPerToken)
	}
	if output == 0 {
		output = int64(outputChars / charsPerToken)
	}

	return input, output
}

// toInt64 converts various numeric types to int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	default:
		return 0, false
	}
}

// NewModel creates an LLM model based on configuration.
// If mc is nil, metrics recording is disabled.
func NewModel(cfg config.Config, mc *metrics.Collector) (*Model, error) {
	var model llms.Model
	var err error

	switch cfg.LLMProvider {
	case config.ProviderOllama:
		model, err = ollama.New(
			ollama.WithModel(cfg.LLMModel),
			ollama.WithServerURL(cfg.OllamaHost),
		)
		if err != nil {
			return nil, fmt.Errorf("create ollama model: %w", err)
		}

	case config.ProviderOpenAI:
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OpenAI API key required")
		}
		model, err = openai.New(
			openai.WithToken(cfg.OpenAIAPIKey),
			openai.WithModel(cfg.LLMModel),
		)
		if err != nil {
			return nil, fmt.Errorf("create openai model: %w", err)
		}

	case config.ProviderAnthropic:
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("Anthropic API key required")
		}
		model, err = anthropic.New(
			anthropic.WithToken(cfg.AnthropicAPIKey),
			anthropic.WithModel(cfg.LLMModel),
		)
		if err != nil {
			return nil, fmt.Errorf("create anthropic model: %w", err)
		}

	case config.ProviderBedrock:
		// AWS SDK automatically picks up env vars: AWS_ACCESS_KEY_ID,
		// AWS_SECRET_ACCESS_KEY, AWS_REGION, HTTPS_PROXY, AWS_CA_BUNDLE
		opts := []bedrock.Option{bedrock.WithModel(cfg.LLMModel)}
		// For inference profiles, provider can't be auto-detected from ARN
		if cfg.BedrockModelProvider != "" {
			opts = append(opts, bedrock.WithModelProvider(cfg.BedrockModelProvider))
		}
		model, err = bedrock.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("create bedrock model: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.LLMProvider)
	}

	return &Model{
		llm:       model,
		modelName: cfg.LLMModel,
		metrics:   mc,
	}, nil
}

// GenerateWithSystem generates text with a system prompt.
func (m *Model) GenerateWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	systemLen := len(systemPrompt)
	userLen := len(userPrompt)
	totalLen := systemLen + userLen

	slog.Debug("LLM generate starting", "model", m.modelName, "system_len", systemLen, "user_len", userLen, "total_len", totalLen)

	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
		llms.TextParts(llms.ChatMessageTypeHuman, userPrompt),
	}

	start := time.Now()
	response, err := m.llm.GenerateContent(ctx, messages, llms.WithMaxTokens(8192))
	duration := time.Since(start)

	if err != nil {
		slog.Warn("LLM generate failed", "model", m.modelName, "total_len", totalLen, "duration_ms", duration.Milliseconds(), "error", err)
		return "", wrapFatalError(fmt.Errorf("generate with system: %w", err))
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	choice := response.Choices[0]
	responseLen := len(choice.Content)
	slog.Debug("LLM generate complete", "model", m.modelName, "total_len", totalLen, "response_len", responseLen, "duration_ms", duration.Milliseconds())

	if m.metrics != nil {
		inputTokens, outputTokens := extractTokenCounts(choice.GenerationInfo, totalLen, responseLen)
		m.metrics.RecordLLMUsage(metrics.OpLLMGenerate, duration, inputTokens, outputTokens)
	}

	return choice.Content, nil
}

// Model returns the LLM model name.
func (m *Model) Model() string {
	return m.modelName
}

// SynthesizeAnswer generates an answer from context and query.
func (m *Model) SynthesizeAnswer(ctx context.Context, query string, context string) (string, error) {
	systemPrompt := `You are a helpful knowledge assistant. Answer the user's question based ONLY on the provided context.
If the context doesn't contain enough information to answer the question, say so.
Be concise and cite specific information from the context where relevant.`

	userPrompt := fmt.Sprintf(`Context:
%s

Question: %s

Answer:`, context, query)

	return m.GenerateWithSystem(ctx, systemPrompt, userPrompt)
}

// FillTemplate fills a template with gathered knowledge.
func (m *Model) FillTemplate(ctx context.Context, templateContent string, knowledge string) (string, error) {
	systemPrompt := `You are a knowledge synthesis assistant. Fill out the template using ONLY the provided knowledge.
- Replace placeholder sections with synthesized content from the knowledge
- If insufficient data exists for a section, note "Insufficient data"
- Cite specific examples from the knowledge where possible
- Maintain the template's structure and formatting`

	userPrompt := fmt.Sprintf(`Template:
%s

Available Knowledge:
%s

Filled Template:`, templateContent, knowledge)

	return m.GenerateWithSystem(ctx, systemPrompt, userPrompt)
}

// GenerateWithSystemStream generates text with a system prompt, streaming tokens via callback.
// The onToken callback is invoked for each token/chunk. Return an error from onToken to abort.
func (m *Model) GenerateWithSystemStream(
	ctx context.Context,
	systemPrompt, userPrompt string,
	onToken func(token string) error,
) error {
	systemLen := len(systemPrompt)
	userLen := len(userPrompt)
	totalLen := systemLen + userLen

	slog.Debug("LLM streaming generate starting", "model", m.modelName, "system_len", systemLen, "user_len", userLen, "total_len", totalLen)

	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
		llms.TextParts(llms.ChatMessageTypeHuman, userPrompt),
	}

	start := time.Now()

	// Track output length for metrics
	var outputLen int

	// Use streaming callback option - supported by all langchaingo providers
	streamingFunc := func(ctx context.Context, chunk []byte) error {
		outputLen += len(chunk)
		return onToken(string(chunk))
	}

	response, err := m.llm.GenerateContent(ctx, messages, llms.WithMaxTokens(8192), llms.WithStreamingFunc(streamingFunc))
	duration := time.Since(start)

	if err != nil {
		slog.Warn("LLM streaming generate failed", "model", m.modelName, "total_len", totalLen, "duration_ms", duration.Milliseconds(), "error", err)
		return wrapFatalError(fmt.Errorf("generate with system stream: %w", err))
	}

	slog.Debug("LLM streaming generate complete", "model", m.modelName, "total_len", totalLen, "output_len", outputLen, "duration_ms", duration.Milliseconds())

	if m.metrics != nil {
		var genInfo map[string]any
		if len(response.Choices) > 0 {
			genInfo = response.Choices[0].GenerationInfo
		}
		inputTokens, outputTokens := extractTokenCounts(genInfo, totalLen, outputLen)
		m.metrics.RecordLLMUsage(metrics.OpLLMStream, duration, inputTokens, outputTokens)
	}

	return nil
}

// SynthesizeAnswerStream generates an answer from context and query, streaming tokens.
func (m *Model) SynthesizeAnswerStream(ctx context.Context, query string, context string, onToken func(token string) error) error {
	systemPrompt := `You are a helpful knowledge assistant. Answer the user's question based ONLY on the provided context.
If the context doesn't contain enough information to answer the question, say so.
Be concise and cite specific information from the context where relevant.`

	userPrompt := fmt.Sprintf(`Context:
%s

Question: %s

Answer:`, context, query)

	return m.GenerateWithSystemStream(ctx, systemPrompt, userPrompt, onToken)
}

// ExtractEntitiesAndRelations extracts entities and relations from text (GraphRAG-style).
func (m *Model) ExtractEntitiesAndRelations(ctx context.Context, text string, existingEntities []string) (string, error) {
	entitiesStr := ""
	if len(existingEntities) > 0 {
		entitiesStr = fmt.Sprintf("\nExisting entities that may be referenced:\n%s", existingEntities)
	}

	systemPrompt := `You are a Knowledge Graph Specialist. Extract entities and relations from the given text.

Entity types: person, service, concept, project, task, document

Output format (one per line):
ENTITY|name|type|description
RELATION|source|target|relation_type|description

Guidelines:
- Extract all meaningful entities with brief descriptions
- Identify relationships between entities
- Use lowercase entity names with hyphens (e.g., "john-doe", "auth-service")
- For relation types use: works_on, owns, depends_on, references, mentions, relates_to`

	userPrompt := fmt.Sprintf(`Text:
%s
%s

Extracted entities and relations:`, text, entitiesStr)

	return m.GenerateWithSystem(ctx, systemPrompt, userPrompt)
}
