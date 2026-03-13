package llm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
)

func TestTimeSinceStart(t *testing.T) {
	t.Run("with start time", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), startTimeKey{}, time.Now().Add(-100*time.Millisecond))
		d := timeSinceStart(ctx)
		if d < 100*time.Millisecond {
			t.Errorf("expected >= 100ms, got %v", d)
		}
	})

	t.Run("without start time", func(t *testing.T) {
		d := timeSinceStart(context.Background())
		if d != 0 {
			t.Errorf("expected 0, got %v", d)
		}
	})
}

func TestRegisterCallbacks(t *testing.T) {
	RegisterCallbacks()

	ctx := callbacks.EnsureRunInfo(context.Background(), "TestModel", components.ComponentOfChatModel)
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{})

	callbacks.OnEnd(ctx, &model.CallbackOutput{
		TokenUsage: &model.TokenUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
		},
	})
}

func TestRegisterCallbacks_NilOutput(t *testing.T) {
	RegisterCallbacks()

	ctx := callbacks.EnsureRunInfo(context.Background(), "TestModel", components.ComponentOfChatModel)
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{})

	// Typed-nil output should not panic
	callbacks.OnEnd(ctx, (*model.CallbackOutput)(nil))
}

func TestRegisterCallbacks_NilTokenUsage(t *testing.T) {
	RegisterCallbacks()

	ctx := callbacks.EnsureRunInfo(context.Background(), "TestModel", components.ComponentOfChatModel)
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{})

	// Non-nil output with nil TokenUsage should not panic
	callbacks.OnEnd(ctx, &model.CallbackOutput{})
}

func TestRegisterCallbacks_OnError(t *testing.T) {
	RegisterCallbacks()

	ctx := callbacks.EnsureRunInfo(context.Background(), "TestModel", components.ComponentOfChatModel)
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{})

	// OnError should log and not panic
	callbacks.OnError(ctx, fmt.Errorf("test error"))
}

func TestRegisterCallbacks_EmbeddingComponent(t *testing.T) {
	RegisterCallbacks()

	ctx := callbacks.EnsureRunInfo(context.Background(), "TestEmbedder", components.ComponentOfEmbedding)
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{})

	// Non-ChatModel component skips token usage extraction
	callbacks.OnEnd(ctx, &model.CallbackOutput{
		TokenUsage: &model.TokenUsage{
			PromptTokens: 200,
		},
	})
}
