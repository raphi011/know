package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// mockInvokableTool is a minimal InvokableTool for testing.
type mockInvokableTool struct {
	name string
}

func (m *mockInvokableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockInvokableTool) InvokableRun(_ context.Context, args string, _ ...tool.Option) (string, error) {
	return "ok", nil
}

// mockBaseTool implements only BaseTool (not InvokableTool).
type mockBaseTool struct {
	name string
}

func (m *mockBaseTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

// mockFailingInfoTool returns an error from Info().
type mockFailingInfoTool struct {
	name string
}

func (m *mockFailingInfoTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return nil, fmt.Errorf("info failed")
}

func (m *mockFailingInfoTool) InvokableRun(_ context.Context, args string, _ ...tool.Option) (string, error) {
	return "ok", nil
}

func TestIsWriteTool(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"create_document", true},
		{"edit_document", true},
		{"edit_document_section", true},
		{"create_memory", true},
		{"search_documents", false},
		{"get_document", false},
		{"list_folder_contents", false},
		{"list_folders", false},
		{"list_labels", false},
		{"get_document_versions", false},
		{"web_search", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWriteTool(tt.name)
			if got != tt.want {
				t.Errorf("isWriteTool(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestWrapWriteToolsForApproval_WriteTool(t *testing.T) {
	ctx := context.Background()
	tools := []tool.BaseTool{
		&mockInvokableTool{name: "create_document"},
	}

	wrapped := wrapWriteToolsForApproval(ctx, tools, nil, "vault-1")

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(wrapped))
	}
	if _, ok := wrapped[0].(*approvalToolWrapper); !ok {
		t.Errorf("expected *approvalToolWrapper, got %T", wrapped[0])
	}
}

func TestWrapWriteToolsForApproval_ReadTool(t *testing.T) {
	ctx := context.Background()
	readTool := &mockInvokableTool{name: "search_documents"}
	tools := []tool.BaseTool{readTool}

	wrapped := wrapWriteToolsForApproval(ctx, tools, nil, "vault-1")

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(wrapped))
	}
	if wrapped[0] != readTool {
		t.Error("read tool should not be wrapped")
	}
}

func TestWrapWriteToolsForApproval_NonInvokable(t *testing.T) {
	ctx := context.Background()
	base := &mockBaseTool{name: "create_document"}
	tools := []tool.BaseTool{base}

	wrapped := wrapWriteToolsForApproval(ctx, tools, nil, "vault-1")

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(wrapped))
	}
	if wrapped[0] != base {
		t.Error("non-invokable tool should pass through unwrapped")
	}
}

func TestWrapWriteToolsForApproval_InfoError(t *testing.T) {
	ctx := context.Background()
	failing := &mockFailingInfoTool{name: "create_document"}
	tools := []tool.BaseTool{failing}

	wrapped := wrapWriteToolsForApproval(ctx, tools, nil, "vault-1")

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(wrapped))
	}
	// Tool with failing Info() should pass through unwrapped (with a warning log).
	if wrapped[0] != failing {
		t.Error("tool with failing Info() should pass through unwrapped")
	}
}

func TestWrapWriteToolsForApproval_MixedTools(t *testing.T) {
	ctx := context.Background()
	write := &mockInvokableTool{name: "edit_document"}
	read := &mockInvokableTool{name: "search_documents"}
	base := &mockBaseTool{name: "some_base_tool"}
	tools := []tool.BaseTool{write, read, base}

	wrapped := wrapWriteToolsForApproval(ctx, tools, nil, "vault-1")

	if len(wrapped) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(wrapped))
	}
	if _, ok := wrapped[0].(*approvalToolWrapper); !ok {
		t.Errorf("write tool should be wrapped, got %T", wrapped[0])
	}
	if wrapped[1] != read {
		t.Error("read tool should not be wrapped")
	}
	if wrapped[2] != base {
		t.Error("base tool should not be wrapped")
	}
}

func TestApprovalToolWrapper_Info(t *testing.T) {
	inner := &mockInvokableTool{name: "create_document"}
	wrapper := &approvalToolWrapper{inner: inner}

	info, err := wrapper.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}
	if info.Name != "create_document" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "create_document")
	}
}
