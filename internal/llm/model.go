package llm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/metrics"
	"google.golang.org/genai"
)

// newBedrockChatModel creates a Bedrock-backed Claude chat model.
//
// HACK: works around two eino-ext bugs:
//
//  1. Config.HTTPClient (*http.Client) is passed to awsConfig.WithHTTPClient,
//     but the AWS SDK's resolveCustomCABundle type-asserts it to
//     *awshttp.BuildableClient and panics when AWS_CA_BUNDLE is set.
//
//  2. For the Bedrock path, eino-ext only passes the HTTP client to
//     awsConfig.WithHTTPClient (for signing), NOT to option.WithHTTPClient
//     (for actual API calls). So the Anthropic SDK always uses
//     http.DefaultClient for Bedrock requests, ignoring Config.HTTPClient.
//
// Workaround: temporarily unset AWS_CA_BUNDLE to prevent the panic (bug 1).
// For bug 2, when tlsSkipVerify is true, set InsecureSkipVerify on
// http.DefaultTransport. Otherwise, inject the CA from AWS_CA_BUNDLE
// into http.DefaultTransport so http.DefaultClient trusts the proxy cert.
//
// TODO: remove this once eino-ext fixes both bugs.
// See: https://github.com/cloudwego/eino-ext/issues/712
// See: docs/teleport-proxy.md (Issues 1 & 2)
func newBedrockChatModel(ctx context.Context, model string, tlsSkipVerify bool) (*claude.ChatModel, error) {
	// NOTE: This temporarily mutates the environment. NewModel and NewEmbedder
	// must not be called concurrently when AWS_CA_BUNDLE is set.
	caBundle := os.Getenv("AWS_CA_BUNDLE")
	if caBundle != "" {
		// Bug 1: unset to prevent AWS SDK panic.
		// Errors from Unsetenv/Setenv are impossible here (key is a valid, non-empty string).
		_ = os.Unsetenv("AWS_CA_BUNDLE")
		defer func() { _ = os.Setenv("AWS_CA_BUNDLE", caBundle) }()
	}

	if tlsSkipVerify {
		// Skip TLS verification takes precedence over CA bundle injection.
		// AWS_CA_BUNDLE is still unset above to prevent the eino-ext panic (bug 1).
		slog.Warn("skipping TLS verification on http.DefaultTransport", "reason", "eino-ext bug #2")
		if err := skipVerifyDefaultTransport(); err != nil {
			return nil, fmt.Errorf("skip verify default transport: %w", err)
		}
	} else if caBundle != "" {
		// Fallback: patch http.DefaultTransport with the CA so the Anthropic
		// SDK's http.DefaultClient trusts the proxy certificate.
		if err := addCAToDefaultTransport(caBundle); err != nil {
			return nil, fmt.Errorf("add CA to default transport: %w", err)
		}
	}

	return claude.NewChatModel(ctx, &claude.Config{
		ByBedrock: true,
		Model:     model,
	})
}

// skipVerifyDefaultTransport sets InsecureSkipVerify on http.DefaultTransport.
// This is process-global but necessary because the Anthropic SDK uses
// http.DefaultClient for Bedrock API calls (eino-ext bug #2).
func skipVerifyDefaultTransport() error {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return fmt.Errorf("http.DefaultTransport is not *http.Transport")
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	return nil
}

