package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// mockChatModel implements model.BaseChatModel for testing.
type mockChatModel struct {
	streamFn func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
}

func (m *mockChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}

func (m *mockChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.streamFn(ctx, input, opts...)
}

func TestGenerateStreamWithTools_TextOnly(t *testing.T) {
	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray([]*schema.Message{
				{Content: "Hello "},
				{Content: "world"},
			}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	var got []string
	err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "hi"}},
		nil,
		func(token string) error {
			got = append(got, token)
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(got))
	}
	if got[0] != "Hello " || got[1] != "world" {
		t.Errorf("unexpected tokens: %v", got)
	}
}

func TestGenerateStreamWithTools_WithToolCall(t *testing.T) {
	callCount := 0

	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			callCount++
			if callCount == 1 {
				// First call: text + tool call
				return schema.StreamReaderFromArray([]*schema.Message{
					{Content: "Thinking... "},
					{
						ToolCalls: []schema.ToolCall{
							{
								ID: "call-1",
								Function: schema.FunctionCall{
									Name:      "search",
									Arguments: `{"q":"golang"}`,
								},
							},
						},
					},
				}), nil
			}
			// Second call: final text
			return schema.StreamReaderFromArray([]*schema.Message{
				{Content: "Done"},
			}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	var tokens []string
	var toolCallNames []string

	err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "search golang"}},
		[]*schema.ToolInfo{{Name: "search"}},
		func(token string) error {
			tokens = append(tokens, token)
			return nil
		},
		func(call schema.ToolCall) (string, error) {
			toolCallNames = append(toolCallNames, call.Function.Name)
			return "result: Go is great", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected Stream called twice, got %d", callCount)
	}
	if len(toolCallNames) != 1 || toolCallNames[0] != "search" {
		t.Errorf("unexpected tool calls: %v", toolCallNames)
	}
	if len(tokens) != 2 || tokens[0] != "Thinking... " || tokens[1] != "Done" {
		t.Errorf("unexpected tokens: %v", tokens)
	}
}
