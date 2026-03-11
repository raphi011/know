package llm

import (
	"context"
	"errors"
	"strings"
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
	_, err := m.GenerateStreamWithTools(
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

	_, err := m.GenerateStreamWithTools(
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

// TestGenerateStreamWithTools_WithFragmentedToolCall simulates the streaming
// pattern from the eino claude adapter where tool calls arrive as separate
// fragments: a ContentBlockStart with ID+name, followed by ContentBlockDeltas
// with partial JSON arguments and empty IDs. ConcatMessages must merge these
// by Index so Bedrock doesn't reject empty-ID tool_use blocks.
func TestGenerateStreamWithTools_WithFragmentedToolCall(t *testing.T) {
	idx0 := 0
	idx1 := 1
	callCount := 0
	var capturedMsgs []*schema.Message

	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			callCount++
			capturedMsgs = input
			if callCount == 1 {
				// Simulate Bedrock streaming: two tool calls, each fragmented
				return schema.StreamReaderFromArray([]*schema.Message{
					// Tool call 0: start event (ID + name)
					{
						Role: schema.Assistant,
						ToolCalls: []schema.ToolCall{{
							Index: &idx0,
							ID:    "toolu_bdrk_abc123",
							Function: schema.FunctionCall{
								Name:      "search",
								Arguments: "",
							},
						}},
					},
					// Tool call 0: delta (partial args)
					{
						ToolCalls: []schema.ToolCall{{
							Index: &idx0,
							ID:    "",
							Function: schema.FunctionCall{
								Name:      "",
								Arguments: `{"q":`,
							},
						}},
					},
					// Tool call 0: delta (rest of args)
					{
						ToolCalls: []schema.ToolCall{{
							Index: &idx0,
							ID:    "",
							Function: schema.FunctionCall{
								Name:      "",
								Arguments: `"golang"}`,
							},
						}},
					},
					// Tool call 1: start event
					{
						ToolCalls: []schema.ToolCall{{
							Index: &idx1,
							ID:    "toolu_bdrk_def456",
							Function: schema.FunctionCall{
								Name:      "list_folders",
								Arguments: "",
							},
						}},
					},
					// Tool call 1: delta
					{
						ToolCalls: []schema.ToolCall{{
							Index: &idx1,
							ID:    "",
							Function: schema.FunctionCall{
								Name:      "",
								Arguments: `{"path":"/"}`,
							},
						}},
					},
				}), nil
			}
			// Second call: final text
			return schema.StreamReaderFromArray([]*schema.Message{
				{Content: "Here are the results"},
			}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	var tokens []string
	var toolCalls []schema.ToolCall

	_, err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "search and list"}},
		[]*schema.ToolInfo{{Name: "search"}, {Name: "list_folders"}},
		func(token string) error {
			tokens = append(tokens, token)
			return nil
		},
		func(call schema.ToolCall) (string, error) {
			toolCalls = append(toolCalls, call)
			return "ok", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected Stream called twice, got %d", callCount)
	}

	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}

	wantCalls := []struct {
		id   string
		name string
		args string
	}{
		{"toolu_bdrk_abc123", "search", `{"q":"golang"}`},
		{"toolu_bdrk_def456", "list_folders", `{"path":"/"}`},
	}
	for i, want := range wantCalls {
		got := toolCalls[i]
		if got.ID != want.id {
			t.Errorf("tool call %d: expected ID %q, got %q", i, want.id, got.ID)
		}
		if got.Function.Name != want.name {
			t.Errorf("tool call %d: expected name %q, got %q", i, want.name, got.Function.Name)
		}
		if got.Function.Arguments != want.args {
			t.Errorf("tool call %d: expected args %q, got %q", i, want.args, got.Function.Arguments)
		}
	}

	// Final text token
	if len(tokens) != 1 || tokens[0] != "Here are the results" {
		t.Errorf("unexpected tokens: %v", tokens)
	}

	// Verify assistant message sent back to the model contains merged tool call IDs
	// (not empty strings that Bedrock would reject).
	if len(capturedMsgs) < 3 {
		t.Fatalf("expected at least 3 messages on second call, got %d", len(capturedMsgs))
	}
	assistantMsg := capturedMsgs[1]
	if len(assistantMsg.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls in assistant message, got %d", len(assistantMsg.ToolCalls))
	}
	for i, want := range wantCalls {
		if assistantMsg.ToolCalls[i].ID != want.id {
			t.Errorf("assistant msg tool call %d: expected ID %q, got %q", i, want.id, assistantMsg.ToolCalls[i].ID)
		}
	}
	// Verify tool result messages reference the correct IDs
	for i, want := range wantCalls {
		toolMsg := capturedMsgs[2+i]
		if toolMsg.ToolCallID != want.id {
			t.Errorf("tool result %d: expected ToolCallID %q, got %q", i, want.id, toolMsg.ToolCallID)
		}
	}
}

func TestGenerateStreamWithTools_EmptyStream(t *testing.T) {
	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			// Return an empty stream (EOF immediately).
			return schema.StreamReaderFromArray([]*schema.Message{}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	_, err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "hi"}},
		nil,
		func(token string) error { return nil },
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty stream, got nil")
	}
	if !strings.Contains(err.Error(), "empty stream") {
		t.Errorf("expected 'empty stream' in error, got %q", err.Error())
	}
}

