package llm

import (
	"context"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/raphi011/knowhow/internal/logutil"
)

type startTimeKey struct{}

// RegisterCallbacks registers a global eino callback handler that provides
// structured logging for all eino components that fire callbacks. All eino-ext
// model/embedding providers (Claude, OpenAI, Ollama, Gemini) fire callbacks.
// The custom Bedrock embedder (langchaingo-based) bypasses eino entirely, so
// the Model/Embedder wrappers retain their own logging as a universal fallback.
//
// Metrics recording stays in the Model/Embedder wrappers because eino
// callbacks return a new context that is not propagated back to the caller,
// making deduplication impossible. The callback handler provides richer
// log context (token usage breakdown, component type) that supplements
// the wrapper's basic logging.
func RegisterCallbacks() {
	handler := callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			logutil.FromCtx(ctx).Debug("eino component starting",
				"component", info.Component,
				"type", info.Type,
			)
			return context.WithValue(ctx, startTimeKey{}, time.Now())
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			duration := timeSinceStart(ctx)

			logger := logutil.FromCtx(ctx).With(
				"component", info.Component,
				"type", info.Type,
				"duration_ms", duration.Milliseconds(),
			)

			if info.Component == components.ComponentOfChatModel {
				if mo := model.ConvCallbackOutput(output); mo != nil && mo.TokenUsage != nil {
					logger = logger.With(
						"prompt_tokens", mo.TokenUsage.PromptTokens,
						"completion_tokens", mo.TokenUsage.CompletionTokens,
						"cached_tokens", mo.TokenUsage.PromptTokenDetails.CachedTokens,
					)
				}
			}

			logger.Debug("eino component complete")
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			duration := timeSinceStart(ctx)

			logutil.FromCtx(ctx).Warn("eino component error",
				"component", info.Component,
				"type", info.Type,
				"duration_ms", duration.Milliseconds(),
				"error", err,
			)
			return ctx
		}).
		Build()

	callbacks.AppendGlobalHandlers(handler)
}

func timeSinceStart(ctx context.Context) time.Duration {
	if start, ok := ctx.Value(startTimeKey{}).(time.Time); ok {
		return time.Since(start)
	}
	logutil.FromCtx(ctx).Warn("eino callback: start time not found in context, duration will be 0")
	return 0
}
