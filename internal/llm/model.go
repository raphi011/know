package llm

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// Model wraps langchaingo LLM for text generation.
type Model struct {
	llm       llms.Model
	modelName string
}

// NewModel creates an LLM model based on configuration.
func NewModel(cfg config.Config) (*Model, error) {
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

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.LLMProvider)
	}

	return &Model{
		llm:       model,
		modelName: cfg.LLMModel,
	}, nil
}

// Generate generates text based on a prompt.
func (m *Model) Generate(ctx context.Context, prompt string) (string, error) {
	response, err := llms.GenerateFromSinglePrompt(ctx, m.llm, prompt)
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}
	return response, nil
}

// GenerateWithSystem generates text with a system prompt.
func (m *Model) GenerateWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
		llms.TextParts(llms.ChatMessageTypeHuman, userPrompt),
	}

	response, err := m.llm.GenerateContent(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("generate with system: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	return response.Choices[0].Content, nil
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