// addCAToDefaultTransport appends a PEM CA certificate to http.DefaultTransport's
// TLS root CAs. This is process-global but necessary because the Anthropic SDK
// uses http.DefaultClient for Bedrock API calls.
func addCAToDefaultTransport(caPath string) error {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("read CA bundle %s: %w", caPath, err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		slog.Warn("failed to load system cert pool, using empty pool", "error", err)
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("no valid certificates found in CA bundle %s", caPath)
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return fmt.Errorf("http.DefaultTransport is not *http.Transport")
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.RootCAs = pool

	return nil
}

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

// Model wraps eino chat models for text generation.
type Model struct {
	chatModel model.BaseChatModel
	modelName string
	metrics   *metrics.Collector
}

// NewModel creates an LLM model based on configuration.
// If mc is nil, metrics recording is disabled.
func NewModel(ctx context.Context, cfg config.Config, mc *metrics.Collector) (*Model, error) {
	var chatModel model.BaseChatModel
	var err error

	switch cfg.LLMProvider {
	case config.ProviderNone:
		return nil, nil

	case config.ProviderOllama:
		chatModel, err = einoollama.NewChatModel(ctx, &einoollama.ChatModelConfig{
			BaseURL: cfg.OllamaHost,
			Model:   cfg.LLMModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create ollama model: %w", err)
		}

	case config.ProviderGoogleAI:
		if cfg.GoogleAIAPIKey == "" {
			return nil, fmt.Errorf("google AI API key required (GOOGLE_AI_API_KEY)")
		}
		client, clientErr := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  cfg.GoogleAIAPIKey,
			Backend: genai.BackendGeminiAPI,
		})
		if clientErr != nil {
			return nil, fmt.Errorf("create google ai client: %w", clientErr)
		}
		chatModel, err = einogemini.NewChatModel(ctx, &einogemini.Config{
			Client: client,
			Model:  cfg.LLMModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create google ai model: %w", err)
		}

	case config.ProviderOpenAI:
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("openAI API key required")
		}
		chatModel, err = einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
			APIKey: cfg.OpenAIAPIKey,
			Model:  cfg.LLMModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create openai model: %w", err)
		}

	case config.ProviderAnthropic:
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("anthropic API key required")
		}
		chatModel, err = claude.NewChatModel(ctx, &claude.Config{
			APIKey: cfg.AnthropicAPIKey,
			Model:  cfg.LLMModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create anthropic model: %w", err)
		}

	case config.ProviderBedrock:
		chatModel, err = newBedrockChatModel(ctx, cfg.LLMModel, cfg.TLSSkipVerify)
		if err != nil {
			return nil, fmt.Errorf("create bedrock model: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.LLMProvider)
	}

	return &Model{
		chatModel: chatModel,
		modelName: cfg.LLMModel,
		metrics:   mc,
	}, nil
}

// extractTokenCounts gets input/output token counts from ResponseMeta.
// Returns actual counts from API response, or estimates if unavailable.
func extractTokenCounts(meta *schema.ResponseMeta, inputChars, outputChars int) (input, output int64) {
	if meta != nil && meta.Usage != nil {
		input = int64(meta.Usage.PromptTokens)
		output = int64(meta.Usage.CompletionTokens)
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

// GenerateWithSystem generates text with a system prompt.
func (m *Model) GenerateWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	systemLen := len(systemPrompt)
	userLen := len(userPrompt)
	totalLen := systemLen + userLen

	slog.Debug("LLM generate starting", "model", m.modelName, "system_len", systemLen, "user_len", userLen, "total_len", totalLen)

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	start := time.Now()
	resp, err := m.chatModel.Generate(ctx, messages, model.WithMaxTokens(8192))
	duration := time.Since(start)

	if err != nil {
		slog.Warn("LLM generate failed", "model", m.modelName, "total_len", totalLen, "duration_ms", duration.Milliseconds(), "error", err)
		return "", wrapFatalError(fmt.Errorf("generate with system: %w", err))
	}

	if resp.Content == "" {
		slog.Warn("LLM returned empty response", "model", m.modelName, "total_len", totalLen, "duration_ms", duration.Milliseconds())
		return "", fmt.Errorf("empty response from model")
	}

	responseLen := len(resp.Content)
	slog.Debug("LLM generate complete", "model", m.modelName, "total_len", totalLen, "response_len", responseLen, "duration_ms", duration.Milliseconds())

	if m.metrics != nil {
		inputTokens, outputTokens := extractTokenCounts(resp.ResponseMeta, totalLen, responseLen)
		m.metrics.RecordLLMUsage(metrics.OpLLMGenerate, duration, inputTokens, outputTokens)
	}

	return resp.Content, nil
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

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	start := time.Now()

	sr, err := m.chatModel.Stream(ctx, messages, model.WithMaxTokens(8192))
	if err != nil {
		slog.Warn("LLM streaming generate failed to start", "model", m.modelName, "total_len", totalLen, "error", err)
		return wrapFatalError(fmt.Errorf("generate with system stream: %w", err))
	}
	defer sr.Close()

	var outputLen int
	var lastMeta *schema.ResponseMeta

	for {
		msg, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			duration := time.Since(start)
			slog.Warn("LLM streaming generate failed", "model", m.modelName, "total_len", totalLen, "duration_ms", duration.Milliseconds(), "error", recvErr)
			return wrapFatalError(fmt.Errorf("generate with system stream: %w", recvErr))
		}
		if msg.Content != "" {
			outputLen += len(msg.Content)
			if tokenErr := onToken(msg.Content); tokenErr != nil {
				return fmt.Errorf("streaming token callback: %w", tokenErr)
			}
		}
		if msg.ResponseMeta != nil {
			lastMeta = msg.ResponseMeta
		}
	}

	duration := time.Since(start)
	slog.Debug("LLM streaming generate complete", "model", m.modelName, "total_len", totalLen, "output_len", outputLen, "duration_ms", duration.Milliseconds())

	if m.metrics != nil {
		inputTokens, outputTokens := extractTokenCounts(lastMeta, totalLen, outputLen)
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

// ChatMessage represents a message in a multi-turn conversation.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// GenerateWithSystemStreamMultiTurn generates text with a system prompt and multi-turn history,
// streaming tokens via callback.
func (m *Model) GenerateWithSystemStreamMultiTurn(
	ctx context.Context,
	systemPrompt string,
	history []ChatMessage,
	currentQuery string,
	onToken func(token string) error,
) error {
	// Build message array: system + history + current query
	messages := make([]*schema.Message, 0, 2+len(history))
	messages = append(messages, &schema.Message{Role: schema.System, Content: systemPrompt})

	for i, msg := range history {
		switch msg.Role {
		case "user":
			messages = append(messages, &schema.Message{Role: schema.User, Content: msg.Content})
		case "assistant":
			messages = append(messages, &schema.Message{Role: schema.Assistant, Content: msg.Content})
		default:
			slog.Warn("unknown chat message role, skipping", "role", msg.Role, "index", i)
		}
	}

	messages = append(messages, &schema.Message{Role: schema.User, Content: currentQuery})

	// Calculate total input length for metrics
	totalLen := len(systemPrompt) + len(currentQuery)
	for _, msg := range history {
		totalLen += len(msg.Content)
	}

	slog.Debug("LLM multi-turn streaming starting", "model", m.modelName, "history_len", len(history), "total_len", totalLen)

	start := time.Now()

	sr, err := m.chatModel.Stream(ctx, messages, model.WithMaxTokens(8192))
	if err != nil {
		slog.Warn("LLM multi-turn streaming failed to start", "model", m.modelName, "total_len", totalLen, "error", err)
		return wrapFatalError(fmt.Errorf("generate multi-turn stream: %w", err))
	}
	defer sr.Close()

	var outputLen int
	var lastMeta *schema.ResponseMeta

	for {
		msg, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			duration := time.Since(start)
			slog.Warn("LLM multi-turn streaming failed", "model", m.modelName, "total_len", totalLen, "duration_ms", duration.Milliseconds(), "error", recvErr)
			return wrapFatalError(fmt.Errorf("generate multi-turn stream: %w", recvErr))
		}
		if msg.Content != "" {
			outputLen += len(msg.Content)
			if tokenErr := onToken(msg.Content); tokenErr != nil {
				return fmt.Errorf("streaming token callback: %w", tokenErr)
			}
		}
		if msg.ResponseMeta != nil {
			lastMeta = msg.ResponseMeta
		}
	}

	duration := time.Since(start)
	slog.Debug("LLM multi-turn streaming complete", "model", m.modelName, "total_len", totalLen, "output_len", outputLen, "duration_ms", duration.Milliseconds())

	if m.metrics != nil {
		inputTokens, outputTokens := extractTokenCounts(lastMeta, totalLen, outputLen)
		m.metrics.RecordLLMUsage(metrics.OpLLMStream, duration, inputTokens, outputTokens)
	}

	return nil
}

// GenerateStreamWithTools runs an agentic loop: stream LLM output, invoke tools when requested,
// and continue until the model produces a final text response or the iteration limit is reached.
//
// onToken is called for each streamed text chunk. onToolCall is called for each tool invocation
// and must return the tool result string. If onToolCall is nil and a tool call is received, an
// error is returned.
func (m *Model) GenerateStreamWithTools(
	ctx context.Context,
	messages []*schema.Message,
	tools []*schema.ToolInfo,
	onToken func(token string) error,
	onToolCall func(call schema.ToolCall) (string, error),
) error {
	opts := []model.Option{model.WithMaxTokens(8192)}
	if len(tools) > 0 {
		opts = append(opts, model.WithTools(tools))
	}

	// Work on a copy so the caller's slice is not mutated.
	msgs := make([]*schema.Message, len(messages))
	copy(msgs, messages)

	const maxIterations = 10
	for i := range maxIterations {
		sr, err := m.chatModel.Stream(ctx, msgs, opts...)
		if err != nil {
			return wrapFatalError(fmt.Errorf("generate stream with tools (iteration %d): %w", i, err))
		}

		var (
			textBuilder strings.Builder
			toolCalls   []schema.ToolCall
		)

		for {
			msg, recvErr := sr.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			if recvErr != nil {
				sr.Close()
				return wrapFatalError(fmt.Errorf("generate stream with tools recv (iteration %d): %w", i, recvErr))
			}
			if msg.Content != "" {
				textBuilder.WriteString(msg.Content)
				if tokenErr := onToken(msg.Content); tokenErr != nil {
					sr.Close()
					return fmt.Errorf("streaming token callback: %w", tokenErr)
				}
			}
			toolCalls = append(toolCalls, msg.ToolCalls...)
		}
		sr.Close()

		// No tool calls — model produced a final answer.
		if len(toolCalls) == 0 {
			return nil
		}

		if onToolCall == nil {
			return fmt.Errorf("model requested tool call but onToolCall is nil")
		}

		// Append assistant turn with the tool calls.
		msgs = append(msgs, schema.AssistantMessage(textBuilder.String(), toolCalls))

		// Execute each tool and append results.
		for _, tc := range toolCalls {
			result, toolErr := onToolCall(tc)
			if toolErr != nil {
				msgs = append(msgs, schema.ToolMessage(fmt.Sprintf("error: %v", toolErr), tc.ID))
			} else {
				msgs = append(msgs, schema.ToolMessage(result, tc.ID))
			}
		}
	}

	return fmt.Errorf("generate stream with tools: exceeded maximum iterations (%d)", maxIterations)
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