func TestGenerateStreamWithTools_MergeError(t *testing.T) {
	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			// Conflicting roles trigger a ConcatMessages error.
			return schema.StreamReaderFromArray([]*schema.Message{
				{Role: schema.Assistant, Content: "hello"},
				{Role: schema.User, Content: "world"},
			}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	_, err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "hi"}},
		nil,
		func(token string) error { return nil },
		nil,
	)
	if err == nil {
		t.Fatal("expected merge error, got nil")
	}
	if !strings.Contains(err.Error(), "merge") {
		t.Errorf("expected 'merge' in error, got %q", err.Error())
	}
}

// TestGenerateStreamWithTools_ToolErrorUseMergedID verifies that when a tool
// call fails, the error message sent back to the model references the merged
// tool call ID (not an empty string from a streaming fragment).
func TestGenerateStreamWithTools_ToolErrorUseMergedID(t *testing.T) {
	idx0 := 0
	callCount := 0
	var capturedMsgs []*schema.Message

	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			callCount++
			capturedMsgs = input
			if callCount == 1 {
				return schema.StreamReaderFromArray([]*schema.Message{
					{
						Role: schema.Assistant,
						ToolCalls: []schema.ToolCall{{
							Index:    &idx0,
							ID:       "toolu_bdrk_err123",
							Function: schema.FunctionCall{Name: "search", Arguments: ""},
						}},
					},
					{
						ToolCalls: []schema.ToolCall{{
							Index:    &idx0,
							ID:       "",
							Function: schema.FunctionCall{Arguments: `{"q":"fail"}`},
						}},
					},
				}), nil
			}
			return schema.StreamReaderFromArray([]*schema.Message{
				{Content: "I see the error"},
			}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	_, err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "test"}},
		[]*schema.ToolInfo{{Name: "search"}},
		func(token string) error { return nil },
		func(call schema.ToolCall) (string, error) {
			return "", errors.New("tool failed")
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On the second call, input should have: [user, assistant, tool error result]
	if len(capturedMsgs) < 3 {
		t.Fatalf("expected at least 3 messages on second call, got %d", len(capturedMsgs))
	}

	toolMsg := capturedMsgs[2]
	if toolMsg.ToolCallID != "toolu_bdrk_err123" {
		t.Errorf("expected tool error ToolCallID 'toolu_bdrk_err123', got %q", toolMsg.ToolCallID)
	}
	if !strings.Contains(toolMsg.Content, "tool failed") {
		t.Errorf("expected error message in tool result, got %q", toolMsg.Content)
	}
}

func TestGenerateStreamWithTools_InterleavedTextAndToolCalls(t *testing.T) {
	idx0 := 0
	callCount := 0
	var capturedMsgs []*schema.Message

	mock := &mockChatModel{
		streamFn: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			callCount++
			capturedMsgs = input
			if callCount == 1 {
				// Text preamble followed by a fragmented tool call.
				return schema.StreamReaderFromArray([]*schema.Message{
					{Role: schema.Assistant, Content: "Let me search "},
					{Content: "for that..."},
					{
						ToolCalls: []schema.ToolCall{{
							Index: &idx0,
							ID:    "toolu_bdrk_mixed",
							Function: schema.FunctionCall{
								Name:      "search",
								Arguments: "",
							},
						}},
					},
					{
						ToolCalls: []schema.ToolCall{{
							Index: &idx0,
							ID:    "",
							Function: schema.FunctionCall{
								Arguments: `{"q":"test"}`,
							},
						}},
					},
				}), nil
			}
			return schema.StreamReaderFromArray([]*schema.Message{
				{Content: "Found it!"},
			}), nil
		},
	}

	m := &Model{chatModel: mock, modelName: "test"}

	var tokens []string
	var toolCalls []schema.ToolCall

	_, err := m.GenerateStreamWithTools(
		context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "search"}},
		[]*schema.ToolInfo{{Name: "search"}},
		func(token string) error {
			tokens = append(tokens, token)
			return nil
		},
		func(call schema.ToolCall) (string, error) {
			toolCalls = append(toolCalls, call)
			return "result", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Text tokens from first iteration + second iteration.
	if len(tokens) != 3 || tokens[0] != "Let me search " || tokens[1] != "for that..." || tokens[2] != "Found it!" {
		t.Errorf("unexpected tokens: %v", tokens)
	}

	// One tool call, properly merged.
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ID != "toolu_bdrk_mixed" {
		t.Errorf("expected tool ID 'toolu_bdrk_mixed', got %q", toolCalls[0].ID)
	}
	if toolCalls[0].Function.Arguments != `{"q":"test"}` {
		t.Errorf("expected merged args, got %q", toolCalls[0].Function.Arguments)
	}

	// Assistant message sent back should contain both text and tool call.
	assistantMsg := capturedMsgs[1]
	if assistantMsg.Content != "Let me search for that..." {
		t.Errorf("expected merged content 'Let me search for that...', got %q", assistantMsg.Content)
	}
	if len(assistantMsg.ToolCalls) != 1 || assistantMsg.ToolCalls[0].ID != "toolu_bdrk_mixed" {
		t.Errorf("expected merged tool call in assistant message, got %v", assistantMsg.ToolCalls)
	}
}
