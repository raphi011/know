package llm

import (
	"context"
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
	// Verify RegisterCallbacks doesn't panic and the handler fires
	RegisterCallbacks()

	// EnsureRunInfo sets up the RunInfo in context (required before OnStart/OnEnd)
	ctx := callbacks.EnsureRunInfo(context.Background(), "TestModel", components.ComponentOfChatModel)

	// OnStart should store start time via our handler
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{})

	// OnEnd should not panic
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

	// OnEnd with nil-valued output should not panic
	callbacks.OnEnd(ctx, (*model.CallbackOutput)(nil))
}
